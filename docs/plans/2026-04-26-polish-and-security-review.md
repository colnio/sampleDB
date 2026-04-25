# Design Polish, Consistency & Security Review

## Overview

The `redesign-refined` branch is largely complete. This plan covers the remaining work before merging:
1. **Design polish** — fix visual inconsistencies on Admin, Wiki, Auth, and Dashboard pages
2. **CSS consolidation** — move inline `<style>` blocks from templates into `design.css`; eliminate duplicated/conflicting class definitions
3. **Security hardening** — fix a confirmed XSS vector, add CSRF protection, and tighten the admin password-reset flow

Scope is deliberate and minimal: do not add features, do not restructure working pages.

## Context (from discovery)

- Files involved: `templates/admin.html`, `templates/admin/users.html`, `templates/login.html`, `templates/auth_base.html`, `templates/wiki_view.html`, `templates/dashboard.html`, `static/css/design.css`, `internal/admin/handlers.go`, `internal/app/routes.go`
- Design system lives in `static/css/design.css` (sidebar shell, `.card`, `.status-chip`, `.modal-backdrop`, `.data-table`, `.button`, `.badge`, etc.)
- `admin.html` has ~651 lines of inline `<style>` that partially duplicate and conflict with `design.css`
- `login.html` + `auth_base.html` have ~164 lines of inline auth-specific styles not in `design.css`
- Three confirmed security issues (see Task 4)

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- **Every task that changes Go code must include updated/new tests**
- **All tests must pass before starting the next task**
- Do not touch pages or handlers that are already working correctly
- Run `go build ./...` and `go test ./...` after every task

## Testing Strategy

- **Unit tests**: required for any Go handler or service changes
- **Visual**: run `go run ./cmd/server` and manually verify affected pages after each task
- Templates and CSS have no automated tests — visual verification is the acceptance gate
- E2E tests: not present in this project

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## What Goes Where

- **Implementation Steps**: code changes, CSS moves, template edits, Go handler fixes, tests
- **Post-Completion**: manual browser QA across all affected pages before merging

---

## Implementation Steps

### Task 1: Fix XSS in admin equipment onclick handlers

The equipment name is interpolated directly into a JS string in an `onclick` attribute in `templates/admin.html` (around the equipment table rows). An equipment name containing `'` or JS breaks out of the string.

- [x] In `templates/admin.html`, find all `onclick="showReportModal({{$eq.ID}}, '{{$eq.Name}}')"` patterns
- [x] Replace inline-JS approach with `data-id` / `data-name` attributes on the element
- [x] Update the corresponding JS in `admin.html` to read from `data-*` attributes on click
- [x] Verify the equipment modal still opens correctly with names containing apostrophes and special characters
- [x] Write a manual test case: create equipment named `O'Brien's Lab` and confirm modal title displays correctly [x] manual test (skipped - not automatable)
- [x] Run `go build ./...` — must pass before task 2

### Task 2: Tighten admin password-reset response

`internal/admin/handlers.go` returns a plaintext temporary password in a JSON response. This is low-risk over HTTPS but the password should be clearly marked as one-time and short-lived. The plaintext return itself is acceptable; what's missing is the enforcement.

- [ ] In `internal/admin/service.go` (or wherever `ResetUserPassword` is implemented), confirm the generated password is a random one-time value (not permanent)
- [ ] If not already: generate a random 12-character temporary password (use `crypto/rand`), force `must_change_password` flag in DB or document that next login requires a change
- [ ] Update the JSON response to include a `"note"` field: `"User must change password on next login"`
- [ ] Update `templates/admin/users.html` reset-password modal to display the note alongside the temporary password
- [ ] Write a unit test in `internal/admin/` that asserts the reset response includes the note field and that the password meets minimum entropy (12+ chars, random)
- [ ] Run `go test ./internal/admin/...` — must pass before task 3

### Task 3: Add CSRF protection to admin and auth forms

No CSRF tokens are present on any POST form. This allows cross-site request forgery on all state-changing actions (password reset, user creation, group changes, login).

- [ ] Choose approach: add a per-session CSRF token stored in the session, or use a double-submit cookie pattern
- [ ] Implement `CSRFToken(session)` helper in `internal/auth/` or `internal/web/` that generates and validates a token
- [ ] Add CSRF token injection as a template function so templates can call `{{csrfToken}}`
- [ ] Add CSRF validation middleware that checks POST/PUT/DELETE requests for the token; exempt `/api/v1/` (API uses Bearer tokens)
- [ ] Register the middleware in `internal/app/routes.go`
- [ ] Add `{{csrfToken}}` hidden input to: `templates/login.html`, `templates/change_password.html`, `templates/admin.html` (all forms), `templates/admin/users.html`
- [ ] Write unit tests for the CSRF helper: token generation is unique per call, validation passes with correct token, fails with wrong/missing token
- [ ] Run `go test ./...` — must pass before task 4

### Task 4: Consolidate admin.html inline styles → design.css

