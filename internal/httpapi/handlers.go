package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"post-go/internal/config"
	"post-go/internal/convert"
	"post-go/internal/core"
	redisx "post-go/internal/redis"
	"post-go/internal/s3"
	"post-go/internal/storage"
	"post-go/internal/utils"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	Cfg core.AppConfig
}

// NewHandler builds handler from env.
func NewHandler() *Handler {
	env := config.Env{}
	cfg := core.LoadConfig(env)
	setDebugEnabled(env.Bool("POST_DEBUG", false))
	return &Handler{Cfg: cfg}
}

// ServeHTTP routes requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger{}
	started := time.Now()
	rec := withRecorder(w)
	logger.Infof("request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	logger.Debugf("headers: ua=%q content-type=%q xff=%q", r.Header.Get("User-Agent"), r.Header.Get("Content-Type"), r.Header.Get("X-Forwarded-For"))
	defer logRequestDone(logger, r, rec, started)
	if r.URL.Path == "/" {
		h.handleRoot(rec, r)
		return
	}
	h.handlePath(rec, r)
}

func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !core.IsAuthenticated(r, h.Cfg.SecretKey) {
			requestLogger{}.Warnf("auth failed: POST / from %s", r.RemoteAddr)
			utils.Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized", nil, nil)
			return
		}
		h.handleCreate(w, r, false)
		return
	case http.MethodPut:
		if !core.IsAuthenticated(r, h.Cfg.SecretKey) {
			requestLogger{}.Warnf("auth failed: PUT / from %s", r.RemoteAddr)
			utils.Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized", nil, nil)
			return
		}
		h.handleCreate(w, r, true)
		return
	case http.MethodDelete:
		if !core.IsAuthenticated(r, h.Cfg.SecretKey) {
			requestLogger{}.Warnf("auth failed: DELETE / from %s", r.RemoteAddr)
			utils.Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized", nil, nil)
			return
		}
		h.handleDelete(w, r)
		return
	case http.MethodGet:
		if core.IsAuthenticated(r, h.Cfg.SecretKey) {
			if h.handleLookupAuthedFromBody(w, r) {
				return
			}
			h.handleList(w, r)
			return
		}
		// unauthenticated GET / -> lookup path "/"
		h.handleLookup(w, r, "/")
		return
	default:
		requestLogger{}.Warnf("method not allowed: %s /", r.Method)
		utils.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil, nil)
		return
	}
}

func (h *Handler) handlePath(w http.ResponseWriter, r *http.Request) {
	pathRaw := strings.TrimPrefix(r.URL.Path, "/")
	if pathRaw == "" {
		requestLogger{}.Warnf("path empty for %s %s", r.Method, r.URL.Path)
		utils.Error(w, http.StatusNotFound, "not_found", "URL not found", nil, nil)
		return
	}
	h.handleLookup(w, r, pathRaw)
}

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
	rdb, err := redisx.GetClient(h.Cfg.RedisURL)
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
	typ, content := storage.ParseStoredValue(stored)
	content = responseContent(typ, content, isExportRequest(r))
	utils.JSON(w, http.StatusOK, ItemResponse{
		SURL:    storage.GetDomain(r) + "/" + path,
		Path:    path,
		Type:    typ,
		Content: content,
	})
}

