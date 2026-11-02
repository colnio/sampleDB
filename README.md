# SampleDB

SampleDB is a lightweight lab management portal written in Go. It combines a searchable sample catalogue, a collaborative wiki, and an equipment booking calendar behind a simple authentication layer. Admins can curate access, manage groups, and review booking activity, while every user can upload files, author wiki entries, and manage their own credentials.

## Features

- **Authentication & Sessions** – user registration with admin approval, server-side persistent sessions (stored in PostgreSQL — survive restarts), and per-user password management. All state-changing web routes protected by CSRF tokens (automatically injected by the UI; Bearer-token API requests are exempt).
- **Dashboard** – KPI cards (sample / booking / wiki / pending-approval counts), Recent samples list, cross-domain Recent activity feed, and Today's bookings timeline. Served at `/dashboard`; `/` redirects authenticated users to it and the samples grid moves to `/samples`.
- **Sample Registry** – search samples by keywords, attach files, and track preparation notes. Each sample has a status (`active` / `in_progress` / `planned` / `archived`) rendered as a chip on the list and detail pages; the list can be filtered by status, owner, and toggled between grid and table views (`/samples?status=&owner=&view=`).
- **Wiki** – Markdown-based knowledge base with attachment support. Articles carry a `tags TEXT[]` column and the list (`/wiki?tag=`) renders tag chips with a filter row; individual articles show an "On this page" TOC sidebar generated from the h2/h3 headings.
- **Equipment Booking** – calendar-style reservations with per-user equipment permissions and conflict detection. Each equipment row has a status (`online` / `offline` / `maintenance`) rendered as a colored dot next to the name. Admins can limit individual users to booking the AFM during working hours, and set those hours and per-date exceptions such as holidays from the Admin Panel.
- **Admin Panel** – manage approvals, groups, permissions, soft-delete user accounts, and export booking reports. Equipment catalogue shows per-equipment status chips and group-access badges (many-to-many `equipment_group_access` with add/remove controls); user management shows status + admin chips and an equipment-access count.
- **Activity Log** – append-only `activity_log` table (see `internal/activity/`) populated by every mutating handler (sample, fabrication step, attachment, booking, wiki article, wiki attachment). Feeds the Dashboard's Recent activity panel and the per-sample Activity card on the sample detail page.
- **HTTPS Ready** – configurable TLS endpoints, HTTP→HTTPS redirects, and hardened response headers.

## Requirements

- Go 1.21+
- PostgreSQL 13+ with the `btree_gist` extension installable by the database role (required for the booking overlap exclusion constraint; `CREATE EXTENSION IF NOT EXISTS btree_gist` runs automatically on startup — the role needs superuser or the extension must be pre-installed on managed Postgres).
- (Optional) `certbot` or another ACME client if you plan to terminate TLS within the Go process.

## Installation

1. Clone the repository and enter the directory: `git clone <repo> && cd sampleDB`.
2. Provision PostgreSQL (local or remote) and create a database/user with permission to create/alter tables.
3. Export `DATABASE_URL` (and other env vars from the table below) so the app can reach your database.
4. Apply the schema:
   - Fast start: simply run the binary; `internal/dbschema` will create/upgrade tables automatically on boot.
   - Manual: `psql "$DATABASE_URL" -f DDL/init.sql` to pre-seed the schema before starting the service.
5. Build and start the server:

   ```bash
   go build ./...
   ./sampleDB
   ```

6. First boot behavior: when the `users` table is empty, SampleDB seeds a default approved admin account (`admin` / `admin`). Change this password immediately via `/change-password` after signing in. Existing deployments are left untouched; only schema fixes run on startup.

## Configuration

The application is configured entirely through environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `APP_ADDR` | `:8010` | Address for the main HTTP/HTTPS listener. |
| `APP_HTTP_REDIRECT_ADDR` | _(empty)_ | Optional plain HTTP listener that permanently redirects to HTTPS. |
| `DATABASE_URL` | _(empty)_ | PostgreSQL connection string; must allow schema changes. |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | _(empty)_ | When both are set, the server starts in HTTPS mode and enforces secure cookies/HSTS. |
| `PUBLIC_HOST` | _(empty)_ | Used when building HTTPS redirects. |
| `APP_BASE_DIR` | binary directory | Base directory used to resolve relative paths for templates, static files, and uploads. |
| `TEMPLATES_DIR` | `<base>/templates` | Location of HTML templates. |
| `STATIC_DIR` | `<base>/static` | Directory served at `/static/`. |
| `UPLOADS_DIR` | `<base>/uploads` | Filesystem destination for uploaded attachments. |

### HTTPS example

```bash
export APP_ADDR=":443"
export APP_HTTP_REDIRECT_ADDR=":80"
export TLS_CERT_FILE="/path/to/fullchain.pem"
export TLS_KEY_FILE="/path/to/privkey.pem"
export PUBLIC_HOST="example.org"
```

### Custom directories

When deploying outside of the repository root, point the service at your layout:

```bash
export APP_BASE_DIR="/opt/sampledb"
export TEMPLATES_DIR="$APP_BASE_DIR/templates"
export STATIC_DIR="$APP_BASE_DIR/static"
export UPLOADS_DIR="$APP_BASE_DIR/uploads"
```

## Database schema & migrations

