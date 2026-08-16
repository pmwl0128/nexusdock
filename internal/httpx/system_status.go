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
	writeJSON(w, http.StatusOK, status)
}
