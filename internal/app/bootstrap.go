package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/dbiface"
)

// SeedDefaultAdmin creates an admin user when the users table is empty and
// both cfg.AdminUsername and cfg.AdminPassword are non-empty. It is
// idempotent: once any user row exists it returns (false, nil) immediately.
func SeedDefaultAdmin(ctx context.Context, db dbiface.Pool, cfg Config) (bool, error) {
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		return false, nil
	}

	if cfg.AdminPassword == "admin" {
		log.Printf("WARNING: ADMIN_PASSWORD is set to the insecure default 'admin'. Set ADMIN_PASSWORD to a strong password before exposing this service.")
	}

	var count int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return false, nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("hash admin password: %w", err)
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO users (username, password_hash, is_approved, admin)
		VALUES ($1, $2, true, true)
		ON CONFLICT (username) DO NOTHING`,
		cfg.AdminUsername, string(hash)); err != nil {
		return false, fmt.Errorf("insert default admin: %w", err)
	}

	return true, nil
}

// NewServer constructs an *http.Server with standardised timeouts.
// TLS configuration is applied when cfg.UseTLS is true.
func NewServer(cfg Config, handler http.Handler) *http.Server {
	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	if cfg.UseTLS {
		srv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	return srv
}
