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
