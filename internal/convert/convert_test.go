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
