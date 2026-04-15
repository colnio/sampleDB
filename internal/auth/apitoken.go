package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrTokenInvalid is returned by ValidateAPIToken when the token does not exist
// or has been revoked. Use errors.Is to distinguish it from database errors.
var ErrTokenInvalid = errors.New("invalid or revoked api token")

// ErrTokenNotFound is returned by RevokeAPIToken when no matching token exists
// for the given tokenID and userID (token absent or owned by another user).
var ErrTokenNotFound = errors.New("token not found or not owned by user")

// APIToken represents an API token record from the database.
type APIToken struct {
	TokenID    int
	UserID     int
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	Revoked    bool
}

// GenerateToken creates a new random 32-byte token, returning both the plain
// hex string (shown to the user once) and its SHA-256 hash (stored in DB).
func GenerateToken() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate api token: %w", err)
	}
	plain = hex.EncodeToString(b)
	hash = HashToken(plain)
	return plain, hash, nil
}

// HashToken returns the hex-encoded SHA-256 hash of a plain token string.
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// CreateAPIToken generates a new token for the given user, stores the hash in
// the database, and returns the plain token (shown once to the user).
func (m *Manager) CreateAPIToken(ctx context.Context, userID int, name string) (string, error) {
	plain, hash, err := GenerateToken()
	if err != nil {
		return "", err
	}

	_, err = m.db.Exec(ctx,
		`INSERT INTO api_tokens (user_id, token_hash, name) VALUES ($1, $2, $3)`,
		userID, hash, name)
	if err != nil {
		return "", fmt.Errorf("create api token: %w", err)
	}
	return plain, nil
}

// ValidateAPIToken checks the plain token against stored hashes and returns the
// owning userID. Returns an error if the token is not found or is revoked.
// On success it updates last_used_at.
func (m *Manager) ValidateAPIToken(ctx context.Context, plain string) (int, error) {
	hash := HashToken(plain)

	var tokenID, userID int
	var revoked bool
	err := m.db.QueryRow(ctx,
		`SELECT token_id, user_id, revoked FROM api_tokens WHERE token_hash = $1`,
		hash).Scan(&tokenID, &userID, &revoked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrTokenInvalid
		}
		return 0, fmt.Errorf("validate api token: %w", err)
	}
	if revoked {
		return 0, ErrTokenInvalid
	}

	// Best-effort update of last_used_at; ignore errors so auth is not blocked.
	_, _ = m.db.Exec(ctx,
		`UPDATE api_tokens SET last_used_at = NOW() WHERE token_id = $1`,
		tokenID)

	return userID, nil
}

// ListAPITokens returns all non-deleted tokens belonging to the given user.
func (m *Manager) ListAPITokens(ctx context.Context, userID int) ([]APIToken, error) {
	rows, err := m.db.Query(ctx,
		`SELECT token_id, user_id, COALESCE(name,''), created_at, last_used_at, revoked
		   FROM api_tokens
		  WHERE user_id = $1
		  ORDER BY created_at DESC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.TokenID, &t.UserID, &t.Name, &t.CreatedAt, &t.LastUsedAt, &t.Revoked); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// RevokeAPIToken marks a token as revoked. It verifies that the token belongs
// to the given userID to prevent cross-user revocations.
func (m *Manager) RevokeAPIToken(ctx context.Context, tokenID, userID int) error {
	tag, err := m.db.Exec(ctx,
		`UPDATE api_tokens SET revoked = true WHERE token_id = $1 AND user_id = $2`,
		tokenID, userID)
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	return nil
}
