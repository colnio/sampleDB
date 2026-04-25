# SampleDB — AI Knowledge Base

## Build & Test

```bash
go build ./...
go test ./...

# Integration tests (require a running PostgreSQL instance)
DATABASE_URL="postgres://..." go test -tags=integration ./internal/integration/... -v
```

## Architecture

### Package layout

- `internal/activity/` — append-only activity log; every mutating handler calls `Logger.Log`. Three query shapes: `Recent(ctx, limit)` for the dashboard feed, `ForSample(ctx, id, limit)` for per-sample panels, `ForTarget(ctx, targetType, targetID, limit)` for non-sample domains (wiki, booking) — use this instead of adding new `For*` wrappers.
- `internal/app/` — HTTP router (`routes.go`), middleware (`csrf_middleware.go`, `RequireAdmin`), and app wiring (`app.go`).
- `internal/dbiface/` — minimal `Pool` interface that wraps `pgxpool.Pool`; all services accept this interface so tests can inject `pgxmock`.
- `internal/dbschema/` — idempotent schema bootstrap run on every startup; matches `DDL/init.sql`.
- Domain packages: `internal/admin`, `internal/booking`, `internal/dashboard`, `internal/samples`, `internal/wiki`, `internal/profile`, `internal/auth` — each owns its types, service interface + DB implementation, and HTTP handlers.
- `internal/wiki/toc.go` exports `BuildTOC(html template.HTML) []TOCEntry` — parses h2/h3 headings from rendered Markdown HTML and returns entries for the "On this page" sidebar. Call this in any handler rendering a full wiki article page.
- `wiki.KnownTags` in `internal/wiki/service.go` is the hard-coded vocabulary for the filter-chip row on `/wiki`. To add a new filterable tag, append to this slice — there is no DB backing table for the tag list.

### CSRF protection

All state-changing routes protected by `RequireCSRF` middleware (`internal/app/csrf_middleware.go`):

- Applied via `withCSRF(handler)` in `routes.go` on every non-GET route.
- Bearer-token API sessions have an empty `session.CSRFToken` and are automatically exempted.
- Web forms use the `{{csrfField .CSRFToken}}` template function (renders a hidden `<input name="csrf_token">`).
- HTMX requests get the token automatically via a global `htmx:configRequest` listener in `base.html` that reads `<meta name="csrf-token">`.
- The `CSRFToken string` field lives in each package's `BasePageData` and is populated from `session.CSRFToken`.

When adding a new state-changing route: wrap it with `withAuth(withCSRF(handler))` and ensure `CSRFToken` is set in the page data.

### CSS architecture

All page-specific CSS lives in named sections of `static/css/design.css`:

```
/* === Admin === */
/* === Auth === */
/* === Dashboard === */
/* === Samples === */
/* === Booking === */
/* === Wiki === */
/* === Profile === */
/* === Tokens === */
```

Inline `<style>` blocks in templates are forbidden. Inline `style=` attributes on individual elements are acceptable only for one-off layout values not worth a CSS class.

### Activity log pattern

Every mutating handler logs an activity entry:

```go
h.logActivity(r.Context(), session, activity.Verb("created"), activity.TargetType("sample"), sampleID, label)
```

The `Activity` field on `Handlers` structs is a `*activity.Logger`; a nil logger is always safe (best-effort write, errors are logged but never returned to the caller).

### Template function conventions

Template functions are registered in `internal/web/renderer.go`:
- `buildMainFuncMap` — used for all pages except booking.
- `buildBookingFuncMap` — extends the main map with booking-specific functions.

Key functions: `csrfField`, `markdown`, `toJSON`, `activityPhraseHTML`, `activityPhrase`, `statusChipClass`, `statusLabel`, `timeAgo`, `formatDate`, `formatTime24`.

### Adding a new admin route

```go
mux.HandleFunc("/admin/my-route", withAuth(adminMW(withCSRF(adminHdlrs.MyHandler))))
```

The ordering is important: `withAuth` → `adminMW` → `withCSRF` → handler.

## Conventions

- `BasePageData` in each package embeds `{Username, UserID, IsAdmin, CSRFToken}` — always populate `CSRFToken: session.CSRFToken`.
- Service interfaces use `dbiface.Pool`; tests inject `pgxmock`.
- Tests call `mock.ExpectationsWereMet()` at the end of every DB test.
- `web.ErrNotFound` / `web.ErrForbidden` are the sentinel errors for 404/403 responses.
