package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/app"
)

// TestSeedDefaultAdmin_MissingCreds verifies that SeedDefaultAdmin is a no-op
// when either AdminUsername or AdminPassword is empty.
func TestSeedDefaultAdmin_MissingCreds(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	cases := []struct {
		name     string
		username string
		password string
	}{
		{"empty username", "", "secret"},
		{"empty password", "admin", ""},
		{"both empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := app.Config{AdminUsername: tc.username, AdminPassword: tc.password}
			created, err := app.SeedDefaultAdmin(ctx, mock, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if created {
				t.Error("expected no-op (created=false) when credentials are missing")
			}
		})
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB calls: %v", err)
	}
}

// TestSeedDefaultAdmin_Idempotent verifies that SeedDefaultAdmin returns false
// without inserting when users already exist.
func TestSeedDefaultAdmin_Idempotent(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(3))

	cfg := app.Config{AdminUsername: "admin", AdminPassword: "admin"}
	created, err := app.SeedDefaultAdmin(ctx, mock, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected no-op (created=false) when users already exist")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

// TestSeedDefaultAdmin_CreatesAdmin verifies that SeedDefaultAdmin inserts a
// user row and returns true when the users table is empty.
func TestSeedDefaultAdmin_CreatesAdmin(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	cfg := app.Config{AdminUsername: "admin", AdminPassword: "admin"}
	created, err := app.SeedDefaultAdmin(ctx, mock, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true when users table is empty")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

// TestNewServer_Defaults verifies that NewServer returns a server with the
// expected address and non-zero timeouts.
func TestNewServer_Defaults(t *testing.T) {
	cfg := app.Config{Addr: ":9090"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	srv := app.NewServer(cfg, handler)

	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.Addr != ":9090" {
		t.Errorf("Addr = %q, want :9090", srv.Addr)
	}
	if srv.ReadTimeout <= 0 {
		t.Error("ReadTimeout must be positive")
	}
	if srv.WriteTimeout <= 0 {
		t.Error("WriteTimeout must be positive")
	}
	if srv.IdleTimeout <= 0 {
		t.Error("IdleTimeout must be positive")
	}
}

// TestNewServer_TLSConfig verifies that NewServer sets TLSConfig when UseTLS is true.
func TestNewServer_TLSConfig(t *testing.T) {
	cfg := app.Config{Addr: ":8443", UseTLS: true}
	srv := app.NewServer(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	if srv.TLSConfig == nil {
		t.Fatal("TLSConfig must not be nil when UseTLS is true")
	}
}

// TestNewServer_NoTLSConfig verifies that NewServer leaves TLSConfig nil when UseTLS is false.
func TestNewServer_NoTLSConfig(t *testing.T) {
	cfg := app.Config{Addr: ":8010", UseTLS: false}
	srv := app.NewServer(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	if srv.TLSConfig != nil {
		t.Error("TLSConfig must be nil when UseTLS is false")
	}
}
