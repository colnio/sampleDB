# SampleDB Full Refactor (Codex Review Implementation)

## Overview

Refactor sampleDB around clear domain boundaries while keeping all current routes and JSON contracts stable. The work is divided into three phases:

- **Phase 0** — Fix critical/major safety and correctness bugs (ownership, atomic booking, transactional admin, request context, schema parity)
- **Phase 1** — Remove global state, centralize wiring, unify template rendering
- **Phase 2** — Split code by domain into `internal/samples`, `internal/wiki`, `internal/booking`, `internal/admin`, `internal/profile`

The result is a testable, deployable app with strong ownership invariants and no in-process global state.

## Context (from discovery)

- **Files involved:** `main.go` (1821 lines), `api.go` (1125 lines), `admin.go` (671 lines), `booking.go`, `wiki.go`, `sharing.go`, `fabrication_steps*.go`, `internal/auth/`, `internal/dbschema/`
- **Tech stack:** Go 1.23, PostgreSQL, standard-library HTTP mux, HTMX, server-rendered HTML templates
- **Patterns found:** session auth + Bearer token API, template re-parsing per request, package-level globals for DB/config/auth/location, runtime schema bootstrap via `dbschema.Ensure`
- **Dependencies identified:** `pgx/v5`, `pgxmock/v3`, `gomarkdown`, `golang.org/x/crypto`

## Development Approach

- **Testing approach:** TDD — write failing tests first, then implement to make them pass
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run `go test ./...` after each change
- Maintain backward compatibility — existing HTML routes and `/api/v1/*` contracts stay stable

## Testing Strategy

- **Unit tests:** required for every task
- **Integration tests:** use `pgxmock` for DB-dependent tests; real-DB tests for concurrency/constraint verification
- No UI e2e test suite exists — validate HTML routes manually or via `httptest`

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): tasks achievable within this codebase
- **Post-Completion** (no checkboxes): items requiring external action or manual verification

---

## Implementation Steps

---

### Phase 0 — Safety Fixes

---

### Task 1: Enforce sample ownership at creation

- [x] write failing test: non-admin creating a sample gets `owner_user_id` set to their own user ID
- [x] write failing test: non-admin cannot POST a sample with an explicit `owner_user_id` different from their own
- [x] write failing test: admin can create sample with arbitrary or empty owner
- [x] update `newSampleHandler` in `main.go` to always assign `owner_user_id = sessionUserID` for non-admins, ignoring any form field value
- [x] update `templates/new_sample.html` to hide or remove the owner selector for non-admins
- [x] run tests — must pass before task 2

### Task 2: Lock down ownerless samples to admins only

- [x] write failing test: `requireSampleOwner` denies edit/share/attachment operations for non-admin when `owner_user_id IS NULL`
- [x] write failing test: admin can still edit ownerless sample
- [x] update `requireSampleOwner` in `sharing.go` to treat NULL ownership as admin-only, not world-editable
- [x] run tests — must pass before task 3

### Task 3: Atomic booking conflict prevention via DB exclusion constraint

- [x] write failing test: two concurrent booking inserts for overlapping time/equipment produce exactly one success
- [x] write a migration in `DDL/migrations/` enabling `btree_gist` and adding an exclusion constraint on `(equipment_id, tsrange(start_time, end_time))`
- [x] update `internal/dbschema/schema.go` to include the new migration in `schemaStatements`
- [x] update `handleBookingSubmission` in `booking.go` to catch the exclusion constraint violation and return a user-facing conflict error instead of a raw DB error
- [x] run tests — must pass before task 4

### Task 4: Make admin access updates transactional

- [x] write failing test: if any equipment permission insert fails inside `handleUpdateAccess`, the entire update is rolled back and no partial state is committed
- [x] wrap the multi-step permission update in `admin.go::handleUpdateAccess` in a single `pgx` transaction; abort and return an error on any failure
- [x] run tests — must pass before task 5

### Task 5: Replace `context.Background()` with `r.Context()` in request handlers

