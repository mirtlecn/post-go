#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
source "$SCRIPT_DIR/lib/smoke_common.sh"
MODULE_TMP_DIR="$(mktemp -d "$ROOT_DIR/tmp-plan2-XXXXXX")"
trap 'rm -rf "$MODULE_TMP_DIR"; cleanup_smoke_tmp' EXIT

TMP_GO="$MODULE_TMP_DIR/main.go"
TMP_BIN="$(mktemp "$TMP_DIR/plan2-XXXXXX.bin")"

cat >"$TMP_GO" <<'EOF'
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"post-go/internal/convert"
	"post-go/internal/topic"
)

func fail(label string, detail string) {
	fmt.Fprintf(os.Stderr, "FAIL %s: %s\n", label, detail)
	os.Exit(1)
}

func pass(label string) {
	fmt.Printf("PASS %s\n", label)
}

func mustContain(label string, haystack string, needle string) {
	if !strings.Contains(haystack, needle) {
		fail(label, fmt.Sprintf("expected %q in output", needle))
	}
}

func main() {
	html, err := convert.ConvertMarkdownToHTMLWithOptions("# Hello", convert.MarkdownOptions{
		PageTitle:      "Anime Archive",
		TopicBackLink:  "/anime",
		TopicBackLabel: "Anime",
	})
	if err != nil {
		fail("convert html", err.Error())
	}
	mustContain("page title", html, "<title>Anime Archive</title>")
	pass("page title")
	mustContain("topic backlink href", html, `href="/anime"`)
	pass("topic backlink href")
	mustContain("topic backlink label", html, "Back to Anime")
	pass("topic backlink label")

	markdown := topic.BuildIndexMarkdown("Anime", []topic.Item{
		{
			Path:      "howl-visual",
			Type:      "html",
			Title:     "Howl Visual Draft",
			UpdatedAt: time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC),
		},
		{
			Path:      "castle-notes",
			Type:      "text",
			Title:     "Castle in the Sky Notes",
			UpdatedAt: time.Date(2026, time.December, 21, 10, 0, 0, 0, time.UTC),
		},
		{
			Path:      "screening-signup",
			Type:      "url",
			UpdatedAt: time.Date(2026, time.December, 20, 10, 0, 0, 0, time.UTC),
		},
		{
			Path:      "poster-pack-winter.zip",
			Type:      "file",
			Title:     "Poster Pack Winter",
			UpdatedAt: time.Date(2025, time.October, 18, 10, 0, 0, 0, time.UTC),
		},
	})
	mustContain("markdown year header", markdown, "## 2026")
	pass("markdown year header")
	mustContain("text type mark", markdown, "[Castle in the Sky Notes](./castle-notes) ☰ · 12-21")
	pass("text type mark")
	mustContain("url type mark", markdown, "[screening-signup](./screening-signup) ↗ · 12-20")
	pass("url type mark")
	mustContain("file type mark", markdown, "[Poster Pack Winter](./poster-pack-winter.zip) ◫ · 10-18")
	pass("file type mark")
	if strings.Contains(markdown, "[Howl Visual Draft](./howl-visual)  · 12-23") {
		fail("html type mark", "html entry should not render an empty mark")
	}
	mustContain("html no type mark", markdown, "[Howl Visual Draft](./howl-visual) · 12-23")
	pass("html no type mark")

	fallbackMarkdown := topic.BuildIndexMarkdown("anime", []topic.Item{
		{
			Path:      "notes/howl-visual",
			FullPath:  "anime/notes/howl-visual",
			Type:      "html",
			UpdatedAt: time.Date(2026, time.December, 19, 10, 0, 0, 0, time.UTC),
		},
	})
	mustContain("title fallback", fallbackMarkdown, "[notes/howl-visual](./notes/howl-visual) · 12-19")
	pass("title fallback")

	indexHTML, err := topic.RenderIndexHTML("Anime", []topic.Item{
		{
			Path:      "howl-visual",
			Type:      "html",
			Title:     "Howl Visual Draft",
			UpdatedAt: time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		fail("render topic html", err.Error())
	}
	mustContain("topic html title", indexHTML, "<title>Anime</title>")
	pass("topic html title")

	fmt.Println("Smoke PLAN2 checks completed successfully.")
}
EOF

cd "$ROOT_DIR"
go build -o "$TMP_BIN" "$TMP_GO"
"$TMP_BIN"
