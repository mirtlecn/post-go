package storage

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestParseJSONBodyPreservesJSONNumber(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"ttl":1}`))

	body, err := ParseJSONBody(request)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := body["ttl"].(json.Number); !ok {
		t.Fatalf("expected json.Number, got %T", body["ttl"])
	}
}

func TestMustIntRejectsDecimalJSONNumber(t *testing.T) {
	value, ok := MustInt(map[string]any{"ttl": json.Number("1.5")}, "ttl")

	if ok {
		t.Fatalf("expected decimal json number to be rejected, got %d", value)
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "", expected: ""},
		{input: "note", expected: "note"},
		{input: "/note", expected: "note"},
		{input: "note/", expected: "note"},
		{input: "/note/", expected: "note"},
		{input: "///", expected: "/"},
		{input: "/", expected: "/"},
		{input: "topic//entry", expected: "topic//entry"},
	}

	for _, test := range tests {
		if got := NormalizePath(test.input); got != test.expected {
			t.Fatalf("expected normalized path %q for %q, got %q", test.expected, test.input, got)
		}
	}
}

func TestValidatePathAllowsExpectedCharacters(t *testing.T) {
	validPaths := []string{
		"abc",
		"a-b_c.d",
		"dir/file",
		"a(b)c",
		"topic/2026/post-1",
	}

	for _, path := range validPaths {
		if err := ValidatePath(path); err != nil {
			t.Fatalf("expected path %q to be valid, got %v", path, err)
		}
	}
}

func TestValidatePathRejectsInvalidCharactersAndBounds(t *testing.T) {
	invalidPaths := []string{
		"",
		strings.Repeat("a", 100),
		"has space",
		"中文",
		"bad?",
		"bad#",
		"bad&",
		"bad:",
		`bad\path`,
	}

	for _, path := range invalidPaths {
		if err := ValidatePath(path); err == nil {
			t.Fatalf("expected path %q to be invalid", path)
		}
	}
}
