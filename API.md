# Post-go Documentation

This document explains Post-go from four angles:

1. how to start, test, depend on, and debug it
2. how Redis storage works
3. how the HTTP API behaves, including parameters and boundaries
4. how frontend rendering and display work

The goal is not only to call the API, but to fully understand the project model so that you can reuse the same design in your own app.

## 1. What This Project Really Is

Post-go is a small self-hosted sharing service built around one core idea:

- every public object lives at `/<path>`

That object can be:

- a text snippet
- a short link
- a Markdown page
- an HTML page
- a file
- a QR code
- a topic index page

The server does not separate "admin route", "public route", and "frontend app" in the usual way. Instead:

- `GET /<path>` is the public read path
- `POST /`, `PUT /`, `DELETE /`, `GET /` are the authenticated management API
- frontend pages are mostly server-rendered HTML

This unified path model is the most important thing to understand before reading the rest.

## 2. Start, Test, Dependencies, and Debugging

### 2.1 Entry and startup order

The process entry is `cmd/post-server/main.go`.

Startup order is:

1. load `.env.local`, otherwise `.env`
2. check embedded frontend assets
3. create HTTP handler with `httpapi.NewHandler()`
4. verify `SECRET_KEY` and `LINKS_REDIS_URL`
5. start `http.ListenAndServe`

Two details matter:

- `.env.local` has higher priority than `.env`
- existing system env vars are not overwritten by file values

### 2.2 Required and optional dependencies

Required runtime dependencies:

- Redis
- `SECRET_KEY`
- `LINKS_REDIS_URL`

Optional but required for file upload:

- S3-compatible object storage
- `S3_ENDPOINT`
- `S3_ACCESS_KEY_ID`
- `S3_SECRET_ACCESS_KEY`
- `S3_BUCKET_NAME`
- `S3_REGION`

Important config values:

- `PORT`, default `3000`
- `MAX_CONTENT_SIZE_KB`, code default `500`
- `MAX_FILE_SIZE_MB`, default `10`
- `POST_DEBUG`, default `false`

The sample file `.env.example` sets `MAX_CONTENT_SIZE_KB=512`, but the code default is `500`. If you do not set the env var, runtime uses `500`.

### 2.3 Local startup flow

Typical local startup flow:

```bash
cp .env.example .env.local
make assets-sync
make build
./post-server
```

Why `make assets-sync` is required:

- frontend assets are embedded into the Go binary
- the server refuses to start if embedded assets are missing or incomplete
- asset sync is implemented by `scripts/update_embedded_assets.go`

If you only need text, short links, Markdown, HTML, QR code, and topic features, Redis is enough.

If you need file upload, S3-compatible storage must be configured as well.

### 2.4 Build and test model

The `Makefile` defines this workflow:

- `make build`: delete old binary, then build `./cmd/post-server`
- `make test`: run `make build`, then `go test ./...`
- `make smoke`: run `make build`, then `./scripts/smoke_all.sh`
- `make assets-sync`: regenerate embedded assets

`scripts/smoke_all.sh` is the real regression entrypoint. It does:

1. build a temporary server binary at `/tmp/post-server-smoke-all`
2. run `go test ./...`
3. run render smoke
4. run embedded asset and HTTP API smoke on port `3012`, Redis DB `15`
5. run topic smoke on port `3013`, Redis DB `14`
6. run Redis storage smoke on port `3014`, Redis DB `13`

So `make smoke` is not just "smoke only". It is effectively:

- build
- unit tests
- render checks
- HTTP API checks
- topic checks
- Redis storage checks

Full smoke requires:

- local Redis on `localhost:6379`
- `curl`
- `jq`
- `redis-cli`

### 2.5 How to debug it effectively

The practical debug switch is:

```bash
POST_DEBUG=true
```

That enables request-level logs from `internal/httpapi/logging.go`, including:

- method
- path
- user agent
- content type
- forwarded headers
- response status
- response size
- elapsed time

Useful startup observations:

- `Loaded env from: .env.local` means config file was loaded
- asset missing error means you need `make assets-sync`
- missing env error means `LINKS_REDIS_URL` or `SECRET_KEY` is not set
- `env: PORT=... LINKS_REDIS_URL=...` means server is about to listen

