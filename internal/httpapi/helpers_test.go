package httpapi

import (
	"encoding/json"
	"testing"
)

func TestResponseContentReturnsRawContentForExport(t *testing.T) {
	if got := responseContent("md", "# Hello", true); got != "# Hello" {
		t.Fatalf("expected raw markdown export, got %q", got)
	}
	if got := responseContent("qrcode", "https://example.com", true); got != "https://example.com" {
		t.Fatalf("expected raw qrcode export, got %q", got)
	}
}

func TestResponseContentPreviewsMarkdownAndQRCodeAsRawText(t *testing.T) {
	if got := responseContent("md", "# Hello from Markdown", false); got != "# Hello from Ma..." {
		t.Fatalf("expected markdown preview, got %q", got)
	}
	if got := responseContent("qrcode", "https://example.com/qr", false); got != "https://example..." {
		t.Fatalf("expected qrcode preview, got %q", got)
	}
}

func TestParseTTLValueHandlesSupportedForms(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int64
		provided bool
	}{
		{name: "nil", input: nil, expected: 0, provided: false},
		{name: "int zero", input: 0, expected: 0, provided: true},
		{name: "int64 positive", input: int64(3), expected: 3, provided: true},
		{name: "json number", input: json.Number("12"), expected: 12, provided: true},
		{name: "max value", input: json.Number("525600"), expected: maxTTLMinutes, provided: true},
	}

	for _, test := range tests {
		value, provided, err := parseTTLValue(test.input)
		if err != nil {
			t.Fatalf("%s: expected no error, got %v", test.name, err)
		}
		if value != test.expected || provided != test.provided {
			t.Fatalf("%s: expected (%d,%v), got (%d,%v)", test.name, test.expected, test.provided, value, provided)
		}
	}
}

func TestParseTTLValueRejectsInvalidForms(t *testing.T) {
	invalidValues := []any{
		-1,
		int64(-1),
		json.Number("-1"),
		json.Number("1.5"),
		json.Number("525601"),
		"10",
		true,
		false,
		map[string]any{},
		[]any{},
	}

	for _, input := range invalidValues {
		if _, _, err := parseTTLValue(input); err == nil {
			t.Fatalf("expected ttl input %#v to be rejected", input)
		}
	}
}

func TestParseTTLFormValueHandlesSupportedForms(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		provided bool
	}{
		{name: "empty", input: "", expected: 0, provided: false},
		{name: "zero", input: "0", expected: 0, provided: true},
		{name: "positive", input: "15", expected: 15, provided: true},
		{name: "max value", input: "525600", expected: maxTTLMinutes, provided: true},
	}

	for _, test := range tests {
		value, provided, err := parseTTLFormValue(test.input)
		if err != nil {
			t.Fatalf("%s: expected no error, got %v", test.name, err)
		}
		if value != test.expected || provided != test.provided {
			t.Fatalf("%s: expected (%d,%v), got (%d,%v)", test.name, test.expected, test.provided, value, provided)
		}
	}
}

func TestParseTTLFormValueRejectsInvalidForms(t *testing.T) {
	invalidValues := []string{"-1", "1.5", "abc", "10m", "525601"}

	for _, input := range invalidValues {
		if _, _, err := parseTTLFormValue(input); err == nil {
			t.Fatalf("expected ttl form input %q to be rejected", input)
		}
	}
}

func TestTTLSecondsFromMinutes(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int64
	}{
		{name: "zero", input: 0, expected: 0},
		{name: "positive", input: 3, expected: 180},
		{name: "max", input: maxTTLMinutes, expected: maxTTLSeconds},
	}

	for _, test := range tests {
		if got := ttlSecondsFromMinutes(test.input); got != test.expected {
			t.Fatalf("%s: expected %d, got %d", test.name, test.expected, got)
		}
	}
}

func TestNormalizeTypeAliasSupportsAliasAndMappings(t *testing.T) {
	tests := []struct {
		name      string
		body      map[string]any
		inputType string
		storeType string
	}{
		{
			name:      "convert alias only",
			body:      map[string]any{"convert": "text"},
			inputType: "text",
			storeType: "text",
		},
		{
			name:      "md2html maps to md",
			body:      map[string]any{"type": "md2html"},
			inputType: "md2html",
			storeType: "md",
		},
		{
			name:      "qrcode keeps qrcode type",
			body:      map[string]any{"convert": "qrcode"},
			inputType: "qrcode",
			storeType: "qrcode",
		},
	}

	for _, test := range tests {
		info, err := normalizeTypeAlias(test.body)
		if err != nil {
			t.Fatalf("%s: expected no error, got %v", test.name, err)
		}
		if info.InputType != test.inputType || info.StoreType != test.storeType {
			t.Fatalf("%s: expected (%s,%s), got (%s,%s)", test.name, test.inputType, test.storeType, info.InputType, info.StoreType)
		}
	}
}

func TestNormalizeTypeAliasRejectsMismatchedTypeAndConvert(t *testing.T) {
	_, err := normalizeTypeAlias(map[string]any{
		"type":    "text",
		"convert": "html",
	})

	if err == nil {
		t.Fatalf("expected mismatched type and convert to be rejected")
	}
}

func TestHasEmptyPathSegment(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{path: "", expected: false},
		{path: "anime", expected: false},
		{path: "anime/castle", expected: false},
		{path: "anime//castle", expected: true},
		{path: "/anime", expected: true},
		{path: "anime/", expected: true},
	}

	for _, test := range tests {
		if got := hasEmptyPathSegment(test.path); got != test.expected {
			t.Fatalf("expected %v for %q, got %v", test.expected, test.path, got)
		}
	}
}
