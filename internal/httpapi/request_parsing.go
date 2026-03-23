package httpapi

import (
	"errors"
	"net/http"

	"post-go/internal/storage"
	"post-go/internal/utils"
)

const (
	jsonBodyMarginBytes = int64(12 * 1024)
	jsonLookupMaxBytes  = int64(64 * 1024)
	jsonDeleteMaxBytes  = int64(64 * 1024)
)

func createJSONBodyMaxBytes(maxContentKB int) int64 {
	maxBytes := int64(maxContentKB)*1024 + jsonBodyMarginBytes
	if maxBytes > storage.DefaultJSONBodyMaxBytes {
		return storage.DefaultJSONBodyMaxBytes
	}
	return maxBytes
}

func parseJSONBodyForCreate(r *http.Request, maxContentKB int) (map[string]any, error) {
	return storage.ParseJSONBodyWithLimit(r, createJSONBodyMaxBytes(maxContentKB))
}

func parseJSONBodyForLookup(r *http.Request) (map[string]any, error) {
	return storage.ParseJSONBodyWithLimit(r, jsonLookupMaxBytes)
}

func parseJSONBodyForDelete(r *http.Request) (map[string]any, error) {
	return storage.ParseJSONBodyWithLimit(r, jsonDeleteMaxBytes)
}

func writeJSONBodyError(w http.ResponseWriter, err error, logger requestLogger, action string) {
	var tooLargeErr *storage.RequestBodyTooLargeError
	if errors.As(err, &tooLargeErr) {
		logger.Warnf("%s body too large: max=%d", action, tooLargeErr.MaxBytes)
		utils.Error(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Request body too large", nil, nil)
		return
	}
	logger.Warnf("%s parse json failed: %v", action, err)
	utils.Error(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", nil, nil)
}

func normalizePathAndTopic(pathVal, topicVal string) (string, string) {
	return storage.NormalizePath(pathVal), storage.NormalizePath(topicVal)
}
