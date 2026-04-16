package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/dbiface"
)

type Session struct {
	Token     string
	UserID    int
	Username  string
	ExpiresAt time.Time
}

type Manager struct {
	db           dbiface.Pool
	cookieSecure bool
	templateDir  string
}

type contextKey string

const userContextKey contextKey = "auth-user"

// dummyHash is a pre-computed bcrypt hash used to perform a constant-time
// comparison when a username is not found, preventing timing-based enumeration.
const dummyHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoOe1Y5aLFMZFTJzYjTsHm5x6BvYHv1Hle"

func NewManager(db dbiface.Pool) *Manager {
	return &Manager{db: db}
}

func (m *Manager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Try session cookie.
		cookie, err := r.Cookie("session_token")
		if err == nil {
			session, exists, sessionErr := m.getSession(r.Context(), cookie.Value)
			if sessionErr != nil {
				log.Printf("auth: session lookup db error: %v", sessionErr)
				if strings.HasPrefix(r.URL.Path, "/api/") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprintln(w, `{"error":"service unavailable"}`)
				} else {
					http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
				}
				return
			}
			if exists {
				ctx := context.WithValue(r.Context(), userContextKey, session)
				next(w, r.WithContext(ctx))
				return
			}
			// Cookie present but session not found/expired — fall through to Bearer check.
		}

		// 2. Try Authorization: Bearer <token>.
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "bearer ") {
			plain := authHeader[7:]
			userID, err := m.ValidateAPIToken(r.Context(), plain)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				if errors.Is(err, ErrTokenInvalid) {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprintln(w, `{"error":"authentication required"}`)
				} else {
					log.Printf("auth: token validation db error: %v", err)
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprintln(w, `{"error":"service unavailable"}`)
				}
				return
			}

			// Load username so the session context is fully populated.
			// Fail auth if the user has been deleted or is not yet approved.
			var username string
			if err := m.db.QueryRow(r.Context(),
				"SELECT username FROM users WHERE user_id = $1 AND is_approved = true AND COALESCE(deleted, false) = false",
				userID).Scan(&username); err != nil {
				w.Header().Set("Content-Type", "application/json")
				if errors.Is(err, pgx.ErrNoRows) {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprintln(w, `{"error":"authentication required"}`)
				} else {
					log.Printf("auth: user lookup db error: %v", err)
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprintln(w, `{"error":"service unavailable"}`)
				}
				return
			}

			session := Session{
				UserID:    userID,
				Username:  username,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}
			ctx := context.WithValue(r.Context(), userContextKey, session)
			next(w, r.WithContext(ctx))
			return
		}

		// 3. No valid auth found — 401 for API callers, redirect HTML clients to login.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"error":"authentication required"}`)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (m *Manager) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := authPageData{
				Error:      r.URL.Query().Get("error"),
				Success:    r.URL.Query().Get("success"),
				IsRegister: false,
				Next:       r.URL.Query().Get("next"),
			}

			tmpl, err := template.ParseFiles(m.templatePath("auth_base.html"), m.templatePath("login.html"))
			if err != nil {
				http.Error(w, "Error loading template", http.StatusInternalServerError)
				return
			}
			if err := tmpl.ExecuteTemplate(w, "auth_base", data); err != nil {
				log.Printf("auth: login template execute error: %v", err)
			}
			return
		}

		username := r.FormValue("username")
		password := r.FormValue("password")

		var (
			userID     int
			passwdHash string
			isApproved bool
			deleted    bool
		)

		err := m.db.QueryRow(r.Context(),
			"SELECT user_id, password_hash, is_approved, COALESCE(deleted, false) FROM users WHERE username = $1",
			username).Scan(&userID, &passwdHash, &isApproved, &deleted)
		if err != nil {
			// Run bcrypt against a dummy hash so response time is indistinguishable
			// from a wrong-password attempt, preventing username enumeration.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
			http.Redirect(w, r, "/login?error=Invalid+username+or+password", http.StatusSeeOther)
			return
		}

		// Always run bcrypt before checking account state to prevent timing
		// side-channels that would reveal whether a username exists and is
		// disabled/unapproved.
		err = bcrypt.CompareHashAndPassword([]byte(passwdHash), []byte(password))
		if err != nil {
			http.Redirect(w, r, "/login?error=Invalid+username+or+password", http.StatusSeeOther)
			return
		}

		if deleted {
			http.Redirect(w, r, "/login?error=Account+is+disabled", http.StatusSeeOther)
			return
		}

		if !isApproved {
			http.Redirect(w, r, "/login?error=Your+account+is+pending+approval", http.StatusSeeOther)
			return
		}

		session, err := m.newSession(r.Context(), username, userID)
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    session.Token,
			Path:     "/",
			Expires:  session.ExpiresAt,
			HttpOnly: true,
			Secure:   m.cookieSecure,
			SameSite: http.SameSiteStrictMode,
		})

		// Honor ?next= redirect only for local paths to prevent open-redirect.
		next := r.FormValue("next")
		if next == "" {
			next = r.URL.Query().Get("next")
		}
		if u, err := url.Parse(next); next != "" && err == nil && u.Host == "" && strings.HasPrefix(next, "/") {
			http.Redirect(w, r, next, http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	}
}