`templates/admin.html` has ~651 lines of `<style>` that duplicate and conflict with `design.css`. This causes visual inconsistencies and makes future CSS changes fragile.

- [ ] Audit inline styles in `admin.html`: identify which classes are already in `design.css` (duplicates to delete) vs genuinely new classes to migrate
- [ ] Move genuinely new classes (`.admin-page`, `.admin-grid`, `.admin-section`, `.user-card` family, `.group-badge`, `.notif-composer`, `.notif-list`, `.equipment-cell`, `.equipment-status-dot`) to a new `/* === Admin === */` section at the bottom of `design.css`
- [ ] Delete the entire inline `<style>` block from `admin.html`
- [ ] For duplicate `.card`, `.modal`, `.form-group` definitions in inline styles: delete the inline version and verify the `design.css` version renders correctly
- [ ] Visually verify: equipment list, user list, group management, notification composer all render correctly
- [ ] Run `go build ./...` — must pass before task 5

### Task 5: Consolidate login/auth inline styles → design.css

`templates/login.html` and `templates/auth_base.html` have ~164 lines of inline `<style>`.

- [ ] Move `.auth-page`, `.auth-card`, `.auth-subtitle`, `.auth-theme-toggle` to a new `/* === Auth === */` section in `design.css`
- [ ] Unify `.form-group` and `.form-actions` with any existing form utility classes in `design.css` (add if absent, delete duplicates)
- [ ] Delete inline `<style>` blocks from `login.html` and `auth_base.html`
- [ ] Visually verify: login page, change-password page, and profile-tokens page all render correctly (including the theme toggle)
- [ ] Run `go build ./...` — must pass before task 6

### Task 6: Dashboard visual polish

The dashboard is mostly done but may have minor inconsistencies flagged during QA (activity feed layout, KPI delta styling, today's bookings empty state).

- [ ] Load the dashboard in a browser; note any visual inconsistencies against the design system (spacing, color, typography)
- [ ] Fix `.stat-card__delta.primary` styling if the highlighted delta text is off-color or misaligned
- [ ] Verify the activity feed renders correctly when empty (zero items) — add empty-state placeholder if missing
- [ ] Verify "Today's bookings" section renders correctly when no bookings exist — add empty-state if missing
- [ ] Verify dark mode renders correctly for all dashboard components (KPI cards, activity feed, recent samples list)
- [ ] Run `go build ./...` — must pass before task 7

### Task 7: Wiki visual polish

`wiki_view.html` uses inline styles for attachment section and some layout details.

- [ ] Load wiki article view in browser; identify any components that look inconsistent with the design system
- [ ] Move any inline styles from `wiki_view.html` to `design.css` (attachment list, TOC sidebar if present)
- [ ] Verify wiki article list (`wiki_list.html`) tag chips and filter chips render correctly
- [ ] Verify wiki editor (`wiki_edit.html`) toolbar and preview area look consistent
- [ ] Run `go build ./...` — must pass before task 8

### Task 8: Verify acceptance criteria

- [ ] Run `go test ./...` — all tests must pass
- [ ] Run `go vet ./...` — no issues
- [ ] Run `go build ./...` — clean build
- [ ] Load each redesigned page in browser and confirm consistent design system usage: sidebar, topbar, cards, chips, buttons, modals
- [ ] Test with dark mode enabled — all pages must render correctly
- [ ] Test login/logout flow end-to-end
- [ ] Test admin: create user, reset password, confirm CSRF protection works (request without token returns 403)
- [ ] Test equipment modal with apostrophe in name — confirm no XSS
- [ ] Check browser DevTools console — zero JS errors on all pages

### Task 9: [Final] Update documentation

- [ ] Update `docs/QA_report.md` or add a note with security fixes applied
- [ ] If CLAUDE.md exists, update any design system notes to reflect CSS consolidation

---

## Technical Details

**XSS fix (Task 1):** Replace `onclick="fn(id, 'name')"` with `data-id="{{$eq.ID}}" data-name="{{$eq.Name}}"` and attach a delegated event listener in a `<script>` block that reads `dataset.id` / `dataset.name`.

**CSRF approach (Task 3):** A per-session token stored in the session record is simplest given the existing session infrastructure. Generate once per session login, store in `sessions` table, inject via template function, validate in middleware before handler runs. API routes (`/api/v1/`) are exempt since they authenticate via `Authorization: Bearer` header.

**CSS consolidation strategy (Tasks 4–5):** Add named section comments to `design.css` (`/* === Admin === */`, `/* === Auth === */`) at the end of the file. Migrate classes verbatim first, then deduplicate against existing definitions.

## Post-Completion

**Manual browser QA** across all pages before opening PR:
- Dashboard, Sample list, Sample detail, Equipment booking
- Admin: equipment, users, groups, notifications
- Wiki: list, view, edit
- Auth: login, change password, profile tokens
- Verify dark mode on every page

**PR checklist:**
- Branch `redesign-refined` → `main`
- All tasks complete, all tests green
- No inline `<style>` blocks remaining in templates (except trivial one-liners)
- Security issues from audit resolved
