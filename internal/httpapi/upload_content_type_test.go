package httpapi

import "testing"

func TestEnsureUTF8CharsetForTextContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        string
	}{
		{
			name:        "empty",
			contentType: "",
			want:        "",
		},
		{
			name:        "text plain",
			contentType: "text/plain",
			want:        "text/plain; charset=utf-8",
		},
		{
			name:        "shell application",
			contentType: "application/x-sh",
			want:        "application/x-sh; charset=utf-8",
		},
		{
			name:        "existing charset",
			contentType: "text/plain; charset=gbk",
			want:        "text/plain; charset=gbk",
		},
		{
			name:        "svg image",
			contentType: "image/svg+xml",
			want:        "image/svg+xml",
		},
		{
			name:        "png image",
			contentType: "image/png",
			want:        "image/png",
		},
		{
			name:        "pdf",
			contentType: "application/pdf",
			want:        "application/pdf",
		},
		{
			name:        "octet stream",
			contentType: "application/octet-stream",
			want:        "application/octet-stream",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ensureUTF8CharsetForTextContentType(test.contentType)
			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}
