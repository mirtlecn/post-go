package httpapi

import (
	"net/http"
	"net/url"

	"post-go/internal/assets"
	"post-go/internal/utils"
)

func (h *Handler) handleEmbeddedAsset(w http.ResponseWriter, r *http.Request) bool {
	if !assets.IsReservedEmbeddedAssetPath(r.URL.Path) {
		return false
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		utils.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil, nil)
		return true
	}
	if !isInternalAssetRequest(r) {
		utils.Error(w, http.StatusForbidden, "forbidden", "This path is reserved for internal use", nil, nil)
		return true
	}

	body, contentType, ok := assets.LookupEmbeddedAsset(r.URL.Path)
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", utils.Itoa64(int64(len(body))))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = w.Write(body)
	return true
}

func isInternalAssetRequest(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "same-site":
		return true
	}
	if hasSameOriginHeader(r, "Referer") {
		return true
	}
	return hasSameOriginHeader(r, "Origin")
}

func hasSameOriginHeader(r *http.Request, headerName string) bool {
	raw := r.Header.Get(headerName)
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Host == r.Host
}
