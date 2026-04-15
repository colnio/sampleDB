# SampleDB Full Feature Overhaul

## Overview

Major upgrade across four areas:
1. **REST API with API key auth** — JSON endpoints for all resources, bearer token authentication alongside existing session cookies. Foundation for MCP/Claude integration and iOS app development.
2. **Samples page overhaul** — Multi-file upload with copy-paste, owner assigned from real users, structured fabrication steps (date/person/equipment/description), image gallery.
3. **Sample sharing by link** — Per-sample shareable URLs with two visibility modes: public (no login) or authenticated (login required).
4. **Wiki UX/UI improvements** — Image gallery for attachments, better non-text content experience.

## Context (from discovery)

- Files/components involved: `main.go` (routes), `booking.go` (samples handlers), `wiki.go` (wiki handlers), `internal/auth/` (session auth), `DDL/init.sql` (schema), `templates/` (HTML templates)
- Related patterns found: HTMX used for inline editing; single-file upload via multipart form; session cookie auth; `sample_owner` is free text (VARCHAR 100); `sample_prep` is Markdown text
- Dependencies identified: pgx v5, gomarkdown, existing HTMX setup; `equipment` table already exists for booking — can reuse for fabrication step equipment field

## Development Approach

- **Testing approach**: TDD — write failing tests first, then implement
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility (existing web UI must remain fully functional)

## Testing Strategy

- **Unit tests**: required for every task
- **Integration tests**: API endpoints should have HTTP-level tests using `httptest`
- **DB tests**: schema migration tests verify table structure after migration
- E2E (manual): no Playwright/Cypress in this project; note manual test scenarios in Post-Completion

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code changes, tests, DB migrations
- **Post-Completion**: manual UI/UX verification, iOS/MCP integration, deployment

---

## Implementation Steps

### Task 1: Schema migrations

- [x] create `DDL/migrations/001_fabrication_steps.sql` — add `fabrication_steps` table: `(step_id SERIAL PK, sample_id INT FK→samples, step_order INT NOT NULL, step_date DATE, person_user_id INT FK→users nullable, description TEXT, equipment_id INT FK→equipment nullable, created_at TIMESTAMP)`
- [x] create `DDL/migrations/002_api_tokens.sql` — add `api_tokens` table: `(token_id SERIAL PK, user_id INT FK→users, token_hash VARCHAR(64) NOT NULL UNIQUE, name VARCHAR(100), created_at TIMESTAMP, last_used_at TIMESTAMP, revoked BOOLEAN DEFAULT false)`
- [x] create `DDL/migrations/003_sample_owner_fk.sql` — add `owner_user_id INT REFERENCES users(user_id) NULL` to `samples` (keep old `sample_owner` text field for legacy display)
- [x] create `DDL/migrations/004_sample_sharing.sql` — add `share_token VARCHAR(64) UNIQUE NULL` and `share_visibility VARCHAR(20) DEFAULT 'private'` (values: `'private'`, `'public'`, `'authenticated'`) to `samples`
- [x] apply all migrations to local dev DB
- [x] write tests in `migrations_test.go` verifying each table/column exists after migration
- [x] run tests — must pass before task 2

### Task 2: API key system + dual auth middleware

- [x] create `internal/auth/apitoken.go` — `GenerateToken() (plain, hash string, err)` using `crypto/rand` (32 bytes, hex-encoded); `HashToken(plain string) string` using SHA-256
- [x] add DB methods to auth package: `CreateAPIToken(userID int, name string) (plainToken string, err error)`, `ValidateAPIToken(plain string) (userID int, err error)` (checks hash, not revoked), `ListAPITokens(userID int) ([]APIToken, error)`, `RevokeAPIToken(tokenID int, userID int) error`
- [x] write tests for `GenerateToken` and `HashToken`
- [x] write tests for `CreateAPIToken`, `ValidateAPIToken` (success + revoked + wrong token)
- [x] update `withAuth()` middleware: check session cookie first; if absent, check `Authorization: Bearer <token>` header and validate via `ValidateAPIToken`; update `last_used_at` on successful token auth
- [x] write tests for dual auth middleware (cookie path, bearer path, invalid token → 401)
- [x] add `GET /profile/tokens` — list user's API tokens with name, created date, last used
- [x] add `POST /profile/tokens` — generate new token (form: token name); display plain token once on success
- [x] add `POST /profile/tokens/{id}/revoke` — revoke token
- [x] add template `templates/profile_tokens.html` with token list + generate form
- [x] run tests — must pass before task 3

