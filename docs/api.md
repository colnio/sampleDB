# SampleDB REST API Reference

All endpoints live under `/api/v1/` and require authentication (session cookie or Bearer token). Responses are JSON. List endpoints use a common envelope format.

## Authentication

### Session cookie
Log in via the web UI at `/login`. The browser sends the session cookie automatically.

### Bearer token
1. Sign in to the web UI and go to **Profile → API Tokens** (`/profile/tokens`).
2. Click **Generate token**, give it a name, and copy the token shown (it is displayed only once).
3. Include the token in every API request:

```
Authorization: Bearer <token>
```

Requests without a valid credential return:
```json
HTTP/1.1 401 Unauthorized
{"error": "authentication required"}
```

---

## Common conventions

### List envelope
All list responses return:
```json
{
  "items": [...],
  "total": 42,
  "limit": 100,
  "offset": 0
}
```

### Pagination query parameters
| Parameter | Default | Description |
| --- | --- | --- |
| `limit` | 100 | Max items returned (1–1000) |
| `offset` | 0 | Number of items to skip |

### Error response
```json
{"error": "human-readable message"}
```

### HTTP status codes
| Code | Meaning |
| --- | --- |
| 200 | Success |
| 201 | Created |
| 204 | No content (delete/update with no body) |
| 400 | Bad request (invalid JSON, missing required field) |
| 401 | Authentication required |
| 404 | Resource not found |
| 405 | Method not allowed |
| 500 | Internal server error |

---

## Samples

### GET /api/v1/samples

List samples with optional filtering.

**Query parameters**

| Parameter | Description |
| --- | --- |
| `q` | Keyword search across name, owner, and keywords fields |
| `owner` | Filter by owner name (partial match) |
| `from` | Filter by creation date ≥ value (YYYY-MM-DD) |
| `to` | Filter by creation date ≤ value (YYYY-MM-DD) |
| `limit` | Max results (default 100, max 1000) |
| `offset` | Pagination offset (default 0) |

**Response 200**
```json
{
  "items": [
    {
      "id": 1,
      "name": "Si wafer",
      "description": "4-inch p-type silicon",
      "keywords": "silicon wafer substrate",
      "owner": "alice",
      "owner_user_id": 3,
      "owner_username": "alice",
      "attachment_count": 2,
      "created_at": "2026-01-15T10:00:00Z"
    }
  ],
  "total": 1,
  "limit": 100,
  "offset": 0
}
```

Note: `owner_user_id` and `owner_username` are omitted when `owner_user_id` is NULL (legacy samples with free-text owner only).

---

### GET /api/v1/samples/{id}

Get full sample detail including fabrication steps and attachments.

**Response 200**
```json
{
  "id": 1,
  "name": "Si wafer",
  "description": "4-inch p-type silicon",
  "keywords": "silicon wafer substrate",
  "owner": "alice",
  "owner_user_id": 3,
  "owner_username": "alice",
  "sample_prep": "Cleaned with piranha solution...",
  "attachment_count": 2,
  "created_at": "2026-01-15T10:00:00Z",
  "fabrication_steps": [
    {
      "id": 10,
      "sample_id": 1,
      "step_order": 1,
      "step_date": "2026-01-14",
      "person_user_id": 3,
      "person_username": "alice",
      "description": "RCA clean",
      "equipment_id": 5,
      "equipment_name": "Wet bench 1",
      "created_at": "2026-01-15T09:00:00Z"
    }
  ],
  "attachments": [
    {
      "id": 20,
      "original_name": "wafer.jpg",
      "url": "/attachment/20",
      "content_type": "image/jpeg",
      "is_image": true,
      "uploaded_at": "2026-01-15T10:05:00Z"
    }
  ]
}
```

**Response 404**
```json
{"error": "sample not found"}
```

---

### POST /api/v1/samples

Create a new sample.

**Request body**
```json
{
  "name": "Si wafer",
  "description": "4-inch p-type silicon",
  "keywords": "silicon wafer substrate",
  "owner": "alice",
  "sample_prep": "Optional preparation notes in Markdown"
}
```

| Field | Required | Description |
| --- | --- | --- |
| `name` | Yes | Sample name |
| `description` | No | Short description |
| `keywords` | No | Space- or comma-separated keywords |
| `owner` | No | Free-text owner name |
| `sample_prep` | No | Markdown preparation notes |

**Response 201**
```json
{
  "id": 42,
  "name": "Si wafer",
  "description": "4-inch p-type silicon",
  "keywords": "silicon wafer substrate",
  "owner": "alice",
  "attachment_count": 0,
  "created_at": "2026-04-16T12:00:00Z"
}
```

---

### PUT /api/v1/samples/{id}

Update sample fields. All fields are replaced (send the full object).

**Request body** — same shape as POST.

