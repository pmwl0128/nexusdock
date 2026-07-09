package httpx

import (
	"net/http"
)

func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	recallRepoDir := s.store.Root()
	status := map[string]any{
		"ok":              true,
		"service":         "nexusdock",
		"database":        "unavailable",
		"schema_version":  0,
		"nexus_data_dir":  s.cfg.NexusDataDir,
		"recall_repo_dir": recallRepoDir,
		// recall_root is kept as a deprecated compatibility alias for older UI bundles.
		"recall_root": recallRepoDir,
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
