# Post-go — Lightweight File, Text & URL Sharing API

This is a Go rewrite of the Post service for high performance, low footprint, and easy deployment.

## Running

Prerequisites:
- Go (Test on Go 1.26)
- Redis
- S3-compatible storage (required for file uploads)

```bash
go mod tidy
cp .env.example .env.local
go build ./cmd/post-server
./post-server
```

Required env:
`LINKS_REDIS_URL`, `SECRET_KEY`

Optional env:
`MAX_CONTENT_SIZE_KB`, `MAX_FILE_SIZE_MB`, `S3_ENDPOINT`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_BUCKET_NAME`, `S3_REGION`

---

## API

Write operations require `Authorization: Bearer <SECRET_KEY>`.

```bash
# POST /  Create an entry (returns 409 if path already exists)
curl -X POST https://example.com/ \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://target.com","path":"mylink","ttl":1440}'

# PUT /  Create or overwrite (201 if new, 200 if overwritten)
curl -X PUT https://example.com/ \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://new-target.com","path":"mylink"}'

# POST /  Upload a binary file (multipart/form-data, stored in S3)
curl -X POST https://example.com/ \
  -H "Authorization: Bearer <token>" \
  -F "file=@photo.jpg" \
  -F "path=myimg" \
  -F "ttl=1440"

# DELETE /  Delete an entry
curl -X DELETE https://example.com/ \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"path":"mylink"}'

# GET /  List all entries (requires auth)
curl https://example.com/ \
  -H "Authorization: Bearer <token>"

# GET /:path  Access content (no auth required)
curl https://example.com/mylink
```

Response example:
```json
{
  "surl": "https://example.com/mylink",
  "path": "mylink",
  "type": "url",
  "content": "https://target.com",
  "expires_in": null
}
```

---

## License

MIT License.

Authors: Mirtle, Codex (OpenAI).
