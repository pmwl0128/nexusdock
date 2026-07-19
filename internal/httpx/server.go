package httpx

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uvwt/nexusdock/internal/agentdock"
	"github.com/uvwt/nexusdock/internal/auth"
	"github.com/uvwt/nexusdock/internal/config"
	"github.com/uvwt/nexusdock/internal/privatenotes"
	"github.com/uvwt/nexusdock/internal/recall"
	"github.com/uvwt/nexusdock/internal/syncer"
)

const maxJSONRequestBytes = 2 << 20

var requestSequence atomic.Uint64

type requestIDContextKey struct{}

type trackedResponseWriter struct {
	http.ResponseWriter
	requestID   string
	statusCode  int
	wroteHeader bool
}

func (w *trackedResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = statusCode
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *trackedResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *trackedResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type Server struct {
	mu           sync.RWMutex
	cfg          config.Config
	db           *sql.DB
	store        *recall.Store
	privateNotes *privatenotes.Store
	agentDock    *agentdock.Store
	syncer       *syncer.Manager
	logger       *slog.Logger
	auth         *auth.Service
	embedding    *recall.EmbeddingService
}

type ServerOption func(*Server)

func WithSystemDatabase(db *sql.DB) ServerOption {
	return func(server *Server) { server.db = db }
}

func WithAgentDockNodes(store *agentdock.Store) ServerOption {
	return func(server *Server) { server.agentDock = store }
}

func WithWebAuthentication(authService *auth.Service) ServerOption {
	return func(server *Server) { server.auth = authService }
}

func WithEmbeddingService(service *recall.EmbeddingService) ServerOption {
	return func(server *Server) { server.embedding = service }
}

func WithPrivateNotes(store *privatenotes.Store) ServerOption {
	return func(server *Server) { server.privateNotes = store }
}

func NewServer(cfg config.Config, store *recall.Store, syncer *syncer.Manager, logger *slog.Logger, options ...ServerOption) *Server {
	server := &Server{cfg: cfg, store: store, syncer: syncer, logger: logger}
	for _, option := range options {
		option(server)
	}
	return server
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	protected := func(next http.HandlerFunc) http.HandlerFunc { return s.withAPIAccess(next) }
	uiProtected := func(next http.HandlerFunc) http.HandlerFunc { return s.withUIAccess(next) }
	mux.HandleFunc("GET /", uiProtected(s.uiIndex))
	mux.HandleFunc("GET /ui/", uiProtected(s.uiIndex))
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/system/status", protected(s.systemStatus))
	s.registerRuntimeRoutes(mux, protected)
	s.registerWorkflowTemplateRoutes(mux, protected)
	if s.privateNotes != nil {
		s.registerPrivateNoteRoutes(mux, protected)
	}
	s.registerWebAuthRoutes(mux)
	mux.HandleFunc("GET /v1/backup/status", protected(s.getBackupStatus))
	mux.HandleFunc("GET /v1/sync/status", protected(s.syncStatus))
	mux.HandleFunc("GET /v1/git/diff", protected(s.gitDiff))
	mux.HandleFunc("POST /v1/git/discard", protected(s.gitDiscard))
	mux.HandleFunc("GET /v1/git/log", protected(s.gitLog))
	mux.HandleFunc("GET /v1/git/commit", protected(s.gitCommit))
	mux.HandleFunc("POST /v1/sync/pull", protected(s.syncPull))
	mux.HandleFunc("POST /v1/sync/push", protected(s.syncPush))
	mux.HandleFunc("POST /v1/sync/now", protected(s.syncNow))
	mux.HandleFunc("GET /v1/recall", protected(s.listMemories))
	mux.HandleFunc("POST /v1/recall", protected(s.writeRecall))
	mux.HandleFunc("POST /v1/recall/move", protected(s.moveRecall))
	mux.HandleFunc("POST /v1/recall/search", protected(s.searchMemories))
	mux.HandleFunc("POST /v1/recall/pack", protected(s.packMemories))
	mux.HandleFunc("GET /v1/recall/cards", protected(s.listCards))
	mux.HandleFunc("POST /v1/recall/cards", protected(s.writeCard))
	mux.HandleFunc("POST /v1/recall/cards/capture", protected(s.captureCard))
	mux.HandleFunc("POST /v1/recall/cards/search", protected(s.searchCards))
	mux.HandleFunc("GET /v1/embeddings/status", protected(s.embeddingStatus))
	mux.HandleFunc("POST /v1/embeddings/reindex", protected(s.reindexEmbeddings))
	mux.HandleFunc("POST /v1/embeddings/search", protected(s.searchEmbeddings))
	mux.HandleFunc("POST /v1/recall/notes/append", protected(s.appendNote))
	mux.HandleFunc("GET /v1/recall/{path...}", protected(s.readRecall))
	mux.HandleFunc("PATCH /v1/recall/{path...}", protected(s.patchRecall))
	mux.HandleFunc("DELETE /v1/recall/{path...}", protected(s.deleteRecall))
	mux.HandleFunc("GET /v1/", http.NotFound)
	mux.HandleFunc("GET /api/", http.NotFound)
	return s.requestBoundary(s.securityHeaders(mux))
}

func (s *Server) requestBoundary(next http.Handler) http.Handler {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		tracked := &trackedResponseWriter{ResponseWriter: w, requestID: requestID}
		tracked.Header().Set("X-Request-ID", requestID)
		started := time.Now()
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("http handler panic", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "panic", recovered, "stack", string(debug.Stack()))
				if !tracked.wroteHeader {
					writeError(tracked, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				}
			}
			statusCode := tracked.statusCode
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
			logger.Debug("http request", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", statusCode, "duration", time.Since(started))
		}()
		next.ServeHTTP(tracked, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err == nil {
		return "req_" + hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("req_%x_%x", time.Now().UnixNano(), requestSequence.Add(1))
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func requestIDFromWriter(w http.ResponseWriter) string {
	for current := w; current != nil; {
		if tracked, ok := current.(*trackedResponseWriter); ok {
			return tracked.requestID
		}
		unwrapper, ok := current.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		current = unwrapper.Unwrap()
	}
	return ""
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; font-src 'self'; form-action 'self'; frame-ancestors 'none'; img-src 'self' data:; object-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'")
		headers.Set("Cross-Origin-Opener-Policy", "same-origin")
		headers.Set("Cross-Origin-Resource-Policy", "same-origin")
		headers.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		headers.Set("Referrer-Policy", "no-referrer")
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/login" || r.URL.Path == "/change-password" {
			headers.Set("Cache-Control", "no-store")
		}
		if r.TLS != nil || s.isTrustedProxy(r) && strings.EqualFold(lastForwardedValue(r.Header.Get("X-Forwarded-Proto")), "https") {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "nexusdock"})
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.syncer.Status(r.Context()))
}

func (s *Server) gitDiff(w http.ResponseWriter, r *http.Request) {
	diff, err := s.syncer.Diff(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, "GIT_DIFF_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (s *Server) gitDiscard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path      string `json:"path"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	status, err := s.syncer.Discard(r.Context(), req.Path, req.Confirmed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "GIT_DISCARD_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) gitLog(w http.ResponseWriter, r *http.Request) {
	log, err := s.syncer.Log(r.Context(), queryInt(r, "limit", 50))
	if err != nil {
		writeError(w, http.StatusConflict, "GIT_LOG_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, log)
}

func (s *Server) gitCommit(w http.ResponseWriter, r *http.Request) {
	detail, err := s.syncer.CommitDetail(r.Context(), r.URL.Query().Get("hash"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "GIT_COMMIT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
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

func (s *Server) readRecall(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recall": mem})
}

func (s *Server) writeRecall(w http.ResponseWriter, r *http.Request) {
	var req recall.WriteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	mem, err := s.store.Write(req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, recall.ErrFileExists) {
			status = http.StatusConflict
		}
		writeError(w, status, "WRITE_FAILED", err.Error())
		return
	}
	s.syncer.MarkChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recall": mem})
}

func (s *Server) patchRecall(w http.ResponseWriter, r *http.Request) {
	path, err := memoryPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}
	var req recall.WriteRequest
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recall": mem})
}

func (s *Server) moveRecall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromPath  string `json:"from_path"`
		ToPath    string `json:"to_path"`
		Confirmed bool   `json:"confirmed"`
		Overwrite bool   `json:"overwrite"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	mem, err := s.store.Move(req.FromPath, req.ToPath, req.Confirmed, req.Overwrite)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, recall.ErrFileExists) {
			status = http.StatusConflict
		}
		writeError(w, status, "MOVE_FAILED", err.Error())
		return
	}
	s.syncer.MarkChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recall": mem})
}

func (s *Server) deleteRecall(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) listCards(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.List("recall/managed/cards", queryInt(r, "max_entries", 200))
	if err != nil {
		writeError(w, http.StatusBadRequest, "LIST_CARDS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entries": entries, "count": len(entries), "prefix": "recall/managed/cards"})
}

func (s *Server) captureCard(w http.ResponseWriter, r *http.Request) {
	var req recall.CardRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.store.CaptureCard(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CAPTURE_CARD_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) writeCard(w http.ResponseWriter, r *http.Request) {
	var req recall.CardRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.store.WriteCard(req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, recall.ErrFileExists) {
			status = http.StatusConflict
		}
		writeError(w, status, "WRITE_CARD_FAILED", err.Error())
		return
	}
	s.syncer.MarkChanged(r.Context())
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) searchCards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	results, err := s.store.Search(req.Query, "recall/managed/cards", req.MaxResults)
	if err != nil {
		writeError(w, http.StatusBadRequest, "SEARCH_CARDS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "query": req.Query, "results": results, "count": len(results), "prefix": "recall/managed/cards"})
}

func (s *Server) embeddingStatus(w http.ResponseWriter, r *http.Request) {
	if s.embedding == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": false, "configured": false, "reason": "embedding service is not configured"})
		return
	}
	writeJSON(w, http.StatusOK, s.embedding.Status(r.Context()))
}

func (s *Server) reindexEmbeddings(w http.ResponseWriter, r *http.Request) {
	if s.embedding == nil {
		writeError(w, http.StatusServiceUnavailable, "EMBEDDING_DISABLED", "embedding service is not configured")
		return
	}
	var req recall.EmbeddingReindexRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.embedding.Reindex(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EMBEDDING_REINDEX_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) searchEmbeddings(w http.ResponseWriter, r *http.Request) {
	if s.embedding == nil {
		writeError(w, http.StatusServiceUnavailable, "EMBEDDING_DISABLED", "embedding service is not configured")
		return
	}
	var req recall.EmbeddingSearchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.embedding.Search(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EMBEDDING_SEARCH_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) appendNote(w http.ResponseWriter, r *http.Request) {
	var req recall.NoteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	mem, err := s.store.AppendNote(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "APPEND_NOTE_FAILED", err.Error())
		return
	}
	s.syncer.MarkChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recall": mem})
}

func memoryPath(r *http.Request) (string, error) {
	path := r.PathValue("path")
	if strings.TrimSpace(path) == "" {
		return "", errors.New("recall path is required")
	}
	return path, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	body := http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSONDecodeError(w, err)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("request body must contain exactly one JSON value")
		}
		writeJSONDecodeError(w, err)
		return false
	}
	return true
}

func writeJSONDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", fmt.Sprintf("JSON request body exceeds %d bytes", tooLarge.Limit))
		return
	}
	writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	if status >= http.StatusBadRequest {
		if object, ok := value.(map[string]any); ok {
			if requestID := requestIDFromWriter(w); requestID != "" {
				copy := make(map[string]any, len(object)+1)
				for key, item := range object {
					copy[key] = item
				}
				if _, exists := copy["request_id"]; !exists {
					copy["request_id"] = requestID
				}
				value = copy
			}
		}
	}
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
