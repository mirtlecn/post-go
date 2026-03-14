package convert

import (
	"strings"
	"testing"
)

func TestConvertMarkdownToHTMLPreservesRawHTML(t *testing.T) {
	output, err := ConvertMarkdownToHTML("Hello\n\n<script>alert('xss')</script>")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, "<script>alert('xss')</script>") {
		t.Fatalf("expected raw html to be preserved")
	}
}

func TestConvertMarkdownToHTMLWithOptionsSetsPageTitle(t *testing.T) {
	output, err := ConvertMarkdownToHTMLWithOptions("# Hello", MarkdownOptions{
		PageTitle: "Anime Archive",
	})
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, "<title>Anime Archive</title>") {
		t.Fatalf("expected page title to be rendered, got %q", output)
	}
}

func TestConvertMarkdownToHTMLWithOptionsAddsBackLink(t *testing.T) {
	output, err := ConvertMarkdownToHTMLWithOptions("# Hello", MarkdownOptions{
		TopicBackLink:  "/anime",
		TopicBackLabel: "Anime",
	})
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, `href="/anime"`) {
		t.Fatalf("expected topic backlink href, got %q", output)
	}
	if !strings.Contains(output, "Back to Anime") {
		t.Fatalf("expected topic backlink label, got %q", output)
	}
}
