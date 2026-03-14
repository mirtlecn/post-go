package storage

import "testing"

func TestBuildStoredValueMarshalsJSON(t *testing.T) {
	stored := BuildStoredValue(StoredValue{
		Type:    "html",
		Content: "<p>Hello</p>",
		Title:   "Greeting",
	})

	expected := `{"type":"html","content":"<p>Hello</p>","title":"Greeting"}`
	if stored != expected {
		t.Fatalf("expected %q, got %q", expected, stored)
	}
}

func TestParseStoredValueReadsJSON(t *testing.T) {
	value := ParseStoredValue(`{"type":"file","content":"post/default/file.txt","title":"Asset"}`)

	if value.Type != "file" {
		t.Fatalf("expected type file, got %q", value.Type)
	}
	if value.Content != "post/default/file.txt" {
		t.Fatalf("expected content to match, got %q", value.Content)
	}
	if value.Title != "Asset" {
		t.Fatalf("expected title Asset, got %q", value.Title)
	}
}

func TestParseStoredValueFallsBackToTextForInvalidJSON(t *testing.T) {
	value := ParseStoredValue("plain text")

	if value.Type != "text" {
		t.Fatalf("expected fallback type text, got %q", value.Type)
	}
	if value.Content != "plain text" {
		t.Fatalf("expected fallback content, got %q", value.Content)
	}
	if value.Title != "" {
		t.Fatalf("expected empty title, got %q", value.Title)
	}
}
