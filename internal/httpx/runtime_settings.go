package httpx

import (
	"errors"
	"net/http"

	"github.com/uvwt/nexusdock/internal/config"
	"github.com/uvwt/nexusdock/internal/recall"
	"github.com/uvwt/nexusdock/internal/settings"
)

func (s *Server) getRuntimeAISettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "运行时 AI 设置存储不可用")
		return
	}
	_, view, err := s.settings.Load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_READ_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": view})
}

func (s *Server) updateRuntimeAISettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "运行时 AI 设置存储不可用")
		return
	}
	var request settings.UpdateInput
	if !decodeJSON(w, r, &request) {
		return
	}
	cfg, view, err := s.settings.Update(r.Context(), request)
	if err != nil {
		var validation settings.ValidationError
		if errors.As(err, &validation) {
			writeError(w, http.StatusBadRequest, "INVALID_RUNTIME_SETTINGS", validation.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "SETTINGS_UPDATE_FAILED", err.Error())
		return
	}
	s.applyRuntimeAIConfig(cfg)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": view})
}

func (s *Server) currentConfig() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.aiCfgSet {
		return s.cfg
	}
	return s.aiCfg
}

func (s *Server) currentEmbedding() *recall.EmbeddingService {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.embedding
}

func (s *Server) applyRuntimeAIConfig(cfg config.Config) {
	embedding := recall.NewEmbeddingService(s.store, recall.EmbeddingConfig{
		Enabled: cfg.EmbeddingEnabled, Endpoint: cfg.EmbeddingEndpoint, Model: cfg.EmbeddingModel, APIKey: cfg.EmbeddingAPIKey,
		IndexPath: cfg.EmbeddingIndexFile, Timeout: cfg.EmbeddingTimeout,
	})

	s.mu.Lock()
	s.aiCfg = cfg
	s.aiCfgSet = true
	s.embedding = embedding
	s.mu.Unlock()

	s.notifyStage3ConfigChanged()
}

func (s *Server) notifyStage3ConfigChanged() {
	if s.stage3Wake == nil {
		return
	}
	select {
	case s.stage3Wake <- struct{}{}:
	default:
	}
}