### Task 3: REST API endpoints (core)

- [x] create `api.go` — all `/api/v1/*` handlers; separate file from HTML handlers
- [x] write helper `writeJSON(w, status, v)` and `writeError(w, status, msg)` for consistent JSON responses
- [x] `GET /api/v1/samples` — list samples; supports query params: `q` (keyword), `owner`, `from`, `to`, `limit`, `offset`; returns JSON array with `id, name, description, keywords, owner_username, created_at, attachment_count`
- [x] `GET /api/v1/samples/{id}` — full sample detail: fields + fabrication steps + attachments list
- [x] `POST /api/v1/samples` — create sample; JSON body; returns created sample
- [x] `PUT /api/v1/samples/{id}` — update sample fields; returns updated sample
- [x] `DELETE /api/v1/samples/{id}` — delete sample; 204 on success
- [x] `GET /api/v1/samples/{id}/steps` — list fabrication steps for sample
- [x] `POST /api/v1/samples/{id}/steps` — add fabrication step
- [x] `PUT /api/v1/steps/{id}` — update step
- [x] `DELETE /api/v1/steps/{id}` — delete step
- [x] `GET /api/v1/wiki` — list articles (id, title, created_at, last_modified_at)
- [x] `GET /api/v1/wiki/{id}` — article detail: title, content (raw markdown), attachments
- [x] `POST /api/v1/wiki` — create article
- [x] `PUT /api/v1/wiki/{id}` — update article
- [x] `GET /api/v1/users` — list approved users (id, username, group) — for owner/person pickers
- [x] `GET /api/v1/equipment` — list equipment (id, name, location)
- [x] register all `/api/v1/*` routes in `main.go`; apply `withAuth()` middleware to all
- [x] write HTTP-level tests for each endpoint: success path, auth required (401 without token), not found (404), create + retrieve round-trip
- [x] run tests — must pass before task 4

### Task 4: Sample owner from user list

- [x] update `getSamples()` and `getSample()` DB queries to JOIN `users` on `owner_user_id` and include `owner_username`
- [x] update sample create handler to accept `owner_user_id` from form (integer); update `updateSample()` to save it
- [x] update `new_sample.html` template: replace owner text input with `<select>` populated with approved users; keep a fallback text display if `owner_user_id` is null (for legacy samples)
- [x] update `sample_detail.html` / edit form similarly
- [x] write tests for `getSample()` with `owner_user_id` set and unset
- [x] write tests for update handler with valid and invalid owner_user_id
- [x] run tests — must pass before task 5

### Task 5: Fabrication steps UI

- [x] add DB functions: `getFabricationSteps(sampleID int)`, `addFabricationStep(sampleID int, step FabricationStep)`, `updateFabricationStep(stepID int, step FabricationStep)`, `deleteFabricationStep(stepID int)`, `reorderFabricationSteps(sampleID int, orderedIDs []int)`
- [x] add HTML handlers: `POST /samples/{id}/steps` (add), `POST /steps/{id}/edit` (update), `POST /steps/{id}/delete` (delete), `POST /samples/{id}/steps/reorder` (reorder via drag)
- [x] create `templates/fabrication_steps.html` partial — HTMX-powered step list; each step shows date, person (username), description, equipment name; inline edit/delete buttons
- [x] create `templates/step_form.html` partial — add-step form: date picker, person dropdown (approved users), equipment dropdown, description textarea; "Add Step" button appends via HTMX
- [x] integrate steps panel into `sample_detail.html` alongside existing prep panel
- [x] write tests for all step DB functions (add, update, delete, get, reorder)
- [x] write tests for step HTTP handlers (add step → verify returned HTML contains step; delete → verify removed)
- [x] run tests — must pass before task 6

### Task 6: Multi-file upload + copy-paste

