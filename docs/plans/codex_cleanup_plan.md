# Root Cleanup and Package Consolidation

## Summary
- Keep the executable entrypoint at repo root and preserve current `go run .`, HTTP routes, and JSON contracts.
- Reduce root Go code to a single thin `main.go`; move reusable bootstrap and page logic into `internal/*`; delete empty/comment-only root files and root tests.
- Remove tracked orphan assets plus obvious local junk like `.DS_Store` and the empty `Users/` directory, while leaving `uploads/` runtime data alone.

## Key Changes
- Slim `main.go` down to startup orchestration only. Move env/config loading, base-dir and port helpers, default-admin seeding, and server middleware helpers out of root.
- Expand `internal/app` with bootstrap-oriented files for config and startup helpers, and remove `App.Handlers` so route registration no longer depends on root-provided handlers.
- Add `internal/agents` for `/agents` and `/agents/examples`, including the page data type, template parsing/cache, example markdown loading, and relative-image rewrite helper.
- Rename `static/examples/scrpiting_automation.md` to `static/examples/scripting_automation.md` and update its references while moving the AI examples logic.
- Standardize on `internal/web` for shared HTTP helpers. Remove the root `api.go` and `markdown.go` wrappers and have bootstrap/API code call shared `web` helpers directly.
- Delete the now-obsolete root files `admin.go`, `booking.go`, `fabrication_steps.go`, `fabrication_steps_handler.go`, `sample_prep_handler.go`, and `sharing.go`.
- Move root tests into the packages they actually validate: JSON and markdown tests into `internal/web`, auth rejection tests into `internal/auth`, route/context tests into `internal/app`, renderer/template smoke tests into `internal/web`, and migration/booking-constraint integration tests into `internal/dbschema`.
- Delete placeholder or low-value root tests: `admin_test.go`, `fabrication_steps_test.go`, `owner_test.go`, `sharing_test.go`, and `gallery_test.go`.
- Remove verified orphan tracked assets: `static/js/admin.js`, `static/js/sample-prep.js`, `static/images/ai_agent_examples/*`, and `static/examples/mfia_plan_page.jpg`.

## Interfaces
- No external route changes and no JSON wire-format changes.
- `internal/app.App` loses the `Handlers` field.
- Add an `internal/agents` handler constructor/type for the two public AI pages.
- Simplify `internal/web.NewRenderer` so callers no longer need a root markdown adapter.

## Test Plan
- Run `go test ./...`.
- Run `go vet ./...`.
- Add package-local tests for config loading, default-admin bootstrap, redirect/security-header middleware, `/agents` page rendering, markdown image rewriting, renderer/template smoke coverage, and schema/booking exclusion integration.
- Verify the repo root no longer contains legacy `*.go` stubs or root `*_test.go` files, and no `.DS_Store` files remain.

## Assumptions
- The entrypoint stays at root `main.go`; moving to `cmd/sampledb/` is out of scope.
- Existing uncommitted edits in modified files are preserved; implementation must not revert unrelated work.
- `docs/plans/completed/*` stay as historical records even if they mention old file names.
