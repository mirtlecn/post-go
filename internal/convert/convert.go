package convert

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/skip2/go-qrcode"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	callouts "github.com/ZMT-Creative/gm-alert-callouts"
	katex "github.com/libkush/goldmark-katex"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// MarkdownOptions customizes Markdown-to-HTML rendering.
type MarkdownOptions struct {
	PageTitle      string
	TopicBackLink  string
	TopicBackLabel string
}

// ConvertMarkdownToHTML converts Markdown (GFM) to a full HTML document.
func ConvertMarkdownToHTML(markdown string) (string, error) {
	return ConvertMarkdownToHTMLWithOptions(markdown, MarkdownOptions{})
}

// ConvertMarkdownToHTMLWithOptions converts Markdown (GFM) to HTML with optional page metadata.
func ConvertMarkdownToHTMLWithOptions(markdown string, options MarkdownOptions) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			// GitHub Alerts (NOTE/TIP/IMPORTANT/WARNING/CAUTION)
			callouts.AlertCallouts,
			// Math (KaTeX) - low priority but enabled
			&katex.Extender{},
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)

	var buf bytes.Buffer
	input := buildMarkdownInput(markdown, options)
	if err := md.Convert([]byte(stripFrontMatter(input)), &buf); err != nil {
		return "", err
	}
	return wrapHTML(buf.String(), alertCSS(), options.PageTitle), nil
}

