package httpx

import (
	"net/http"
)

func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"ok":             true,
		"service":        "memorydock",
		"database":       "unavailable",
		"schema_version": 0,
		"memory_root":    s.store.Root(),
		"artifact_root":  "",
	}
	if s.artifacts != nil {
		status["artifact_root"] = s.artifacts.Root()
	}
	if s.db == nil {
		status["ok"] = false
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	var check string
	if err := s.db.QueryRowContext(r.Context(), `PRAGMA quick_check`).Scan(&check); err != nil || check != "ok" {
		status["ok"] = false
		status["database"] = check
		if check == "" {
			status["database"] = "error"
		}
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	status["database"] = "ok"
	var version int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err == nil {
		status["schema_version"] = version
	}
	writeJSON(w, http.StatusOK, status)
}