- [x] write a test that cancels a request context and verifies the in-flight DB query is also canceled (pick one representative handler, e.g. the sample list query)
- [x] grep `context.Background()` in `main.go`, `admin.go`, `booking.go`, `wiki.go`, `api.go` and replace each occurrence inside an HTTP handler with `r.Context()`
- [x] run tests — must pass before task 6

### Task 6: Align runtime schema bootstrap with `DDL/init.sql`

- [x] write failing test: after running `dbschema.Ensure` on a blank DB, the schema state is equivalent to applying `DDL/init.sql` (spot-check key tables, seed rows, indexes)
- [x] audit `DDL/init.sql` for seed/backfill SQL that `dataStatements` in `internal/dbschema/schema.go` is missing
- [x] add missing statements to `dataStatements` (or `schemaStatements` if structural) so both bootstrap paths are equivalent
- [x] run tests — must pass before phase 1

### Task 7: Fix booking calendar boundary query

- [x] write failing test: a booking that spans a week boundary appears in both adjacent week views
- [x] update `getBookingsForWeek` in `booking.go` to use an overlap condition (`start_time < week_end AND end_time > week_start`) instead of a containment condition
- [x] run tests — must pass before phase 1

---

### Phase 1 — Remove Globals, Centralize Wiring

---

### Task 8: Introduce `internal/app.App` struct

- [x] write failing test: constructing `app.New(cfg, db, auth, renderer, logger, clock, loc)` returns a non-nil `App` with each field accessible
- [x] create `internal/app/app.go` with `App` struct holding `Config`, `DB`, `Auth`, `Renderer`, `Logger`, `Clock`, `Location`
- [x] create constructor `app.New(...)` that validates required fields
- [x] run tests — must pass before task 9

### Task 9: Eliminate package-level globals

- [x] write failing test: the application can be initialized twice in the same process (proves no global side-effects)
- [x] replace `dbPool`, `appConfig`, `authManagerInstance`, and `loc` globals in `main.go` with fields on `app.App`
- [x] thread `*app.App` (or its individual fields) into handlers that previously read globals
- [x] run tests — must pass before task 10

### Task 10: Introduce `internal/web` helpers

- [ ] write failing test: `web.RenderError(w, err)` maps sentinel errors (`ErrNotFound`, `ErrForbidden`, `ErrConflict`, `ErrValidation`) to correct HTTP status codes
- [ ] create `internal/web/errors.go` with sentinel error types and `RenderError`
- [ ] create `internal/web/json.go` with `JSON(w, status, v)` and `DecodeJSON(r, v)` helpers
- [ ] create `internal/web/paths.go` with path segment parsing helpers, consolidating duplicated logic from `main.go`, `wiki.go`, `fabrication_steps_handler.go`
- [ ] run tests — must pass before task 11

### Task 11: Centralize route registration

- [ ] write failing test: calling `app.RegisterRoutes(mux)` registers all expected route patterns (sample, wiki, booking, admin, api, auth)
- [ ] create `internal/app/routes.go` with `RegisterRoutes(*http.ServeMux)` that wires all handlers
- [ ] remove inline route wiring from `main.go` and replace with `app.RegisterRoutes(mux)`
- [ ] run tests — must pass before task 12

### Task 12: Unify template rendering

- [ ] write failing test: `web.Renderer.Render(w, "template_name", data)` selects and executes the correct template
- [ ] write failing test: `Renderer` is initialized once (not per-request) and concurrent calls are safe
- [ ] replace `parseTemplates` and `parseTemplatesBooking` in `main.go` with a single `web.Renderer` that caches parsed templates at startup
- [ ] update all call sites to use `Renderer.Render`
- [ ] run tests — must pass before phase 2

---

### Phase 2 — Domain Split

---

### Task 13: Extract `internal/samples` package

