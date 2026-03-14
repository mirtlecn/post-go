package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"post-go/internal/convert"
	"post-go/internal/storage"
	"post-go/internal/utils"

	"github.com/redis/go-redis/v9"
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
	typeInfo, err := normalizeTypeAlias(body)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	topicVal, _ := storage.MustString(body, "topic")
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if typeInfo.InputType == topicType {
		h.handleTopicDelete(w, r, rdb, pathVal)
		return
	}
	resolvedPath, err := h.resolveTopicPath(ctx, rdb, topicVal, pathVal)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	if resolvedPath.IsTopicItem {
		pathVal = resolvedPath.FullPath
	}
	if exists, err := h.topicExists(ctx, rdb, pathVal); err == nil && exists {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "topic home must be managed with `type=topic`", nil, nil)
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
	if resolvedPath.IsTopicItem {
		if err := rdb.ZRem(ctx, topicItemsKey(resolvedPath.TopicName), resolvedPath.RelativePath).Err(); err != nil {
			requestLogger{}.Errorf("topic zrem failed: %v", err)
			return
		}
		if err := h.rebuildTopicIndex(ctx, rdb, resolvedPath.TopicName); err != nil {
			requestLogger{}.Errorf("topic rebuild failed: %v", err)
		}
	}
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
	pathVal, _ := storage.MustString(body, "path")
	topicVal, _ := storage.MustString(body, "topic")
	titleVal, _ := storage.MustString(body, "title")
	typeInfo, err := normalizeTypeAlias(body)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	var ttlMinutes int64
	ttlProvided := hasKey(body, "ttl")
	if v, ok := storage.MustInt(body, "ttl"); ok {
		ttlMinutes = v
	}

	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if typeInfo.InputType == topicType {
		h.handleTopicCreate(w, r, rdb, pathVal, ttlProvided, allowOverwrite)
		return
	}
	if pathVal == "" {
		pathVal = randomPath()
	}
	resolvedPath, err := h.resolveTopicPath(ctx, rdb, topicVal, pathVal)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	if resolvedPath.IsTopicItem {
		pathVal = resolvedPath.FullPath
	}
	if err := storage.ValidatePath(pathVal); err != nil {
		requestLogger{}.Warnf("invalid path: %s (%v)", pathVal, err)
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	if resolvedPath.IsTopicItem && resolvedPath.RelativePath == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required", nil, nil)
		return
	}
	if exists, err := h.topicExists(ctx, rdb, pathVal); err == nil && exists {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "topic home must be managed with `type=topic`", nil, nil)
		return
	}
	if typeInfo.InputType != "" && typeInfo.InputType != "url" && typeInfo.InputType != "text" && typeInfo.InputType != "html" && typeInfo.InputType != "md2html" && typeInfo.InputType != "qrcode" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`type` must be one of: url, text, html, md2html, qrcode, topic", nil, nil)
		return
	}
	if resolvedPath.ExistingTopic && pathVal == resolvedPath.TopicName {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "topic home must be managed with `type=topic`", nil, nil)
		return
	}
	inputContent, ok := storage.MustString(body, "url")
	if !ok || inputContent == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`url` is required", nil, nil)
		return
	}

	switch typeInfo.InputType {
	case "md2html":
		options := convert.MarkdownOptions{}
		if resolvedPath.IsTopicItem {
			options.PageTitle = titleVal
			options.TopicBackLink = "/" + resolvedPath.TopicName
			options.TopicBackLabel = resolvedPath.TopicName
		}
		html, err := convert.ConvertMarkdownToHTMLWithOptions(inputContent, options)
		if err != nil {
			requestLogger{}.Warnf("md2html failed: %v", err)
			utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
			return
		}
		inputContent = html
	case "qrcode":
		qr, err := convert.ConvertToQRCode(inputContent)
		if err != nil {
			requestLogger{}.Warnf("qrcode failed: %v", err)
			utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
			return
		}
		inputContent = qr
	}

	maxBytes := h.Cfg.MaxContentKB * 1024
	if len([]byte(inputContent)) > maxBytes {
		utils.Error(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Content too large (max "+itoa(maxBytes/1024)+"KB)", nil, nil)
		return
	}

	contentType := typeInfo.StoreType
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
	if resolvedPath.IsTopicItem {
		if err := rdb.ZAdd(ctx, topicItemsKey(resolvedPath.TopicName), redis.Z{
			Score:  float64(time.Now().Unix()),
			Member: resolvedPath.RelativePath,
		}).Err(); err != nil {
			requestLogger{}.Errorf("topic zadd failed: %v", err)
			return
		}
		if err := h.rebuildTopicIndex(ctx, rdb, resolvedPath.TopicName); err != nil {
			requestLogger{}.Errorf("topic rebuild failed: %v", err)
		}
	}
}

func (h *Handler) handleTopicCreate(w http.ResponseWriter, r *http.Request, rdb redisStore, topicName string, ttlProvided bool, allowOverwrite bool) {
	if topicName == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required", nil, nil)
		return
	}
	if err := storage.ValidatePath(topicName); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	if ttlProvided {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "topic does not support ttl", nil, nil)
		return
	}
	ctx := context.Background()
	existing, err := rdb.Get(ctx, storage.LinksPrefix+topicName).Result()
	if err != nil && err != redis.Nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if existing != "" && storage.ParseStoredValue(existing).Type != topicType {
		utils.Error(w, http.StatusConflict, "conflict", "path \""+topicName+"\" already exists", nil, nil)
		return
	}
	if existing != "" && !allowOverwrite {
		utils.Error(w, http.StatusConflict, "conflict", "path \""+topicName+"\" already exists", "Use PUT to overwrite", nil)
		return
	}
	if err := h.adoptTopicItems(ctx, rdb, topicName); err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if err := h.rebuildTopicIndex(ctx, rdb, topicName); err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	count, err := rdb.ZCard(ctx, topicItemsKey(topicName)).Result()
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	status := http.StatusCreated
	if allowOverwrite {
		status = http.StatusOK
	}
	utils.JSON(w, status, CreateResponse{
		SURL:      storage.GetDomain(r) + "/" + topicName,
		Path:      topicName,
		Type:      topicType,
		Content:   topicCountString(count),
		ExpiresIn: nil,
	})
}

func (h *Handler) handleTopicDelete(w http.ResponseWriter, r *http.Request, rdb redisStore, topicName string) {
	ctx := context.Background()
	exists, err := h.topicExists(ctx, rdb, topicName)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if !exists {
		utils.Error(w, http.StatusNotFound, "not_found", "path \""+topicName+"\" not found", nil, nil)
		return
	}
	count, err := rdb.ZCard(ctx, topicItemsKey(topicName)).Result()
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if err := rdb.Del(ctx, storage.LinksPrefix+topicName, topicItemsKey(topicName)).Err(); err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	utils.JSON(w, http.StatusOK, DeleteResponse{
		Deleted: topicName,
		Type:    topicType,
		Content: topicCountString(count),
	})
}