**Response 200** — full sample detail (same as GET /api/v1/samples/{id}).

**Response 404**
```json
{"error": "sample not found"}
```

---

### DELETE /api/v1/samples/{id}

Delete a sample and all its fabrication steps. Attachments are also removed.

**Response 204** — no body.

**Response 404**
```json
{"error": "sample not found"}
```

---

## Fabrication steps

### GET /api/v1/samples/{id}/steps

List all fabrication steps for a sample, ordered by `step_order`.

**Response 200**
```json
{
  "items": [
    {
      "id": 10,
      "sample_id": 1,
      "step_order": 1,
      "step_date": "2026-01-14",
      "person_user_id": 3,
      "person_username": "alice",
      "description": "RCA clean",
      "equipment_id": 5,
      "equipment_name": "Wet bench 1",
      "created_at": "2026-01-15T09:00:00Z"
    }
  ],
  "total": 1,
  "limit": 1,
  "offset": 0
}
```

---

### POST /api/v1/samples/{id}/steps

Add a fabrication step to a sample.

**Request body**
```json
{
  "step_order": 1,
  "step_date": "2026-01-14",
  "person_user_id": 3,
  "description": "RCA clean",
  "equipment_id": 5
}
```

| Field | Required | Description |
| --- | --- | --- |
| `step_order` | No | Integer display order (default 0) |
| `step_date` | No | Date string YYYY-MM-DD |
| `person_user_id` | No | ID of the user who performed the step |
| `description` | No | Free-text step description |
| `equipment_id` | No | ID of equipment used |

**Response 201** — the created step object (same shape as items in GET /steps).

---

### PUT /api/v1/steps/{id}

Update a fabrication step. All fields are replaced.

**Request body** — same shape as POST /samples/{id}/steps.

**Response 204** — no body.

**Response 404**
```json
{"error": "step not found"}
```

---

### DELETE /api/v1/steps/{id}

Delete a fabrication step.

**Response 204** — no body.

**Response 404**
```json
{"error": "step not found"}
```

---

## Wiki

### GET /api/v1/wiki

List wiki articles, newest first.

**Query parameters** — `limit`, `offset` (pagination).

**Response 200**
```json
{
  "items": [
    {
      "id": 1,
      "title": "RCA Clean Protocol",
      "created_at": "2026-01-10T09:00:00Z",
      "last_modified_at": "2026-03-01T14:30:00Z"
    }
  ],
  "total": 1,
  "limit": 100,
  "offset": 0
}
```

---

### GET /api/v1/wiki/{id}

Get a wiki article including its raw Markdown content and attachment list.

**Response 200**
```json
{
  "id": 1,
  "title": "RCA Clean Protocol",
  "content": "## Overview\n\nThe RCA clean consists of...",
  "created_at": "2026-01-10T09:00:00Z",
  "last_modified_at": "2026-03-01T14:30:00Z",
  "attachments": [
    {
      "id": 5,
      "original_name": "diagram.png",
      "url": "/wiki/attachment/5",
      "is_image": true,
      "uploaded_at": "2026-01-10T09:05:00Z"
    }
  ]
}
```

**Response 404**
```json
{"error": "article not found"}
```

---

### POST /api/v1/wiki

Create a new wiki article. The authenticated user is recorded as the author.

**Request body**
```json
{
  "title": "RCA Clean Protocol",
  "content": "## Overview\n\nThe RCA clean consists of..."
}
```

| Field | Required | Description |
| --- | --- | --- |
| `title` | Yes | Article title |
| `content` | No | Raw Markdown content |

**Response 201** — full article detail (same shape as GET /api/v1/wiki/{id}).

---

### PUT /api/v1/wiki/{id}

Update a wiki article. The authenticated user is recorded as the last editor.

**Request body** — same shape as POST.

**Response 200** — full article detail (same shape as GET /api/v1/wiki/{id}).

**Response 404**
```json
{"error": "article not found"}
```

---

## Users

### GET /api/v1/users

List all approved, non-deleted users. Used to populate owner and person dropdowns.

**Response 200**
```json
{
  "items": [
    {
      "id": 1,
      "username": "alice",
      "group": "fab"
    }
  ],
  "total": 3,
  "limit": 3,
  "offset": 0
}
```

---

## Equipment

### GET /api/v1/equipment

List all equipment entries.

**Response 200**
```json
{
  "items": [
    {
      "equipment_id": 5,
      "name": "Wet bench 1",
      "location": "Room 204"
    }
  ],
  "total": 1,
  "limit": 1,
  "offset": 0
}
```

---

## Sample sharing (web, not API)

Shared sample links (`/s/{token}`) are managed through the web UI only (no dedicated API endpoint). From the sample detail page you can generate a link, set visibility (Public or Authenticated), copy it, or revoke it. The `/s/{token}` route is accessible without authentication for public-visibility samples.
