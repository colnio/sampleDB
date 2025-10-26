package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Session struct {
	Token     string
	UserID    int
	Username  string
	ExpiresAt time.Time
}

type Manager struct {
	db           *pgxpool.Pool
	sessions     map[string]Session
	mu           sync.RWMutex
	cookieSecure bool
}

type contextKey string

const userContextKey contextKey = "auth-user"

func NewManager(db *pgxpool.Pool) *Manager {
	return &Manager{
		db:       db,
		sessions: make(map[string]Session),
	}
}

func (m *Manager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, exists := m.getSession(cookie.Value)
		if !exists || time.Now().After(session.ExpiresAt) {
			if exists {
				m.deleteSession(cookie.Value)
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, session)
		next(w, r.WithContext(ctx))
	}
}

func (m *Manager) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := authPageData{
				Error:      r.URL.Query().Get("error"),
				Success:    r.URL.Query().Get("success"),
				IsRegister: false,
			}

			tmpl, err := template.ParseFiles("templates/login.html")
			if err != nil {
				http.Error(w, "Error loading template", http.StatusInternalServerError)
				return
			}
			_ = tmpl.Execute(w, data)
			return
		}

		username := r.FormValue("username")
		password := r.FormValue("password")

		var (
			userID     int
			passwdHash string
			isApproved bool
		)

		err := m.db.QueryRow(context.Background(),
			"SELECT user_id, password_hash, is_approved FROM users WHERE username = $1",
			username).Scan(&userID, &passwdHash, &isApproved)
		if err != nil {
			http.Redirect(w, r, "/login?error=Invalid+username+or+password", http.StatusSeeOther)
			return
		}

		if !isApproved {
			http.Redirect(w, r, "/login?error=Your+account+is+pending+approval", http.StatusSeeOther)
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(passwdHash), []byte(password))
		if err != nil {
			http.Redirect(w, r, "/login?error=Invalid+username+or+password", http.StatusSeeOther)
			return
		}

		session, err := m.newSession(username, userID)
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

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (m *Manager) RegisterHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := authPageData{
				Error:      r.URL.Query().Get("error"),
				IsRegister: true,
			}

			tmpl, err := template.ParseFiles("templates/login.html")
			if err != nil {
				http.Error(w, "Error loading template", http.StatusInternalServerError)
				return
			}
			_ = tmpl.Execute(w, data)
			return
		}

		username := r.FormValue("username")
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

		var exists bool
		err := m.db.QueryRow(context.Background(),
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

		_, err = m.db.Exec(context.Background(),
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
			m.deleteSession(cookie.Value)
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    "",
			Path:     "/",
			Expires:  time.Now().Add(-time.Hour),
			HttpOnly: true,
			Secure:   m.cookieSecure,
		})

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (m *Manager) SetCookieSecure(enable bool) {
	m.cookieSecure = enable
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

func (m *Manager) newSession(username string, userID int) (Session, error) {
	token, err := generateSessionToken()
	if err != nil {
		return Session{}, err
	}

	session := Session{
		Token:     token,
		UserID:    userID,
		Username:  username,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	m.mu.Lock()
	m.sessions[token] = session
	m.mu.Unlock()

	return session, nil
}

func (m *Manager) getSession(token string) (Session, bool) {
	m.mu.RLock()
	session, ok := m.sessions[token]
	m.mu.RUnlock()
	return session, ok
}

func (m *Manager) deleteSession(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

type authPageData struct {
	Error      string
	Success    string
	IsRegister bool
}
