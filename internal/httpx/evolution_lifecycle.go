package httpx

import (
	"net/http"
	"strings"

	"github.com/uvwt/nexusdock/internal/recall"
)

func (s *Server) registerEvolutionLifecycleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/recall/lifecycle/query", s.withEvolutionAccess(s.lifecycleQuery))
	mux.HandleFunc("POST /internal/recall/lifecycle/transition", s.withEvolutionAccess(s.lifecycleTransition))
}

func (s *Server) withEvolutionAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		token := strings.TrimSpace(s.cfg.AuthToken)
		s.mu.RUnlock()
		if token == "" {
			writeError(w, http.StatusServiceUnavailable, "EVOLUTION_NOT_CONFIGURED", "Nexus programmatic API token is not configured")
			return
		}
		if !bearerMatches(r.Header.Get("Authorization"), token) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid evolution credentials")
			return
		}
		next(w, r)
	}
}

func (s *Server) lifecycleQuery(w http.ResponseWriter, r *http.Request) {
	var request recall.LifecycleQuery
	if !decodeJSON(w, r, &request) {
		return
	}
	records, err := s.store.QueryLifecycle(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "LIFECYCLE_QUERY_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records, "count": len(records)})
}

func (s *Server) lifecycleTransition(w http.ResponseWriter, r *http.Request) {
	var request recall.LifecycleTransition
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := s.store.TransitionLifecycle(request)
	if err != nil {
		switch err {
		case recall.ErrLifecycleRevisionConflict:
			writeError(w, http.StatusConflict, "LIFECYCLE_REVISION_CONFLICT", err.Error())
			return
		case recall.ErrLifecyclePolicyVersionConflict:
			writeError(w, http.StatusConflict, "LIFECYCLE_POLICY_VERSION_CONFLICT", err.Error())
			return
		case recall.ErrLifecycleOperationConflict:
			writeError(w, http.StatusConflict, "LIFECYCLE_OPERATION_CONFLICT", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "LIFECYCLE_TRANSITION_FAILED", err.Error())
		return
	}
	if !result.Idempotent && s.syncer != nil {
		s.syncer.MarkChanged(r.Context())
	}
	writeJSON(w, http.StatusOK, result)
}
