package httpapi

import (
	"mime"
	"net/http"
	"path"
	"strings"
)

func resolveUploadContentType(filename, declaredContentType string, body []byte) string {
	normalizedDeclaredType := strings.TrimSpace(declaredContentType)
	if isUsableDeclaredContentType(normalizedDeclaredType) {
		return ensureUTF8CharsetForTextContentType(normalizedDeclaredType)
	}

	if inferredFromExtension := inferContentTypeFromExtension(filename); inferredFromExtension != "" {
		return ensureUTF8CharsetForTextContentType(inferredFromExtension)
	}
	if inferredFromBody := inferContentTypeFromBody(body); inferredFromBody != "" {
		return ensureUTF8CharsetForTextContentType(inferredFromBody)
	}
	return ensureUTF8CharsetForTextContentType("application/octet-stream")
}

func isUsableDeclaredContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(contentType))
	} else {
		mediaType = strings.ToLower(mediaType)
	}
	switch mediaType {
	case "", "application/octet-stream", "binary/octet-stream":
		return false
	default:
		return true
	}
}

func inferContentTypeFromExtension(filename string) string {
	extension := strings.ToLower(path.Ext(filename))
	if extension == "" {
		return ""
	}
	contentType := mime.TypeByExtension(extension)
	if contentType == "" {
		return ""
	}
	return normalizeInferredContentType(contentType)
}

func inferContentTypeFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	contentType := http.DetectContentType(body)
	if contentType == "application/octet-stream" {
		return ""
	}
	return normalizeInferredContentType(contentType)
}

func normalizeInferredContentType(contentType string) string {
	return ensureUTF8CharsetForTextContentType(contentType)
}

func ensureUTF8CharsetForTextContentType(contentType string) string {
	trimmed := strings.TrimSpace(contentType)
	if trimmed == "" {
		return ""
	}
	mediaType, params, err := mime.ParseMediaType(trimmed)
	if err != nil {
		mediaType = fallbackMediaType(contentType)
		if fallbackHasCharsetParameter(contentType) || !shouldAddUTF8Charset(mediaType) {
			return trimmed
		}
		return trimmed + "; charset=utf-8"
	}
	if _, hasCharset := params["charset"]; hasCharset {
		return trimmed
	}
	if shouldAddUTF8Charset(strings.ToLower(mediaType)) {
		return trimmed + "; charset=utf-8"
	}
	return trimmed
}

func shouldAddUTF8Charset(mediaType string) bool {
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json",
		"application/javascript",
		"text/javascript",
		"application/xml",
		"application/xhtml+xml",
		"application/x-sh",
		"application/x-shellscript",
		"text/x-sh",
		"text/x-shellscript":
		return true
	default:
		return false
	}
}

func fallbackMediaType(contentType string) string {
	base, _, _ := strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(base))
}

func fallbackHasCharsetParameter(contentType string) bool {
	_, params, found := strings.Cut(contentType, ";")
	for found {
		param, rest, hasMore := strings.Cut(params, ";")
		name, _, hasValue := strings.Cut(strings.TrimSpace(param), "=")
		if hasValue && strings.EqualFold(strings.TrimSpace(name), "charset") {
			return true
		}
		params = rest
		found = hasMore
	}
	return false
}