// ConvertToQRCode converts text to a small terminal-friendly QR code string.
func ConvertToQRCode(text string) (string, error) {
	if len(text) > 250 {
		return "", fmt.Errorf("QR code conversion failed: input length %d exceeds 250 characters", len(text))
	}
	qr, err := qrcode.New(text, qrcode.Low)
	if err != nil {
		return "", err
	}
	bitmap := qr.Bitmap()
	if len(bitmap) == 0 {
		return "", errors.New("QR code generation produced empty output")
	}
	const (
		upper = "▀" // upper half block
		lower = "▄" // lower half block
		full  = "█" // full block
		space = " " // background
	)
	var sb strings.Builder
	sb.WriteString("📷 Scan this QR code\n\n")

	height := len(bitmap)
	width := len(bitmap[0])

	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x++ {
			top := bitmap[y][x]
			bot := false
			if y+1 < height {
				bot = bitmap[y+1][x]
			}
			switch {
			case top && bot:
				sb.WriteString(full)
			case top && !bot:
				sb.WriteString(upper)
			case !top && bot:
				sb.WriteString(lower)
			default:
				sb.WriteString(space)
			}
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func stripFrontMatter(input string) string {
	// Remove YAML front matter: --- ... ---
	if len(input) < 6 {
		return input
	}
	if !bytes.HasPrefix([]byte(input), []byte("---")) {
		return input
	}
	// simple scan for second ---
	idx := bytes.Index([]byte(input[3:]), []byte("\n---"))
	if idx == -1 {
		return input
	}
	return input[3+idx+4:]
}

func buildMarkdownInput(markdown string, options MarkdownOptions) string {
	if options.TopicBackLink == "" {
		return markdown
	}
	backLabel := options.TopicBackLabel
	if backLabel == "" {
		backLabel = "Topic"
	}
	var builder strings.Builder
	builder.WriteString("<div style=\"font-size: 1.3em; font-weight: bold\">")
	builder.WriteString(html.EscapeString(CapitalizeTopicLabel(backLabel)))
	builder.WriteString("</div>\n\n")
	builder.WriteString("[**Home**](")
	builder.WriteString(formatMarkdownLinkDestination(options.TopicBackLink))
	builder.WriteString(")")
	if options.PageTitle != "" {
		builder.WriteString(" / <span style=\"color: #666;\">")
		builder.WriteString(html.EscapeString(options.PageTitle))
		builder.WriteString("</span>")
	}
	builder.WriteString("\n\n\n\n\n\n")
	builder.WriteString(markdown)
	return builder.String()
}

func escapeMarkdownLinkText(text string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"<", "\\<",
		">", "\\>",
		"[", "\\[",
		"]", "\\]",
	)
	return replacer.Replace(text)
}

func formatMarkdownLinkDestination(destination string) string {
	return "<" + destination + ">"
}

// CapitalizeTopicLabel uppercases the first rune of a topic label.
func CapitalizeTopicLabel(label string) string {
	if label == "" {
		return ""
	}
	firstRune, size := utf8.DecodeRuneInString(label)
	if firstRune == utf8.RuneError && size == 0 {
		return ""
	}
	return string(unicode.ToUpper(firstRune)) + label[size:]
}

func wrapHTML(body, alertsStyle, pageTitle string) string {
	// 基础资源
	cssURL := "https://cdn.jsdelivr.net/gh/sindresorhus/github-markdown-css/github-markdown.min.css"
	darkBg := "#0d1117"

	// 外部资源定义
	hlCSSLight := "https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@11.11.1/build/styles/github.min.css"
	hlCSSDark := "https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@11.11.1/build/styles/github-dark.min.css"
	hlJS := "https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@11.11.1/build/highlight.min.js"
	tocJS := "https://cdn.jsdelivr.net/gh/mirtlecn/public/gfm-addon.min.js"
	tocCSS := "https://cdn.jsdelivr.net/gh/mirtlecn/public/gfm-addon.min.css"
	katexCSS := "https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.css"

	// 动态标签容器
	var extraHead strings.Builder
	var extraBody strings.Builder

	// 1. 检查 TOC：统计 h1~h6 标签数量
	reHeaders := regexp.MustCompile(`(?i)<h[1-6]`)
	headerMatches := reHeaders.FindAllString(body, -1)
	if len(headerMatches) >= 2 {
		extraHead.WriteString("<link rel=\"stylesheet\" href=\"" + tocCSS + "\">\n")
		extraBody.WriteString("<script src=\"" + tocJS + "\"></script>\n")
	}

	// 2. 检查代码高亮
	if strings.Contains(body, "<code class=\"language-") {
		extraHead.WriteString("<link rel=\"stylesheet\" href=\"" + hlCSSLight + "\" media=\"(prefers-color-scheme: light)\">\n")
		extraHead.WriteString("<link rel=\"stylesheet\" href=\"" + hlCSSDark + "\" media=\"(prefers-color-scheme: dark)\">\n")
		extraBody.WriteString("<script src=\"" + hlJS + "\" defer></script>\n")
		extraBody.WriteString("<script>window.addEventListener('DOMContentLoaded', function(){ if (window.hljs && hljs.highlightAll) hljs.highlightAll(); });</script>\n")
	}

	// 3. 检查公式
	if strings.Contains(body, "<span class=\"katex-display\">") {
		extraHead.WriteString("<link rel=\"stylesheet\" href=\"" + katexCSS + "\">\n")
	}

	return "<!doctype html>\n" +
		"<html>\n" +
		"<head>\n" +
		"<meta charset=\"utf-8\">\n" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1, minimal-ui\">\n" +
		"<title>" + html.EscapeString(pageTitle) + "</title>\n" +
		"<link rel=\"stylesheet\" href=\"" + cssURL + "\">\n" +
		extraHead.String() +
		"<style>\n" +
		"  body { box-sizing: border-box; min-width: 200px; max-width: 980px; margin: 0 auto; padding: 45px; }\n" +
		"  .markdown-body .markdown-alert { padding: 0.5rem 1rem; }\n" +
		alertsStyle + "\n" +
		"  @media (prefers-color-scheme: dark) { body { background-color: " + darkBg + "; } }\n" +
		"  @media (max-width: 767px) { body { max-width: 100%; padding: 25px; } }\n" +
		"</style>\n" +
		"</head>\n" +
		"<body>\n" +
		"<article class=\"markdown-body\">\n" +
		body +
		"\n</article>\n" +
		extraBody.String() +
		"</body>\n" +
		"</html>"
}

func alertCSS() string {
	// Minimal styling that keeps parity with GitHub-like alerts.
	// These classes are emitted by the goldmark callouts extension.
	css := []string{
		".markdown-body .callout { border-left: 4px solid #9e9e9e; padding: 0.75rem 1rem; margin: 1rem 0; background: #f6f8fa; border-radius: 6px; }",
		".markdown-body .callout-title { display: flex; align-items: center; gap: 0.5rem; font-weight: 600; }",
		".markdown-body .callout-title-text { margin: 0; }",
		".markdown-body .callout-body > :first-child { margin-top: 0.5rem; }",
		".markdown-body .callout-note { border-color: #2f81f7; }",
		".markdown-body .callout-tip { border-color: #3fb950; }",
		".markdown-body .callout-important { border-color: #a371f7; }",
		".markdown-body .callout-warning { border-color: #d29922; }",
		".markdown-body .callout-caution { border-color: #f85149; }",
		"@media (prefers-color-scheme: dark) {",
		"  .markdown-body .callout { background: #161b22; color: inherit; }",
		"  .markdown-body .callout-note { border-color: #58a6ff; }",
		"  .markdown-body .callout-tip { border-color: #3fb950; }",
		"  .markdown-body .callout-important { border-color: #a371f7; }",
		"  .markdown-body .callout-warning { border-color: #d29922; }",
		"  .markdown-body .callout-caution { border-color: #f85149; }",
		"}",
	}
	return strings.Join(css, "\n")
}