- On every startup, `internal/dbschema.Ensure` brings the schema up to date (tables, columns, and indexes) without dropping data. Keep the configured PostgreSQL role privileged enough to run `CREATE TABLE`/`ALTER TABLE`.
- The runtime schema matches `DDL/init.sql`, so you can also pre-provision the database with `psql -f DDL/init.sql` if desired.
- When no users exist, the service creates one default admin (`admin` / `admin`, approved and with admin rights). If users are already present, only schema adjustments are applied—no data changes are made.
- Sessions are stored in the `sessions` table and survive server restarts. Deleting a user immediately revokes all their active sessions.

## Running locally

```bash
go build ./...
./sampleDB
```

Navigate to `http://localhost:8010` (or your configured address). Without TLS the server stays on plain HTTP; adding the TLS variables upgrades it automatically.

## Systemd unit example

```ini
[Unit]
Description=SampleDB
After=network.target

[Service]
Type=simple
User=sampledb
Group=sampledb
WorkingDirectory=/opt/sampledb
Environment="APP_ADDR=:443"
Environment="APP_HTTP_REDIRECT_ADDR=:80"
Environment="DATABASE_URL=${DATABASE_URL}"
Environment="TLS_CERT_FILE=/path/to/fullchain.pem"
Environment="TLS_KEY_FILE=/path/to/privkey.pem"
Environment="PUBLIC_HOST=example.org"
Environment="APP_BASE_DIR=/opt/sampledb"
Environment="TEMPLATES_DIR=/opt/sampledb/templates"
Environment="STATIC_DIR=/opt/sampledb/static"
Environment="UPLOADS_DIR=/srv/sampledb/uploads"
ExecStart=/opt/sampledb/sampleDB
Restart=always
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Reload systemd and restart once the unit file is saved:

```bash
sudo systemctl daemon-reload
sudo systemctl restart sampledb
```

## REST API

SampleDB exposes a JSON REST API under `/api/v1/`. All endpoints require authentication via either a session cookie (browser) or a Bearer token (programmatic access).

### Authentication

**Session cookie** — log in via the web UI; the session cookie is sent automatically by the browser.

**Bearer token** — generate a token at `/profile/tokens`, then include it in every request:

```
Authorization: Bearer <token>
```

Requests without valid credentials receive `401 Unauthorized`.

### Base URL

```
http(s)://<host>/api/v1
```

### Key endpoints

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/samples` | List samples (supports `q`, `owner`, `from`, `to`, `limit`, `offset`) |
| `POST` | `/samples` | Create a sample |
| `GET` | `/samples/{id}` | Get sample detail with fabrication steps and attachments |
| `PUT` | `/samples/{id}` | Update sample fields |
| `DELETE` | `/samples/{id}` | Delete sample |
| `GET` | `/samples/{id}/steps` | List fabrication steps for a sample |
| `POST` | `/samples/{id}/steps` | Add a fabrication step |
| `PUT` | `/steps/{id}` | Update a fabrication step |
| `DELETE` | `/steps/{id}` | Delete a fabrication step |
| `GET` | `/wiki` | List wiki articles |
| `POST` | `/wiki` | Create a wiki article |
| `GET` | `/wiki/{id}` | Get article with content and attachments |
| `PUT` | `/wiki/{id}` | Update a wiki article |
| `GET` | `/samples/{id}/attachments` | List attachments for a sample |
| `POST` | `/samples/{id}/attachments` | Upload one or more attachments to a sample |
| `GET` | `/attachments/{id}` | Download an attachment |
| `DELETE` | `/attachments/{id}` | Delete an attachment |
| `GET` | `/users` | List approved users (for owner/person pickers) |
| `GET` | `/equipment` | List equipment |

All list responses follow the envelope `{"items": [...], "total": N, "limit": N, "offset": N}`. Error responses use `{"error": "message"}`. See `docs/api.md` for full request/response details.

### Quick example

```bash
# Generate a token at /profile/tokens, then:
TOKEN=<your_token>

# List samples
curl -H "Authorization: Bearer $TOKEN" http://localhost:8010/api/v1/samples

# Create a sample
curl -X POST -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"name":"Si wafer","description":"4-inch p-type","keywords":"silicon wafer"}' \
     http://localhost:8010/api/v1/samples
```

## Tests

```bash
# Unit and contract tests (no database required)
go test ./...

# Integration tests (requires PostgreSQL)
DATABASE_URL=postgres://app:app@localhost:5432/sampledb \
  go test -tags=integration ./internal/integration/... -v
```

## Project structure

```
DDL/                    -- Stand-alone SQL for provisioning / reference schema
docs/                   -- Architecture notes, API docs, review findings
internal/
  app/                  -- App struct, config, and central route registration
  auth/                 -- Session management, Bearer-token auth, password hashing
  activity/             -- Append-only activity log (Logger, Recent, ForTarget)
  admin/                -- User, group, and equipment management handlers
  booking/              -- Equipment booking calendar: service + HTTP handlers
  dashboard/            -- /dashboard handler: KPIs, recent samples, activity, bookings
  dbschema/             -- Runtime schema bootstrap (applied on every startup)
  dbiface/              -- Minimal pgx pool interface for testability
  profile/              -- Change-password and API-token management handlers
  samples/              -- Sample CRUD, attachments, fabrication steps, sharing
  web/                  -- Shared HTTP helpers: JSON, error rendering, path parsing
  wiki/                 -- Wiki article CRUD and attachment handlers
  agents/               -- /agents and /agents/examples page handlers
  integration/          -- End-to-end HTTP tests (build tag: integration)
static/                 -- Public assets served at /static/
templates/              -- HTML templates (base layout, admin, wiki, booking, etc.)
uploads/                -- File uploads (created at runtime if missing)
```

## License

SampleDB is distributed under the MIT License. See `LICENSE` for details.
