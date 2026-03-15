# Post-go API

## Overview

- Base URL: `http://host:port`
- Write operations require: `Authorization: Bearer <SECRET_KEY>`
- Public read route: `GET /<path>`
- Main management route: `GET|POST|PUT|DELETE /`

This service has two resource layers:

- regular item
  - text, url, html, file
- topic
  - a namespace with its own home page and member index

Examples:

- regular item: `note`, `docs/intro`
- topic home: `anime`
- topic member: `anime/castle-notes`
- nested topic home: `blog/2026`
- nested topic member: `blog/2026/post-1`

---

## Data Model

### Stored content JSON

All `surl:<path>` values are stored as JSON:

```json
{
  "type": "text",
  "content": "hello",
  "title": "Greeting"
}
```

Current fields:

- `type: string` required
- `content: string` required
- `title: string` optional

Stored `type` values:

- `url`
- `text`
- `html`
- `file`
- `topic`

Write-time aliases:

- `convert` is accepted as an alias of `type`
- `md2html` writes stored `type=html`
- `qrcode` writes stored `type=text`

Normalization rules:

- if both `type` and `convert` are provided, they must match
- if neither is provided:
  - URL-like input becomes `url`
  - other input becomes `text`

---

## Response Model

### Item response

Used by:

- authenticated lookup
- authenticated list
- authenticated topic lookup
- authenticated topic list

```json
{
  "surl": "http://host/path",
  "path": "path",
  "type": "text",
  "title": "Greeting",
  "ttl": 10,
  "content": "hello"
}
```

Fields:

- `surl: string`
- `path: string`
- `type: string`
- `title: string`
- `ttl: number | null`
- `content: string`

Rules:

- all successful JSON responses always include `title`
- if stored title is missing, API returns `""`

### Create / update response

```json
{
  "surl": "http://host/path",
  "path": "path",
  "type": "text",
  "title": "Greeting",
  "content": "hello",
  "ttl": 10,
  "overwritten": "old value"
}
```

Additional field:

- `overwritten: string` optional

### Delete response

```json
{
  "deleted": "path",
  "type": "text",
  "title": "Greeting",
  "content": "hello"
}
```

### Error response

```json
{
  "error": "Invalid JSON body",
  "code": "invalid_request",
  "hint": null,
  "details": null
}
```

---

## Write Rules

### Path rules

- if `path` is missing for normal item creation, server generates a random short path
- all backend CRUD paths are normalized before use:
  - non-slash-only input removes all leading `/`
  - non-slash-only input removes all trailing `/`
  - slash-only input such as `/` or `///` becomes `/`
- normalized path is the only Redis key form:
  - `path` and `path/` always map to the same key
  - trailing slash never creates an extra key
- allowed path characters:
  - `a-z A-Z 0-9 - _ . / ( )`
- max path length:
  - `99`

### TTL rules

- TTL unit is minutes
- `ttl` must be a natural number: `>= 0`
- `ttl = 0` means no expiration
- invalid TTL returns `400 invalid_request`
- topic itself does not support TTL

### Topic path resolution

There are two ways to create a topic member:

Form A:

```json
{
  "topic": "anime",
  "path": "castle-notes",
  "url": "# Castle",
  "type": "md2html"
}
```

Form B:

```json
{
  "path": "anime/castle-notes",
  "url": "# Castle",
  "type": "md2html"
}
```

Resolution rules:

- if `topic` is provided, that topic must already exist
- when `topic` is provided, `path` is normalized first
- when `topic` is provided and normalized `path` contains no `/`, it is treated as `<topic>/<path>`
- when `topic` is provided, normalized `path = "/"` is rejected
- when `topic` is provided, empty topic members such as `anime//castle` are rejected
- if `path` starts with an existing topic prefix, it is treated as a topic member
- if multiple topic prefixes match, the longest existing prefix wins
- topic item create/update returns success only after:
  - item content is stored
  - `topic:<topic>:items` is updated
  - topic home is rebuilt

Example:

- existing topics:
  - `blog`
  - `blog/2026`
- request path:
  - `blog/2026/post-1`
- resolved topic:
  - `blog/2026`
- resolved relative path:
  - `post-1`

Protection rules:

- if `path=<topic>` and that topic exists, normal `POST / PUT / DELETE` are rejected
- topic home must be managed with `type=topic`

---

## REST API

## `POST /`

Create a regular item, topic, or file upload.

### JSON request

```json
{
  "url": "hello",
  "path": "note",
  "title": "Greeting",
  "type": "text",
  "ttl": 10,
  "topic": "anime"
}
```

Supported fields:

- `url: string` required
- `path: string` optional
- `title: string` optional
- `type: string` optional
- `convert: string` optional alias of `type`
- `ttl: number` optional
- `topic: string` optional

Type behavior:

- `url`
  - validates URL and trims surrounding spaces
- `text`
  - stores plain text
- `html`
  - stores raw HTML
- `md2html`
  - converts Markdown to full HTML before storing
  - generated HTML `<title>` uses stored `title`
- `qrcode`
  - converts input to terminal QR text
- `topic`
  - creates a topic resource

### Multipart upload

Content type:

- `multipart/form-data`

Supported fields:

- `file` required
- `path` optional
- `title` optional
- `ttl` optional
- `topic` optional

Rules:

- file uploads require configured S3-compatible storage
- if `path` has no extension, uploaded file extension is appended
- topic path resolution follows the same rules as JSON create

### Topic creation

```json
{
  "path": "anime",
  "type": "topic"
}
```