Best debugging entrypoints by problem type:

- startup problem: `cmd/post-server/main.go`, `cmd/post-server/main_test.go`
- HTTP behavior: `internal/httpapi/*`, `scripts/smoke_http_api.sh`
- topic behavior: `internal/httpapi/topic_helpers.go`, `scripts/smoke_topic_api.sh`
- Redis persistence: `internal/storage/storage.go`, `scripts/smoke_redis_storage.sh`
- render/frontend behavior: `internal/convert/convert.go`, `internal/topic/render.go`, `scripts/smoke_render.sh`

This project does not provide:

- health endpoint
- pprof endpoint
- hot reload
- built-in delve flow

So the real debugging stack is:

- request logs
- unit tests
- smoke scripts
- direct Redis inspection

## 3. Redis Storage Logic

### 3.1 Redis has three jobs

Redis is used for three different things:

1. main object storage
2. topic member index
3. file cache

The client is created in `internal/redis/client.go`. Clients are cached by Redis URL, and the code performs `PING` before using a new client.

### 3.2 Main content keys

The main content key format is:

```text
surl:<path>
```

Examples:

```text
surl:hello
surl:docs/api
surl:anime
surl:anime/castle
```

The stored value is JSON defined by `internal/storage/storage.go`:

```json
{
  "type": "text",
  "content": "hello",
  "title": "Greeting",
  "created": "2026-03-23T10:00:00Z"
}
```

Meaning of fields:

- `type`: stored content type
- `content`: raw payload or object key
- `title`: display title
- `created`: business timestamp, not Redis timestamp

If old data is not valid JSON, the parser falls back to:

- `type=text`
- `content=<raw stored string>`

That fallback is useful for backward compatibility.

### 3.3 TTL behavior

TTL is only supported for normal objects.

Rules:

- omitted `ttl`: persistent
- `ttl=0`: persistent
- `ttl>0`: expire after that many minutes
- max value: `525600` minutes

Topic home does not support TTL. If `type=topic` and `ttl` is provided, the request is rejected.

TTL is applied to the Redis key itself, not to a field inside the JSON payload.

### 3.4 Topic storage model

A topic is not a separate table or service. It is a combination of:

1. one main object at `surl:<topic>`
2. one sorted set at `topic:<topic>:items`
3. one regenerated HTML index page stored back into `surl:<topic>`

Example:

```text
surl:anime
topic:anime:items
surl:anime/castle
surl:anime/howl
```

The sorted set stores topic members by relative path:

- member: `castle`
- member: `howl`

not full path.

The set also contains a placeholder member:

```text
__topic_placeholder__
```

This keeps the zset alive even when the topic has no real members.

### 3.5 How topic write flows work

When you create or update a topic member:

1. write the normal object to `surl:<topic>/<member>`
2. `ZADD topic:<topic>:items <member>`
3. scan existing keys matching `surl:<topic>/*` and adopt them into `topic:<topic>:items`
4. ensure placeholder member exists
5. rebuild topic index HTML
6. write rebuilt HTML back to `surl:<topic>`

When you delete a topic member:

1. delete `surl:<topic>/<member>`
2. `ZREM topic:<topic>:items <member>`
3. scan existing keys matching `surl:<topic>/*` and adopt them into `topic:<topic>:items`
4. ensure placeholder member exists
5. rebuild topic index HTML

When you create a topic home:

1. write `surl:<topic>` with `type=topic`
2. scan existing keys matching `surl:<topic>/*`
3. adopt them into `topic:<topic>:items`
4. ensure placeholder member exists
5. rebuild topic HTML

This means a topic can "adopt" old paths that already existed before the topic home was created.

When you refresh a topic home with `PUT` and `type=topic`, the server also re-scans `surl:<topic>/*`, adopts matching paths into `topic:<topic>:items`, ensures the placeholder member exists, and rebuilds the topic HTML.

### 3.6 Topic delete semantics

Deleting a topic does not delete its members.

It only deletes:

- `surl:<topic>`
- `topic:<topic>:items`

Child objects like `surl:<topic>/entry` remain in Redis.

After topic deletion, those paths become orphaned normal objects.

This is a very important behavior if you want to embed the model into your own app.

### 3.7 Sorting and listing

