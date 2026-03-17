package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"post-go/internal/storage"
	"post-go/internal/utils"
)

func (h *Handler) handleLookupAuthedFromBody(w http.ResponseWriter, r *http.Request) bool {
	body, err := storage.ParseJSONBody(r)
	if err != nil {
		requestLogger{}.Warnf("lookup parse json failed: %v", err)
		utils.Error(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", nil, nil)
		return true
	}
	typeInfo, err := normalizeTypeAlias(body)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
		return true
	}
	pathVal, hasPath := storage.MustString(body, "path")
	if typeInfo.InputType == topicType && !hasPath {
		h.handleTopicListAuthed(w, r)
		return true
	}
	if !hasPath {
		return false
	}
	if pathVal == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required", nil, nil)
		return true
	}
	pathVal = storage.NormalizePath(pathVal)
	if typeInfo.InputType == topicType {
		h.handleTopicLookupAuthed(w, r, pathVal)
		return true
	}
	h.handleLookupAuthed(w, r, pathVal)
	return true
}

func (h *Handler) handleTopicLookupAuthed(w http.ResponseWriter, r *http.Request, topicName string) {
	topicName = storage.NormalizePath(topicName)
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	exists, err := h.topicExists(ctx, rdb, topicName)
	if err != nil {
		requestLogger{}.Errorf("topic lookup failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if !exists {
		utils.Error(w, http.StatusNotFound, "not_found", "URL not found", nil, nil)
		return
	}
	storedValue, err := h.getTopicStoredValue(ctx, rdb, topicName)
	if err != nil {
		requestLogger{}.Errorf("topic get failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	count, err := countTopicItems(ctx, rdb, topicName)
	if err != nil {
		requestLogger{}.Errorf("topic count failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	utils.JSON(w, http.StatusOK, ItemResponse{
		SURL:    storage.GetDomain(r) + "/" + topicName,
		Path:    topicName,
		Type:    topicType,
		Title:   topicDisplayTitle(topicName, storedValue),
		Content: topicCountString(count),
	})
}

func (h *Handler) handleTopicListAuthed(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	var cursor uint64
	var keys []string
	for {
		foundKeys, nextCursor, err := rdb.Scan(ctx, cursor, "topic:*:items", 100).Result()
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
			return
		}
		keys = append(keys, foundKeys...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	sort.Strings(keys)
	topics := make([]ItemResponse, 0, len(keys))
	domain := storage.GetDomain(r)
	for _, key := range keys {
		topicName := topicNameFromItemsKey(key)
		if topicName == "" {
			continue
		}
		storedValue, err := h.getTopicStoredValue(ctx, rdb, topicName)
		if err != nil {
			requestLogger{}.Warnf("topic list get failed: %s (%v)", topicName, err)
			continue
		}
		if storedValue.Type != topicType {
			continue
		}
		count, err := countTopicItems(ctx, rdb, topicName)
		if err != nil {
			requestLogger{}.Warnf("topic list count failed: %s (%v)", topicName, err)
			continue
		}
		topics = append(topics, ItemResponse{
			SURL:    domain + "/" + topicName,
			Path:    topicName,
			Type:    topicType,
			Title:   topicDisplayTitle(topicName, storedValue),
			Content: topicCountString(count),
		})
	}
	utils.JSON(w, http.StatusOK, topics)
}

func (h *Handler) handleLookupAuthed(w http.ResponseWriter, r *http.Request, path string) {
	path = storage.NormalizePath(path)
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	stored, err := rdb.Get(ctx, storage.LinksPrefix+path).Result()
	if err != nil {
		requestLogger{}.Infof("lookup miss: %s (%v)", path, err)
		utils.Error(w, http.StatusNotFound, "not_found", "URL not found", nil, nil)
		return
	}
	ttlDuration, err := rdb.TTL(ctx, storage.LinksPrefix+path).Result()
	if err != nil {
		requestLogger{}.Warnf("lookup ttl failed: %s (%v)", path, err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	storedValue := storage.ParseStoredValue(stored)
	utils.JSON(w, http.StatusOK, buildItemResponse(storage.GetDomain(r), path, storedValue, ttlMinutesFromDuration(ttlDuration), isExportRequest(r)))
}

func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request, path string) {
	path = storage.NormalizePath(path)
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	stored, err := rdb.Get(ctx, storage.LinksPrefix+path).Result()
	if err != nil {
		requestLogger{}.Infof("lookup miss: %s (%v)", path, err)
		utils.Error(w, http.StatusNotFound, "not_found", "URL not found", nil, nil)
		return
	}
	storedValue := storage.ParseStoredValue(stored)
	switch storedValue.Type {
	case "url":
		utils.Redirect(w, r, storedValue.Content, false)
		return
	case topicType:
		utils.HTML(w, http.StatusOK, storedValue.Content, true)
		return
	case "html":
		utils.HTML(w, http.StatusOK, storedValue.Content, true)
		return
	case "file":
		h.serveFile(w, r, path, storedValue.Content)
		return
	default:
		utils.Text(w, http.StatusOK, storedValue.Content, true)
		return
	}
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	rdb, err := h.deps.getRedisStore(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}

	domain := storage.GetDomain(r)
	isExport := isExportRequest(r)
	var cursor uint64
	var keys []string
	for {
		ks, cur, err := rdb.Scan(ctx, cursor, storage.LinksPrefix+"*", 100).Result()
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
			return
		}
		keys = append(keys, ks...)
		cursor = cur
		if cursor == 0 {
			break
		}
	}
	links := make([]ItemResponse, 0, len(keys))
	for _, key := range keys {
		path := strings.TrimPrefix(key, storage.LinksPrefix)
		stored, err := rdb.Get(ctx, key).Result()
		if err != nil {
			requestLogger{}.Warnf("list get failed: %s (%v)", key, err)
			continue
		}
		ttlSeconds, err := rdb.TTL(ctx, key).Result()
		if err != nil {
			requestLogger{}.Warnf("list ttl failed: %s (%v)", key, err)
			continue
		}
		storedValue := storage.ParseStoredValue(stored)
		content := responseContent(storedValue.Type, storedValue.Content, isExport)
		ttl := ttlMinutesFromDuration(ttlSeconds)
		if storedValue.Type == topicType {
			count, err := countTopicItems(ctx, rdb, path)
			if err != nil {
				requestLogger{}.Warnf("topic count failed: %s (%v)", path, err)
				continue
			}
			content = topicCountString(count)
		}
		item := buildItemResponse(domain, path, storedValue, ttl, isExport)
		if storedValue.Type == topicType {
			item.Content = content
		}
		links = append(links, item)
	}
	utils.JSON(w, http.StatusOK, links)
}
