# Post-go API

## Overview

- Base URL: `http://host:port`
- Write operations require: `Authorization: Bearer <SECRET_KEY>`
- Content storage key model:
  - Regular item: `surl:<path>`
  - Topic index: `surl:<topic>`
  - Topic members: `surl:<topic>/<path>`
  - Topic member set: `topic:<topic>:items`

## Content Model

Redis content values are stored as JSON:

```json
{
  "type": "text",
  "content": "hello",
  "title": "Greeting"
}
```

Current stored fields:

- `type: string`
- `content: string`
- `title: string` optional

Stored `type` values:

- `url`
- `text`
- `html`
- `file`
- `topic`

Write-time alias values:

- `md2html` -> stored as `html`
- `qrcode` -> stored as `text`
- `convert` is accepted as an alias of `type`

If both `type` and `convert` are provided:

- they must match
- otherwise the request fails with `400 invalid_request`

## Response Shapes

### Item response

Used by:

- authenticated single-item lookup
- authenticated list items
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

- successful JSON responses always include `title`
- when the stored value has no title, the API returns `""`

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

Fields:

- `surl: string`
- `path: string`
- `type: string`
- `title: string`
- `content: string`
- `ttl: number | null`
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

Fields:

- `deleted: string`
- `type: string`
- `title: string`
- `content: string`

### Error response

```json
{
  "error": "Invalid JSON body",
  "code": "invalid_request",
  "hint": null,
  "details": null
}
```

## Authentication

Required for:

- `POST /`
- `PUT /`
- `DELETE /`
- authenticated `GET /`

Public access:

- `GET /<path>`

## Routes

### `POST /`

Create a regular item, topic, or upload a file.

#### JSON body for regular items

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

Parameters:

- `url: string` required
- `path: string` optional
- `title: string` optional
- `type: string` optional
- `convert: string` optional alias of `type`
- `ttl: number` optional, minutes
- `topic: string` optional

Type behavior:

- if omitted:
  - URL-like values become `url`
  - all others become `text`
- `type=url`
  - validates URL and normalizes surrounding spaces
- `type=text`
  - stores plain text
- `type=html`
  - stores raw HTML
- `type=md2html` or `convert=md2html`
  - converts Markdown to HTML before storing
  - when `title` is present, generated HTML `<title>` uses stored `title`
- `type=qrcode` or `convert=qrcode`
  - converts text to terminal QR code before storing
- `type=topic` or `convert=topic`
  - creates a topic resource

Regular item path rules:

- if `path` is missing, a random short path is generated
- allowed characters:
  - `a-z A-Z 0-9 - _ . / ( )`
- maximum path length:
  - `99`

TTL rules:

- item TTL is in minutes
- `ttl` must be a natural number (`>= 0`)
- `ttl = 0` means no expiration
- invalid values return `400 invalid_request`

#### Topic creation body

```json
{
  "path": "anime",
  "type": "topic"
}
```

Rules:

- topic must be explicitly created
- topic does not support `ttl`
- `path` is the topic name
- topic home is stored at `surl:<topic>`
- `topic:<topic>:items` is created when the topic is created

Topic create response:

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

#### Topic item creation

Two supported forms:

Form A:

```json
{
  "topic": "anime",
  "path": "castle-notes",
  "url": "# Castle",
  "type": "md2html",
  "title": "Castle Notes"
}
```

Form B:

```json
{
  "path": "anime/castle-notes",
  "url": "# Castle",
  "type": "md2html",
  "title": "Castle Notes"
}
```

Rules:

- if `topic` is set, the topic must already exist
- if `path` begins with an existing topic prefix, it is treated as a topic item
- if both `topic` and full topic path are given, they must match
- topic item final path becomes `<topic>/<path>`

#### Multipart upload

Content type:

- `multipart/form-data`

Fields:

- `file` required
- `path` optional
- `title` optional
- `ttl` optional, natural number in minutes
- `topic` optional

Example:

```bash
curl "$POST_BASE_URL/" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@./photo.jpg" \
  -F "path=poster" \
  -F "title=Poster Pack" \
  -F "topic=anime"
```

Rules:

- file uploads require configured S3-compatible storage
- if `path` has no extension, uploaded file extension is appended
- `ttl` must be a natural number (`>= 0`)
- `ttl = 0` means no expiration
- if `topic` is present, final path becomes `<topic>/<path>`
- if full path starts with an existing topic prefix, it is treated as a topic item

