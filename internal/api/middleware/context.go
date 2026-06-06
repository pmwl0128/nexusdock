package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/uvwt/memorydock/internal/auth"
	"github.com/uvwt/memorydock/internal/core"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	principalKey contextKey = "principal"
)

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func PrincipalFromContext(ctx context.Context) (auth.Principal, bool) {
	value, ok := ctx.Value(principalKey).(auth.Principal)
	return value, ok
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" || len(id) > 128 {
			raw := make([]byte, 12)
			_, _ = rand.Read(raw)
			id = "req_" + hex.EncodeToString(raw)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func AccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		capture := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(capture, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "status", capture.status,
			"bytes", capture.bytes, "duration_ms", time.Since(started).Milliseconds(), "request_id", RequestIDFromContext(r.Context()))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func Authenticate(service auth.AuthService, requiredScope string, onError func(http.ResponseWriter, error), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			onError(w, core.NewError(core.CodeAuthRequired, "bearer token is required", nil))
			return
		}
		principal, err := service.Authenticate(r.Context(), strings.TrimSpace(header[7:]))
		if err != nil {
			onError(w, err)
			return
		}
		if requiredScope != "" && !principal.HasScope(requiredScope) {
			onError(w, core.NewError(core.CodeForbidden, "missing scope "+requiredScope, nil))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal)))
	})
}
