package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/api/middleware"
	"github.com/uvwt/agentdock-nexus/internal/audit"
	"github.com/uvwt/agentdock-nexus/internal/auth"
	"github.com/uvwt/agentdock-nexus/internal/core"
	"github.com/uvwt/agentdock-nexus/internal/runs"
)

type app struct {
	db         *sql.DB
	migrations *core.MigrationRunner
	auth       *auth.Service
	audit      audit.AuditService
	runs       runs.RunService
	logger     *slog.Logger
}

func newApp(db *sql.DB, migrations *core.MigrationRunner, authService *auth.Service, auditService audit.AuditService, runService runs.RunService, logger *slog.Logger) *app {
	return &app{db: db, migrations: migrations, auth: authService, audit: auditService, runs: runService, logger: logger}
}

func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /ready", a.ready)
	mux.Handle("GET /v1/auth/me", a.require("", http.HandlerFunc(a.me)))
	mux.Handle("POST /v1/auth/tokens", a.require("auth:tokens:write", http.HandlerFunc(a.issueToken)))
	mux.Handle("POST /v1/auth/tokens/{token_id}/revoke", a.require("auth:tokens:write", http.HandlerFunc(a.revokeToken)))
	mux.Handle("GET /v1/audit/events", a.require("audit:read", http.HandlerFunc(a.listAudit)))
	mux.Handle("POST /v1/runs", a.require("runs:write", http.HandlerFunc(a.createRun)))
	mux.Handle("GET /v1/runs/{run_id}", a.require("runs:read", http.HandlerFunc(a.getRun)))
	mux.Handle("POST /v1/runs/{run_id}/steps", a.require("runs:write", http.HandlerFunc(a.appendStep)))
	mux.Handle("POST /v1/runs/{run_id}/evidence", a.require("runs:write", http.HandlerFunc(a.addEvidence)))
	mux.Handle("POST /v1/runs/{run_id}/verifications", a.require("runs:write", http.HandlerFunc(a.addVerification)))
	mux.Handle("POST /v1/runs/{run_id}/complete", a.require("runs:write", http.HandlerFunc(a.completeRun)))
	return middleware.RequestID(middleware.AccessLog(a.logger, mux))
}

func (a *app) require(scope string, next http.Handler) http.Handler {
	return middleware.Authenticate(a.auth, scope, writeError, next)
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "agentdock-nexus"})
}

func (a *app) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		writeErrorStatus(w, r, http.StatusServiceUnavailable, core.CodeInternal, "database unavailable")
		return
	}
	version, err := a.migrations.CurrentVersion(ctx)
	if err != nil || version == 0 {
		writeErrorStatus(w, r, http.StatusServiceUnavailable, core.CodeInternal, "schema unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "schema_version": version})
}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	principal, _ := middleware.PrincipalFromContext(r.Context())
	writeJSON(w, http.StatusOK, principal)
}

func (a *app) listAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := a.audit.List(r.Context(), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": events})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, r, core.NewError(core.CodeValidation, "invalid JSON request body", err))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, r, core.NewError(core.CodeValidation, "request body must contain one JSON document", err))
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := core.ErrorCodeOf(err)
	status := http.StatusInternalServerError
	switch code {
	case core.CodeAuthRequired, core.CodeInvalidToken, core.CodeTokenRevoked:
		status = http.StatusUnauthorized
	case core.CodeForbidden:
		status = http.StatusForbidden
	case core.CodeValidation:
		status = http.StatusBadRequest
	case core.CodeNotFound:
		status = http.StatusNotFound
	case core.CodeVersionConflict, core.CodeDBConflict:
		status = http.StatusConflict
	}
	message := err.Error()
	var coded *core.CodedError
	if errors.As(err, &coded) && coded.Message != "" {
		message = coded.Message
	}
	writeErrorStatus(w, r, status, code, message)
}

func writeErrorStatus(w http.ResponseWriter, r *http.Request, status int, code core.ErrorCode, message string) {
	requestID := middleware.RequestIDFromContext(r.Context())
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
	}
	writeJSON(w, status, map[string]any{"code": code, "message": message, "request_id": requestID})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Fprintln(w, `{"ok":false,"code":"INTERNAL_ERROR","message":"encode response failed"}`)
	}
}

func (a *app) recordAudit(r *http.Request, actor core.Actor, action, objectType, objectID, risk, runID string, metadata map[string]any) error {
	_, err := a.audit.Record(r.Context(), audit.Event{
		Actor: actor, Action: action, ObjectType: objectType, ObjectID: objectID,
		Result: "succeeded", Risk: risk, Approval: "not_required", RunID: runID,
		RequestID: middleware.RequestIDFromContext(r.Context()), Metadata: metadata,
	})
	return err
}