Global list behavior:

- scan `surl:*`
- batch read values with `MGET`
- sort by `created` descending
- if `created` cannot be parsed, place later
- then sort by path ascending

Topic index behavior:

- read `ZREVRANGE WITHSCORES`
- batch load member objects
- prefer each member's `created` as the display timestamp
- if `created` is missing or invalid, fall back to zset score
- sort descending by time, then ascending by path

Topic member count is:

```text
ZCARD - 1
```

because the placeholder member is excluded.

### 3.8 File cache

Small files are cached in Redis after being fetched from S3.

Keys:

```text
cache:file:<path>
cache:filemeta:<path>
```

File cache TTL is fixed at:

- 1 hour

Cache metadata stores:

- content type
- content length
- encoding
- checksum

This cache is separate from the main object storage and separate from the topic zset model.

## 4. HTTP API Behavior, Parameters, and Boundaries

### 4.1 Routing model

The root router is in `internal/httpapi/router.go`.

`/` behavior:

- `POST /`: authenticated create
- `PUT /`: authenticated upsert
- `DELETE /`: authenticated delete
- `GET /`: authenticated management API, otherwise public lookup of path `/`

`/<path>` behavior:

- if reserved embedded asset path, serve asset
- otherwise do public content lookup

### 4.2 Authentication model

Authentication is simple:

```http
Authorization: Bearer <SECRET_KEY>
```

It is used only for:

- `POST /`
- `PUT /`
- `DELETE /`
- management `GET /`

Public `GET /<path>` needs no authentication.

The system does not support:

- cookie auth
- query token
- basic auth

### 4.3 Create and update requests

There are two write formats:

- JSON body
- `multipart/form-data` for file upload

Common JSON fields:

- `url`
- `path`
- `title`
- `type`
- `convert`
- `created`
- `ttl`
- `topic`

Type rules:

- `md2html` is only an input alias
- it is stored as `md`
- real rendering happens on public read

Automatic type inference:

- if no type is provided and `url` looks like a full URL with scheme, store as `url`
- otherwise store as `text`

Path rules:

- length `1-99`
- allowed chars: `a-z A-Z 0-9 - _ . / ( )`
- leading and trailing slash are trimmed
- empty inner path segment is rejected in topic member context
- reserved asset paths cannot be used

If `path` is omitted for normal `POST`, the server generates a 5-character short path.

### 4.4 `POST /`

`POST /` means create only.

If path already exists:

- response is `409 conflict`
- hint is `Use PUT to overwrite`

If creation succeeds:

- response is `201`

### 4.5 `PUT /`

`PUT /` means upsert.

Behavior:

- existing path: update, return `200`
- missing path: create, return `201`

If the target already exists and `created` is not explicitly provided:

- old `created` is preserved

This matters if your app uses `created` for sorting or topic ordering.

### 4.6 `DELETE /`

Delete uses JSON body and requires `path`.

Special cases:

- if `type=topic`, delete topic home and topic index only
- if deleting a topic member, also update the topic zset and rebuilt topic HTML
- if deleting a file object and S3 is configured, the server tries to delete the object from storage too
- if `path` ends with a single `*`, delete all matching items and return a summary object
- wildcard delete returns `200` even when nothing matches
- topic wildcard delete only matches topic homes and still does not delete topic members

Wildcard delete response shape:

```json
{
  "deleted": [
    {
      "deleted": "note-a",
      "type": "text",
      "title": "",
      "created": "2026-03-26T00:00:00Z",
      "content": "hello"
    }
  ],
  "errors": [
    {
      "path": "note-b",
      "code": "internal",
      "message": "Internal server error"
    }
  ]
}
```

### 4.7 Authenticated `GET /`

This is a management API, not a public content route.

It supports JSON request body, which is unusual for `GET`.

Three modes:

1. body contains `path`: lookup one object
2. body has `type=topic` and no `path`: list all topics
3. otherwise: list all stored objects

This is easy to miss if you only read the route table and assume query params.

If `path` ends with a single `*`, lookup becomes a prefix query:

- normal wildcard lookup returns an array of non-topic objects
- `type=topic` wildcard lookup returns an array of topic homes only
- wildcard lookup returns `200` with `[]` when nothing matches
- wildcard lookup keeps the same `ttl` and `x-export` behavior as normal management lookup

