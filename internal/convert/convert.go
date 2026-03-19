package convert

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/skip2/go-qrcode"
	"html"
	"post-go/internal/assets"
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
	"go.abhg.dev/goldmark/frontmatter"
	"go.yaml.in/yaml/v3"
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
	enableFrontMatter := hasClosedFrontMatter(markdown)

	extensions := []goldmark.Extender{
		extension.GFM,
		extension.Footnote,
		// GitHub Alerts (NOTE/TIP/IMPORTANT/WARNING/CAUTION)
		callouts.AlertCallouts,
		// Math (KaTeX) follows the original master branch behavior.
		&katex.Extender{},
	}
	if enableFrontMatter {
		extensions = append(extensions, &frontmatter.Extender{})
	}

	md := goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)

	var buf bytes.Buffer
	context := parser.NewContext()
	if err := md.Convert([]byte(markdown), &buf, parser.WithContext(context)); err != nil {
		return "", err
	}

	frontMatterHTML, err := renderFrontMatterHTML(context)
	if err != nil {
		return "", err
	}

	var body strings.Builder
	body.WriteString(renderLeadingHTML(options))
	body.WriteString(frontMatterHTML)
	body.WriteString(buf.String())

	return wrapHTML(body.String(), alertCSS(), options.PageTitle), nil
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

// Legacy fallback kept unused during the front matter rendering experiment.
func stripFrontMatter(input string) string {
	if len(input) < 4 {
		return input
	}
	firstLineEnd, hasFirstLine := consumeFrontMatterLine(input, 0)
	if !hasFirstLine || input[:firstLineEnd] != "---" {
		return input
	}

	offset := skipFrontMatterLineBreak(input, firstLineEnd)
	for offset < len(input) {
		lineEnd, ok := consumeFrontMatterLine(input, offset)
		if !ok {
			return input
		}
		line := input[offset:lineEnd]
		if line == "---" || line == "..." {
			return input[skipFrontMatterLineBreak(input, lineEnd):]
		}
		offset = skipFrontMatterLineBreak(input, lineEnd)
	}

	return input
}

func hasClosedFrontMatter(input string) bool {
	if len(input) < 4 {
		return false
	}
	firstLineEnd, hasFirstLine := consumeFrontMatterLine(input, 0)
	if !hasFirstLine || input[:firstLineEnd] != "---" {
		return false
	}

	offset := skipFrontMatterLineBreak(input, firstLineEnd)
	for offset < len(input) {
		lineEnd, ok := consumeFrontMatterLine(input, offset)
		if !ok {
			return false
		}
		line := input[offset:lineEnd]
		if line == "---" || line == "..." {
			return true
		}
		offset = skipFrontMatterLineBreak(input, lineEnd)
	}

	return false
}

func consumeFrontMatterLine(input string, start int) (int, bool) {
	switch idx := bytes.IndexAny([]byte(input[start:]), "\r\n"); {
	case idx >= 0:
		return start + idx, true
	case start < len(input):
		return len(input), true
	default:
		return 0, false
	}
}

func skipFrontMatterLineBreak(input string, index int) int {
	if index >= len(input) {
		return index
	}
	if input[index] == '\r' {
		index++
	}
	if index < len(input) && input[index] == '\n' {
		index++
	}
	return index
}