- [ ] write failing tests for sample CRUD, attachment, sharing, and fabrication-step operations against the new package's service interface
- [ ] create `internal/samples/` with service interface and implementation covering: sample CRUD, attachments, sharing tokens, fabrication steps
- [ ] move sample HTML handlers from `main.go` to `internal/samples/handlers.go`
- [ ] move sample API handlers from `api.go` to `internal/samples/api_handlers.go`
- [ ] update `internal/app/routes.go` to mount the new handlers
- [ ] run tests — must pass before task 14

### Task 14: Extract `internal/wiki` package

- [ ] write failing tests for article CRUD, attachments, and preview against the new package's service interface
- [ ] create `internal/wiki/` with service + handlers migrated from `wiki.go` and `api.go`
- [ ] update route registration
- [ ] run tests — must pass before task 15

### Task 15: Extract `internal/booking` package

- [ ] write failing tests for equipment query, permission check, booking create/delete/list, and calendar/report handlers
- [ ] create `internal/booking/` with service + handlers migrated from `booking.go` and `api.go`
- [ ] verify concurrency test from Task 3 still passes against new service layer
- [ ] update route registration
- [ ] run tests — must pass before task 16

### Task 16: Extract `internal/admin` package

- [ ] write failing tests for user/group/equipment management handlers
- [ ] create `internal/admin/` with handlers migrated from `admin.go`
- [ ] update route registration
- [ ] run tests — must pass before task 17

### Task 17: Extract `internal/profile` package

- [ ] write failing tests for change-password and API token management handlers
- [ ] create `internal/profile/` with handlers migrated from relevant sections of `main.go` and `api.go`
- [ ] update route registration
- [ ] run tests — must pass before task 18

### Task 18: Persist sessions in PostgreSQL

- [ ] write failing test: sessions survive an `auth.Manager` restart (simulated by constructing a new Manager backed by the same DB)
- [ ] add a `sessions` table migration to `internal/dbschema`
- [ ] update `internal/auth/manager.go` to store/load sessions via Postgres instead of the in-memory map
- [ ] run tests — must pass before task 19

### Task 19: Verify acceptance criteria

- [ ] run `go test ./...` — all tests pass
- [ ] run `go vet ./...` — no issues
- [ ] verify all critical/major findings from `docs/plans/codex_review.md` are addressed
- [ ] verify existing HTML routes return correct responses (spot-check via `httptest`)
- [ ] verify `/api/v1/*` JSON contracts are unchanged
- [ ] verify test coverage is ≥ 80% on changed packages

### Task 20: [Final] Update documentation

- [ ] update `README.md` to reflect new package structure and bootstrap instructions
- [ ] remove or update any references to old globals or monolithic `main.go`
- [ ] update inline code comments where architecture descriptions are now outdated

*Note: ralphex automatically moves completed plans to `docs/plans/completed/`*

---

## Technical Details

- **Exclusion constraint:** requires `btree_gist` extension; migration must call `CREATE EXTENSION IF NOT EXISTS btree_gist` before adding the constraint
- **Session storage:** new `sessions` table with columns `(id TEXT PRIMARY KEY, user_id INT, data JSONB, expires_at TIMESTAMPTZ)`
- **Sentinel errors:** `ErrNotFound`, `ErrForbidden`, `ErrConflict`, `ErrValidation` defined in `internal/web/errors.go`; handlers map these to 404/403/409/400
- **App struct fields:** `Config`, `DB *pgxpool.Pool`, `Auth *auth.Manager`, `Renderer *web.Renderer`, `Logger *slog.Logger`, `Clock func() time.Time`, `Location *time.Location`
- **Template caching:** parse all templates once at `app.New()` time; store in `Renderer`; avoid per-request parsing

## Post-Completion

**Manual verification:**
- Test equipment booking conflict in a browser with two simultaneous form submissions
- Test session persistence by restarting the binary and confirming an existing browser session remains valid
- Verify admin access updates roll back cleanly when a permission insert is forced to fail (can be triggered via a DB-side constraint violation)

**External considerations:**
- If deploying behind a load balancer, confirm session table is accessible from all instances
- Consumers of the `/api/v1/*` endpoints should not need changes (contract is stable)
