# Root Cleanup and Package Consolidation

## Overview
Reduce the repo root to a single thin `main.go` entry point by migrating all reusable bootstrap logic, HTTP helpers, and page handlers into their correct `internal/*` packages. No external HTTP routes or JSON wire formats change. The work finishes with a new `internal/integration` package that exercises the full stack against a real Postgres DB at `localhost:5432`, proving that the wire format and schema stay consistent after the refactor.

## Context (from discovery)
- **Files involved**: `main.go`, `api.go`, `admin.go`, `booking.go`, `fabrication_steps.go`, `fabrication_steps_handler.go`, `sample_prep_handler.go`, `sharing.go`, `markdown.go`, plus 12 root `*_test.go` files
- **Internal packages**: `internal/app`, `internal/agents` (new), `internal/web`, `internal/auth`, `internal/dbschema`, `internal/samples`, `internal/wiki`, `internal/booking`, `internal/admin`
- **DB pool**: `pgxpool.New()` → `dbiface.Pool` → `app.App.DB`; default DSN `postgres://app:app@localhost:5432/sampledb`
- **Router**: stdlib `http.ServeMux`
- **Existing contract tests**: `internal/samples/api_contract_test.go`, `internal/wiki/api_contract_test.go` (stub-based, not real DB)

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes — preserve existing uncommitted edits in already-modified files
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: no route changes, no JSON wire-format changes**

## Testing Strategy
- **Unit tests**: required for every task that adds or modifies logic
- **Integration tests** (Task 9): build-tagged `integration`, hit real Postgres, clean up own rows

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): tasks achievable within this codebase
- **Post-Completion** (no checkboxes): items requiring manual or external action

---

## Implementation Steps

### Task 1: Bootstrap config into `internal/app`
- [x] Create `internal/app/config.go` — export `Config` struct holding `Port`, `BaseDir`, `DatabaseURL`, `AdminEmail`, `AdminPassword`
- [x] Move `getEnv`/port-detection/base-dir helpers from `main.go` into `internal/app/config.go` as `LoadConfig() Config`
- [x] Update `main.go` to call `app.LoadConfig()` and pass the result to `app.New()`
- [x] Write unit tests in `internal/app/config_test.go` — env overrides, defaults, edge cases
- [x] Run `go test ./internal/app/... ./...` — must pass before Task 2

### Task 2: Bootstrap server startup into `internal/app`
- [ ] Create `internal/app/bootstrap.go` — export `SeedDefaultAdmin(ctx, db, cfg Config)` and `NewServer(cfg Config, db dbiface.Pool) *http.Server`
- [ ] Move default-admin seeding logic from `main.go` into `SeedDefaultAdmin`
- [ ] Move server construction (timeouts, addr) from `main.go` into `NewServer`
- [ ] Shrink `main.go` to: load config → open DB pool → run migrations → seed admin → start server
- [ ] Remove `App.Handlers` field from `internal/app/app.go`; verify route registration compiles without it
- [ ] Write unit tests for `SeedDefaultAdmin` (idempotent, missing creds no-op) in `internal/app/bootstrap_test.go`
- [ ] Run `go test ./internal/app/... ./...` — must pass before Task 3

### Task 3: Standardise `internal/web` helpers; remove root wrappers
- [ ] Audit root `api.go` and `markdown.go` — list every exported symbol they expose
- [ ] Verify each symbol already exists (or move it) into `internal/web`
- [ ] Delete `api.go` and `markdown.go` from repo root; fix any import errors
- [ ] Add/update unit tests in `internal/web` for any logic that moved (JSON helpers, markdown renderer)
- [ ] Run `go test ./internal/web/... ./...` — must pass before Task 4

### Task 4: Add `internal/agents` package
- [ ] Create `internal/agents/` with `handlers.go` — `AgentsHandler` and `AgentsExamplesHandler` (page data type, template parse/cache, example markdown loading)
- [ ] Add `rewriteRelativeImages(md string, prefix string) string` helper in `internal/agents/markdown.go`
- [ ] Wire `/agents` and `/agents/examples` routes in `internal/app/routes.go` to use `agents.AgentsHandler` / `agents.AgentsExamplesHandler`; remove any root-level handler stubs
- [ ] Rename `static/examples/scrpiting_automation.md` → `static/examples/scripting_automation.md`; update all string references (grep for the old name)
- [ ] Write unit tests in `internal/agents/handlers_test.go` — template rendering smoke test, image rewrite cases
- [ ] Run `go test ./internal/agents/... ./...` — must pass before Task 5

### Task 5: Delete obsolete root handler files
- [ ] Delete `admin.go`, `booking.go`, `fabrication_steps.go`, `fabrication_steps_handler.go`, `sample_prep_handler.go`, `sharing.go` from repo root
- [ ] Confirm all symbols previously defined there are now in their respective `internal/*` packages (grep for any remaining references)
- [ ] Run `go build ./...` and `go vet ./...` — must be clean before Task 6

