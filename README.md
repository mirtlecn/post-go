# Post-go

一个 Post API 服务。

## Run

```bash
cp .env.example .env.local
make
./post-server
```

默认地址：

```text
http://localhost:3000
```

## Examples

```bash
export POST_BASE_URL="http://localhost:3000"
export POST_TOKEN="your-secret-key"
```

Create text:

```bash
curl -X POST "$POST_BASE_URL/create" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"hello","url":"Hello Post-go","type":"text"}'
```

Create link:

```bash
curl -X POST "$POST_BASE_URL/create" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"openai","url":"https://openai.com","type":"url"}'
```

Create Markdown:

```bash
curl -X POST "$POST_BASE_URL/create" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"note","url":"# Hello","type":"md2html"}'
```

Upload file:

```bash
curl -X POST "$POST_BASE_URL/create" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "path=manual" \
  -F "file=@./manual.pdf"
```

Read:

```bash
curl "$POST_BASE_URL/hello"
curl "$POST_BASE_URL/note"
curl "$POST_BASE_URL/note?raw"
```

## API

See [API.md](./API.md).

## License

MIT