func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request, path string) {
	ctx := context.Background()
	rdb, err := redisx.GetClient(h.Cfg.RedisURL)
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
	typ, content := storage.ParseStoredValue(stored)
	switch typ {
	case "url":
		utils.Redirect(w, r, content, false)
		return
	case "html":
		utils.HTML(w, http.StatusOK, content, true)
		return
	case "file":
		h.serveFile(w, r, path, content)
		return
	default:
		utils.Text(w, http.StatusOK, content, true)
		return
	}
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	rdb, err := redisx.GetClient(h.Cfg.RedisURL)
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
		typ, content := storage.ParseStoredValue(stored)
		content = responseContent(typ, content, isExport)
		links = append(links, ItemResponse{
			SURL:    domain + "/" + path,
			Path:    path,
			Type:    typ,
			Content: content,
		})
	}
	utils.JSON(w, http.StatusOK, links)
}

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
	rdb, err := redisx.GetClient(h.Cfg.RedisURL)
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
	_, _ = rdb.Del(ctx, key).Result()
	_ = core.ClearFileCache(ctx, rdb, pathVal)

	typ, content := storage.ParseStoredValue(stored)
	if typ == "file" {
		conf := h.Cfg.S3Config()
		if conf.IsConfigured() {
			if client, err := s3.NewClient(conf); err == nil {
				if err := client.DeleteObject(ctx, content); err != nil {
					requestLogger{}.Errorf("s3 delete failed: %s (%v)", content, err)
				}
			}
		}
	}

	isExport := isExportRequest(r)
	utils.JSON(w, http.StatusOK, DeleteResponse{
		Deleted: pathVal,
		Type:    typ,
		Content: responseContent(typ, content, isExport),
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
	ttlVal := r.FormValue("ttl")

	if r.Method == http.MethodPut && pathVal == "" {
		requestLogger{}.Warnf("file upload PUT missing path")
		utils.Error(w, http.StatusBadRequest, "invalid_request", "`path` is required for PUT requests", nil, nil)
		return
	}

	fileExt := strings.ToLower(pathpkgExt(header.Filename))
	if pathVal != "" {
		if err := storage.ValidatePath(pathVal); err != nil {
			requestLogger{}.Warnf("invalid path: %s (%v)", pathVal, err)
			utils.Error(w, http.StatusBadRequest, "invalid_request", err.Error(), nil, nil)
			return
		}
		if fileExt != "" && strings.ToLower(pathpkgExt(pathVal)) != fileExt {
			pathVal = pathVal + fileExt
		}
	} else {
		pathVal = randomPath() + fileExt
	}

	ctx := context.Background()
	rdb, err := redisx.GetClient(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	key := storage.LinksPrefix + pathVal
	existing, _ := rdb.Get(ctx, key).Result()
	if existing != "" && !allowOverwrite {
		requestLogger{}.Warnf("conflict on path: %s", pathVal)
		utils.Error(w, http.StatusConflict, "conflict", "path \""+pathVal+"\" already exists", "Use PUT to overwrite", nil)
		return
	}
	if existing != "" && allowOverwrite {
		_ = core.ClearFileCache(ctx, rdb, pathVal)
	}

	var ttlSeconds int64
	if ttlVal != "" {
		if ttlMin, err := parseInt64(ttlVal); err == nil {
			ttlSeconds = ttlMin * 60
		}
	}

	conf := h.Cfg.S3Config()
	client, err := s3.NewClient(conf)
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

	storedValue := storage.BuildStoredValue("file", objectKey)
	var expiresIn any = nil
	if ttlVal != "" {
		ttlMinutes, err := parseInt64(ttlVal)
		if err != nil || ttlMinutes < 1 {
			ttlMinutes = 1
		}
		_ = rdb.SetEx(ctx, key, storedValue, time.Duration(ttlMinutes)*time.Minute).Err()
		expiresIn = ttlMinutes
	} else {
		_ = rdb.Set(ctx, key, storedValue, 0).Err()
	}

	status := http.StatusCreated
	if allowOverwrite && existing != "" {
		status = http.StatusOK
	}
	isExport := isExportRequest(r)
	utils.JSON(w, status, CreateResponse{
		SURL:      storage.GetDomain(r) + "/" + pathVal,
		Path:      pathVal,
		Type:      "file",
		Content:   responseContent("file", objectKey, isExport),
		ExpiresIn: expiresIn,
	})
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

	ctx := context.Background()
	rdb, err := redisx.GetClient(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	key := storage.LinksPrefix + pathVal
	stored := storage.BuildStoredValue(contentType, inputContent)
	existing, _ := rdb.Get(ctx, key).Result()
	isExport := isExportRequest(r)
	if existing != "" && !allowOverwrite {
		exType, exContent := storage.ParseStoredValue(existing)
		details := map[string]any{
			"existing": ItemResponse{
				SURL:    storage.GetDomain(r) + "/" + pathVal,
				Path:    pathVal,
				Type:    exType,
				Content: responseContent(exType, exContent, isExport),
			},
		}
		utils.Error(w, http.StatusConflict, "conflict", "path \""+pathVal+"\" already exists", "Use PUT to overwrite", details)
		return
	}
	if existing != "" && allowOverwrite {
		_ = core.ClearFileCache(ctx, rdb, pathVal)
	}

	var expiresIn any = nil
	var ttlWarning any = nil
	if ttlProvided {
		if ttlMinutes < 1 {
			ttlMinutes = 1
			ttlWarning = "invalid ttl, fallback to 1 minute"
		}
		if err := rdb.SetEx(ctx, key, stored, time.Duration(ttlMinutes)*time.Minute).Err(); err != nil {
			requestLogger{}.Errorf("redis setex failed: %v", err)
		}
		expiresIn = ttlMinutes
	} else {
		if err := rdb.Set(ctx, key, stored, 0).Err(); err != nil {
			requestLogger{}.Errorf("redis set failed: %v", err)
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
		exType, exContent := storage.ParseStoredValue(existing)
		result.Overwritten = responseContent(exType, exContent, isExport)
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

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, pathVal, objectKey string) {
	conf := h.Cfg.S3Config()
	if !conf.IsConfigured() {
		requestLogger{}.Warnf("file fetch requested but S3 not configured")
		utils.Error(w, http.StatusNotImplemented, "s3_not_configured", "S3 service is not configured", nil, nil)
		return
	}
	client, err := s3.NewClient(conf)
	if err != nil {
		requestLogger{}.Errorf("s3 client init failed: %v", err)
		utils.Error(w, http.StatusNotImplemented, "s3_not_configured", "S3 service is not configured", nil, nil)
		return
	}
	ctx := context.Background()
	rdb, err := redisx.GetClient(h.Cfg.RedisURL)
	if err != nil {
		requestLogger{}.Errorf("redis connect failed: %v", err)
		utils.Error(w, http.StatusInternalServerError, "internal", "Internal server error", nil, nil)
		return
	}
	if cached, err := core.GetFileCache(ctx, rdb, pathVal); err == nil && cached != nil {
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
		_ = core.SetFileCache(ctx, rdb, pathVal, &core.FileCacheItem{
			Buffer:        buf.Bytes(),
			ContentType:   info.ContentType,
			ContentLength: info.Size,
		})
		return
	}
	// stream without caching
	w.Header().Set("Content-Type", info.ContentType)
	if info.Size > 0 {
		w.Header().Set("Content-Length", itoa(int(info.Size)))
	}
	w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj)
}

func randomPath() string {
	chars := "23456789abcdefghjkmnpqrstuvwxyz"
	b := make([]byte, 5)
	for i := 0; i < 5; i++ {
		idx, err := randIndex(len(chars))
		if err != nil {
			idx = 0
		}
		b[i] = chars[idx]
	}
	return string(b)
}

func randIndex(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("invalid max")
	}
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return int(b[0]) % max, nil
}

func isURL(s string) bool {
	_, err := url.ParseRequestURI(s)
	return err == nil
}

func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("invalid")
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func isExportRequest(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("x-export")) == "true"
}

func responseContent(typ, content string, isExport bool) string {
	if isExport {
		return content
	}
	return storage.PreviewContent(typ, content)
}

func pathpkgExt(name string) string {
	if name == "" {
		return ""
	}
	return path.Ext(name)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for v > 0 {
		buf = append(buf, byte('0'+v%10))
		v /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// Ensure multipart.File implements io.Reader
var _ io.Reader = (multipart.File)(nil)
