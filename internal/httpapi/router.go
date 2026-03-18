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
		h.handleLookup(w, r, "/")
		return
	default:
		requestLogger{}.Warnf("method not allowed: %s /", r.Method)
		utils.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil, nil)
		return
	}
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