- [ ] update `POST /samples/{id}/upload` handler to iterate `r.MultipartForm.File["files"]` (plural) instead of a single file; process each file in a loop
- [ ] update `POST /wiki/upload/{article_id}` handler similarly
- [ ] update `templates/sample_detail.html` upload section: replace single-file `<input type="file">` with `<input type="file" multiple>`; add drag-and-drop zone div with JS handler
- [ ] add `static/js/upload.js` — handles: (a) drag-and-drop onto zone, (b) paste event on `document` detecting `image/*` MIME type in clipboard items, (c) submits files via `fetch()` to upload endpoint; updates attachment list via HTMX swap after success
- [ ] apply same drag-drop + paste zone to wiki article attachment upload
- [ ] show per-file upload progress using `XMLHttpRequest` upload events or `fetch` with ReadableStream
- [ ] write tests for multi-file upload handler (2 files in one request → 2 attachment rows created)
- [ ] write tests for paste upload (simulate FormData with image blob)
- [ ] run tests — must pass before task 7

### Task 7: Image gallery/player

- [ ] add `static/js/gallery.js` — lightweight lightbox: clicking a thumbnail opens a modal overlay; left/right arrow key + click navigation between images in the set; ESC to close; no external dependencies
- [ ] add `static/css/gallery.css` — modal overlay, thumbnail grid, navigation arrows
- [ ] update `sample_detail.html` attachment display: group images into a thumbnail grid; each thumbnail links to lightbox; non-image files remain as a separate file list below
- [ ] update wiki article view (`templates/wiki_view.html` or equivalent): same thumbnail grid + lightbox for article image attachments
- [ ] write tests for gallery JS (jsdom or simple DOM tests if project has a JS test setup; else document as manual test)
- [ ] write tests for attachment handler returning correct `Content-Type` header for images (needed for gallery detection)
- [ ] run tests — must pass before task 8

### Task 8: Sample sharing by link

- [ ] add DB functions: `generateShareToken(sampleID int, visibility string) (token string, err error)` — creates a random 32-byte hex token, saves to `share_token`/`share_visibility`; `getSampleByShareToken(token string) (Sample, error)`; `revokeShareToken(sampleID int) error`
- [ ] add handler `GET /s/{token}` — public route (no `withAuth()` middleware): fetch sample by token; if `share_visibility == 'public'` serve without login; if `'authenticated'` require valid session or bearer token; if `'private'` return 404
- [ ] create `templates/sample_shared.html` — read-only sample view (name, description, keywords, owner, fabrication steps, image gallery); no edit controls; shows "Login to see more" if unauthenticated and visibility is `'authenticated'`
- [ ] add share management panel to `sample_detail.html`: show current share status; "Generate link" button (`POST /samples/{id}/share`); visibility radio (Public / Authenticated users); copy-to-clipboard button; "Revoke" button
- [ ] add handlers: `POST /samples/{id}/share` (generate/update share), `POST /samples/{id}/share/revoke`
- [ ] write tests for `generateShareToken`, `getSampleByShareToken` (public + authenticated + private visibility)
- [ ] write tests for `GET /s/{token}` (public access without auth, authenticated access with/without session, revoked token → 404)
- [ ] run tests — must pass before task 9

### Task 9: Wiki UX/UI improvements

- [ ] update wiki edit page (`templates/wiki_edit.html`) to add a live Markdown preview pane (split view: editor left, preview right; rendered via `POST /wiki/preview` or client-side JS)
- [ ] add `POST /wiki/preview` handler — accepts `{"content": "..."}`, returns rendered HTML; used by editor for live preview
- [ ] add "Insert image" helper button in wiki editor — opens a mini attachment picker showing already-uploaded images for the article; inserts `![filename](url)` at cursor
- [ ] apply the image gallery/player (Task 7 gallery.js) to wiki article view for inline image attachments
- [ ] write tests for `POST /wiki/preview` handler (valid markdown → HTML response)
- [ ] run tests — must pass before task 10

### Task 10: General UX/UI polish

- [ ] sample list (`templates/main.html`): show first image thumbnail as a card preview if sample has image attachments; add subtle hover shadow on sample cards
- [ ] sample detail (`templates/sample_detail.html`): collapse/expand panels (steps, attachments, edit form) using `<details>` or HTMX toggle; improve mobile layout (stack panels vertically on small screens)
- [ ] add loading spinners for HTMX operations (use `htmx:beforeRequest` / `htmx:afterRequest` events in existing JS)
- [ ] profile page: add token management section link to nav (only shown to logged-in users)
- [ ] write basic render tests for main.html and sample_detail.html templates (verify template executes without error with mock data)
- [ ] run tests — must pass before task 11

