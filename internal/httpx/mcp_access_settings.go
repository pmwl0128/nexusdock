package httpx

import "net/http"

type mcpAccessTokenResponse struct {
	OK    bool   `json:"ok"`
	Token string `json:"token"`
}

func (s *Server) getMCPAccessToken(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.mcpToken == nil {
		writeError(w, http.StatusServiceUnavailable, "MCP_TOKEN_UNAVAILABLE", "MCP Token 存储不可用")
		return
	}
	writeJSON(w, http.StatusOK, mcpAccessTokenResponse{OK: true, Token: s.mcpToken.Token()})
}

func (s *Server) resetMCPAccessToken(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.mcpToken == nil {
		writeError(w, http.StatusServiceUnavailable, "MCP_TOKEN_UNAVAILABLE", "MCP Token 存储不可用")
		return
	}
	token, err := s.mcpToken.Reset()
	if err != nil {
		if s.logger != nil {
			s.logger.Error("reset MCP access token failed", "error", err)
		}
		writeError(w, http.StatusInternalServerError, "MCP_TOKEN_RESET_FAILED", "unable to reset MCP access token")
		return
	}
	writeJSON(w, http.StatusOK, mcpAccessTokenResponse{OK: true, Token: token})
}
