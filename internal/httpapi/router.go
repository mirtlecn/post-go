package httpapi

import (
	"net/http"
	"strings"
	"time"

	"post-go/internal/config"
	"post-go/internal/core"
	"post-go/internal/storage"
	"post-go/internal/utils"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	Cfg  core.AppConfig
	deps handlerDependencies
}

// NewHandler builds handler from env.
func NewHandler() *Handler {
	env := config.Env{}
	cfg := core.LoadConfig(env)
	setDebugEnabled(env.Bool("POST_DEBUG", false))
	return &Handler{Cfg: cfg, deps: defaultHandlerDependencies()}
}

// ServeHTTP routes requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger{}
	started := time.Now()
	rec := withRecorder(w)
	logger.Infof("request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	logger.Debugf("headers: ua=%q content-type=%q xff=%q", r.Header.Get("User-Agent"), r.Header.Get("Content-Type"), r.Header.Get("X-Forwarded-For"))
	defer logRequestDone(logger, r, rec, started)
	if h.handleAction(rec, r) {
		return
	}
	if r.URL.Path == "/" {
		h.handleRoot(rec, r)
		return
	}
	h.handlePath(rec, r)
}

func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleLookup(w, r, "/")
		return
	default:
		requestLogger{}.Warnf("method not allowed: %s /", r.Method)
		utils.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil, nil)
		return
	}
}

func (h *Handler) handleAction(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/query":
	case "/create":
	case "/update":
	case "/delete":
	default:
		return false
	}
	if !core.IsAuthenticated(r, h.Cfg.SecretKey) {
		requestLogger{}.Warnf("auth failed: POST %s from %s", r.URL.Path, r.RemoteAddr)
		utils.Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized", nil, nil)
		return true
	}
	switch r.URL.Path {
	case "/query":
		if h.handleLookupAuthedFromBody(w, r) {
			return true
		}
		h.handleList(w, r)
	case "/create":
		h.handleCreate(w, r, false)
	case "/update":
		h.handleCreate(w, r, true)
	case "/delete":
		h.handleDelete(w, r)
	}
	return true
}

func (h *Handler) handlePath(w http.ResponseWriter, r *http.Request) {
	if h.handleEmbeddedAsset(w, r) {
		return
	}
	pathRaw := storage.NormalizePath(strings.TrimPrefix(r.URL.Path, "/"))
	if pathRaw == "" {
		requestLogger{}.Infof("path empty for %s %s", r.Method, r.URL.Path)
		utils.Error(w, http.StatusNotFound, "not_found", "URL not found", nil, nil)
		return
	}
	h.handleLookup(w, r, pathRaw)
}
