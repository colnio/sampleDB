# Codex Review

## Findings

### Critical

1. **Unassigned samples are mutable by any authenticated user**
   - The create form allows `owner_user_id` to be empty in `templates/new_sample.html`.
   - `newSampleHandler` persists `owner_user_id = NULL` in `main.go`.
   - `requireSampleOwner` treats `NULL` ownership as editable by any logged-in user in `sharing.go`.
   - Impact: any authenticated user can edit, upload/delete attachments, or create/revoke share links for ownerless samples.

2. **Booking conflict prevention is not enforced atomically**
   - `handleBookingSubmission` performs a read-then-write conflict check in `booking.go`.
   - The schema only has a regular index on booking times in `internal/dbschema/schema.go`.
   - Impact: concurrent requests can double-book the same equipment slot.

### Major

1. **Runtime schema bootstrap does not match manual SQL bootstrap**
   - `dbschema.Ensure` executes `schemaStatements`, but `dataStatements` is empty in `internal/dbschema/schema.go`.
   - `DDL/init.sql` includes seed/backfill logic that runtime bootstrap does not execute.
   - README presents runtime bootstrap as the main fast path.
   - Impact: app behavior differs depending on whether schema was applied manually or by app startup.

2. **Session state is process-local**
   - `auth.Manager` stores sessions in an in-memory map in `internal/auth/manager.go`.
   - Impact: all sessions are lost on restart, and multi-instance deployment is not viable.

3. **Admin access updates can partially succeed**
   - `handleUpdateAccess` logs and skips equipment permission insert failures, then still commits in `admin.go`.
   - Impact: UI can report success while permissions are only partially applied.

4. **Request-scoped DB work often ignores request context**
   - Many handlers call `context.Background()` instead of `r.Context()`.
   - Seen in `main.go`, `admin.go`, `booking.go`, and `wiki.go`.
   - Impact: canceled requests still consume DB work and resources.

5. **Booking calendar query excludes overlapping boundary bookings**
   - `getBookingsForWeek` only returns bookings fully contained inside the selected range in `booking.go`.
   - Impact: bookings overlapping week boundaries may not appear in the UI.

### Minor

1. **Routing and path parsing are duplicated**
   - Path dispatch logic is spread across `main.go`, `wiki.go`, and `fabrication_steps_handler.go`.
   - Impact: higher maintenance cost and greater risk of inconsistent behavior.

2. **Template rendering is duplicated and reparsed per request**
   - `parseTemplates` and `parseTemplatesBooking` duplicate renderer setup.
   - Impact: unnecessary complexity and overhead.

3. **The root package is too large**
   - `main.go` is 1821 lines, `api.go` is 1125 lines, and the app relies on globals for DB/config/auth/location.
   - Impact: low cohesion, weak testability, and high change risk.

## Verification

- `go test ./...` passes
- `go vet ./...` passes

This indicates the main issues are architectural, authorization, and runtime behavior problems rather than active compile/test failures.

## Refactoring And Restructuring Plan

### Summary

Refactor the app around clear domain boundaries while keeping current routes and JSON contracts stable. Fix the authorization and booking-integrity bugs first, then remove global state and extract domain services and handlers.

### Public Interfaces And Structural Changes

1. Introduce `internal/app.App`
   - Holds `Config`, `DB`, `Auth`, `Renderer`, `Logger`, `Clock`, and `Location`.
   - Root `main.go` becomes bootstrap-only.

2. Keep current route surface stable
   - Preserve existing HTML routes and `/api/v1/*` endpoints.
   - Move route registration into a dedicated app/router layer.

3. Add shared infrastructure packages
   - `internal/web` for rendering, JSON helpers, error mapping, and path parsing.
   - Shared typed sentinel errors such as `ErrNotFound`, `ErrForbidden`, `ErrConflict`, `ErrValidation`.

### Implementation Changes

#### Phase 0: Safety fixes before structural moves

1. Tighten sample ownership rules
   - New samples should default to the authenticated owner.
   - Non-admin users should not be able to create unassigned samples.
   - Legacy ownerless samples should become admin-fixable, not globally editable.

2. Enforce booking conflicts in PostgreSQL
   - Add a migration using `btree_gist` and an exclusion constraint on equipment/time ranges.
   - Replace application-only conflict prevention with DB-backed enforcement.

3. Make admin access updates transactional
   - Any permission insert failure should abort and roll back the whole update.

4. Replace `context.Background()` in request flows
   - Use `r.Context()` consistently across handlers and data access.

5. Align runtime schema bootstrap with `DDL/init.sql`
   - Move seed/backfill logic into runtime bootstrap or otherwise guarantee parity.
   - Add tests to ensure both bootstrapping paths produce equivalent state.

#### Phase 1: Remove globals and centralize wiring

1. Eliminate package-level globals
   - Replace `dbPool`, `appConfig`, `authManagerInstance`, and `loc` with injected dependencies through `App`.

2. Centralize route registration
   - Move route wiring out of `main.go` into `internal/app/routes.go`.

3. Unify template rendering
   - Replace `parseTemplates` and `parseTemplatesBooking` with one renderer abstraction.
   - Cache parsed templates and support partial rendering cleanly.

#### Phase 2: Split by domain

1. `internal/samples`
   - Sample CRUD
   - Attachments
   - Sharing
   - Fabrication steps
   - Sample HTML handlers
   - Sample API handlers

2. `internal/wiki`
   - Article CRUD
   - Article attachments
   - Preview
   - Wiki HTML handlers
   - Wiki API handlers

3. `internal/booking`
   - Equipment queries
   - Permission checks
   - Booking create/delete/list logic
   - Booking calendar/report handlers

4. `internal/admin`
   - User/group/equipment management
   - Admin page handlers

5. `internal/profile`
   - Change password
   - API token management

6. Keep `internal/auth`
   - Extend it with persistent session storage via Postgres-backed session records.

### Testing Plan

1. Ownership tests
   - Verify non-owners cannot edit/delete/share samples.
   - Cover legacy ownerless samples after migration.

2. Booking concurrency tests
   - Submit overlapping bookings in parallel.
   - Verify only one succeeds.

3. Bootstrap parity tests
   - Validate runtime `Ensure` and `DDL/init.sql` produce equivalent schema/data behavior.

4. Session persistence tests
   - Verify restart-safe session behavior once session storage is persisted.

5. Route regression tests
   - Keep current HTML paths and `/api/v1/*` response contracts stable during refactor.

### Assumptions

- Standard library routing remains in place.
- Current URLs and API payload shapes remain stable.
- PostgreSQL is available for additional migrations, including persistent sessions and booking exclusion constraints.
