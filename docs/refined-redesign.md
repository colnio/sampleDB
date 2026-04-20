# Refined Redesign (2026-04)

Summary of the production-app changes driven by the "Direction A: Refined" prototype that lived under `SampleDB_design/`. Scope was scoped to the 10 reference screenshots captured on 2026-04-20 (`SampleDB_design/screenshots/`); the full plan is archived at `docs/plans/completed/2026-04-20-apply-refined-design.md`.

## Scope shipped

- **Design shell** — `static/css/design.css` (1,430+ lines): design tokens, sidebar shell, card/chip/timeline primitives. `templates/header.html` rebuilt with Workspace / Account / Admin groups and a user footer; every major template updated to use the new card + chip vocabulary.
- **Dashboard** (new) — `GET /dashboard` served by `internal/dashboard/` with 4 KPI cards (total samples, active bookings, wiki articles, pending approvals), Recent samples (5), Recent activity (7), and Today's bookings timeline. `/` now redirects authenticated users to `/dashboard`; the samples grid moved to `/samples`.
- **Sample status** — `samples.status` (`active` / `in_progress` / `archived`) with a `CHECK` constraint. Rendered as a chip on the list cards, table view, and detail header. List handler accepts `?status=` + `?owner=` + `?view=grid|table`.
- **Wiki tags** — `articles.tags TEXT[]`. Wiki list renders a grid of cards with tag chips; filter row `All | Safety | Equipment | Protocols | Admin` plus free-form tags via `?tag=`.
- **Wiki TOC** — "On this page" sidebar generated from h2/h3 headings; anchor IDs injected into the rendered HTML.
- **Equipment status + group-access badges** — `equipment.status` (`online` / `offline` / `maintenance`) and the new `equipment_group_access` many-to-many. Admin panel gained equipment status chips, group badges, and add/remove controls; the booking page shows a status dot next to each equipment name.
- **Activity log** — append-only `activity_log` table surfaced through a new `internal/activity/` package. `activity.Logger` is wired into every mutating handler (sample, fabrication step, attachment, booking, wiki article, wiki attachment) as a best-effort write. `activity.Recent(ctx, n)` powers the Dashboard feed; `activity.ForTarget(ctx, "sample", id, n)` powers the per-sample Activity card.
- **Admin user management** — status chip (approved/pending), admin chip, equipment-access count, and the existing multi-select granular editor.

## Deliberate divergences from the prototype

- **Sample thumbnails** — the prototype generates gradient art per sample. Production uses the first uploaded image when one exists, with a neutral placeholder otherwise. No generated gradients.
- **Share / Edit panels on sample detail** — kept as collapsed `<details>` panels; top-of-page Share/Edit buttons deferred to a later pass.
- **API-token expiration** — dropdown rendered in the Generate form for visual parity but disabled (no backend support in this iteration).

## Data-model additions

- `samples.status` — VARCHAR(20), CHECK, default `'active'`.
- `equipment.status` — VARCHAR(20), CHECK, default `'online'`.
- `articles.tags` — `TEXT[] NOT NULL DEFAULT '{}'`.
- `equipment_group_access` — `(equipment_id, group_id)` PK, cascade on both sides.
- `activity_log` — `id BIGSERIAL`, denormalized `actor_username` so rows survive user deletion, indexes on `(created_at DESC)` and `(target_type, target_id)`.

Migrations are appended to `DDL/init.sql` with the existing `IF NOT EXISTS` / `DO $$ EXCEPTION WHEN duplicate_object $$` pattern and are applied on startup by `internal/dbschema`.

## Route changes

- `GET /dashboard` — new.
- `GET /` — redirects authenticated users to `/dashboard`.
- `GET /samples` — new home of the samples grid (previously `/`); extended with `?status=`, `?owner=`, `?view=grid|table`.
- `GET /wiki` — extended with `?tag=`.
- `POST /admin/equipment/{id}/status` — new.
- `POST /admin/equipment/{id}/groups/add` and `POST /admin/equipment/{id}/groups/remove` — new.

## Activity vocabulary

Verbs: `created`, `updated`, `deleted`, `booked`, `uploaded`, `published`, `added_step`, `edited_step`, `granted`, `revoked`.

Target types: `sample`, `attachment`, `fabrication_step`, `booking`, `article`, `article_attachment`, `equipment`, `user`.

## Using the activity logger

```go
if a.Activity != nil {
    _ = a.Activity.Log(ctx, activity.Actor{ID: userID, Username: username},
        "created", "sample", sampleID, sampleName)
}
```

The logger is no-op-safe when the `*Logger` is nil (tests and alternative entry points that don't wire tracking still compile). Errors are logged via `slog` but never bubble up — a failure to write the activity row must never fail the user's request.