### Task 6: Migrate root tests into their correct packages
- [ ] Move JSON/markdown root tests → `internal/web` (new or augmented test file)
- [ ] Move auth-rejection tests → `internal/auth`
- [ ] Move route/context tests → `internal/app`
- [ ] Move renderer/template smoke tests → `internal/web`
- [ ] Move migration/booking-constraint integration tests → `internal/dbschema`
- [ ] Verify moved tests still compile and pass in their new locations
- [ ] Run `go test ./internal/...` — must pass before Task 7

### Task 7: Delete low-value root tests and orphan assets
- [ ] Delete root test files: `admin_test.go`, `fabrication_steps_test.go`, `owner_test.go`, `sharing_test.go`, `gallery_test.go`
- [ ] Remove orphan tracked assets: `static/js/admin.js`, `static/js/sample-prep.js`, `static/images/ai_agent_examples/` (whole dir), `static/examples/mfia_plan_page.jpg`
- [ ] Remove any `.DS_Store` files tracked by git (`git rm --cached`)
- [ ] Remove empty `Users/` directory if tracked
- [ ] Confirm repo root contains only `main.go` plus any remaining legitimate root tests (e.g. `main_test.go` if it tests startup)
- [ ] Run `go test ./...` and `go vet ./...` — must pass before Task 8

### Task 8: Verify acceptance criteria
- [ ] Confirm no external HTTP routes changed: diff `internal/app/routes.go` route table against original
- [ ] Confirm no JSON wire-format changes: existing contract tests in `internal/samples` and `internal/wiki` still pass without modification
- [ ] Run full test suite: `go test ./...`
- [ ] Run `go vet ./...` — zero issues
- [ ] Confirm repo root contains only `main.go` (and possibly `main_test.go`)
- [ ] Confirm no `.DS_Store` or empty tracked directories remain

### Task 9: `internal/integration` — API tests against real Postgres
- [ ] Create `internal/integration/` package with `//go:build integration` tag on all files
- [ ] Create `internal/integration/setup_test.go` — `TestMain` opens `pgxpool`, runs `dbschema.Ensure()`, spins up `httptest.NewServer` with the full app router, exposes a `*http.Client` + base URL to subtests; defers server/pool teardown
- [ ] Create `internal/integration/samples_test.go` — golden-path HTTP contract: `POST /samples` (create), `GET /samples/:id` (read), `PUT /samples/:id` (update), `DELETE /samples/:id`; each test cleans up its own rows via `t.Cleanup`
- [ ] Create `internal/integration/wiki_test.go` — golden-path: create wiki entry, read, update, delete; `t.Cleanup`
- [ ] Create `internal/integration/booking_test.go` — golden-path: create booking, list bookings; `t.Cleanup`
- [ ] Create `internal/integration/auth_test.go` — register new user, login (expect session cookie), logout; `t.Cleanup` deletes test user row
- [ ] Create `internal/integration/admin_test.go` — verify default admin exists after `SeedDefaultAdmin`; list admin endpoint returns 200
- [ ] Each test asserts HTTP status, `Content-Type: application/json`, and key response fields (no schema-dump, just sentinel values)
- [ ] Document run command in `README` or `docs/api.md`: `DATABASE_URL=postgres://app:app@localhost:5432/sampledb go test -tags=integration ./internal/integration/... -v`
- [ ] Run integration suite locally against `localhost:5432` — all tests green
- [ ] Run `go test ./...` (no tag) — must still pass (integration files excluded)

---

## Technical Details

### Config struct (Task 1)
```go
type Config struct {
    Port          string
    BaseDir       string
    DatabaseURL   string
    AdminEmail    string
    AdminPassword string
}
```

### Integration test setup pattern (Task 9)
```go
//go:build integration

func TestMain(m *testing.M) {
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        dsn = "postgres://app:app@localhost:5432/sampledb"
    }
    pool, _ := pgxpool.New(context.Background(), dsn)
    dbschema.Ensure(context.Background(), pool)
    cfg := app.LoadConfig()
    srv := httptest.NewServer(app.NewRouter(cfg, pool))
    // expose srv.URL and pool to test helpers
    os.Exit(m.Run())
}
```

### Row cleanup pattern (Task 9)
```go
t.Cleanup(func() {
    pool.Exec(ctx, "DELETE FROM samples WHERE id = $1", id)
})
```

---

## Post-Completion

**Manual verification**
- Browse `/agents` and `/agents/examples` pages in a running dev server to confirm templates still render
- Verify `/agents/examples` still loads the renamed `scripting_automation.md` file
- Check browser console for 404s on removed static assets (`admin.js`, `sample-prep.js`)

**CI / deployment**
- Add `go test -tags=integration ./internal/integration/... -v` as a separate CI step that requires a Postgres service container (`postgres://app:app@localhost:5432/sampledb`)
- Ensure the renamed `scrpiting_automation.md` → `scripting_automation.md` is reflected in any external links or documentation

*Note: ralphex automatically moves completed plans to `docs/plans/completed/`*