func (m *Manager) RegisterHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := authPageData{
				Error:      r.URL.Query().Get("error"),
				Success:    r.URL.Query().Get("success"),
				IsRegister: true,
			}

			tmpl, err := template.ParseFiles(m.templatePath("auth_base.html"), m.templatePath("login.html"))
			if err != nil {
				http.Error(w, "Error loading template", http.StatusInternalServerError)
				return
			}
			if err := tmpl.ExecuteTemplate(w, "auth_base", data); err != nil {
				log.Printf("auth: register template execute error: %v", err)
			}
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")

		if username == "" || password == "" {
			http.Redirect(w, r, "/register?error=Username+and+password+are+required", http.StatusSeeOther)
			return
		}

		if password != confirmPassword {
			http.Redirect(w, r, "/register?error=Passwords+do+not+match", http.StatusSeeOther)
			return
		}

		if len(password) < 8 {
			http.Redirect(w, r, "/register?error=Password+must+be+at+least+8+characters", http.StatusSeeOther)
			return
		}

		var exists bool
		err := m.db.QueryRow(r.Context(),
			"SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)",
			username).Scan(&exists)
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		if exists {
			http.Redirect(w, r, "/register?error=Username+already+taken", http.StatusSeeOther)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		_, err = m.db.Exec(r.Context(),
			"INSERT INTO users (username, password_hash, is_approved) VALUES ($1, $2, false)",
			username, string(hashedPassword))
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/login?success=Registration+successful.+Please+wait+for+admin+approval", http.StatusSeeOther)
	}
}

func (m *Manager) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err == nil {
			m.deleteSession(r.Context(), cookie.Value)
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    "",
			Path:     "/",
			Expires:  time.Now().Add(-time.Hour),
			HttpOnly: true,
			Secure:   m.cookieSecure,
			SameSite: http.SameSiteStrictMode,
		})

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (m *Manager) SetCookieSecure(enable bool) {
	m.cookieSecure = enable
}

func (m *Manager) RevokeUserSessions(ctx context.Context, userID int) {
	if _, err := m.db.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", userID); err != nil {
		log.Printf("auth: RevokeUserSessions failed for user %d: %v", userID, err)
	}
}

func (m *Manager) SetTemplateDir(dir string) {
	m.templateDir = dir
}

func (m *Manager) templatePath(name string) string {
	if filepath.IsAbs(name) {
		return name
	}

	clean := strings.TrimPrefix(name, "templates/")
	clean = strings.TrimPrefix(clean, "templates\\")
	clean = filepath.Clean(clean)

	if m.templateDir != "" {
		return filepath.Join(m.templateDir, clean)
	}

	return filepath.Join("templates", clean)
}

