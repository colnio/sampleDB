//go:build integration

package integration_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/dbschema"
)

var (
	srv      *httptest.Server
	pool     *pgxpool.Pool
	apiToken string
	testUser struct {
		id       int
		username string
	}
	bgCtx = context.Background()
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:app@localhost:5432/sampledb"
	}

	var err error
	pool, err = pgxpool.New(bgCtx, dsn)
	if err != nil {
		log.Fatalf("integration: connect postgres: %v", err)
	}

	if err = dbschema.Ensure(bgCtx, pool); err != nil {
		log.Fatalf("integration: ensure schema: %v", err)
	}

	cfg := app.LoadConfig()
	cfg.DatabaseURL = dsn

	srv = httptest.NewServer(app.NewRouter(cfg, pool))

	testUser.username = "inttest_" + randomHex(8)
	hash, _ := bcrypt.GenerateFromPassword([]byte("inttest_password"), bcrypt.DefaultCost)
	if err = pool.QueryRow(bgCtx,
		"INSERT INTO users (username, password_hash, is_approved, admin) VALUES ($1, $2, true, true) RETURNING user_id",
		testUser.username, string(hash)).Scan(&testUser.id); err != nil {
		log.Fatalf("integration: create test user: %v", err)
	}

	plain, tokenHash, genErr := auth.GenerateToken()
	if genErr != nil {
		log.Fatalf("integration: generate api token: %v", genErr)
	}
	if _, err = pool.Exec(bgCtx,
		"INSERT INTO api_tokens (user_id, token_hash, name) VALUES ($1, $2, 'integration-test')",
		testUser.id, tokenHash); err != nil {
		log.Fatalf("integration: store api token: %v", err)
	}
	apiToken = plain

	code := m.Run()

	pool.Exec(bgCtx, "DELETE FROM users WHERE user_id = $1", testUser.id)
	srv.Close()
	pool.Close()
	os.Exit(code)
}

// apiDo executes an authenticated JSON request and returns the response.
func apiDo(t testing.TB, method, path string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, body)
	if err != nil {
		t.Fatalf("build request %s %s: %v", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, path, err)
	}
	return resp
}

// noRedirectClient is an http.Client that does not follow redirects.
var noRedirectClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// randomHex returns n random hex bytes.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// mustReadBody reads and closes the response body, failing the test on error.
func mustReadBody(t testing.TB, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// jsonBody wraps a JSON string as an io.Reader.
func jsonBody(s string) io.Reader {
	return strings.NewReader(s)
}
