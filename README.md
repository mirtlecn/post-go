[Vercel / Node.js / Web GUI](https://github.com/mirtlecn/post) | [CLI Client](https://github.com/mirtlecn/post-cli)

# Post-go — Lightweight File, Text & URL Sharing API

This is a Go rewrite of the Post service for high performance, low footprint, and easy deployment.

## Running

Prerequisites:
- Go (Test on Go 1.26)
- Redis
- S3-compatible storage (required for file uploads)

```bash
go mod tidy
cp .env.example .env.local # The server loads env from `.env.local` first, then `.env`.
go build ./cmd/post-server
./post-server
```

Required env:
`LINKS_REDIS_URL`, `SECRET_KEY`

Optional env:
`MAX_CONTENT_SIZE_KB`, `MAX_FILE_SIZE_MB`, `S3_ENDPOINT`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_BUCKET_NAME`, `S3_REGION`

---

## Testing

Run focused Go tests while working on a specific area:

```bash
go test ./internal/redis
go test ./internal/httpapi
go test ./internal/convert
```

Recommended verification flow for staged changes:

```bash
# Goal 1
go test ./internal/redis

# Goal 2-4
go test ./internal/httpapi ./internal/convert

# Final verification
go test ./...
```

The current automated coverage includes:
- Redis client lifecycle behavior
- HTTP create/delete failure handling
- Upload compensation when Redis persistence fails
- Existing path extension behavior for file uploads
- Existing raw HTML preservation in Markdown conversion

---

## API

Write operations require `Authorization: Bearer <SECRET_KEY>`.

Suggested shell variables:

```bash
export POST_BASE_URL="https://example.com"
export POST_TOKEN="your-secret-key"
```

```bash
# Create a short URL or text snippet with JSON.
# `type` can be omitted for normal URLs, or set to `text` / `html`.
curl "$POST_BASE_URL" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://target.com",
    "path": "mylink",
    "ttl": 1440
  }'

# Create rendered HTML from Markdown on write.
curl "$POST_BASE_URL" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "# Title\n\nHello from Markdown",
    "path": "doc/readme",
    "convert": "md2html"
  }'

# Upload a file to S3-compatible storage.
curl "$POST_BASE_URL" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "file=@./photo.jpg" \
  -F "path=uploads/photo"

# Update an existing entry with PUT.
curl "$POST_BASE_URL" \
  -X PUT \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://new-target.com",
    "path": "mylink"
  }'

# List all entries.
curl "$POST_BASE_URL" \
  -H "Authorization: Bearer $POST_TOKEN"

# Export full content instead of preview.
# Works for list, single-path lookup, create/update, and delete responses.
curl "$POST_BASE_URL" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "x-export: true"

# Read one entry as JSON metadata.
curl "$POST_BASE_URL" \
  -X GET \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"mylink"}'

# Read one entry as JSON metadata and export full content.
curl "$POST_BASE_URL" \
  -X GET \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -H "x-export: true" \
  -d '{"path":"mylink"}'

# Read publicly by path: URL entries redirect, text/html return directly, files stream.
curl -L "$POST_BASE_URL/mylink"

# Create and export full content in response.
curl "$POST_BASE_URL" \
  -X POST \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -H "x-export: true" \
  -d '{"url":"https://target.com","path":"mylink"}'

# Delete by path.
curl "$POST_BASE_URL" \
  -X DELETE \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"mylink"}'

# Delete and export full content in response.
curl "$POST_BASE_URL" \
  -X DELETE \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -H "x-export: true" \
  -d '{"path":"mylink"}'
```

---

## Credits

MIT Licence

© Mirtle together with OpenAI Codex
