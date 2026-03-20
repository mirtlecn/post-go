[Vercel / Node.js / Web GUI](https://github.com/mirtlecn/post) | [CLI client](https://github.com/mirtlecn/post-cli) | [Skills for AI Agents](https://github.com/mirtlecn/post-cli/tree/master/skills)

# Post-go

A lightweight service for sharing **text, links, and files**. Think of it as a self-hosted temporary clipboard plus short-link tool:

- Post text and get a short URL
- Post a URL and auto-redirect on access
- Upload files and get downloadable links
- Group multiple items under a Topic page

---

## Who is this for?

- Individuals or small teams who want a self-hosted sharing service
- Users who prefer a minimal API over a complex admin system
- Teams that want one model for text, links, and file sharing

---

## What you get

- **Unified path model**: every item is available at `/<path>`
- **Public read + authenticated write**: anyone can open public links; writes require a token
- **TTL support**: optional expiration in minutes
- **Topic aggregation page**: automatically list content under a topic

---

## Quick Start

### 1) Requirements

Required:

- Redis
- `SECRET_KEY` (write API authentication)
- `LINKS_REDIS_URL`

If you need file uploads, configure S3-compatible object storage as well.

### 2) Run locally

```bash
cp .env.example .env.local
make assets-sync
make
./post-server
```

By default, the service is available at `http://localhost:3000` (unless you set `PORT`).

---

## Common Usage

Set environment variables first:

```bash
export POST_BASE_URL="http://localhost:3000"
export POST_TOKEN="your-secret-key"
```

### Create a text item

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "hello",
    "url": "Hello Post-go",
    "type": "text"
  }'
```

Then open:

- `http://localhost:3000/hello`

### Create a short link

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

Accessing `/openai` returns a `302` redirect to the target URL.

### Upload a file

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "path=manual" \
  -F "file=@./manual.pdf"
```

---

## Authentication

Write operations require:

```http
Authorization: Bearer <SECRET_KEY>
```

Operations that typically require authentication: `POST /`, `PUT /`, `DELETE /`, `GET /` (management query).

Public content access uses `GET /<path>` and does not require authentication.

---

## API documentation

For full endpoint details (fields, error codes, complete examples), see:

- [API.md](./API.md)

---

## License

MIT

© Mirtle together with OpenAI Codex
