# Apply "Refined" Design Direction to Production App

## Overview

Port the "Direction A: Refined" prototype from `SampleDB_design/` to the production Go/template-based app so every screen matches the 10 reference screenshots captured on 2026-04-20 (`SampleDB_design/screenshots/`).

The app shell, sidebar, typography, and most template-level polish have already landed on the `redesign-refined` branch (commit `a6dd0bb "Redisign 0.1"` and follow-ups; `static/css/design.css` is 1,430 lines). The remaining work is **feature-level**: ship the missing Dashboard page, introduce sample status + wiki tags + equipment status, add an activity-log backend, and close visual gaps on each existing screen.

Key problems solved:
- **Dashboard** — no such page exists; screenshots #1 and #2 show KPI cards, Recent samples, Activity feed, and Today's bookings timeline.
- **Samples status system** — screenshots show "Active / In Progress / Archived" chips; the DB has no status column.
- **Activity feed** — sidebar on dashboard and sample-detail requires a cross-domain activity stream; no such infrastructure exists.
- **Wiki tags** — grid view shows tag chips (`Safety`, `Equipment`, `Protocols`); no tag column/table.
- **Equipment status + group access badges** — admin panel shows `● Online` chips and group-badges (`Barbaros`, `Ahmet`) per equipment row; schema has no status column and no group↔equipment link.
- **Wiki view TOC** — "On this page" sidebar listing headings is missing.

Thumbnails: per the user's choice, sample cards always show the first uploaded image; a neutral placeholder is shown when no image exists (no generated gradient art — divergence from prototype dir A is accepted).

## Context (from discovery)

**Partial integration already in place (commit `a6dd0bb`):**
- `static/css/design.css` — 1,430 lines, design tokens + sidebar shell + cards + chips + timeline styles.
- `templates/header.html` — new sidebar markup with Workspace / Account / Admin groups, user footer.
- `templates/base.html`, `main.html`, `sample_detail.html`, `wiki_list.html`, `wiki_view.html`, `admin.html`, `booking.html`, `profile_tokens.html`, `login.html`, `change_password.html`, `new_sample.html`, `fabrication_steps.html` — all updated toward the new design.
- QA report (`docs/QA_report.md`) already remediated via commit `fcbde3e`.

**Files/packages involved (Go):**
- `internal/app/routes.go` — registers all HTTP routes.
- `internal/samples/` — sample CRUD + list handler (reads `samples` table, composes the grid view in `templates/main.html`).
- `internal/booking/` — equipment + booking logic.
- `internal/wiki/` — articles + attachments.
- `internal/admin/` — user/group/equipment management.
- `internal/profile/` — API tokens.
- `internal/auth/` — session + approval.
- `internal/dbschema` — (may hold migration helpers; verify during Task 1).
- `DDL/init.sql` + `DDL/migrations/` — authoritative SQL.

**Relevant patterns discovered:**
- Migrations are appended to `DDL/init.sql` using `IF NOT EXISTS` / `DO $$ ... EXCEPTION WHEN duplicate_object $$` guards (see lines ~130–190) — same pattern must be used for new columns/tables.
- Handlers render templates via `web.Renderer`; list queries already support search + date filters (`main.html` toolbar form).
- Integration tests live in `internal/integration/` and spin up real Postgres; unit tests live alongside each package (e.g. `internal/samples/*_test.go`).
- HTMX is used for partial swaps (e.g. samples search `hx-target="#samples-panel"`).

**Dependencies identified:**
- Activity-log writes must be wired from the existing sample / booking / wiki / attachment handlers — every mutating handler gains one insert.
- Sample status must be displayed on samples list, sample detail header, and filterable via the query string (toolbar).
- Equipment group-access is a new many-to-many; admin UI must allow editing it.

## Development Approach

- **Testing approach**: Regular — implement feature, then add unit tests for handlers/queries and update integration tests for new routes.
- Complete each task fully before moving to the next.
- Make small, focused changes — do not rewrite handlers that already match the screenshot.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - unit tests for new functions and modified functions (success + error cases)
  - integration-level tests where a new route or template context is added
  - SQL migrations get tested via integration tests that assert the columns/tables exist and behave correctly under `internal/integration/`
