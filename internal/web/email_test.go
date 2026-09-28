package web

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "alice@example.com", want: "alice@example.com"},
		{in: "  Alice.Smith+lab@Example.org \n", want: "Alice.Smith+lab@Example.org"},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "not-an-email", wantErr: true},
		{in: "alice@", wantErr: true},
		{in: "alice@localhost", wantErr: true},
		{in: "Alice <alice@example.com>", wantErr: true},
		{in: "alice@example.com, bob@example.com", wantErr: true},
		{in: strings.Repeat("a", 250) + "@x.io", wantErr: true},
		{in: "user?bcc=copy@example.com", wantErr: true},
		{in: "a#b@example.com", wantErr: true},
		{in: "a%b@example.com", wantErr: true},
		{in: "a&b@example.com", wantErr: true},
		{in: `"odd user"@example.com`, wantErr: true},
		{in: "o'brien@example.com", want: "o'brien@example.com"},
	}
	for _, tt := range tests {
		got, err := NormalizeEmail(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NormalizeEmail(%q) = %q, want error", tt.in, got)
			} else if !errors.Is(err, ErrValidation) {
				t.Errorf("NormalizeEmail(%q) error %v does not wrap ErrValidation", tt.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeEmail(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsUniqueViolation(t *testing.T) {
	if !IsUniqueViolation(&pgconn.PgError{Code: "23505"}) {
		t.Error("23505 should be a unique violation")
	}
	if IsUniqueViolation(&pgconn.PgError{Code: "23503"}) {
		t.Error("23503 should not be a unique violation")
	}
	if IsUniqueViolation(errors.New("boom")) {
		t.Error("plain error should not be a unique violation")
	}
}

func TestErrEmailTakenIsConflict(t *testing.T) {
	if !errors.Is(ErrEmailTaken, ErrConflict) {
		t.Error("ErrEmailTaken should wrap ErrConflict")
	}
}
