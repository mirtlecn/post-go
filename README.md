[Vercel / Node.js / Web GUI](https://github.com/mirtlecn/post) | [CLI client](https://github.com/mirtlecn/post-cli) | [Skills for AI Agents](https://github.com/mirtlecn/post-cli/tree/master/skills)

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
make
./post-server
```

Build commands:

```bash
make       # remove old ./post-server and rebuild
make test  # run go test ./...
make smoke # run ./scripts/smoke_all.sh
```

CI:
- GitHub Actions runs `go test ./...`
- GitHub Actions runs a multi-platform snapshot build through GoReleaser on push and pull request
- build outputs are uploaded from `dist/` as a GitHub Actions artifact

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

For detailed API documentation, please refer to the [API Reference](./API.md).

---

## Credits

MIT Licence

© Mirtle together with OpenAI Codex
