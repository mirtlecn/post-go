# Post-go REST API

> Reference for API consumers: request patterns, fields, examples, and error handling.

## 1. Base URL

```text
http://host:port
```

Example:

```text
http://localhost:3000
```

---

## 2. Authentication

Write operations and management queries require a Bearer token:

```http
Authorization: Bearer <SECRET_KEY>
```

Common authenticated operations:

- `POST /`
- `PUT /`
- `DELETE /`
- `GET /` (management API)

Public reads use `GET /{path}` and do not require authentication.

---

## 3. Content Types

Set `type` when creating/updating content:

- `text`: plain text
- `url`: URL link (public reads return `302` redirect)
- `html`: HTML content
- `file`: file upload (`multipart/form-data`)
- `topic`: topic page for grouping members
- `md2html`: convert Markdown to `html` on write
- `qrcode`: convert input into a text QR code (stored as `text`)

`convert` can be used as an alias for `type`.

---

## 4. Common Request Fields

Common JSON fields:

| Field | Type | Required | Description |
|---|---|---:|---|
| `url` | string | Yes* | Main payload (text, link, markdown, etc.) |
| `path` | string | No | Short path; server may auto-generate if omitted |
| `title` | string | No | Display title |
| `created` | string | No | Creation timestamp |
| `type` | string | No | Content type |
| `convert` | string | No | Alias of `type` |
| `ttl` | integer | No | Expiration in minutes |
| `topic` | string | No | Topic assignment |

> `url` is not required when `type=topic`.

### Path Rules

- Max length: 99
- Allowed characters: `a-z A-Z 0-9 - _ . / ( )`
- Leading/trailing `/` is trimmed
- `path` and `path/` are treated as the same path
- `asset/...` is reserved and cannot be used for user content

### TTL Rules

- Unit: minutes
- Range: `0 ~ 525600`
- `0` means no expiration
- `topic` itself does not support TTL

---

## 5. Response Format

### 5.1 Item Object

```json
{
  "surl": "http://host/path",
  "path": "path",
  "type": "text",
  "title": "Greeting",
  "created": "2022-10-11T01:11:01Z",
  "ttl": 10,
  "content": "hello"
}
```

Field notes:

- `surl`: full public URL
- `path`: short path
- `type`: content type
- `title`: title (always present; defaults to `""`)
- `created`: timestamp (`"illegal"` may appear if missing/invalid in storage)
- `ttl`: remaining TTL (`null` when non-expiring)
- `content`: preview by default

### 5.2 Error Object

```json
{
  "error": "Invalid JSON body",
  "code": "invalid_request",
  "hint": null,
  "details": null
}
```

Common `code` values:

- `unauthorized`
- `invalid_request`
- `conflict`
- `not_found`
- `payload_too_large`
- `internal`
- `s3_not_configured`

---

## 6. Endpoints

## 6.1 Create: `POST /`

Create regular content, Topic pages, or upload files.

### 6.1.1 Create text (JSON)

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "note",
    "url": "hello",
    "title": "Greeting",
    "type": "text",
    "ttl": 10
  }'
```

### 6.1.2 Create short link (JSON)

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "openai",
    "url": "https://openai.com",
    "type": "url"
  }'
```

### 6.1.3 Create Topic

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "anime",
    "type": "topic",
    "title": "Anime Archive"
  }'
```

### 6.1.4 Upload file (multipart)

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "path=manual" \
  -F "title=Manual" \
  -F "file=@./manual.pdf"
```

Notes:

- Requires configured S3-compatible storage
- If `path` has no extension, the server appends the uploaded file extension

---

## 6.2 Upsert: `PUT /`

Update by `path`; create if not found.

```bash
curl -X PUT "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "note",
    "url": "hello v2",
    "type": "text"
  }'
```

Behavior:

- Exists: update and return `200`
- Missing: create and return `201`
- May include `overwritten` (preview or export payload of previous content)

### Topic rebuild

```bash
curl -X PUT "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "anime",
    "type": "topic",
    "title": "Anime Archive"
  }'
```

Use this to refresh Topic home and member indexes.

---

## 6.3 Delete: `DELETE /`

### Delete regular content

```bash
curl -X DELETE "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path": "note"}'
```

### Delete Topic

```bash
curl -X DELETE "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "anime",
    "type": "topic"
  }'
```

> Deleting a Topic does not delete the underlying items under that path prefix.

---

## 6.4 Management Query: `GET /`

Authenticated management lookup APIs.

### List all content

```bash
curl -X GET "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN"
```

### Lookup single path

```bash
curl -X GET "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path": "note"}'
```

### Lookup Topic

```bash
curl -X GET "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "anime",
    "type": "topic"
  }'
```

---

## 6.5 Public Read: `GET /{path}`

Public access path behavior by `type`:

- `url`: `302 Found` redirect
- `text`: plain text response
- `html`: HTML response
- `file`: file stream response
- `topic`: topic HTML page

Example:

```bash
curl -i "$POST_BASE_URL/note"
```

---

## 7. Topic Usage

Two ways to write content into a Topic:

### Method A: Explicit `topic`

```json
{
  "topic": "anime",
  "path": "castle-notes",
  "url": "# Castle",
  "type": "md2html"
}
```

### Method B: Prefix path directly

```json
{
  "path": "anime/castle-notes",
  "url": "# Castle",
  "type": "md2html"
}
```

If multiple Topics match by prefix, the **longest prefix** wins.

---

## 8. Export Mode

Add header:

```http
x-export: true
```

Available for create/update/delete/lookup/list.

Effect:

- Regular content: `content` returns full raw content (not preview)
- Topic: `content` remains the member count string

---

## 9. Date & Time

`created` accepts multiple input formats (for example RFC3339 or `2006-01-02`).

The server normalizes and stores it as UTC RFC3339.

---

## 10. Quick Start Snippets

```bash
export POST_BASE_URL="http://localhost:3000"
export POST_TOKEN="your-secret-key"
```

Create text:

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"hello","url":"Hello","type":"text"}'
```

Read text:

```bash
curl "$POST_BASE_URL/hello"
```