### 4.8 Public `GET /<path>`

Public reads switch behavior by stored type:

- `url`: `302` redirect
- `topic`: HTML page
- `html`: raw HTML
- `md`: rendered HTML
- `qrcode`: text QR output
- `file`: stream from S3
- anything else: plain text

This means the public read route is a type-driven renderer.

### 4.9 Response content and export mode

Management API responses do not always return full content.

Default `content` behavior:

- `text`, `html`, `md`, `qrcode`: first 15 characters, then `...` if needed
- `url`, `file`: full stored value
- `topic`: member count as string

If request header contains:

```http
x-export: true
```

then management responses return full stored content for non-topic objects.

Topic still returns member count, not full HTML.

### 4.10 File upload behavior

File upload requires `multipart/form-data`.

Required field:

- `file`

Optional fields:

- `path`
- `title`
- `ttl`
- `topic`
- `created`

Important behaviors:

- if S3 is not configured, upload returns `501 s3_not_configured`
- `PUT` upload requires `path`
- if uploaded filename has an extension and the path does not, the server appends the extension automatically
- upload MIME type is chosen by:
  1. usable multipart content type
  2. extension inference
  3. body sniffing
  4. fallback to `application/octet-stream`

Large files are streamed back from object storage.

Small files, if size is within `MaxContentKB`, are also cached into Redis after first read.

### 4.11 Error and edge behavior

Common error codes:

- `unauthorized`
- `invalid_request`
- `conflict`
- `not_found`
- `payload_too_large`
- `internal`
- `s3_not_configured`
- `method_not_allowed`
- `forbidden`

Important boundaries you should design around in your own app:

- `type` and `convert` must match if both are set
- `type=url` requires a scheme like `https://`
- `ttl` must be a natural number
- `ttl` max is `525600`
- `type=topic` cannot use `ttl`
- reserved embedded asset paths cannot be used for user objects
- topic home cannot be overwritten as a normal object
- topic delete does not cascade to children
- invalid or missing stored `created` does not block read, but response may show `illegal`
- internal asset paths are same-origin only

## 5. Frontend Display and Rendering Logic

### 5.1 This project does not have a separate frontend app

There is no SPA and no standalone frontend build pipeline.

Frontend behavior is mostly:

- server-rendered HTML
- embedded static assets
- a few client-side helpers loaded only when needed

That makes Post-go closer to a content server than a classic frontend-backend split app.

### 5.2 Embedded asset model

Embedded assets are declared in `internal/assets/manifest.json` and loaded by `internal/assets/embedded.go`.

They are embedded with:

```go
//go:embed manifest.json files/*
```

Current asset categories include:

- base CSS
- highlight.js CSS and JS
- GitHub-flavored Markdown addon CSS and JS

These assets are generated and synced by `scripts/update_embedded_assets.go`.

They are not intended to be maintained by hand.

### 5.3 Reserved asset routes

Embedded assets are served from hashed paths such as:

```text
/asset/md-base-7f7c1c5a.css
```

Behavior:

- only `GET` and `HEAD`
- same-origin or same-site only
- cache policy: `public, max-age=31536000, immutable`

Two consequences:

1. business content cannot occupy those paths
2. these assets are public-cacheable but not intended for arbitrary cross-site hotlinking

### 5.4 Markdown rendering model

Markdown conversion is implemented in `internal/convert/convert.go` using Goldmark.

Enabled features include:

- GitHub Flavored Markdown
- footnotes
- GitHub alert/callout blocks
- KaTeX

Markdown is rendered into a complete HTML document, not a fragment.

The HTML shell includes:

- `<!doctype html>`
- `meta charset`
- viewport
- `<title>`
- embedded base CSS
- inline layout CSS
- `<article class="markdown-body">`

The layout is intentionally simple:

- centered reading page
- max width `838px`
- desktop padding `45px`
- mobile padding `25px`

### 5.5 Dynamic asset injection

Assets are injected based on rendered HTML features.

If the page has at least two headings:

- inject GFM addon CSS and JS

If the page contains fenced code blocks with language classes:

- inject highlight.js and light/dark theme CSS

If the page contains KaTeX output:

- inject external KaTeX CSS from CDN

That last point matters:

- most frontend assets are embedded
- KaTeX CSS is an explicit external exception

### 5.6 How each content type is displayed

Public display behavior:

- `url`: browser redirect
- `html`: raw HTML response
- `md`: rendered HTML page
- `topic`: prebuilt HTML page from Redis
- `qrcode`: terminal-style text QR code
- `file`: raw file response
- `text`: plain text

So "frontend behavior" is actually distributed across content types instead of being one central browser app.

### 5.7 Topic page rendering

Topic page generation is in `internal/topic/render.go`.

A topic page is not rebuilt on every request.

Instead:

- when topic members change
- the server regenerates the topic index
- stores the resulting HTML into `surl:<topic>`
- later public reads simply return that HTML

This is an important architecture choice:

- lower request-time cost
- simpler public serving path
- topic index becomes a materialized view

Topic page display rules:

- heading uses topic title, defaulting to path
- show `Home`
- render a flat Markdown list of members
- sort by updated time descending
- use title if present, otherwise use relative path

Type marks:

- `url`: `↗`
- `text` and `qrcode`: `☰`
- `file`: `◫`
- `html` and `md`: no extra mark

Display date uses:

- `Asia/Shanghai`

### 5.8 Topic item Markdown pages

When a Markdown page belongs to a topic, public rendering adds a topic header above the content.

That header includes:

- topic title
- a `Home` link back to the topic page
- optional current page title

This is not handled by a separate template engine.

Instead, the renderer prepends extra Markdown/HTML structure before conversion.

## 6. How To Reuse This Design In Your Own App

If you want to build your own app on top of the same ideas, the easiest way is to copy the architecture, not just the endpoints.

### 6.1 Keep the unified path model

Use one public namespace:

- `/<path>`

and let object type decide read behavior.

This keeps URLs simple and makes all content share one routing contract.

### 6.2 Separate write auth from public read

Post-go uses a simple and effective rule:

- public read is anonymous
- write and management read require one secret token

For internal tools, temporary share systems, AI agent bridges, and app-to-app posting, this is often enough.

### 6.3 Treat topic as a materialized index

Do not compute topic pages on every request.

Instead:

1. store child objects normally
2. maintain a zset index
3. rebuild topic HTML whenever membership changes

That gives you:

- fast public read
- deterministic ordering
- simple cache behavior

### 6.4 Separate blob storage from metadata storage

The project uses:

- Redis for metadata and indexes
- S3 for file bodies
- Redis file cache for hot small files

That split is practical for apps that want:

- simple object lookup
- object TTL
- cheap listing
- scalable file download path

### 6.5 Keep content types coarse and predictable

The type system here is intentionally small:

- `text`
- `url`
- `md`
- `html`
- `file`
- `qrcode`
- `topic`

That keeps the read path easy to reason about. If you add too many variants, `GET /<path>` quickly becomes hard to maintain.

## 7. Suggested Learning Order For A Go Developer

If you already understand basic Go but want to fully understand this project, read in this order:

1. `README.md`
2. `cmd/post-server/main.go`
3. `internal/httpapi/router.go`
4. `internal/httpapi/write_handlers.go`
5. `internal/httpapi/read_handlers.go`
6. `internal/httpapi/topic_helpers.go`
7. `internal/storage/storage.go`
8. `internal/convert/convert.go`
9. `internal/topic/render.go`
10. `scripts/smoke_http_api.sh`
11. `scripts/smoke_topic_api.sh`
12. `scripts/smoke_redis_storage.sh`

If you read in this order, you will understand the system from:

- process entry
- route dispatch
- write path
- read path
- storage model
- render model
- regression coverage

## 8. Key Takeaways

The most important facts about Post-go are:

- it is a unified-path content server
- Redis stores both objects and topic indexes
- topic pages are prebuilt and materialized
- public reads are type-driven
- frontend is mostly server-rendered HTML plus embedded assets
- the real operational contract is best learned from smoke tests, not only from `API.md`

If you want to adapt this project into your own app, the strongest reusable parts are:

- the `/<path>` data model
- the Redis `surl:<path>` storage layout
- the topic zset plus materialized index pattern
- the separation between public read and authenticated write
