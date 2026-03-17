package topic

import (
	"strings"
	"testing"
	"time"
)

func TestBuildIndexMarkdownSortsByUpdatedAtAsFlatList(t *testing.T) {
	items := []Item{
		{
			Path:      "castle-notes",
			Type:      "text",
			Title:     "Castle in the Sky Notes",
			UpdatedAt: time.Date(2026, time.December, 21, 10, 0, 0, 0, time.UTC),
		},
		{
			Path:      "howl-visual",
			Type:      "html",
			Title:     "Howl Visual Draft",
			UpdatedAt: time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC),
		},
		{
			Path:      "poster-pack-winter.zip",
			Type:      "file",
			Title:     "Poster Pack Winter",
			UpdatedAt: time.Date(2025, time.October, 18, 10, 0, 0, 0, time.UTC),
		},
	}

	output := BuildIndexMarkdown("anime", "Anime", items)

	expected := strings.Join([]string{
		"# Anime",
		"",
		"- [Howl Visual Draft](</anime/howl-visual>) 2026-12-23",
		"- [Castle in the Sky Notes](</anime/castle-notes>) ☰ 2026-12-21",
		"- [Poster Pack Winter](</anime/poster-pack-winter.zip>) ◫ 2025-10-18",
		"",
	}, "\n")

	if output != expected {
		t.Fatalf("unexpected markdown output:\n%s", output)
	}
}

func TestBuildIndexMarkdownUsesFullPathFallbackForEmptyTitle(t *testing.T) {
	items := []Item{
		{
			Path:      "notes/howl-visual",
			FullPath:  "anime/notes/howl-visual",
			Type:      "url",
			UpdatedAt: time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC),
		},
	}

	output := BuildIndexMarkdown("anime", "anime", items)

	if !strings.Contains(output, "[notes/howl-visual](</anime/notes/howl-visual>) ↗ 2026-12-23") {
		t.Fatalf("expected fallback title from full path, got %q", output)
	}
}

func TestBuildIndexMarkdownWrapsHrefDestinationForSpecialCharacters(t *testing.T) {
	items := []Item{
		{
			Path:      "drafts/hello-(world)",
			Type:      "text",
			Title:     "Hello",
			UpdatedAt: time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC),
		},
	}

	output := BuildIndexMarkdown("anime", "Anime", items)

	if !strings.Contains(output, "[Hello](</anime/drafts/hello-(world)>) ☰ 2026-12-23") {
		t.Fatalf("expected markdown destination to be wrapped for special characters, got %q", output)
	}
}

func TestRenderIndexHTMLUsesPageTitle(t *testing.T) {
	html, err := RenderIndexHTML("anime", "Anime", []Item{
		{
			Path:      "howl-visual",
			Type:      "html",
			Title:     "Howl Visual Draft",
			UpdatedAt: time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("expected render to succeed, got %v", err)
	}
	if !strings.Contains(html, "<title>Anime</title>") {
		t.Fatalf("expected page title, got %q", html)
	}
	if !strings.Contains(html, "Howl Visual Draft") {
		t.Fatalf("expected entry title in html, got %q", html)
	}
	if !strings.Contains(html, `href="/anime/howl-visual"`) {
		t.Fatalf("expected absolute topic href, got %q", html)
	}
}

func TestBuildIndexMarkdownUsesRootAbsoluteHrefForNestedTopic(t *testing.T) {
	items := []Item{
		{
			Path:      "post-1",
			Type:      "text",
			Title:     "Post 1",
			UpdatedAt: time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC),
		},
	}

	output := BuildIndexMarkdown("blog/2026", "blog/2026", items)

	if !strings.Contains(output, "[Post 1](</blog/2026/post-1>) ☰ 2026-12-23") {
		t.Fatalf("expected nested topic link to stay root-absolute, got %q", output)
	}
	if strings.Contains(output, "](post-1)") {
		t.Fatalf("expected topic index to avoid browser-dependent relative href, got %q", output)
	}
}
