package httpapi

import (
	"context"
	"math"
	"net/http"
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
	pathVal, hasPath := storage.MustString(body, "path")
	if !hasPath {
		return false
	}
	if pathVal == "" {
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required", nil, nil)
		return true
	}
	h.handleLookupAuthed(w, r, pathVal)
	return true
}

func (h *Handler) handleLookupAuthed(w http.ResponseWriter, r *http.Request, path string) {
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
	content := responseContent(storedValue.Type, storedValue.Content, isExportRequest(r))
	utils.JSON(w, http.StatusOK, ItemResponse{
		SURL:    storage.GetDomain(r) + "/" + path,
		Path:    path,
		Type:    storedValue.Type,
		Content: content,
	})
}

func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request, path string) {
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
		var ttl *int64
		if ttlSeconds > 0 {
			ttlMinutes := int64(math.Ceil(ttlSeconds.Minutes()))
			if ttlMinutes < 1 {
				ttlMinutes = 1
			}
			ttl = &ttlMinutes
		}
		links = append(links, ItemResponse{
			SURL:    domain + "/" + path,
			Path:    path,
			Type:    storedValue.Type,
			TTL:     ttl,
			Content: content,
		})
	}
	utils.JSON(w, http.StatusOK, links)
}
