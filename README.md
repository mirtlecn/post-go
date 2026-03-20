[Vercel / Node.js / Web GUI](https://github.com/mirtlecn/post) | [CLI client](https://github.com/mirtlecn/post-cli) | [Skills for AI Agents](https://github.com/mirtlecn/post-cli/tree/master/skills)

# Post-go

一个轻量的「文本 / 链接 / 文件」分享服务。你可以把它当成自托管的临时剪贴板和短链接工具：

- 发一段文本，得到一个短地址
- 发一个 URL，访问短地址自动跳转
- 上传文件，得到可下载地址
- 用 Topic（主题）把多条内容组织在一个页面里

---

## 适合谁

- 想自托管分享服务的个人或小团队
- 想要极简 API，而不是复杂后台系统
- 想统一管理「文本、链接、文件」三类分享内容

---

## 你会得到什么

- **统一地址模型**：所有内容都对应 `/<path>`
- **公开读取 + 写入鉴权**：任何人可访问公开链接，写操作需要 Token
- **支持过期时间（TTL）**：可设置分钟级有效期
- **Topic 聚合页**：把同一主题下的内容自动整理为列表页

---

## 快速开始

### 1) 准备环境

必需：

- Redis
- `SECRET_KEY`（写接口鉴权）
- `LINKS_REDIS_URL`

如果你需要文件上传，还需要配置 S3 兼容对象存储。

### 2) 本地启动

```bash
cp .env.example .env.local
make assets-sync
make
./post-server
```

默认启动后即可通过 `http://localhost:3011`（或你配置的端口）访问。

---

## 最常见用法

先设置变量：

```bash
export POST_BASE_URL="http://localhost:3011"
export POST_TOKEN="your-secret-key"
```

### 新建一条文本

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

然后直接打开：

- `http://localhost:3011/hello`

### 新建一个短链接

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

访问 `/openai` 会 302 跳转到目标站点。

### 上传文件

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "path=manual" \
  -F "file=@./manual.pdf"
```

---

## 鉴权说明

写操作需要：

```http
Authorization: Bearer <SECRET_KEY>
```

通常需要鉴权的方法：`POST /`、`PUT /`、`DELETE /`、`GET /`（管理查询）。

公开访问内容使用 `GET /<path>`，不需要鉴权。

---

## API 文档

详细接口（字段、错误码、完整示例）请看：

- [API.md](./API.md)

---

## License

MIT

© Mirtle together with OpenAI Codex
