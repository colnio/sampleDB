# SampleDB

SampleDB is a lightweight lab management portal written in Go. It combines a searchable sample catalogue, a collaborative wiki, and an equipment booking calendar behind a simple authentication layer. Admins can curate access, manage groups, and review booking activity, while every user can upload files, author wiki entries, and manage their own credentials.

## Features

- **Authentication & Sessions** – user registration with admin approval, secure session cookies, and per-user password management.
- **Sample Registry** – search samples by keywords, attach files, and track preparation notes.
- **Wiki** – Markdown-based knowledge base with attachment support.
- **Equipment Booking** – calendar-style reservations with per-user equipment permissions and conflict detection.
- **Admin Panel** – manage approvals, groups, permissions, soft-delete user accounts, and export booking reports.
- **HTTPS Ready** – configurable TLS endpoints, HTTP→HTTPS redirects, and hardened response headers.

## Requirements

- Go 1.21+
- PostgreSQL 13+
- (Optional) `certbot` or another ACME client if you plan to terminate TLS within the Go process.

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

## Database schema

The application keeps the runtime schema in sync at startup via `internal/dbschema`. It now verifies every table the project depends on (users, samples, wiki, attachments, equipment, bookings, and groups), adding missing columns such as `users.deleted` and seeding baseline data where appropriate. The bootstrapped schema matches the `DDL/init.sql` file so you can provision the database manually if you prefer.

> **Note:** Schema verification issues (e.g., lacking privileges to `ALTER TABLE users`) will abort the process on launch. Ensure the configured PostgreSQL role owns the relevant tables or run the statements from `DDL/init.sql` as a superuser beforehand.

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

## Tests

```bash
go test ./...
```

## Project structure

```
DDL/                    -- Stand-alone SQL for provisioning
internal/auth/          -- Session management and auth flows
internal/dbschema/      -- Runtime schema verification helpers
static/                 -- Public assets served at /static/
templates/              -- HTML templates (base, admin, wiki, etc.)
uploads/                -- File uploads (created at runtime)
```

## License

SampleDB is distributed under the MIT License. See `LICENSE` for details.