### `PUT /`

Update an existing item or trigger topic rebuild.

#### Regular update

Same body shape as `POST /`.

Rules:

- overwrites existing item at `path`
- returns `200 OK`
- `overwritten` contains previous preview or exported content

#### Topic rebuild

```json
{
  "path": "anime",
  "type": "topic"
}
```

Rules:

- rebuilds topic index page from `topic:<topic>:items`
- topic does not support `ttl`

### `DELETE /`

Delete an item or delete a topic resource.

#### Regular delete

```json
{
  "path": "note"
}
```

Behavior:

- deletes `surl:<path>`
- if stored type is `file`, also deletes the S3 object
- if item belongs to a topic, also:
  - removes member from `topic:<topic>:items`
  - rebuilds topic home

#### Topic delete

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
- does not delete topic child items under `surl:<topic>/...`
- deleting a topic returns current topic member count in `content`

### `GET /`

Authenticated mode.

#### List all items

Request:

- authenticated `GET /`
- empty body

Response:

- array of item responses

Notes:

- topics are included in the list
- for topic entries:
  - `type = "topic"`
  - `title = stored topic title`
  - `content = current member count as string`
  - `ttl = null`

#### List all topics

```json
{
  "type": "topic"
}
```

Returns:

- array of topic item responses
- topic data is discovered by scanning `topic:*:items`
- each item reuses the normal item response shape
- topic `title` is read from stored `surl:<topic>`

#### Lookup one item

```json
{
  "path": "note"
}
```

Returns regular item metadata.

#### Lookup one topic

```json
{
  "path": "anime",
  "type": "topic"
}
```

or

```json
{
  "path": "anime",
  "convert": "topic"
}
```

Returns:

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

## Public Read

### `GET /<path>`

Public route.

Behavior depends on stored type:

- `url`
  - `302 Found` redirect
- `text`
  - plain text response
- `html`
  - HTML response
- `file`
  - streams file from S3
- `topic`
  - returns topic home HTML

Cache behavior:

- public `text`, `html`, `topic`, `url`, `file` responses set:
  - `Cache-Control: public, max-age=86400, s-maxage=86400`
- authenticated JSON API responses do not set public cache headers

## File Cache

Small file reads are cached in Redis.

Cache keys:

- `cache:file:<path>`
- `cache:filemeta:<path>`

Rules:

- only used for stored `file` content
- only used when fetched file size is less than or equal to `MAX_CONTENT_SIZE_KB`
- cache TTL is `1 hour`
- cache is cleared when the item is overwritten or deleted
- larger files are streamed directly from S3 without Redis body cache

## Topic Index Rendering

Topic home is generated from `topic:<topic>:items`.

Notes:

- topic member zset exists even for an empty topic

Markdown shape:

```md
# <topic>

## <year>
- [<title>](/<topic>/<path>) <mark> · <MM-dd>
```

Type marks:

- `url` -> `↗`
- `text` -> `☰`
- `file` -> `◫`
- `html` -> no mark

Title fallback:

- use stored `title` when present
- otherwise use path without topic prefix

Markdown conversion:

- all `md2html` content gets:
  - HTML `<title>` from stored `title`
- topic Markdown items also get:
  - top backlink:
    - `Back to <topic>`

## Special Rules and Boundaries

### Topic home protection

If `path=<topic>` and the topic exists:

- regular `POST / PUT / DELETE` without `type=topic` are rejected

Error:

```json
{
  "error": "topic home must be managed with `type=topic`",
  "code": "invalid_request"
}
```

### Topic existence

- `topic` must exist before creating topic items via `topic=<topic>`
- if `path` uses `<topic>/<path>` form:
  - it is treated as topic content only when `<topic>` already exists

### Topic TTL

- topic itself does not support TTL
- topic item TTL is allowed
- expired topic items may leave stale members in `topic:<topic>:items`
- topic count and index are only corrected on later topic writes

### Topic recreation

If a topic is deleted and later recreated:

- existing child items under the same prefix are re-adopted into the topic index

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
  - `content` becomes full content instead of preview
- topics:
  - current implementation still returns member count string in `content`

## Common Error Codes

- `unauthorized`
- `invalid_request`
- `conflict`
- `not_found`
- `payload_too_large`
- `internal`
- `s3_not_configured`