Rules:

- topic must be explicitly created
- `path` is the topic name
- topic home is stored at `surl:<topic>`
- topic member set is stored at `topic:<topic>:items`
- empty topic is valid

Create response:

```json
{
  "surl": "http://host/anime",
  "path": "anime",
  "type": "topic",
  "title": "anime",
  "content": "0",
  "ttl": null
}
```

## `PUT /`

Update an existing item, or rebuild a topic.

### Regular update

- same body shape as `POST /`
- overwrites existing item at `path`
- returns `200 OK`
- `overwritten` contains previous preview or exported content

### Topic rebuild

```json
{
  "path": "anime",
  "type": "topic"
}
```

Rules:

- rebuilds `surl:<topic>` from `topic:<topic>:items`
- removes stale topic members whose `surl:<topic>/<path>` content no longer exists
- does not change child items

## `DELETE /`

Delete an item or a topic.

### Regular delete

```json
{
  "path": "note"
}
```

Behavior:

- deletes `surl:<path>`
- if stored type is `file`, also deletes the S3 object
- if the item belongs to a topic:
  - removes member from `topic:<topic>:items`
  - rebuilds topic home

### Topic delete

```json
{
  "path": "anime",
  "type": "topic"
}
```

Behavior:

- deletes:
  - `surl:<topic>`
  - `topic:<topic>:items`
- does not delete child items under `surl:<topic>/...`
- delete response `content` is the current topic member count as string

If the same topic is later recreated:

- existing child items under the same prefix are re-adopted into the topic index

## `GET /`

Authenticated route.

### List all items

Request:

- `GET /`
- empty body

Returns:

- array of item responses

Topic entries in this list use:

- `type = "topic"`
- `title = stored topic title`
- `content = member count as string`
- `ttl = null`

### List all topics

Request body:

```json
{
  "type": "topic"
}
```

Returns:

- array of topic item responses

Implementation note:

- topic list is discovered by scanning `topic:*:items`
- topic title is read from stored `surl:<topic>`

### Lookup one item

Request body:

```json
{
  "path": "note"
}
```

### Lookup one topic

Request body:

```json
{
  "path": "anime",
  "type": "topic"
}
```

or:

```json
{
  "path": "anime",
  "convert": "topic"
}
```

Example response:

```json
{
  "surl": "http://host/anime",
  "path": "anime",
  "type": "topic",
  "title": "anime",
  "ttl": null,
  "content": "2"
}
```

## `GET /<path>`

Public route.

Behavior by stored type:

- `url`
  - `302 Found` redirect
- `text`
  - plain text
- `html`
  - rendered HTML
- `file`
  - streamed from S3
- `topic`
  - topic home HTML

Cache headers:

- public `text`, `html`, `topic`, `url`, `file` responses set:
  - `Cache-Control: public, max-age=86400, s-maxage=86400`
- authenticated JSON responses do not set public cache headers

---

## Topic Rendering Rules

Topic home is generated from `topic:<topic>:items`.

Markdown shape:

```md
# <topic>

- [<title>](/<topic>/<path>) <mark> YYYY-MM-DD
```

Type marks:

- `url` -> `↗`
- `text` -> `☰`
- `file` -> `◫`
- `html` -> no mark

Title rules:

- use stored `title` when present
- otherwise use path without topic prefix

Markdown conversion rules:

- all `md2html` content writes full HTML
- generated HTML `<title>` uses stored `title`
- topic Markdown content also gets a top backlink:
  - `◂ [Back to \<\<topic\>\>](/<topic>)`

---

## Redis Storage

This section is the source of truth for reimplementation.

## Main content keys

### Regular item

- key: `surl:<path>`
- value: stored content JSON

Examples:

- `surl:note`
- `surl:docs/intro`

### Topic home

- key: `surl:<topic>`
- value: stored content JSON with `type=topic`

Examples:

- `surl:anime`
- `surl:blog/2026`

### Topic member

- key: `surl:<topic>/<relative-path>`
- value: stored content JSON

Examples:

- `surl:anime/castle-notes`
- `surl:blog/2026/post-1`

## Topic member index

- key: `topic:<topic>:items`
- Redis type: `zset`

Semantics:

- member: topic-relative path
- score: last updated Unix timestamp in seconds

Example:

- key: `topic:anime:items`
- member: `castle-notes`
- score: `1797984000`

### Empty topic placeholder

Empty topics still create a `zset`.

Implementation detail:

- member: `__topic_placeholder__`
- score: `0`

Purpose:

- keeps the topic zset key present even when the topic is empty

Rules:

- topic count ignores the placeholder
- topic rendering ignores the placeholder
- topic list and topic lookup do not expose the placeholder

## File cache keys

- `cache:file:<path>`
- `cache:filemeta:<path>`

Rules:

- only used for stored `file` items
- only used for small file reads
- cache TTL is `1 hour`
- cache is cleared on overwrite and delete

Important boundary:

- topic count and topic index are not actively repaired on TTL expiration
- expired topic items may leave stale members in `topic:<topic>:items`
- `PUT /` with `type=topic` repairs stale members and refreshes count/index

---

## Export Mode

Header:

```text
x-export: true
```

Supported on:

- create
- update
- delete
- lookup
- list

Behavior:

- regular items:
  - `content` returns full content instead of preview
- topics:
  - current implementation still returns member count string in `content`

---

## Common Error Codes

- `unauthorized`
- `invalid_request`
- `conflict`
- `not_found`
- `payload_too_large`
- `internal`
- `s3_not_configured`