- **CRITICAL: all tests must pass before starting next task** — no exceptions (`go test ./...`)
- **CRITICAL: update this plan file when scope changes during implementation**
- Run `go test ./...` and `go vet ./...` after every task; fix any linter/vet warnings before proceeding.
- Maintain backward compatibility of existing API routes under `/api/v1/` unless the task explicitly changes wire format (none do).

## Testing Strategy

- **Unit tests**: required for every task (see Development Approach above).
- **Integration tests** (`internal/integration/`): when a new route or DB table is added, extend the integration suite to hit the real Postgres and verify the response shape.
- **Visual verification**: the final "acceptance" task (Task 11) runs the app locally and compares each of the 10 screenshots side-by-side with the rendered page. Diffs must be reconciled in favor of the screenshot unless explicitly deviating (e.g. no gradient thumbnails).
- No UI-based e2e framework exists in this repo — skip e2e. Manual verification substitutes.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with `➕` prefix.
- Document issues/blockers with `⚠️` prefix.
- Update plan if implementation deviates from original scope.
- Keep plan in sync with actual work done.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes) — code, SQL migrations, templates, tests, package-level documentation updates achievable inside this repo.
- **Post-Completion** (no checkboxes) — manual visual QA at `http://localhost:8010`, optional tweaks based on in-browser review, deployment notes.

## Implementation Steps

### Task 1: Schema migrations (sample status, equipment status, wiki tags, equipment group-access, activity log)
- [x] append migration block to `DDL/init.sql` adding `samples.status VARCHAR(20) NOT NULL DEFAULT 'active'` with `CHECK (status IN ('active','in_progress','archived'))` guarded by `DO $$ BEGIN ... EXCEPTION WHEN duplicate_object THEN NULL; END $$` pattern
- [x] same block: `equipment.status VARCHAR(20) NOT NULL DEFAULT 'online'` with `CHECK (status IN ('online','offline','maintenance'))`
- [x] same block: `articles.tags TEXT[] NOT NULL DEFAULT '{}'`
- [x] same block: `CREATE TABLE IF NOT EXISTS equipment_group_access (equipment_id INT REFERENCES equipment(equipment_id) ON DELETE CASCADE, group_id INT REFERENCES groups(group_id) ON DELETE CASCADE, PRIMARY KEY (equipment_id, group_id))`
- [x] same block: `CREATE TABLE IF NOT EXISTS activity_log (id BIGSERIAL PRIMARY KEY, actor_user_id INT REFERENCES users(user_id) ON DELETE SET NULL, actor_username TEXT NOT NULL, verb VARCHAR(40) NOT NULL, target_type VARCHAR(40) NOT NULL, target_id INT, target_label TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)` + index on `(created_at DESC)` and `(target_type, target_id)`
- [x] add integration test in `internal/integration/` that connects to real Postgres, asserts all five schema changes are present, and verifies the CHECK constraints reject bad values
- [x] run `go test ./...` — must pass before next task

### Task 2: Activity logging infrastructure (new internal/activity package)
- [x] create `internal/activity/activity.go` with a `Logger` struct holding a `dbiface.Pool` and a `Log(ctx, actor, verb, targetType, targetID, targetLabel) error` method
- [x] expose helper `Recent(ctx, limit int) ([]Entry, error)` that returns the latest N entries joined with `users` for display
- [x] expose helper `ForTarget(ctx, targetType string, targetID int, limit int) ([]Entry, error)` for the sample-detail "Activity" panel
- [x] wire the logger into `App` in `internal/app/app.go` (new optional field `Activity *activity.Logger`, constructed in `NewRouter`)
- [x] hook activity inserts into: sample create/update/delete, attachment upload/delete (sample), fabrication-step add/update/delete, booking create/delete, wiki article create/update/delete, wiki attachment upload — each call-site gets one `a.Activity.Log(...)` right after the DB write succeeds (best-effort: log error, do not fail the request)
- [x] write unit tests for `Log`, `Recent`, `ForTarget` using `pgxmock`
- [x] write integration test that creates a sample, edits it, and verifies the activity_log has two rows with the right verbs
- [x] run `go test ./...` — must pass before next task

