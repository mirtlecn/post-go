package convert

import (
	"post-go/internal/assets"
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
		PageTitle:      "Howl Visual Draft",
		TopicBackLink:  "/anime",
		TopicBackLabel: "anime",
	})
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, "<div style=\"font-size: 1.3em; font-weight: bold\">Anime</div>") {
		t.Fatalf("expected topic heading, got %q", output)
	}
	if !strings.Contains(output, `href="/anime"`) {
		t.Fatalf("expected topic backlink href, got %q", output)
	}
	if !strings.Contains(output, "<strong>Home</strong>") {
		t.Fatalf("expected bold home label, got %q", output)
	}
	if !strings.Contains(output, `<span style="color: #666;">Howl Visual Draft</span>`) {
		t.Fatalf("expected page title suffix, got %q", output)
	}
}

func TestConvertMarkdownToHTMLWithOptionsEscapesBackLinkLabel(t *testing.T) {
	output, err := ConvertMarkdownToHTMLWithOptions("# Hello", MarkdownOptions{
		TopicBackLink:  "/anime",
		TopicBackLabel: "<Anime>",
	})
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, "<div style=\"font-size: 1.3em; font-weight: bold\">&lt;Anime&gt;</div>") {
		t.Fatalf("expected escaped topic heading label, got %q", output)
	}
}

func TestConvertMarkdownToHTMLUsesEmbeddedBaseAsset(t *testing.T) {
	output, err := ConvertMarkdownToHTML("# Hello")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if strings.Contains(output, "cdn.jsdelivr.net") {
		t.Fatalf("expected no external asset host, got %q", output)
	}
	if !strings.Contains(output, assets.MustAssetURL("base_css")) {
		t.Fatalf("expected embedded base asset url, got %q", output)
	}
}

func TestConvertMarkdownToHTMLAddsEmbeddedHighlightAssetsWhenCodeExists(t *testing.T) {
	output, err := ConvertMarkdownToHTML("```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, assets.MustAssetURL("highlight_light_css")) {
		t.Fatalf("expected light highlight css, got %q", output)
	}
	if !strings.Contains(output, assets.MustAssetURL("highlight_dark_css")) {
		t.Fatalf("expected dark highlight css, got %q", output)
	}
	if !strings.Contains(output, assets.MustAssetURL("highlight_js")) {
		t.Fatalf("expected highlight js, got %q", output)
	}
}

func TestConvertMarkdownToHTMLAddsEmbeddedTOCAssetsWhenMultipleHeadersExist(t *testing.T) {
	output, err := ConvertMarkdownToHTML("# One\n\n## Two")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, assets.MustAssetURL("gfm_addon_css")) {
		t.Fatalf("expected toc css, got %q", output)
	}
	if !strings.Contains(output, assets.MustAssetURL("gfm_addon_js")) {
		t.Fatalf("expected toc js, got %q", output)
	}
}

func TestConvertMarkdownToHTMLAddsKaTeXCSSForDisplayMath(t *testing.T) {
	output, err := ConvertMarkdownToHTML("$$\na+b\n$$")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, "katex.min.css") {
		t.Fatalf("expected external katex css link, got %q", output)
	}
}

func TestConvertMarkdownToHTMLStripsYAMLFrontMatter(t *testing.T) {
	output, err := ConvertMarkdownToHTML("---\ndate: 2015/12/01\ntitle: Hello\n---\nnihc\n\"\"\"")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if strings.Contains(output, "date: 2015/12/01") || strings.Contains(output, "title: Hello") {
		t.Fatalf("expected yaml front matter to be removed, got %q", output)
	}
	if !strings.Contains(output, "<p>nihc\n&quot;&quot;&quot;</p>") {
		t.Fatalf("expected markdown body to remain, got %q", output)
	}
}

func TestConvertMarkdownToHTMLKeepsInputWithoutClosingFrontMatterDelimiter(t *testing.T) {
	output, err := ConvertMarkdownToHTML("---\nnot: closed\nbody")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, "<hr>") {
		t.Fatalf("expected markdown to be preserved when front matter is not closed, got %q", output)
	}
	if !strings.Contains(output, "<p>not: closed\nbody</p>") {
		t.Fatalf("expected content after opening marker to remain, got %q", output)
	}
}

func TestConvertMarkdownToHTMLWithOptionsStripsYAMLFrontMatter(t *testing.T) {
	output, err := ConvertMarkdownToHTMLWithOptions("---\ndate: 2015/12/01\ntitle: Hello\n---\n# Body", MarkdownOptions{
		PageTitle:      "Anime Archive",
		TopicBackLink:  "/anime",
		TopicBackLabel: "anime",
	})
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if strings.Contains(output, "date: 2015/12/01") || strings.Contains(output, "title: Hello") {
		t.Fatalf("expected yaml front matter to be removed before adding topic chrome, got %q", output)
	}
	if !strings.Contains(output, `<a href="/anime"><strong>Home</strong></a>`) {
		t.Fatalf("expected topic backlink to remain, got %q", output)
	}
	if !strings.Contains(output, "<h1 id=\"body\">Body</h1>") {
		t.Fatalf("expected markdown body to remain, got %q", output)
	}
}
