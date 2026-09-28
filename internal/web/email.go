package web

import (
	"errors"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// maxEmailLen is the longest address the users.email column accepts
// (RFC 5321 path limit).
const maxEmailLen = 254

// NormalizeEmail trims s and validates it as a bare email address
// ("user@example.com"). Display-name forms such as "Bob <bob@example.com>"
// are rejected so the stored value is always directly usable in a mailto: link.
// The returned error wraps ErrValidation and has a user-facing message.
func NormalizeEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", validationError("Email is required")
	}
	if len(s) > maxEmailLen {
		return "", validationError("Email must be 254 characters or fewer")
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return "", validationError("Please enter a valid email address")
	}
	// RFC 5322 allows "?", "#", "%", "&", "/" and quoted local parts, but any of
	// them would change the meaning of a mailto: link built from the address
	// (e.g. "user?bcc=x@example.com" adds a BCC), and real lab addresses never
	// use them.
	if strings.ContainsAny(s, "?#%&/\\\"") {
		return "", validationError("Please enter a valid email address")
	}
	// net/mail accepts dotless domains ("alice@localhost"); a lab contact
	// address always has a real domain, so require at least one dot.
	if domain := s[strings.LastIndexByte(s, '@')+1:]; !strings.Contains(domain, ".") {
		return "", validationError("Please enter a valid email address")
	}
	return s, nil
}

// IsUniqueViolation reports whether err is a PostgreSQL unique_violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type validationErr struct{ msg string }

func (e validationErr) Error() string { return e.msg }
func (e validationErr) Unwrap() error { return ErrValidation }

func validationError(msg string) error { return validationErr{msg: msg} }