// TryAuth attempts to authenticate the request from either a session cookie or
// Bearer token. It returns the session, true, and nil on success. Unlike RequireAuth
// it never writes to the response writer and never redirects. A non-nil error
// means a transient DB failure occurred; callers should return 503 in that case.
func (m *Manager) TryAuth(r *http.Request) (Session, bool, error) {
	// 1. Try session cookie.
	cookie, err := r.Cookie("session_token")
	if err == nil {
		session, exists, sessionErr := m.getSession(r.Context(), cookie.Value)
		if sessionErr != nil {
			log.Printf("auth: session lookup db error: %v", sessionErr)
			return Session{}, false, sessionErr
		}
		if exists {
			return session, true, nil
		}
	}

	// 2. Try Authorization: Bearer <token>.
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "bearer ") {
		plain := authHeader[7:]
		userID, err := m.ValidateAPIToken(r.Context(), plain)
		if err != nil {
			if errors.Is(err, ErrTokenInvalid) {
				return Session{}, false, nil
			}
			return Session{}, false, err
		}
		// Fail auth if the user has been deleted or is not yet approved.
		var username string
		if err := m.db.QueryRow(r.Context(),
			"SELECT username FROM users WHERE user_id = $1 AND is_approved = true AND COALESCE(deleted, false) = false",
			userID).Scan(&username); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Session{}, false, nil
			}
			return Session{}, false, err
		}
		return Session{
			UserID:    userID,
			Username:  username,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}, true, nil
	}

	return Session{}, false, nil
}

func SessionFromContext(ctx context.Context) (Session, bool) {
	session, ok := ctx.Value(userContextKey).(Session)
	return session, ok
}

func MustSessionFromContext(ctx context.Context) Session {
	session, ok := SessionFromContext(ctx)
	if !ok {
		panic("auth: session missing from context")
	}
	return session
}

// ContextWithSession returns a context with the given session stored at the
// auth key. Intended for use in tests that need to call handlers directly
// without going through the RequireAuth middleware.
func ContextWithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, userContextKey, s)
}

func (m *Manager) newSession(ctx context.Context, username string, userID int) (Session, error) {
	// Opportunistically remove expired sessions to prevent unbounded table growth.
	if _, err := m.db.Exec(ctx, "DELETE FROM sessions WHERE expires_at < NOW()"); err != nil {
		log.Printf("auth: purge expired sessions: %v", err)
	}

	token, err := generateSessionToken()
	if err != nil {
		return Session{}, err
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err = m.db.Exec(ctx,
		"INSERT INTO sessions (id, user_id, username, expires_at) VALUES ($1, $2, $3, $4)",
		token, userID, username, expiresAt)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return Session{
		Token:     token,
		UserID:    userID,
		Username:  username,
		ExpiresAt: expiresAt,
	}, nil
}

func (m *Manager) getSession(ctx context.Context, token string) (Session, bool, error) {
	var s Session
	s.Token = token
	err := m.db.QueryRow(ctx,
		`SELECT s.user_id, s.username, s.expires_at
		   FROM sessions s
		   JOIN users u ON u.user_id = s.user_id
		  WHERE s.id = $1 AND s.expires_at > NOW()
		    AND u.is_approved = true AND COALESCE(u.deleted, false) = false`,
		token).Scan(&s.UserID, &s.Username, &s.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, false, nil
		}
		return Session{}, false, err
	}
	return s, true, nil
}

func (m *Manager) deleteSession(ctx context.Context, token string) {
	_, _ = m.db.Exec(ctx, "DELETE FROM sessions WHERE id = $1", token)
}

func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type authPageData struct {
	Error      string
	Success    string
	IsRegister bool
	Username   string
	IsAdmin    bool
	Next       string
}
