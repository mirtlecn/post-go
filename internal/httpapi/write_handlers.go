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
	body, err := parseJSONBodyForDelete(r)
	if err != nil {
		writeJSONBodyError(w, err, requestLogger{}, "delete")
		return
	}
	pathVal, ok := storage.MustString(body, "path")
	if !ok || pathVal == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required", nil, nil)
		return
	}
	topicVal, _ := storage.MustString(body, "topic")
	pathVal, topicVal = normalizePathAndTopic(pathVal, topicVal)
	if isReservedAssetPath(pathVal) {
		utils.Error(w, http.StatusBadRequest, "invalid_request", reservedAssetPathError(pathVal).Error(), nil, nil)
		return
	}
	typeInfo, err := normalizeTypeAlias(body)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	result, err := h.deleteItem(ctx, rdb, pathVal, topicVal, typeInfo)
	if err == errDeleteNotFound {
		utils.Error(w, http.StatusNotFound, "not_found", "path \""+pathVal+"\" not found", nil, nil)
		return
	}
	if err != nil {
		switch err.Error() {
		case topicHomeManagedError, "topic does not exist", "`topic` and `path` must match", "`path` is required", "`path` must not be \"/\" when `topic` is provided", "`path` must not contain empty topic members":
			utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		default:
			requestLogger{}.Errorf("delete failed: path=%s topic=%s err=%v", pathVal, topicVal, err)
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		}
		return
	}
	if result.StoredValue.Type == "file" {
		conf := h.Cfg.S3Config()
		if conf.IsConfigured() {
			if client, err := h.deps.newFileStore(conf); err == nil {
				if err := client.DeleteObject(ctx, result.StoredValue.Content); err != nil {
					requestLogger{}.Errorf("s3 delete failed: %s (%v)", result.StoredValue.Content, err)
				}
			}
		}
	}
	writeDeleteResult(w, r, result)
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
	body, err := parseJSONBodyForCreate(r, h.Cfg.MaxContentKB)
	if err != nil {
		writeJSONBodyError(w, err, requestLogger{}, "create")
		return
	}
	pathVal, _ := storage.MustString(body, "path")
	topicVal, _ := storage.MustString(body, "topic")
	pathVal, topicVal = normalizePathAndTopic(pathVal, topicVal)
	titleVal, _ := storage.MustString(body, "title")
	typeInfo, err := normalizeTypeAlias(body)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	createdVal, createdProvided, err := parseCreatedValue(body["created"])
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	ttlMinutes, ttlProvided, err := parseTTLValue(body["ttl"])
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}

	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if typeInfo.InputType == topicType {
		h.handleTopicCreate(w, r, rdb, pathVal, titleVal, createdVal, createdProvided, ttlProvided, allowOverwrite)
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
	if isReservedAssetPath(pathVal) {
		utils.Error(w, http.StatusBadRequest, "invalid_request", reservedAssetPathError(pathVal).Error(), nil, nil)
		return
	}
	if resolvedPath.IsTopicItem && resolvedPath.RelativePath == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required", nil, nil)
		return
	}
	if exists, err := h.topicExists(ctx, rdb, pathVal); err == nil && exists {
		utils.Error(w, http.StatusBadRequest, "invalid_request", topicHomeManagedError, nil, nil)
		return
	}
	if typeInfo.InputType != "" && typeInfo.InputType != "url" && typeInfo.InputType != "text" && typeInfo.InputType != "html" && typeInfo.InputType != "md" && typeInfo.InputType != "md2html" && typeInfo.InputType != "qrcode" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`type` must be one of: url, text, html, md, md2html, qrcode, topic", nil, nil)
		return
	}
	if resolvedPath.ExistingTopic && pathVal == resolvedPath.TopicName {
		utils.Error(w, http.StatusBadRequest, "invalid_request", topicHomeManagedError, nil, nil)
		return
	}
	inputContent, ok := storage.MustString(body, "url")
	if !ok || inputContent == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`url` is required", nil, nil)
		return
	}

	switch typeInfo.InputType {
	case "qrcode":
		_, err := convert.ConvertToQRCode(inputContent)
		if err != nil {
			requestLogger{}.Warnf("qrcode failed: %v", err)
			utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
			return
		}
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
	existing, _ := rdb.Get(ctx, key).Result()
	existingTTL, _ := rdb.TTL(ctx, key).Result()
	if !createdProvided {
		if allowOverwrite && existing != "" {
			createdVal = storage.ParseStoredValue(existing).Created
		} else {
			createdVal = time.Now().UTC().Format(time.RFC3339)
		}
	}
	stored := storage.BuildStoredValue(storage.StoredValue{
		Type:    contentType,
		Content: inputContent,
		Title:   titleVal,
		Created: createdVal,
	})
	isExport := isExportRequest(r)
	if existing != "" && !allowOverwrite {
		existingValue := storage.ParseStoredValue(existing)
		details := map[string]any{
			"existing": buildItemResponse(storage.GetDomain(r), pathVal, existingValue, nil, isExport),
		}
		utils.Error(w, http.StatusConflict, "conflict", "path \""+pathVal+"\" already exists", "Use PUT to overwrite", details)
		return
	}
	if existing != "" && allowOverwrite {
		if err := h.deps.clearFileCache(ctx, rdb, pathVal); err != nil {
			requestLogger{}.Warnf("clear file cache failed: %s (%v)", pathVal, err)
		}
	}

	ttlResponse, err := setStoredValueWithTTL(ctx, rdb, key, stored, ttlMinutes, ttlProvided)
	if err != nil {
		requestLogger{}.Errorf("redis write failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}

	result := CreateResponse{
		SURL:    storage.GetDomain(r) + "/" + pathVal,
		Path:    pathVal,
		Type:    contentType,
		Title:   titleVal,
		Created: responseCreatedValue(createdVal),
		Content: responseContent(contentType, inputContent, isExport),
		TTL:     ttlResponse,
	}
	if existing != "" {
		existingValue := storage.ParseStoredValue(existing)
		result.Overwritten = responseContent(existingValue.Type, existingValue.Content, isExport)
	}

	if resolvedPath.IsTopicItem {
		if err := rdb.ZAdd(ctx, topicItemsKey(resolvedPath.TopicName), redis.Z{
			Score:  float64(time.Now().Unix()),
			Member: resolvedPath.RelativePath,
		}).Err(); err != nil {
			requestLogger{}.Errorf("topic zadd failed: %v", err)
			if existing != "" {
				_ = restoreStoredValueWithTTL(ctx, rdb, key, existing, existingTTL)
			} else {
				_ = rdb.Del(ctx, key).Err()
			}
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
			return
		}
		if err := h.syncTopicIndex(ctx, rdb, resolvedPath.TopicName); err != nil {
			requestLogger{}.Errorf("topic rebuild failed: %v", err)
			_ = rdb.ZRem(ctx, topicItemsKey(resolvedPath.TopicName), resolvedPath.RelativePath).Err()
			if existing != "" {
				_ = restoreStoredValueWithTTL(ctx, rdb, key, existing, existingTTL)
			} else {
				_ = rdb.Del(ctx, key).Err()
			}
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
			return
		}
	}
	status := http.StatusCreated
	if allowOverwrite && existing != "" {
		status = http.StatusOK
	}
	utils.JSON(w, status, result)
}

func (h *Handler) handleTopicCreate(w http.ResponseWriter, r *http.Request, rdb redisStore, topicName, titleVal, createdVal string, createdProvided, ttlProvided bool, allowOverwrite bool) {
	topicName = storage.NormalizePath(topicName)
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
	topicTitle := resolveTopicTitle(topicName, titleVal)
	if !createdProvided {
		createdVal = time.Now().UTC().Format(time.RFC3339)
	}
	if existing != "" {
		existingStoredValue := storage.ParseStoredValue(existing)
		if existingStoredValue.Type == topicType && strings.TrimSpace(titleVal) == "" {
			topicTitle = topicDisplayTitle(topicName, existingStoredValue)
		}
		if allowOverwrite && !createdProvided {
			createdVal = existingStoredValue.Created
		}
	}
	if err := rdb.Set(ctx, storage.LinksPrefix+topicName, storage.BuildStoredValue(storage.StoredValue{
		Type:    topicType,
		Content: "",
		Title:   topicTitle,
		Created: createdVal,
	}), 0).Err(); err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if err := h.syncTopicIndex(ctx, rdb, topicName); err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	count, err := countTopicItems(ctx, rdb, topicName)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	status := http.StatusCreated
	if allowOverwrite {
		status = http.StatusOK
	}
	utils.JSON(w, status, CreateResponse{
		SURL:    storage.GetDomain(r) + "/" + topicName,
		Path:    topicName,
		Type:    topicType,
		Title:   topicTitle,
		Created: responseCreatedValue(createdVal),
		Content: topicCountString(count),
		TTL:     nil,
	})
}

func (h *Handler) handleTopicDelete(w http.ResponseWriter, r *http.Request, rdb redisStore, topicName string) {
	topicName = storage.NormalizePath(topicName)
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
	storedValue, err := h.getTopicStoredValue(ctx, rdb, topicName)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	count, err := countTopicItems(ctx, rdb, topicName)
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
		Title:   topicDisplayTitle(topicName, storedValue),
		Created: responseCreatedValue(storedValue.Created),
		Content: topicCountString(count),
	})
}