### Task 3: Dashboard page (new `/dashboard` route, KPIs, Recent samples, Activity feed, Today's bookings)
- [x] add `GET /dashboard` route in `internal/app/routes.go`; wire a new handler in `internal/samples/` (or a new `internal/dashboard/` package if it stays small) that aggregates KPI counts, recent samples, activity entries, and today's bookings
- [x] KPI queries: total samples (count), active bookings today (count from bookings where current_timestamp BETWEEN start_time AND end_time), wiki articles (count), pending approvals (count of users where is_approved=false AND deleted=false)
- [x] Recent samples: `SELECT ... ORDER BY created_at DESC LIMIT 5` with status + first-image join (reuse query helper from samples list)
- [x] Activity entries: `activity.Recent(ctx, 7)`
- [x] Today's bookings: `SELECT equipment.name, bookings.* WHERE start_time::date = current_date` ordered by start_time
- [x] create `templates/dashboard.html` matching screenshot #1/#2 — page-topbar with "Welcome back, {{.Username}}" + date, 4-KPI card grid, Recent samples list (left), Activity feed (right), Today's bookings timeline (bottom, reuse booking.html timeline CSS)
- [x] update `templates/header.html` sidebar — add "Dashboard" link at top of Workspace group (already shown in screenshots)
- [x] decide landing page: keep `/` = samples list (current behaviour) OR make `/` redirect to `/dashboard`. Per screenshots, Dashboard is the primary landing — redirect `/` to `/dashboard` for authenticated users; keep samples list at `/samples`
- [x] update all `/` references in templates and integration tests to `/samples`
- [x] write unit tests for dashboard handler (mocked DB, assert counts computed from mock rows)
- [x] write integration test hitting `/dashboard` with a seeded user + samples + booking + activity entry
- [x] run `go test ./...` — must pass before next task

