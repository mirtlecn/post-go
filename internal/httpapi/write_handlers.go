package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"post-go/internal/convert"
	"post-go/internal/storage"
	"post-go/internal/utils"
)

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	body, err := storage.ParseJSONBody(r)
	if err != nil {
		requestLogger{}.Warnf("delete parse json failed: %v", err)
		utils.Error(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", nil, nil)
		return
	}
	pathVal, ok := storage.MustString(body, "path")
	if !ok || pathVal == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required", nil, nil)
		return
	}
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	key := storage.LinksPrefix + pathVal
	stored, err := rdb.Get(ctx, key).Result()
	if err != nil {
		requestLogger{}.Warnf("delete miss: %s (%v)", pathVal, err)
		utils.Error(w, http.StatusNotFound, "not_found", "path \""+pathVal+"\" not found", nil, nil)
		return
	}
	if err := rdb.Del(ctx, key).Err(); err != nil {
		requestLogger{}.Errorf("redis delete failed: %s (%v)", pathVal, err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if err := h.deps.clearFileCache(ctx, rdb, pathVal); err != nil {
		requestLogger{}.Warnf("clear file cache failed: %s (%v)", pathVal, err)
	}

	storedValue := storage.ParseStoredValue(stored)
	if storedValue.Type == "file" {
		conf := h.Cfg.S3Config()
		if conf.IsConfigured() {
			if client, err := h.deps.newFileStore(conf); err == nil {
				if err := client.DeleteObject(ctx, storedValue.Content); err != nil {
					requestLogger{}.Errorf("s3 delete failed: %s (%v)", storedValue.Content, err)
				}
			}
		}
	}

	isExport := isExportRequest(r)
	utils.JSON(w, http.StatusOK, DeleteResponse{
		Deleted: pathVal,
		Type:    storedValue.Type,
		Content: responseContent(storedValue.Type, storedValue.Content, isExport),
	})
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request, allowOverwrite bool) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if !h.Cfg.S3Config().IsConfigured() {
			requestLogger{}.Warnf("file upload requested but S3 not configured")
			utils.Error(w, http.StatusNotImplemented, "s3_not_configured", "S3 service is not configured", nil, nil)
			return
		}
		h.handleFileUpload(w, r, allowOverwrite)
		return
	}
	h.handleJSONCreate(w, r, allowOverwrite)
}

func (h *Handler) handleJSONCreate(w http.ResponseWriter, r *http.Request, allowOverwrite bool) {
	body, err := storage.ParseJSONBody(r)
	if err != nil {
		requestLogger{}.Warnf("create parse json failed: %v", err)
		utils.Error(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", nil, nil)
		return
	}
	inputContent, ok := storage.MustString(body, "url")
	if !ok || inputContent == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`url` is required", nil, nil)
		return
	}
	pathVal, _ := storage.MustString(body, "path")
	inputType, _ := storage.MustString(body, "type")
	convertVal, _ := storage.MustString(body, "convert")
	titleVal, _ := storage.MustString(body, "title")
	var ttlMinutes int64
	ttlProvided := hasKey(body, "ttl")
	if v, ok := storage.MustInt(body, "ttl"); ok {
		ttlMinutes = v
	}

	if pathVal != "" {
		if err := storage.ValidatePath(pathVal); err != nil {
			requestLogger{}.Warnf("invalid path: %s (%v)", pathVal, err)
			utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
			return
		}
	} else {
		pathVal = randomPath()
	}

	if inputType != "" && inputType != "url" && inputType != "text" && inputType != "html" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`type` must be one of: url, text, html", nil, nil)
		return
	}

	if convertVal != "" {
		switch convertVal {
		case "md2html":
			html, err := convert.ConvertMarkdownToHTML(inputContent)
			if err != nil {
				requestLogger{}.Warnf("md2html failed: %v", err)
				utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
				return
			}
			inputContent = html
			inputType = "html"
		case "qrcode":
			qr, err := convert.ConvertToQRCode(inputContent)
			if err != nil {
				requestLogger{}.Warnf("qrcode failed: %v", err)
				utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
				return
			}
			inputContent = qr
		case "html", "url", "text":
			inputType = convertVal
		default:
			utils.Error(w, http.StatusBadRequest, "invalid_request", "Invalid convert value: "+convertVal+". Must be one of: md2html, qrcode, html, url, text", nil, nil)
			return
		}
	}

	maxBytes := h.Cfg.MaxContentKB * 1024
	if len([]byte(inputContent)) > maxBytes {
		utils.Error(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Content too large (max "+itoa(maxBytes/1024)+"KB)", nil, nil)
		return
	}

	contentType := inputType
	if contentType == "" {
		if isURL(inputContent) {
			contentType = "url"
		} else {
			contentType = "text"
		}
	}
	if contentType == "url" {
		normalizedURLContent, err := normalizeURLContent(inputContent)
		if err != nil {
			utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
			return
		}
		inputContent = normalizedURLContent
	}

	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	key := storage.LinksPrefix + pathVal
	stored := storage.BuildStoredValue(storage.StoredValue{
		Type:    contentType,
		Content: inputContent,
		Title:   titleVal,
	})
	existing, _ := rdb.Get(ctx, key).Result()
	isExport := isExportRequest(r)
	if existing != "" && !allowOverwrite {
		existingValue := storage.ParseStoredValue(existing)
		details := map[string]any{
			"existing": ItemResponse{
				SURL:    storage.GetDomain(r) + "/" + pathVal,
				Path:    pathVal,
				Type:    existingValue.Type,
				Content: responseContent(existingValue.Type, existingValue.Content, isExport),
			},
		}
		utils.Error(w, http.StatusConflict, "conflict", "path \""+pathVal+"\" already exists", "Use PUT to overwrite", details)
		return
	}
	if existing != "" && allowOverwrite {
		if err := h.deps.clearFileCache(ctx, rdb, pathVal); err != nil {
			requestLogger{}.Warnf("clear file cache failed: %s (%v)", pathVal, err)
		}
	}

	var expiresIn any
	var ttlWarning any
	if ttlProvided {
		if ttlMinutes < 1 {
			ttlMinutes = 1
			ttlWarning = "invalid ttl, fallback to 1 minute"
		}
		if err := rdb.SetEx(ctx, key, stored, time.Duration(ttlMinutes)*time.Minute).Err(); err != nil {
			requestLogger{}.Errorf("redis setex failed: %v", err)
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
			return
		}
		expiresIn = ttlMinutes
	} else {
		if err := rdb.Set(ctx, key, stored, 0).Err(); err != nil {
			requestLogger{}.Errorf("redis set failed: %v", err)
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
			return
		}
	}

	result := CreateResponse{
		SURL:      storage.GetDomain(r) + "/" + pathVal,
		Path:      pathVal,
		Type:      contentType,
		Content:   responseContent(contentType, inputContent, isExport),
		ExpiresIn: expiresIn,
	}
	if existing != "" {
		existingValue := storage.ParseStoredValue(existing)
		result.Overwritten = responseContent(existingValue.Type, existingValue.Content, isExport)
	}
	if ttlWarning != nil {
		if s, ok := ttlWarning.(string); ok {
			result.Warning = s
		}
	}

	status := http.StatusCreated
	if allowOverwrite && existing != "" {
		status = http.StatusOK
	}
	utils.JSON(w, status, result)
}
