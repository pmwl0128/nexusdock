package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/uvwt/memorydock/internal/config"
	"github.com/uvwt/memorydock/internal/memory"
	"github.com/uvwt/memorydock/internal/syncer"
)

type Server struct {
	cfg    config.Config
	store  *memory.Store
	syncer *syncer.Manager
	logger *slog.Logger
}

func NewServer(cfg config.Config, store *memory.Store, syncer *syncer.Manager, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, store: store, syncer: syncer, logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/sync/status", s.withAuth(s.syncStatus))
	mux.HandleFunc("POST /v1/sync/pull", s.withAuth(s.syncPull))
	mux.HandleFunc("POST /v1/sync/push", s.withAuth(s.syncPush))
	mux.HandleFunc("POST /v1/sync/now", s.withAuth(s.syncNow))
	mux.HandleFunc("GET /v1/memories", s.withAuth(s.listMemories))
	mux.HandleFunc("POST /v1/memories", s.withAuth(s.writeMemory))
	mux.HandleFunc("POST /v1/memories/search", s.withAuth(s.searchMemories))
	mux.HandleFunc("POST /v1/memories/pack", s.withAuth(s.packMemories))
	mux.HandleFunc("POST /v1/notes/append", s.withAuth(s.appendNote))
	mux.HandleFunc("GET /v1/memories/", s.withAuth(s.readMemory))
	mux.HandleFunc("PATCH /v1/memories/", s.withAuth(s.patchMemory))
	mux.HandleFunc("DELETE /v1/memories/", s.withAuth(s.deleteMemory))
	return logRequests(mux, s.logger)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "memorydock"})
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.syncer.Status(r.Context()))
}

func (s *Server) syncPull(w http.ResponseWriter, r *http.Request) {
	if err := s.syncer.Pull(r.Context()); err != nil {
		writeError(w, http.StatusConflict, "SYNC_PULL_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.syncer.Status(r.Context()))
}

func (s *Server) syncPush(w http.ResponseWriter, r *http.Request) {
	if err := s.syncer.Push(r.Context()); err != nil {
		writeError(w, http.StatusConflict, "SYNC_PUSH_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.syncer.Status(r.Context()))
}

func (s *Server) syncNow(w http.ResponseWriter, r *http.Request) {
	if err := s.syncer.Sync(r.Context()); err != nil {
		writeError(w, http.StatusConflict, "SYNC_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.syncer.Status(r.Context()))
}

func (s *Server) listMemories(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.List(r.URL.Query().Get("prefix"), queryInt(r, "max_entries", 200))
	if err != nil {
		writeError(w, http.StatusBadRequest, "LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entries": entries, "count": len(entries), "root": s.store.Root()})
}

func (s *Server) readMemory(w http.ResponseWriter, r *http.Request) {
	path, err := memoryPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}
	mem, err := s.store.Read(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "READ_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "memory": mem})
}

func (s *Server) writeMemory(w http.ResponseWriter, r *http.Request) {
	var req memory.WriteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	mem, err := s.store.Write(req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, memory.ErrFileExists) {
			status = http.StatusConflict
		}
		writeError(w, status, "WRITE_FAILED", err.Error())
		return
	}
	s.syncer.MarkChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "memory": mem})
}

func (s *Server) patchMemory(w http.ResponseWriter, r *http.Request) {
	path, err := memoryPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}
	var req memory.WriteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Path = path
	req.Overwrite = true
	mem, err := s.store.Write(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PATCH_FAILED", err.Error())
		return
	}
	s.syncer.MarkChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "memory": mem})
}

func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request) {
	path, err := memoryPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}
	confirmed := r.URL.Query().Get("confirmed") == "true" || r.URL.Query().Get("confirmed") == "1"
	if err := s.store.Delete(path, confirmed); err != nil {
		writeError(w, http.StatusBadRequest, "DELETE_FAILED", err.Error())
		return
	}
	s.syncer.MarkChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func (s *Server) searchMemories(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query      string `json:"query"`
		Prefix     string `json:"prefix"`
		MaxResults int    `json:"max_results"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	results, err := s.store.Search(req.Query, req.Prefix, req.MaxResults)
	if err != nil {
		writeError(w, http.StatusBadRequest, "SEARCH_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "query": req.Query, "results": results, "count": len(results)})
}

func (s *Server) packMemories(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Project  string `json:"project"`
		MaxBytes int    `json:"max_bytes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sections, bytes, err := s.store.Pack(req.Project, req.MaxBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PACK_FAILED", err.Error())
		return
	}
	runbookIndex, indexErr := s.store.RunbookIndex(req.Project, 50)
	if indexErr != nil {
		runbookIndex = nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "project": req.Project, "sections": sections, "count": len(sections), "bytes": bytes, "runbook_index": runbookIndex, "runbook_index_count": len(runbookIndex)})
}

func (s *Server) appendNote(w http.ResponseWriter, r *http.Request) {
	var req memory.NoteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	mem, err := s.store.AppendNote(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "APPEND_NOTE_FAILED", err.Error())
		return
	}
	s.syncer.MarkChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "memory": mem})
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthToken == "" {
			next(w, r)
			return
		}
		want := "Bearer " + s.cfg.AuthToken
		if r.Header.Get("Authorization") != want {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
			return
		}
		next(w, r)
	}
}

func memoryPath(r *http.Request) (string, error) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/memories/")
	path, err := url.PathUnescape(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", errors.New("memory path is required")
	}
	return path, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": map[string]any{"code": code, "message": message}})
}

func queryInt(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func logRequests(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("http request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