func renderLeadingHTML(options MarkdownOptions) string {
	if options.TopicBackLink == "" {
		return ""
	}
	backLabel := options.TopicBackLabel
	if backLabel == "" {
		backLabel = "Topic"
	}
	var builder strings.Builder
	builder.WriteString("<div style=\"font-size: 1.3em; font-weight: bold\">")
	builder.WriteString(html.EscapeString(CapitalizeTopicLabel(backLabel)))
	builder.WriteString("</div>\n")
	builder.WriteString("<p><a href=\"")
	builder.WriteString(html.EscapeString(options.TopicBackLink))
	builder.WriteString("\"><strong>Home</strong></a>")
	if options.PageTitle != "" {
		builder.WriteString(" / <span style=\"color: #666;\">")
		builder.WriteString(html.EscapeString(options.PageTitle))
		builder.WriteString("</span>")
	}
	builder.WriteString("</p>\n")
	return builder.String()
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
	darkBg := "#0d1117"
	katexCSS := "https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.css"

	// 动态标签容器
	var extraHead strings.Builder
	var extraBody strings.Builder

	// 1. 检查 TOC：统计 h1~h6 标签数量
	reHeaders := regexp.MustCompile(`(?i)<h[1-6]`)
	headerMatches := reHeaders.FindAllString(body, -1)
	if len(headerMatches) >= 2 {
		extraHead.WriteString("<link rel=\"stylesheet\" href=\"" + assets.MustAssetURL("gfm_addon_css") + "\">\n")
		extraBody.WriteString("<script src=\"" + assets.MustAssetURL("gfm_addon_js") + "\"></script>\n")
	}

	// 2. 检查代码高亮
	if strings.Contains(body, "<code class=\"language-") {
		extraHead.WriteString("<link rel=\"stylesheet\" href=\"" + assets.MustAssetURL("highlight_light_css") + "\" media=\"(prefers-color-scheme: light)\">\n")
		extraHead.WriteString("<link rel=\"stylesheet\" href=\"" + assets.MustAssetURL("highlight_dark_css") + "\" media=\"(prefers-color-scheme: dark)\">\n")
		extraBody.WriteString("<script src=\"" + assets.MustAssetURL("highlight_js") + "\" defer></script>\n")
		extraBody.WriteString("<script>window.addEventListener('DOMContentLoaded', function(){ if (window.hljs && hljs.highlightAll) hljs.highlightAll(); });</script>\n")
	}

	// 3. Keep KaTeX CSS as an explicit external exception.
	if strings.Contains(body, "<span class=\"katex-display\">") {
		extraHead.WriteString("<link rel=\"stylesheet\" href=\"" + katexCSS + "\">\n")
	}

	return "<!doctype html>\n" +
		"<html>\n" +
		"<head>\n" +
		"<meta charset=\"utf-8\">\n" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1, minimal-ui\">\n" +
		"<title>" + html.EscapeString(pageTitle) + "</title>\n" +
		"<link rel=\"stylesheet\" href=\"" + assets.MustAssetURL("base_css") + "\">\n" +
		extraHead.String() +
		"<style>\n" +
		"  body { box-sizing: border-box; min-width: 200px; max-width: 838px; margin: 0 auto; padding: 45px; }\n" +
		"  .markdown-body .markdown-alert { padding: 0.5rem 1rem; }\n" +
		"  .markdown-body .frontmatter-yaml { margin: 1.25rem 0 1.5rem; border: 1px solid #d0d7de; border-radius: 8px; overflow: hidden; }\n" +
		"  .markdown-body .frontmatter-yaml-title { margin: 0; padding: 0.6rem 0.9rem; font-size: 0.85rem; font-weight: 600; letter-spacing: 0.02em; text-transform: uppercase; color: #57606a; background: #f6f8fa; border-bottom: 1px solid #d0d7de; }\n" +
		"  .markdown-body .frontmatter-yaml pre { margin: 0; border-radius: 0; }\n" +
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

func renderFrontMatterHTML(context parser.Context) (string, error) {
	data := frontmatter.Get(context)
	if data == nil {
		return "", nil
	}

	var metadata map[string]any
	if err := data.Decode(&metadata); err != nil {
		return "", err
	}
	if len(metadata) == 0 {
		return "", nil
	}

	renderedYAML, err := yaml.Marshal(metadata)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	builder.WriteString("<section class=\"frontmatter-yaml\">")
	builder.WriteString("<div class=\"frontmatter-yaml-title\">Front Matter</div>")
	builder.WriteString("<pre><code class=\"language-yaml\">")
	builder.WriteString(html.EscapeString(strings.TrimSpace(string(renderedYAML))))
	builder.WriteString("</code></pre>")
	builder.WriteString("</section>\n")
	return builder.String(), nil
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