### Task 11: Verify acceptance criteria

- [ ] verify multi-file upload and paste upload work end-to-end
- [ ] verify fabrication steps add/edit/delete/reorder work
- [ ] verify owner dropdown populates with real users
- [ ] verify share links work for both visibility modes
- [ ] verify `GET /api/v1/samples` returns JSON with valid Bearer token and 401 without
- [ ] verify all REST API endpoints respond correctly
- [ ] verify wiki live preview works
- [ ] verify gallery lightbox opens and navigates
- [ ] run full test suite: `go test ./...` — all must pass
- [ ] run linter: `golangci-lint run` or `go vet ./...` — all issues fixed
- [ ] verify no regressions: existing login, booking, admin, wiki edit flows still work

### Task 12: [Final] Update documentation

- [ ] update `README.md` — add REST API section: auth (Bearer token), base URL, key endpoints
- [ ] create `docs/api.md` — full endpoint reference: method, path, auth required, request/response JSON examples for all `/api/v1/*` routes
- [ ] update README environment variables section if any new config added

---

## Technical Details

### fabrication_steps table
```sql
CREATE TABLE fabrication_steps (
    step_id       SERIAL PRIMARY KEY,
    sample_id     INT NOT NULL REFERENCES samples(sample_id) ON DELETE CASCADE,
    step_order    INT NOT NULL DEFAULT 0,
    step_date     DATE,
    person_user_id INT REFERENCES users(user_id) ON DELETE SET NULL,
    description   TEXT,
    equipment_id  INT REFERENCES equipment(equipment_id) ON DELETE SET NULL,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### api_tokens table
```sql
CREATE TABLE api_tokens (
    token_id    SERIAL PRIMARY KEY,
    user_id     INT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    token_hash  VARCHAR(64) NOT NULL UNIQUE,  -- SHA-256 hex of plain token
    name        VARCHAR(100),
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    revoked     BOOLEAN NOT NULL DEFAULT false
);
```

### Samples additions
```sql
ALTER TABLE samples ADD COLUMN owner_user_id INT REFERENCES users(user_id) ON DELETE SET NULL;
ALTER TABLE samples ADD COLUMN share_token VARCHAR(64) UNIQUE;
ALTER TABLE samples ADD COLUMN share_visibility VARCHAR(20) NOT NULL DEFAULT 'private';
```

### API token flow
1. User visits `/profile/tokens`, clicks "Generate token", enters a name
2. Server generates 32 random bytes → hex string (plain token), stores SHA-256 hash in DB
3. Plain token shown once in UI (copy it now message)
4. iOS/MCP client sends `Authorization: Bearer <plain_token>` on every request
5. Middleware hashes the incoming token and looks up the hash in `api_tokens`

### Share token flow
- Token: 32 random bytes as hex (64 chars)
- `GET /s/{token}` is unauthenticated route
- Visibility `'public'`: serve without any auth check
- Visibility `'authenticated'`: check session cookie or Bearer token; 403 if absent
- Visibility `'private'` or token not found: 404

### REST API response conventions
- Success: `200 OK` with JSON body, or `201 Created`, or `204 No Content`
- Errors: `{"error": "message"}` with appropriate HTTP status
- Lists: `{"items": [...], "total": N, "limit": N, "offset": N}`
- Auth failure: `401 Unauthorized` with `{"error": "authentication required"}`

---

## Post-Completion

**Manual UI/UX verification:**
- Test copy-paste image upload in Safari, Chrome, Firefox
- Test drag-and-drop upload with multiple files simultaneously
- Test gallery lightbox on mobile (touch swipe navigation)
- Test wiki live preview with large Markdown documents
- Verify sample detail page layout on iPhone-sized viewport

**iOS app integration (future):**
- Use `/api/v1/samples` and `/api/v1/wiki` as data sources
- Authenticate with Bearer tokens generated from `/profile/tokens`
- Upload images via `POST /api/v1/samples/{id}/attachments` (to be added in iOS-specific plan)

**MCP/Claude integration (future):**
- Expose `api.md` endpoint list as MCP tool definitions
- Each API endpoint maps to an MCP tool (e.g., `list_samples`, `get_sample`, `add_fabrication_step`)
- MCP server wraps REST API calls with an API key stored in MCP config

*Note: ralphex automatically moves completed plans to `docs/plans/completed/`*
