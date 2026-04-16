package profile

import (
	"context"
	"fmt"

	"sampleDB/internal/dbiface"
	"sampleDB/internal/web"
)

// Service is the domain interface for profile operations.
// All database access goes through this interface, making handlers testable.
type Service interface {
	// GetAdminStatus returns whether the given user is an admin. It is used to
	// populate BasePageData for templates that need the navigation state.
	GetAdminStatus(ctx context.Context, userID int) (bool, error)

	// GetPasswordHash retrieves the bcrypt hash stored for the given user. Returns
	// web.ErrNotFound if the user does not exist or has been deleted.
	GetPasswordHash(ctx context.Context, userID int) (string, error)

	// UpdatePassword replaces the stored password hash for the given user. Returns
	// web.ErrNotFound if the user does not exist or has been deleted.
	UpdatePassword(ctx context.Context, userID int, newHash string) error
}

// DBService is the concrete Service implementation backed by PostgreSQL.
type DBService struct {
	db dbiface.Pool
}

// NewService constructs a DBService backed by pool.
func NewService(db dbiface.Pool) *DBService {
	return &DBService{db: db}
}

// GetAdminStatus returns true if userID belongs to an active admin account.
func (s *DBService) GetAdminStatus(ctx context.Context, userID int) (bool, error) {
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false`,
		userID).Scan(&isAdmin)
	if err != nil {
		return false, fmt.Errorf("profile: get admin status: %w", err)
	}
	return isAdmin, nil
}

// GetPasswordHash retrieves the stored bcrypt hash for the given user.
// Returns web.ErrNotFound if the user record is absent or soft-deleted.
func (s *DBService) GetPasswordHash(ctx context.Context, userID int) (string, error) {
	var hash string
	err := s.db.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false`,
		userID).Scan(&hash)
	if err != nil {
		return "", web.ErrNotFound
	}
	return hash, nil
}

// UpdatePassword replaces the stored password hash for the given user.
// Returns web.ErrNotFound if no row was updated (user missing or soft-deleted).
func (s *DBService) UpdatePassword(ctx context.Context, userID int, newHash string) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE users SET password_hash = $1 WHERE user_id = $2 AND COALESCE(deleted, false) = false`,
		newHash, userID)
	if err != nil {
		return fmt.Errorf("profile: update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}
