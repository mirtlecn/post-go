package convert

import (
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	gfmit "github.com/mirtlecn/gfm-it"
	"github.com/skip2/go-qrcode"
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
	input := buildMarkdownInput(markdown, options)
	return gfmit.RenderMarkdownToHTML(input, gfmit.RenderOptions{
		Title:        options.PageTitle,
		AssetMode:    "local",
		AssetBaseURL: "/asset/",
		FooterHTML:   getConfiguredFooterHTML(),
		Slots: gfmit.RenderSlots{
			HeadEnd:   `<link rel="alternate" type="text/plain" href="?raw">`,
			BodyStart: `<!-- hint: append ?raw to view the raw file -->`,
		},
	})
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

func consumeFrontMatterLine(input string, start int) (int, bool) {
	if start >= len(input) {
		return 0, false
	}
	if idx := strings.IndexAny(input[start:], "\r\n"); idx >= 0 {
		return start + idx, true
	}
	return len(input), true
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

func buildMarkdownInput(markdown string, options MarkdownOptions) string {
	if options.TopicBackLink == "" {
		return markdown
	}
	markdown = stripFrontMatter(markdown)
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

func formatMarkdownLinkDestination(destination string) string {
	return "<" + destination + ">"
}

func getConfiguredFooterHTML() string {
	encodedFooter := strings.TrimSpace(os.Getenv("FOOTER"))
	if encodedFooter == "" {
		return ""
	}
	footerHTML, ok := decodeFooterBase64(encodedFooter)
	if !ok {
		return ""
	}
	return strings.TrimSpace(footerHTML)
}

func decodeFooterBase64(encoded string) (string, bool) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(encoded)
		if err == nil && utf8.Valid(decoded) {
			return string(decoded), true
		}
	}
	return "", false
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
