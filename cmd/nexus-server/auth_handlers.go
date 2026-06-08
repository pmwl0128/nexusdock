package main

import (
	"net/http"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/api/middleware"
	"github.com/uvwt/agentdock-nexus/internal/core"
)

func (a *app) issueToken(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SubjectType string   `json:"subject_type"`
		SubjectID   string   `json:"subject_id"`
		TokenKind   string   `json:"token_kind"`
		Scopes      []string `json:"scopes"`
		TTLSeconds  int64    `json:"ttl_seconds"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	issued, err := a.auth.IssueToken(
		r.Context(),
		core.Actor{Type: core.ActorType(request.SubjectType), ID: request.SubjectID},
		request.TokenKind,
		request.Scopes,
		time.Duration(request.TTLSeconds)*time.Second,
	)
	if err != nil {
		writeError(w, r, err)
		return
	}
	principal, _ := middleware.PrincipalFromContext(r.Context())
	if err := a.recordAudit(r, principal.Actor, "auth.token.issue", "auth_token", issued.ID, "medium", "", map[string]any{
		"subject_type": request.SubjectType,
		"subject_id":   request.SubjectID,
		"token_kind":   request.TokenKind,
		"scopes":       request.Scopes,
	}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, issued)
}

func (a *app) revokeToken(w http.ResponseWriter, r *http.Request) {
	principal, _ := middleware.PrincipalFromContext(r.Context())
	tokenID := r.PathValue("token_id")
	if err := a.auth.Revoke(r.Context(), tokenID, principal.Actor); err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.recordAudit(r, principal.Actor, "auth.token.revoke", "auth_token", tokenID, "high", "", nil); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token_id": tokenID})
}
