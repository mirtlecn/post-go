# Post-go REST API

> 面向 API 使用者的参考文档（调用方式、字段、示例、错误处理）。

## 1. Base URL

```text
http://host:port
```

例如：

```text
http://localhost:3011
```

---

## 2. Authentication

写操作与管理查询需要 Bearer Token：

```http
Authorization: Bearer <SECRET_KEY>
```

需要鉴权的典型请求：

- `POST /`
- `PUT /`
- `DELETE /`
- `GET /`（管理接口）

公开访问内容用 `GET /{path}`，无需鉴权。

---

## 3. Content Types（内容类型）

创建/更新时通过 `type` 指定：

- `text`：纯文本
- `url`：链接（公开访问时 302 跳转）
- `html`：HTML 内容
- `file`：文件（multipart 上传）
- `topic`：主题页（用于组织成员内容）
- `md2html`：写入时将 Markdown 转为 `html`
- `qrcode`：写入时将输入转为文本二维码（存储类型为 `text`）

`convert` 可作为 `type` 的别名。

---

## 4. Common Request Fields

JSON 请求常见字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `url` | string | 是* | 内容本体（文本、链接、Markdown 等） |
| `path` | string | 否 | 短路径；省略时服务端可自动生成 |
| `title` | string | 否 | 展示标题 |
| `created` | string | 否 | 创建时间 |
| `type` | string | 否 | 内容类型 |
| `convert` | string | 否 | `type` 别名 |
| `ttl` | integer | 否 | 过期时间（分钟） |
| `topic` | string | 否 | 指定归属主题 |

> `url` 在 `type=topic` 时不需要。

### Path 规则

- 最大长度：99
- 允许字符：`a-z A-Z 0-9 - _ . / ( )`
- 自动去除首尾多余 `/`
- `path` 与 `path/` 视为同一路径
- `asset/...` 为保留路径，不能用于业务内容

### TTL 规则

- 单位：分钟
- 范围：`0 ~ 525600`
- `0` 表示不过期
- `topic` 本身不支持 TTL

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

字段说明：

- `surl`：完整访问地址
- `path`：短路径
- `type`：内容类型
- `title`：标题（总会返回，缺省为 `""`）
- `created`：时间（缺失/异常时可能返回 `"illegal"`）
- `ttl`：剩余 TTL（无过期为 `null`）
- `content`：默认是预览内容

### 5.2 Error Object

```json
{
  "error": "Invalid JSON body",
  "code": "invalid_request",
  "hint": null,
  "details": null
}
```

常见 `code`：

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

创建普通内容、Topic，或上传文件。

### 6.1.1 创建文本（JSON）

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

### 6.1.2 创建短链接（JSON）

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

### 6.1.3 创建 Topic

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

### 6.1.4 上传文件（multipart）

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -F "path=manual" \
  -F "title=Manual" \
  -F "file=@./manual.pdf"
```

说明：

- 需要配置 S3 兼容存储
- 如果 `path` 无扩展名，服务端会补上上传文件扩展名

---

## 6.2 Upsert: `PUT /`

按 `path` 更新；不存在时创建。

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

行为：

- 已存在：更新，返回 `200`
- 不存在：创建，返回 `201`
- 可能返回 `overwritten` 字段（旧内容预览或导出内容）

### Topic 重建

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

用于刷新 Topic 首页与成员索引。

---

## 6.3 Delete: `DELETE /`

### 删除普通内容

```bash
curl -X DELETE "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path": "note"}'
```

### 删除 Topic

```bash
curl -X DELETE "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "anime",
    "type": "topic"
  }'
```

> 删除 Topic 不会删除其子路径下的内容对象本身。

---

## 6.4 Management Query: `GET /`

用于鉴权后的后台查询。

### 列出全部内容

```bash
curl -X GET "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN"
```

### 查询单个 path

```bash
curl -X GET "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path": "note"}'
```

### 查询 Topic

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

公开访问路径。

不同 `type` 的返回行为：

- `url`：`302 Found` 跳转
- `text`：返回纯文本
- `html`：返回 HTML
- `file`：返回文件流
- `topic`：返回主题页 HTML

示例：

```bash
curl -i "$POST_BASE_URL/note"
```

---

## 7. Topic 用法

有两种把内容写入 Topic 的方式：

### 方式 A：显式指定 `topic`

```json
{
  "topic": "anime",
  "path": "castle-notes",
  "url": "# Castle",
  "type": "md2html"
}
```

### 方式 B：直接使用带前缀路径

```json
{
  "path": "anime/castle-notes",
  "url": "# Castle",
  "type": "md2html"
}
```

如果同名前缀存在多个 Topic，系统会匹配**最长前缀**。

---

## 8. Export Mode

在请求头加：

```http
x-export: true
```

可用于 create/update/delete/lookup/list。

效果：

- 普通内容：`content` 返回完整原文（而不是预览）
- Topic：`content` 仍是成员数量字符串

---

## 9. Date & Time

`created` 支持多种输入格式（如 RFC3339、`2006-01-02` 等）。

服务端会统一存成 UTC RFC3339。

---

## 10. Quick Start Snippets

```bash
export POST_BASE_URL="http://localhost:3011"
export POST_TOKEN="your-secret-key"
```

创建文本：

```bash
curl -X POST "$POST_BASE_URL/" \
  -H "Authorization: Bearer $POST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"hello","url":"Hello","type":"text"}'
```

读取文本：

```bash
curl "$POST_BASE_URL/hello"
```
