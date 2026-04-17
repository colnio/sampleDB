package app_test

import (
	"os"
	"strings"
	"testing"

	"sampleDB/internal/app"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear relevant env vars so defaults are exercised.
	unset := []string{
		"APP_ADDR", "APP_HTTP_REDIRECT_ADDR", "DATABASE_URL",
		"TLS_CERT_FILE", "TLS_KEY_FILE", "PUBLIC_HOST",
		"APP_BASE_DIR", "TEMPLATES_DIR", "STATIC_DIR", "UPLOADS_DIR",
	}
	for _, k := range unset {
		t.Setenv(k, "")
	}

	cfg := app.LoadConfig()

	if cfg.Addr != ":8010" {
		t.Errorf("Addr = %q, want :8010", cfg.Addr)
	}
	if cfg.DatabaseURL != "postgres://app:app@localhost:5432/sampledb" {
		t.Errorf("DatabaseURL = %q, want default DSN", cfg.DatabaseURL)
	}
	if cfg.UseTLS {
		t.Error("UseTLS should be false when no cert/key provided")
	}
	if cfg.TLSPort != "8010" {
		t.Errorf("TLSPort = %q, want 8010", cfg.TLSPort)
	}
	if cfg.PublicHost == "" {
		t.Error("PublicHost should not be empty")
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/mydb")
	t.Setenv("PUBLIC_HOST", "example.com")
	t.Setenv("APP_HTTP_REDIRECT_ADDR", ":8080")

	cfg := app.LoadConfig()

	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want :9090", cfg.Addr)
	}
	if cfg.DatabaseURL != "postgres://user:pass@db:5432/mydb" {
		t.Errorf("DatabaseURL = %q, want custom DSN", cfg.DatabaseURL)
	}
	if cfg.PublicHost != "example.com" {
		t.Errorf("PublicHost = %q, want example.com", cfg.PublicHost)
	}
	if cfg.RedirectAddr != ":8080" {
		t.Errorf("RedirectAddr = %q, want :8080", cfg.RedirectAddr)
	}
}

func TestLoadConfig_TLSDefaultsAddr(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "cert*.pem")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	k, err := os.CreateTemp(t.TempDir(), "key*.pem")
	if err != nil {
		t.Fatal(err)
	}
	k.Close()

	t.Setenv("TLS_CERT_FILE", f.Name())
	t.Setenv("TLS_KEY_FILE", k.Name())
	t.Setenv("APP_ADDR", "") // force default

	cfg := app.LoadConfig()

	if !cfg.UseTLS {
		t.Error("UseTLS should be true when cert and key are provided")
	}
	// When TLS is enabled and addr was the HTTP default, it should switch to :8443.
	if cfg.Addr != ":8443" {
		t.Errorf("Addr = %q, want :8443 when TLS enabled with default addr", cfg.Addr)
	}
}

func TestLoadConfig_DirsInheritBaseDir(t *testing.T) {
	t.Setenv("APP_BASE_DIR", "/srv/app")
	t.Setenv("TEMPLATES_DIR", "")
	t.Setenv("STATIC_DIR", "")
	t.Setenv("UPLOADS_DIR", "")

	cfg := app.LoadConfig()

	if cfg.BaseDir != "/srv/app" {
		t.Errorf("BaseDir = %q, want /srv/app", cfg.BaseDir)
	}
	if !strings.HasPrefix(cfg.TemplatesDir, "/srv/app") {
		t.Errorf("TemplatesDir = %q, want prefix /srv/app", cfg.TemplatesDir)
	}
	if !strings.HasPrefix(cfg.StaticDir, "/srv/app") {
		t.Errorf("StaticDir = %q, want prefix /srv/app", cfg.StaticDir)
	}
	if !strings.HasPrefix(cfg.UploadsDir, "/srv/app") {
		t.Errorf("UploadsDir = %q, want prefix /srv/app", cfg.UploadsDir)
	}
}

func TestLoadConfig_PublicHostDerivedFromAddr(t *testing.T) {
	t.Setenv("APP_ADDR", ":9999")
	t.Setenv("PUBLIC_HOST", "")
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_KEY_FILE", "")

	cfg := app.LoadConfig()

	if cfg.PublicHost != "localhost:9999" {
		t.Errorf("PublicHost = %q, want localhost:9999", cfg.PublicHost)
	}
}
