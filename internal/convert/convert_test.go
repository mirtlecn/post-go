package convert

import (
	"encoding/base64"
	"os"
	"post-go/internal/assets"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Unsetenv("FOOTER")
	os.Exit(m.Run())
}

func withFooterEnv(t *testing.T, value *string) {
	t.Helper()
	previousValue, hadPreviousValue := os.LookupEnv("FOOTER")
	if value == nil {
		_ = os.Unsetenv("FOOTER")
	} else {
		t.Setenv("FOOTER", *value)
		return
	}
	t.Cleanup(func() {
		if hadPreviousValue {
			_ = os.Setenv("FOOTER", previousValue)
		} else {
			_ = os.Unsetenv("FOOTER")
		}
	})
}

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

func TestConvertMarkdownToHTMLAddsRawReadHints(t *testing.T) {
	output, err := ConvertMarkdownToHTMLWithOptions("# Hello", MarkdownOptions{
		PageTitle: "Anime Archive",
	})
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}

	viewportIndex := strings.Index(output, `<meta name="viewport" content="width=device-width, initial-scale=1, minimal-ui">`)
	alternateIndex := strings.Index(output, `<link rel="alternate" type="text/plain" href="?raw">`)
	titleIndex := strings.Index(output, "<title>Anime Archive</title>")
	if viewportIndex == -1 || alternateIndex == -1 || titleIndex == -1 {
		t.Fatalf("expected viewport, raw alternate link, and title in output, got %q", output)
	}
	if !(viewportIndex < alternateIndex && alternateIndex < titleIndex) {
		t.Fatalf("expected raw alternate link between viewport and title, got %q", output)
	}

	bodyIndex := strings.Index(output, "<body>")
	hintIndex := strings.Index(output, "<!-- hint: append ?raw to view the raw file -->")
	articleIndex := strings.Index(output, `<article class="markdown-body">`)
	if bodyIndex == -1 || hintIndex == -1 || articleIndex == -1 {
		t.Fatalf("expected body hint before article, got %q", output)
	}
	if !(bodyIndex < hintIndex && hintIndex < articleIndex) {
		t.Fatalf("expected body hint before article, got %q", output)
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
	if !strings.Contains(output, assets.MustAssetURL("ravel_gfm_css")) {
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
	if !strings.Contains(output, assets.MustAssetURL("gfm_addons_css")) {
		t.Fatalf("expected toc css, got %q", output)
	}
	if !strings.Contains(output, assets.MustAssetURL("gfm_addons_js")) {
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

func TestConvertMarkdownToHTMLOmitsFooterWhenUnsetBlankOrInvalidBase64(t *testing.T) {
	withFooterEnv(t, nil)
	output, err := ConvertMarkdownToHTML("# Hello")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if strings.Contains(output, "post-footer") {
		t.Fatalf("expected footer to be omitted when FOOTER is unset, got %q", output)
	}

	blankFooter := " \n\t "
	withFooterEnv(t, &blankFooter)
	output, err = ConvertMarkdownToHTML("# Hello")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if strings.Contains(output, "post-footer") {
		t.Fatalf("expected footer to be omitted when FOOTER is blank, got %q", output)
	}

	invalidFooter := "not base64 <a>footer</a>"
	withFooterEnv(t, &invalidFooter)
	output, err = ConvertMarkdownToHTML("# Hello")
	if err != nil {
		t.Fatalf("expected invalid footer env to be ignored, got %v", err)
	}
	if strings.Contains(output, "post-footer") {
		t.Fatalf("expected footer to be omitted when FOOTER is invalid base64, got %q", output)
	}
}

func TestConvertMarkdownToHTMLInjectsConfiguredFooterHTML(t *testing.T) {
	footer := base64.StdEncoding.EncodeToString([]byte(`  footer-e8c3a91f <a href="https://example.test/link-42">link-17b92</a>  `))
	withFooterEnv(t, &footer)

	output, err := ConvertMarkdownToHTML("# One\n\n## Two")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}

	articleEndIndex := strings.Index(output, "</article>")
	tocScriptIndex := strings.Index(output, assets.MustAssetURL("gfm_addons_js"))
	footerIndex := strings.Index(output, `<footer class="markdown-body post-footer">`)
	if footerIndex == -1 {
		t.Fatalf("expected configured footer markup, got %q", output)
	}
	if articleEndIndex == -1 || articleEndIndex > footerIndex {
		t.Fatalf("expected footer after article, got %q", output)
	}
	if tocScriptIndex == -1 || tocScriptIndex > footerIndex {
		t.Fatalf("expected footer after dynamic body assets, got %q", output)
	}
	if !strings.Contains(output, "<footer class=\"markdown-body post-footer\">\nfooter-e8c3a91f <a href=\"https://example.test/link-42\">link-17b92</a>\n</footer>") {
		t.Fatalf("expected trimmed raw footer html, got %q", output)
	}
	if !strings.Contains(output, "margin-top: auto;") {
		t.Fatalf("expected footer layout css, got %q", output)
	}
	if !strings.Contains(output, "min-height: 100vh; display: flex; flex-direction: column;") {
		t.Fatalf("expected body flex layout css, got %q", output)
	}
}

func TestConvertMarkdownToHTMLInjectsRawBase64Footer(t *testing.T) {
	footer := base64.RawURLEncoding.EncodeToString([]byte(`footer-url-safe-2f65`))
	withFooterEnv(t, &footer)

	output, err := ConvertMarkdownToHTML("# Hello")
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if !strings.Contains(output, "footer-url-safe-2f65") {
		t.Fatalf("expected url-safe raw base64 footer to be decoded, got %q", output)
	}
}
