package app

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Config holds all runtime configuration for the application.
// Values are populated by LoadConfig from environment variables.
type Config struct {
	Addr         string
	RedirectAddr string
	DatabaseURL  string
	TLSCertFile  string
	TLSKeyFile   string
	PublicHost   string
	TLSPort      string
	BaseDir      string
	TemplatesDir string
	StaticDir    string
	UploadsDir   string
	UseTLS       bool
}

// LoadConfig reads environment variables and returns a fully populated Config.
func LoadConfig() Config {
	cfg := Config{
		Addr:         getEnv("APP_ADDR", ":8010"),
		RedirectAddr: os.Getenv("APP_HTTP_REDIRECT_ADDR"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://app:app@localhost:5432/sampledb"),
		TLSCertFile:  os.Getenv("TLS_CERT_FILE"),
		TLSKeyFile:   os.Getenv("TLS_KEY_FILE"),
		PublicHost:   os.Getenv("PUBLIC_HOST"),
	}

	cfg.UseTLS = cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
	if cfg.UseTLS && cfg.Addr == ":8010" {
		cfg.Addr = ":8443"
	}

	cfg.BaseDir = determineBaseDir()
	cfg.TemplatesDir = getEnv("TEMPLATES_DIR", filepath.Join(cfg.BaseDir, "templates"))
	cfg.StaticDir = getEnv("STATIC_DIR", filepath.Join(cfg.BaseDir, "static"))
	cfg.UploadsDir = getEnv("UPLOADS_DIR", filepath.Join(cfg.BaseDir, "uploads"))

	cfg.TLSPort = extractPort(cfg.Addr)

	if cfg.PublicHost == "" {
		if cfg.UseTLS {
			if cfg.TLSPort != "" && cfg.TLSPort != "443" {
				cfg.PublicHost = fmt.Sprintf("localhost:%s", cfg.TLSPort)
			} else {
				cfg.PublicHost = "localhost"
			}
		} else if cfg.TLSPort != "" && cfg.TLSPort != "80" {
			cfg.PublicHost = fmt.Sprintf("localhost:%s", cfg.TLSPort)
		} else {
			cfg.PublicHost = "localhost"
		}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func determineBaseDir() string {
	if base := os.Getenv("APP_BASE_DIR"); base != "" {
		if abs, err := filepath.Abs(base); err == nil {
			return abs
		}
	}

	if exe, err := os.Executable(); err == nil {
		if abs, err := filepath.Abs(filepath.Dir(exe)); err == nil {
			return abs
		}
	}

	if wd, err := os.Getwd(); err == nil {
		if abs, err := filepath.Abs(wd); err == nil {
			return abs
		}
	}

	return "."
}

func extractPort(addr string) string {
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, ":") {
		return strings.TrimPrefix(addr, ":")
	}

	_, port, err := net.SplitHostPort(addr)
	if err == nil {
		return port
	}

	return ""
}