### Task 4: Samples list — status chips, owner/status filters, Grid/Table toggle
- [x] extend `samples` list query in `internal/samples/` to include `status` column and allow filtering by `status` (`?status=active|in_progress|archived|all`) and by owner (`?owner=<username>|all`)
- [x] update template `main.html` toolbar: add "All owners" / "All status" chip-filters (dropdown or segmented control matching screenshot #3), "Grid | Table" toggle in top-right of samples panel
- [x] render status chip on each `sample-card` using `status-chip status-chip--{{.Status}}` (CSS classes already in `design.css` or add now — verify)
- [x] add Table view rendering inside `samples_panel` template — rows with columns: ID, Name, Owner, Status, Steps, Files, Created. Selected via `?view=table` query param; toggle button pushes URL via HTMX
- [x] add "New Sample" page support for setting initial status on create (admin override only; default `active`)
- [x] add "Edit sample" support for changing status (inline control or existing edit panel)
- [x] write unit tests for list-query filter combinations (`status=active`, `owner=foo`, `view=table`)
- [x] write integration tests for the new query params and the table-view render
- [x] run `go test ./...` — must pass before next task

### Task 5: Sample detail — Activity panel, Details/Danger-Zone polish, layout per screenshots
- [x] update `templates/sample_detail.html` to match screenshot #4/#5 — left column: Description card, Fabrication Steps, Attachments grid; right column: Details card (Owner, Status chip, Created, Last edit, Steps count, Files count), Activity card (last 6 entries from `activity.ForTarget("sample", id, 6)`), Danger Zone card
- [x] update the sample-detail handler in `internal/samples/` to fetch activity entries for the sample and pass them into the template context
- [x] add status chip next to the sample name in the page header (matches screenshot)
- [x] verify the Fabrication Steps rendering matches screenshot #5 — numbered rows with date, person, equipment, description
- [x] verify Share Link + Edit Sample Details panels — per QA INFO-1 these were noted as collapsed; decide whether to open by default or move into a top-right dropdown (match screenshot — a "Share" and "Edit" button in the page header) — kept as collapsed `<details>` panels for now; top-header Share/Edit buttons deferred
- [x] write unit tests for the handler's activity-panel data-fetch
- [x] write test that asserts the page header includes the status chip when `status != 'active'` (and still renders correctly for `active`)
- [x] run `go test ./...` — must pass before next task

### Task 6: Wiki list — tag grid with filters, article cards
- [x] extend `articles` query in `internal/wiki/handlers.go` to return `tags` and support `?tag=<tag>` filtering
- [x] define a stable tag vocabulary for the filter chips (`All | Safety | Equipment | Protocols | Admin`) matching screenshot #6; allow free-form tags beyond these (just not featured in chips)
- [x] update `templates/wiki_list.html` to render grid of article cards (screenshot #6) — each card: tag chip, title, one-line excerpt, date, author initial-avatar; filter-chip row above the grid
- [x] update "New Article" + "Edit Article" flows in `templates/wiki_edit.html` to accept tags (comma-separated input stored as TEXT[])
- [x] write unit tests for the list handler's tag filtering + for save-with-tags
- [x] write integration tests: create article with tags, list filtered by tag, verify tags render
- [x] run `go test ./...` — must pass before next task

### Task 7: Wiki view — "On this page" TOC sidebar
- [ ] in `internal/wiki/handlers.go` parse the article's markdown headings (h2/h3) to build a TOC slice `[{Anchor, Title, Level}]`; inject `id` attributes into the rendered HTML so anchor links work
- [ ] update `templates/wiki_view.html` to render the TOC as a right-column card matching screenshot #7; include "Back to Wiki" link above the article title
- [ ] ensure the existing edit/cancel/delete/attachment flows still function (preserve behaviour around QA BUG-4 fix — which already landed)
- [ ] write unit tests for the TOC extractor (edge cases: no headings, nested headings, duplicate titles)
- [ ] write integration test hitting `/wiki/view/<title>` and asserting TOC anchors match rendered h2 ids
- [ ] run `go test ./...` — must pass before next task

### Task 8: Equipment booking — verify screenshot match, minor polish
- [ ] compare `templates/booking.html` rendered output against screenshot #8 (timeline per-equipment rows, date navigation, Available-now chip, Book Slot button)
- [ ] fix any visual gaps (equipment status dot `● online` next to equipment name, `Book Slot` button style, date-picker layout)
- [ ] remove the commented-out "Rules" link stub per QA UX-4 (already remediated? verify in `templates/booking.html`)
- [ ] if booking data-fetch needs augmentation (e.g. include equipment.status), extend handler + tests accordingly
- [ ] write unit test asserting equipment status is included in the render context
- [ ] run `go test ./...` — must pass before next task

### Task 9: API Tokens page — verify screenshot match, minor polish
- [ ] compare `templates/profile_tokens.html` against screenshot #9 (two-column: Generate form left, Existing tokens table right + Usage example card below)
- [ ] ensure revoke button uses standardized `button--destructive button--small` (QA UX-1 — verify already fixed in commit `fcbde3e`)
- [ ] fix any visual gaps — layout split, token list columns (Name, Created, Last used, Status chip), Usage monospace block
- [ ] write unit test asserting revoke button renders with standard classes
- [ ] run `go test ./...` — must pass before next task

### Task 10: Admin panel — equipment status + group-access badges, user management polish
- [ ] update `internal/admin/` to fetch equipment with status and joined group-access rows; expose per-equipment "Add/Remove Group" controls
- [ ] extend `templates/admin.html` Equipment Catalogue section to render group badges per equipment and status chip per screenshot #10; wire buttons to new admin routes that add/remove rows in `equipment_group_access`
- [ ] add User Groups panel (list group names with member count + Add/Remove); already partly in admin.html — verify matches screenshot
- [ ] extend User Management table columns to match screenshot: User / Group (select) / Status chip / Admin toggle / Equipment Access (count + edit button)
- [ ] add Report Modal `name` attrs per QA UX-2 (verify already fixed in commit `fcbde3e`)
- [ ] write unit tests for the new group-access add/remove handlers and the equipment status field
- [ ] write integration test that creates an equipment, grants it to a group, and asserts the admin page renders the badge
- [ ] run `go test ./...` — must pass before next task

### Task 11: Verify acceptance criteria — visual compare + full regression
- [ ] verify all requirements from Overview are implemented (checklist: Dashboard exists, sample status chips + filter, activity log powering Dashboard + Sample detail, Wiki tags + grid, Wiki TOC, Equipment status + group badges)
- [ ] verify edge cases: empty dashboard (new install), sample with no activity, wiki article with no headings, user with no groups
- [ ] run full unit + integration test suite (`go test ./...`) — 0 failures
- [ ] run `go vet ./...` and `gofmt -l .` — 0 issues
- [ ] manual: start the server (`go run .` at repo root, or via Makefile if present), log in, visit every page in sidebar order, spot-compare against the 10 screenshots; note any remaining pixel-level drift
- [ ] verify dark and light themes both render correctly on every page (screenshots are in dark theme; light must also work)

### Task 12: [Final] Documentation updates
- [ ] update `README.md` — document `/dashboard`, sample status, wiki tags, equipment status, activity_log table
- [ ] update `CLAUDE.md` (if present) with any new internal patterns (activity.Logger usage)
- [ ] add a brief "Refined redesign" note to `docs/` summarizing the implemented scope vs the SampleDB_design/ prototype

*Note: ralphex automatically moves completed plans to `docs/plans/completed/`*

## Technical Details

**Data model additions:**
- `samples.status` — VARCHAR(20), CHECK (active | in_progress | archived), default `active`.
- `equipment.status` — VARCHAR(20), CHECK (online | offline | maintenance), default `online`.
- `articles.tags` — `TEXT[] NOT NULL DEFAULT '{}'`.
- `equipment_group_access` — `(equipment_id, group_id)` composite PK, cascade on both sides.
- `activity_log` — append-only, actor denormalized for display when user is deleted, `(target_type, target_id)` for efficient per-entity lookup.

**Activity verbs (controlled vocabulary):**
`created`, `updated`, `deleted`, `booked`, `uploaded`, `published`, `added_step`, `edited_step`, `granted`, `revoked`.

**Target types:**
`sample`, `attachment`, `fabrication_step`, `booking`, `article`, `article_attachment`, `equipment`, `user`.

**Route changes:**
- `GET /dashboard` — new.
- `GET /` — redirect to `/dashboard` for authenticated users; samples list moves to `GET /samples`.
- `GET /samples?status=...&owner=...&view=grid|table` — extended.
- `GET /wiki?tag=...` — extended.
- `POST /admin/equipment/{id}/groups/add` + `POST /admin/equipment/{id}/groups/remove` — new.

**Template context extensions (non-breaking):**
- Samples list: `.StatusFilter`, `.OwnerFilter`, `.View`, plus `.Status` on each sample.
- Sample detail: `.Activity []activity.Entry`.
- Wiki list: `.Articles[].Tags`, `.TagFilter`, `.Tags` (for filter chips).
- Wiki view: `.TOC []TOCEntry`.
- Admin: `.Equipment[].Status`, `.Equipment[].Groups []string`.
- Dashboard (new): `.KPIs`, `.RecentSamples`, `.Activity`, `.TodayBookings`, `.Username`, `.Now`.

**Processing flow (Activity pipeline):**
Handler → DB write succeeds → `App.Activity.Log(ctx, actor, verb, type, id, label)` → `INSERT INTO activity_log`. Errors in logging are logged via slog but never bubble up; they never fail the user's request.

## Post-Completion

*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification:**
- Visually compare each of the 10 screenshots against rendered pages in both dark and light themes.
- Smoke-test the full user journey: register → admin approves → login → Dashboard → create sample → add fabrication step → upload attachment → book equipment → write wiki article → verify Activity feed reflects each step.
- Performance: Dashboard issues ~5 queries per page load; if the app scales, add caching or reduce queries.

**External system updates:**
- If a production DB already exists, the new `NOT NULL DEFAULT` columns apply immediately; the `activity_log` table starts empty, so historical activity will not retroactively appear on the Dashboard.
- Commit the staged `SampleDB_design/` directory separately (it's design reference material, not production code); consider adding a `.gitattributes` entry or moving screenshots into `docs/design-reference/` to keep the repo root tidy. Out of scope for this plan.
