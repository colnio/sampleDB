package app_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

func TestNew_ReturnsNonNilApp(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	cfg := app.Config{
		DatabaseURL: "postgres://localhost/test",
		BaseDir:     "/tmp",
	}
	authMgr := auth.NewManager(mock)
	renderer := &web.Renderer{}
	logger := slog.Default()
	clock := time.Now
	loc := time.UTC

	a, err := app.New(cfg, mock, authMgr, renderer, logger, clock, loc)
	if err != nil {
		t.Fatalf("app.New returned error: %v", err)
	}
	if a == nil {
		t.Fatal("app.New returned nil App")
	}
}

func TestNew_FieldsAccessible(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	cfg := app.Config{
		DatabaseURL: "postgres://localhost/test",
		BaseDir:     "/tmp",
	}
	authMgr := auth.NewManager(mock)
	renderer := &web.Renderer{}
	logger := slog.Default()
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixedTime }
	loc := time.FixedZone("TestZone", 3600)

	a, err := app.New(cfg, mock, authMgr, renderer, logger, clock, loc)
	if err != nil {
		t.Fatalf("app.New returned error: %v", err)
	}

	if a.Config.DatabaseURL != cfg.DatabaseURL {
		t.Errorf("Config.DatabaseURL = %q, want %q", a.Config.DatabaseURL, cfg.DatabaseURL)
	}
	if a.DB != mock {
		t.Error("DB field does not match provided pool")
	}
	if a.Auth != authMgr {
		t.Error("Auth field does not match provided manager")
	}
	if a.Renderer != renderer {
		t.Error("Renderer field does not match provided renderer")
	}
	if a.Logger != logger {
		t.Error("Logger field does not match provided logger")
	}
	if a.Clock == nil {
		t.Error("Clock field is nil")
	}
	if got := a.Clock(); !got.Equal(fixedTime) {
		t.Errorf("Clock() = %v, want %v", got, fixedTime)
	}
	if a.Location != loc {
		t.Error("Location field does not match provided location")
	}
}

func TestNew_RequiresDB(t *testing.T) {
	cfg := app.Config{DatabaseURL: "postgres://localhost/test"}
	authMgr := &auth.Manager{}
	renderer := &web.Renderer{}
	logger := slog.Default()
	clock := time.Now
	loc := time.UTC

	_, err := app.New(cfg, nil, authMgr, renderer, logger, clock, loc)
	if err == nil {
		t.Fatal("expected error when DB is nil, got nil")
	}
}

func TestNew_RequiresAuth(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	cfg := app.Config{DatabaseURL: "postgres://localhost/test"}
	renderer := &web.Renderer{}
	logger := slog.Default()
	clock := time.Now
	loc := time.UTC

	_, err = app.New(cfg, mock, nil, renderer, logger, clock, loc)
	if err == nil {
		t.Fatal("expected error when Auth is nil, got nil")
	}
}

func TestNew_RequiresLocation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	cfg := app.Config{DatabaseURL: "postgres://localhost/test"}
	authMgr := auth.NewManager(mock)
	renderer := &web.Renderer{}
	logger := slog.Default()
	clock := time.Now

	_, err = app.New(cfg, mock, authMgr, renderer, logger, clock, nil)
	if err == nil {
		t.Fatal("expected error when Location is nil, got nil")
	}
}

// TestDoubleInitialization proves that two App instances created in the same
// process are fully independent — no package-level globals are shared between
// them.  Each instance gets its own DB mock; writing to one must not affect the
// other.
func TestDoubleInitialization(t *testing.T) {
	mock1, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock1.Close()

	mock2, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock2.Close()

	cfg := app.Config{DatabaseURL: "postgres://localhost/test", BaseDir: "/tmp"}
	loc1 := time.FixedZone("Zone1", 3600)
	loc2 := time.FixedZone("Zone2", 7200)

	a1, err := app.New(cfg, mock1, auth.NewManager(mock1), &web.Renderer{}, slog.Default(), time.Now, loc1)
	if err != nil {
		t.Fatalf("app.New (first instance): %v", err)
	}
	a2, err := app.New(cfg, mock2, auth.NewManager(mock2), &web.Renderer{}, slog.Default(), time.Now, loc2)
	if err != nil {
		t.Fatalf("app.New (second instance): %v", err)
	}

	if a1 == a2 {
		t.Fatal("both App instances point to the same address — not independent")
	}
	if a1.DB == a2.DB {
		t.Error("a1 and a2 share the same DB pool — global state leaked")
	}
	if a1.Auth == a2.Auth {
		t.Error("a1 and a2 share the same Auth manager — global state leaked")
	}
	if a1.Location == a2.Location {
		t.Error("a1 and a2 share the same Location — global state leaked")
	}
	if a1.Location.String() != "Zone1" {
		t.Errorf("a1.Location = %q, want Zone1", a1.Location.String())
	}
	if a2.Location.String() != "Zone2" {
		t.Errorf("a2.Location = %q, want Zone2", a2.Location.String())
	}
}
