---
name: post-share-link
description: 当用户要把文字、链接、Markdown、文件或剪贴板内容分享到网上，并只返回一个可访问链接时使用。适用于“分享xx到网上”“给xx创建链接”“把这个发出去”“生成可访问链接”“上传并给我地址”“把剪贴板内容发到网上”“把这个文件变成链接”等请求。
---

# Post Share Link

这个 Skill 用 `post-cli` 把内容发到 Post 服务，并默认只返回生成后的链接。

优先处理这类请求：

- 分享 xx 到网上
- 给 xx 创建链接
- 把这段内容发出去
- 给我一个可访问地址
- 把这个文件上传并返回链接
- 把剪贴板内容分享出去
- 给这段 Markdown / 文本 / URL 生成链接

默认规则：

1. 默认使用 Skill 目录里的 `post-cli`。
2. 默认执行 `./skills/post-share-link/scripts/share_to_post.sh`。
3. 这个脚本第一次运行时，如果 Skill 目录里没有 `post-cli`，就自动下载到 `skills/post-share-link/post-cli`。
4. 下载地址固定为 `https://raw.githubusercontent.com/mirtlecn/post-go/refs/heads/master/post-cli`。
5. `-y` 是脚本内部传给 `post-cli new` 的，不要在调用 `share_to_post.sh` 时额外再传 `-y`。
6. `share_to_post.sh` 当前不支持 `--help`；需要看参数时，直接读脚本内容。
7. 成功时只返回链接，不附带 JSON、说明或额外文案。
8. 只有用户明确要求更多信息时，才返回完整响应。

环境要求：

- 运行前需要有 `POST_HOST` 和 `POST_TOKEN`。
- 第一次触发这个 Skill，若环境变量没配好，要明确提醒用户设置。
- 可直接提示用户执行：
  - `export POST_HOST='https://your-post-host'`
  - `export POST_TOKEN='your-token'`
- 如果用户说自己已经配了，但脚本仍报错，就原样返回缺失哪个环境变量。

参数建议：

- `slug`：
  - 用户明确指定时，优先使用用户给的值。
  - 用户没指定时，先根据内容生成一个有意义、可读的 slug。
  - 如果创建失败且看起来像重名冲突，就换一个 slug 再试一次。
  - 仍失败时，最后退回随机 slug。
- `ttl`：
  - 用户明确指定时，使用用户给的值，单位是分钟。
  - 用户没指定时，默认 `7` 天，也就是 `10080` 分钟。
  - 只有用户明确要求永久时，才不传 `ttl`。
- `convert`：
  - 用户明确指定时，使用用户给的值。
  - 用户没指定时，由内容和意图自动判断。
  - 明确是“创建短链”“把这个 URL 变成短链接”“跳转到这个地址”时，用 `url`。
  - 明确是 Markdown 内容或用户说“这是 md”时，用 `md2html`。
  - 明确是一整段原始 HTML 时，用 `html`。
  - 明确是一段代码，或只是普通短文本时，用 `text`。
  - 明确要上传二进制文件时，用 `file` 并配合 `-f`。
- `update`：只有用户明确说“覆盖”“更新原链接”时才传。
- `export`：默认不要传；只有用户要完整 JSON 才传。

执行方式：

- 分享纯文本：`./skills/post-share-link/scripts/share_to_post.sh --text "内容"`
- 分享 URL：`./skills/post-share-link/scripts/share_to_post.sh --text "https://example.com" --convert url`
- 分享 Markdown 并转 HTML：`./skills/post-share-link/scripts/share_to_post.sh --file /abs/path/doc.md --convert md2html`
- 上传文件：`./skills/post-share-link/scripts/share_to_post.sh --file /abs/path/a.png --convert file`
- 指定短路径和过期时间：`./skills/post-share-link/scripts/share_to_post.sh --text "内容" --slug demo --ttl 60`
- 覆盖已有路径：`./skills/post-share-link/scripts/share_to_post.sh --text "新内容" --slug demo --update`
- 分享剪贴板：`./skills/post-share-link/scripts/share_to_post.sh --clipboard`
- 删除已发布链接：直接调用 `post-cli rm <slug>`，不要通过 `share_to_post.sh`

决策规则：

- 默认使用 Skill 目录里的 `post-cli`，不要依赖宿主项目根目录是否存在该脚本。
- 若 Skill 目录里没有 `post-cli`，先自动下载，再继续执行。
- 用户给了文件路径，就优先走 `--file`。
- 用户没给文件路径，但明确说“剪贴板”，就走 `--clipboard`。
- 用户给的是一段文字或链接，就走 `--text`。
- `slug` 的优先级：用户指定 > 根据内容生成有意义的 slug > 随机 slug。
- `ttl` 的优先级：用户指定 > 默认 `10080` 分钟 > 用户明确要求永久时不传。
- `convert` 的优先级：用户指定 > 按内容和意图自动判断。
- 自动判断 `convert` 时，按下面顺序看：
  - 用户说“短链”“短网址”“跳转链接”，或内容本身就是 URL，用 `url`。
  - 用户说“md”“markdown”，或内容明显是 Markdown，用 `md2html`。
  - 内容明显是一整段 HTML，用 `html`。
  - 内容是代码块、命令、配置片段，或只是简单文本，用 `text`。
- 用户只说“分享这个文件”，如果路径在当前对话里可见，就直接上传；路径不明确时再向用户确认。
- 用户说“按文件上传”“原样上传文件”时，必须用 `--convert file`，不要转成 `md2html` 或 `text`。
- 文件上传时，服务端最终 slug 可能带上文件后缀，例如传 `README.md` 且 slug 用 `readme`，结果可能变成 `readme.md`；返回结果时以实际生成的链接为准。
- 删除链接时，直接用 `post-cli rm`，不要加 `-y`。
- 一次删多个 slug 时，可能只返回第一个结果；稳妥做法是逐个删除并分别确认返回值。

失败处理：

- 若缺少 `POST_HOST` 或 `POST_TOKEN`，直接报环境变量缺失。
- 若 `post-cli` 返回错误，原样转述核心错误，不要编造原因。
- 若用户要求的参数组合无效，比如 `convert=file` 但没有文件路径，应明确指出缺少哪个参数。
- 若删除时出现网络解析失败，比如 `curl: (6) Could not resolve host`，应视为网络问题，按需要提权重试。
