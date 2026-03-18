package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"post-go/internal/core"
	"post-go/internal/storage"
	"post-go/internal/utils"

	"github.com/redis/go-redis/v9"
)

func (h *Handler) handleFileUpload(w http.ResponseWriter, r *http.Request, allowOverwrite bool) {
	maxFileBytes := int64(h.Cfg.MaxFileMB) * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxFileBytes+1024*1024)
	if err := r.ParseMultipartForm(maxFileBytes + 1024*1024); err != nil {
		requestLogger{}.Warnf("multipart parse failed: %v", err)
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		requestLogger{}.Warnf("file missing in multipart: %v", err)
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`file` field is required for multipart/form-data", nil, nil)
		return
	}
	defer file.Close()

	pathVal := r.FormValue("path")
	pathVal = storage.NormalizePath(pathVal)
	ttlVal := r.FormValue("ttl")
	titleVal := r.FormValue("title")
	topicVal := r.FormValue("topic")
	createdVal, createdProvided, err := parseCreatedFormValue(r.FormValue("created"))
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}
	var ttlMinutes int64
	ttlMinutes, ttlProvided, err := parseTTLFormValue(ttlVal)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return
	}

	if r.Method == http.MethodPut && pathVal == "" {
		requestLogger{}.Warnf("file upload PUT missing path")
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required for PUT requests", nil, nil)
		return
	}

	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	fileExt := strings.ToLower(pathpkgExt(header.Filename))
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
	if fileExt != "" && strings.ToLower(pathpkgExt(pathVal)) != fileExt {
		pathVal = pathVal + fileExt
		if resolvedPath.IsTopicItem {
			resolvedPath.RelativePath = resolvedPath.RelativePath + fileExt
			resolvedPath.FullPath = resolvedPath.TopicName + "/" + resolvedPath.RelativePath
		}
	}
	if exists, err := h.topicExists(ctx, rdb, pathVal); err == nil && exists {
		utils.Error(w, http.StatusBadRequest, "invalid_request", topicHomeManagedError, nil, nil)
		return
	}
	key := storage.LinksPrefix + pathVal
	existing, _ := rdb.Get(ctx, key).Result()
	if !createdProvided {
		if allowOverwrite && existing != "" {
			createdVal = storage.ParseStoredValue(existing).Created
		} else {
			createdVal = time.Now().UTC().Format(time.RFC3339)
		}
	}
	if existing != "" && storage.ParseStoredValue(existing).Type == topicType {
		utils.Error(w, http.StatusBadRequest, "invalid_request", topicHomeManagedError, nil, nil)
		return
	}
	if existing != "" && !allowOverwrite {
		requestLogger{}.Warnf("conflict on path: %s", pathVal)
		utils.Error(w, http.StatusConflict, "conflict", "path \""+pathVal+"\" already exists", "Use PUT to overwrite", nil)
		return
	}
	if existing != "" && allowOverwrite {
		if err := h.deps.clearFileCache(ctx, rdb, pathVal); err != nil {
			requestLogger{}.Warnf("clear file cache failed: %s (%v)", pathVal, err)
		}
	}

	var ttlSeconds int64
	if ttlProvided && ttlMinutes > 0 {
		ttlSeconds = ttlSecondsFromMinutes(ttlMinutes)
	}

	conf := h.Cfg.S3Config()
	client, err := h.deps.newFileStore(conf)
	if err != nil {
		requestLogger{}.Errorf("s3 client init failed: %v", err)
		utils.Error(w, http.StatusNotImplemented, "s3_not_configured", "S3 service is not configured", nil, nil)
		return
	}

	size := header.Size
	reader := io.Reader(file)
	if size <= 0 {
		buf, err := io.ReadAll(file)
		if err != nil {
			requestLogger{}.Errorf("read upload failed: %v", err)
			utils.Error(w, http.StatusInternalServerError, "internal", "Failed to read upload", nil, nil)
			return
		}
		size = int64(len(buf))
		reader = bytes.NewReader(buf)
	}

	objectKey, err := client.UploadFile(ctx, header.Filename, size, header.Header.Get("Content-Type"), reader, ttlSeconds)
	if err != nil {
		requestLogger{}.Errorf("s3 upload failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Failed to upload file", nil, nil)
		return
	}

	storedValue := storage.BuildStoredValue(storage.StoredValue{
		Type:    "file",
		Content: objectKey,
		Title:   titleVal,
		Created: createdVal,
	})
	existingTTL, _ := rdb.TTL(ctx, key).Result()
	ttlResponse, err := setStoredValueWithTTL(ctx, rdb, key, storedValue, ttlMinutes, ttlProvided)
	if err != nil {
		h.handleUploadPersistenceFailure(ctx, client, objectKey, err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}

	if resolvedPath.IsTopicItem {
		if err := rdb.ZAdd(ctx, topicItemsKey(resolvedPath.TopicName), redis.Z{
			Score:  float64(time.Now().Unix()),
			Member: resolvedPath.RelativePath,
		}).Err(); err != nil {
			_ = rdb.Del(ctx, key).Err()
			if existing != "" {
				_ = restoreStoredValueWithTTL(ctx, rdb, key, existing, existingTTL)
			}
			_ = client.DeleteObject(ctx, objectKey)
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
			return
		}
		if err := h.rebuildTopicIndex(ctx, rdb, resolvedPath.TopicName); err != nil {
			_ = rdb.ZRem(ctx, topicItemsKey(resolvedPath.TopicName), resolvedPath.RelativePath).Err()
			_ = rdb.Del(ctx, key).Err()
			if existing != "" {
				_ = restoreStoredValueWithTTL(ctx, rdb, key, existing, existingTTL)
			}
			_ = client.DeleteObject(ctx, objectKey)
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
			return
		}
	}
	status := http.StatusCreated
	if allowOverwrite && existing != "" {
		status = http.StatusOK
	}
	isExport := isExportRequest(r)
	utils.JSON(w, status, CreateResponse{
		SURL:    storage.GetDomain(r) + "/" + pathVal,
		Path:    pathVal,
		Type:    "file",
		Title:   titleVal,
		Created: responseCreatedValue(createdVal),
		Content: responseContent("file", objectKey, isExport),
		TTL:     ttlResponse,
	})
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, pathVal, objectKey string) {
	conf := h.Cfg.S3Config()
	if !conf.IsConfigured() {
		requestLogger{}.Warnf("file fetch requested but S3 not configured")
		utils.Error(w, http.StatusNotImplemented, "s3_not_configured", "S3 service is not configured", nil, nil)
		return
	}
	client, err := h.deps.newFileStore(conf)
	if err != nil {
		requestLogger{}.Errorf("s3 client init failed: %v", err)
		utils.Error(w, http.StatusNotImplemented, "s3_not_configured", "S3 service is not configured", nil, nil)
		return
	}
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if cached, err := h.deps.getFileCache(ctx, rdb, pathVal); err == nil && cached != nil {
		requestLogger{}.Infof("file cache hit: %s", pathVal)
		utils.Binary(w, http.StatusOK, cached.Buffer, cached.ContentType, cached.ContentLength, true)
		return
	}
	requestLogger{}.Infof("file cache miss: %s", pathVal)

	obj, info, err := client.GetObject(ctx, objectKey)
	if err != nil {
		requestLogger{}.Errorf("s3 get failed: %s (%v)", objectKey, err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Failed to retrieve file", nil, nil)
		return
	}
	defer obj.Close()

	maxBytes := int64(h.Cfg.MaxContentKB) * 1024
	if info.Size > 0 && info.Size <= maxBytes {
		buf := &bytes.Buffer{}
		mw := io.MultiWriter(w, buf)
		w.Header().Set("Content-Type", info.ContentType)
		w.Header().Set("Content-Length", itoa(int(info.Size)))
		w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=86400")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(mw, obj)
		_ = h.deps.setFileCache(ctx, rdb, pathVal, &core.FileCacheItem{
			Buffer:        buf.Bytes(),
			ContentType:   info.ContentType,
			ContentLength: info.Size,
		})
		return
	}

	w.Header().Set("Content-Type", info.ContentType)
	if info.Size > 0 {
		w.Header().Set("Content-Length", itoa(int(info.Size)))
	}
	w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj)
}

func (h *Handler) handleUploadPersistenceFailure(ctx context.Context, fileStore fileObjectStore, objectKey string, writeErr error) {
	requestLogger{}.Errorf("redis write failed after upload: %v", writeErr)
	if err := fileStore.DeleteObject(ctx, objectKey); err != nil {
		requestLogger{}.Errorf("s3 compensation delete failed: %s (%v)", objectKey, err)
	}
}

func pathpkgExt(name string) string {
	if name == "" {
		return ""
	}
	return path.Ext(name)
}
