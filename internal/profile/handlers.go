package profile

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// Handlers groups all HTTP handler methods for the profile domain.
type Handlers struct {
	svc     Service
	renderer *web.Renderer
	authMgr  *auth.Manager
}

// NewHandlers constructs a Handlers value.
func NewHandlers(svc Service, r *web.Renderer, a *auth.Manager) *Handlers {
	return &Handlers{svc: svc, renderer: r, authMgr: a}
}

// baseData builds a BasePageData for the current session user. On error it
// writes an HTTP error and returns false so the caller can return early.
func (h *Handlers) baseData(w http.ResponseWriter, r *http.Request, session auth.Session) (BasePageData, bool) {
	isAdmin, err := h.svc.GetAdminStatus(r.Context(), session.UserID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Redirect(w, r, "/logout", http.StatusSeeOther)
			return BasePageData{}, false
		}
		http.Error(w, "Unable to load account information", http.StatusInternalServerError)
		return BasePageData{}, false
	}
	return BasePageData{
		Username: session.Username,
		UserID:   session.UserID,
		IsAdmin:  isAdmin,
	}, true
}

// ChangePassword handles GET and POST /change-password.
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	base, ok := h.baseData(w, r, session)
	if !ok {
		return
	}

	data := ChangePasswordPageData{BasePageData: base}

	switch r.Method {
	case http.MethodGet:
		h.renderChangePassword(w, data)

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = "Invalid form submission"
			w.WriteHeader(http.StatusBadRequest)
			h.renderChangePassword(w, data)
			return
		}

		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		if currentPassword == "" || newPassword == "" || confirmPassword == "" {
			data.Error = "All fields are required"
			w.WriteHeader(http.StatusBadRequest)
			h.renderChangePassword(w, data)
			return
		}

		if len(newPassword) < 8 {
			data.Error = "New password must be at least 8 characters long"
			w.WriteHeader(http.StatusBadRequest)
			h.renderChangePassword(w, data)
			return
		}

		if len(newPassword) > 72 {
			data.Error = "New password must be 72 characters or fewer"
			w.WriteHeader(http.StatusBadRequest)
			h.renderChangePassword(w, data)
			return
		}

		if newPassword != confirmPassword {
			data.Error = "New passwords do not match"
			w.WriteHeader(http.StatusBadRequest)
			h.renderChangePassword(w, data)
			return
		}

		storedHash, err := h.svc.GetPasswordHash(r.Context(), session.UserID)
		if err != nil {
			if errors.Is(err, web.ErrNotFound) {
				http.Redirect(w, r, "/logout", http.StatusSeeOther)
				return
			}
			data.Error = "Unable to verify current password"
			w.WriteHeader(http.StatusBadRequest)
			h.renderChangePassword(w, data)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(currentPassword)); err != nil {
			data.Error = "Current password is incorrect"
			w.WriteHeader(http.StatusBadRequest)
			h.renderChangePassword(w, data)
			return
		}

		if currentPassword == newPassword {
			data.Error = "New password must be different from the current password"
			w.WriteHeader(http.StatusBadRequest)
			h.renderChangePassword(w, data)
			return
		}

		newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			data.Error = "Failed to update password"
			w.WriteHeader(http.StatusInternalServerError)
			h.renderChangePassword(w, data)
			return
		}

		if err := h.svc.UpdatePassword(r.Context(), session.UserID, string(newHash)); err != nil {
			data.Error = "Failed to update password"
			w.WriteHeader(http.StatusInternalServerError)
			h.renderChangePassword(w, data)
			return
		}

		data.Success = "Password updated successfully"
		h.renderChangePassword(w, data)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) renderChangePassword(w http.ResponseWriter, data ChangePasswordPageData) {
	if err := h.renderer.Render(w, "change_password", data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

// ProfileTokens handles GET and POST /profile/tokens.
func (h *Handlers) ProfileTokens(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	base, ok := h.baseData(w, r, session)
	if !ok {
		return
	}

	data := ProfileTokensPageData{BasePageData: base}

	switch r.Method {
	case http.MethodGet:
		tokens, err := h.authMgr.ListAPITokens(r.Context(), session.UserID)
		if err != nil {
			data.Error = "Could not load tokens"
		} else {
			data.Tokens = tokens
		}
		h.renderProfileTokens(w, data)

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = "Invalid form submission"
			w.WriteHeader(http.StatusBadRequest)
			h.renderProfileTokens(w, data)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			data.Error = "Token name is required"
			w.WriteHeader(http.StatusBadRequest)
			h.renderProfileTokens(w, data)
			return
		}
		plain, err := h.authMgr.CreateAPIToken(r.Context(), session.UserID, name)
		if err != nil {
			data.Error = "Could not create token"
			w.WriteHeader(http.StatusInternalServerError)
			h.renderProfileTokens(w, data)
			return
		}
		data.PlainToken = plain
		data.Success = "Token generated. Copy it now — it will not be shown again."
		tokens, _ := h.authMgr.ListAPITokens(r.Context(), session.UserID)
		data.Tokens = tokens
		h.renderProfileTokens(w, data)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) renderProfileTokens(w http.ResponseWriter, data ProfileTokensPageData) {
	if err := h.renderer.Render(w, "profile_tokens", data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

// ProfileTokenRevoke handles POST /profile/tokens/{id}/revoke.
func (h *Handlers) ProfileTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	// Path: /profile/tokens/{id}/revoke
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["profile", "tokens", "{id}", "revoke"]
	if len(parts) != 4 || parts[3] != "revoke" {
		http.NotFound(w, r)
		return
	}
	tokenID, err := strconv.Atoi(parts[2])
	if err != nil || tokenID <= 0 {
		http.Error(w, "Invalid token ID", http.StatusBadRequest)
		return
	}

	if err := h.authMgr.RevokeAPIToken(r.Context(), tokenID, session.UserID); err != nil {
		if errors.Is(err, auth.ErrTokenNotFound) {
			http.Error(w, "Token not found", http.StatusNotFound)
		} else {
			http.Error(w, "Could not revoke token", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, "/profile/tokens", http.StatusSeeOther)
}
