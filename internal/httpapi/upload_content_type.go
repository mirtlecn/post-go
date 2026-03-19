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
		return normalizedDeclaredType
	}

	if inferredFromExtension := inferContentTypeFromExtension(filename); inferredFromExtension != "" {
		return inferredFromExtension
	}
	if inferredFromBody := inferContentTypeFromBody(body); inferredFromBody != "" {
		return inferredFromBody
	}
	return "application/octet-stream"
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
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.TrimSpace(contentType)
	}
	if strings.HasPrefix(mediaType, "text/") {
		if _, hasCharset := params["charset"]; !hasCharset {
			params["charset"] = "utf-8"
		}
	}
	if mediaType == "application/json" {
		if _, hasCharset := params["charset"]; !hasCharset {
			params["charset"] = "utf-8"
		}
	}
	return mime.FormatMediaType(mediaType, params)
}
