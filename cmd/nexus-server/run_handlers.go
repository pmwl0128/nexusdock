package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/uvwt/agentdock-nexus/internal/api/middleware"
	"github.com/uvwt/agentdock-nexus/internal/runs"
)

func (a *app) createRun(w http.ResponseWriter, r *http.Request) {
	principal, _ := middleware.PrincipalFromContext(r.Context())
	var request struct {
		Kind           string          `json:"kind"`
		DeviceID       string          `json:"device_id"`
		SkillID        string          `json:"skill_id"`
		TaskID         string          `json:"task_id"`
		IdempotencyKey string          `json:"idempotency_key"`
		Input          json.RawMessage `json:"input"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	run, err := a.runs.Create(r.Context(), runs.CreateInput{
		Kind: request.Kind, Actor: principal.Actor, DeviceID: request.DeviceID,
		SkillID: request.SkillID, TaskID: request.TaskID,
		IdempotencyKey: request.IdempotencyKey, Input: request.Input,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.recordAudit(r, principal.Actor, "run.create", "run", run.ID, "low", run.ID, map[string]any{"kind": run.Kind}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (a *app) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := a.runs.Get(r.Context(), r.PathValue("run_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (a *app) appendStep(w http.ResponseWriter, r *http.Request) {
	principal, _ := middleware.PrincipalFromContext(r.Context())
	var request runs.Step
	if !decodeJSON(w, r, &request) {
		return
	}
	request.RunID = r.PathValue("run_id")
	step, err := a.runs.AppendStep(r.Context(), request)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.recordAudit(r, principal.Actor, "run.step.append", "run_step", step.ID, "low", step.RunID, map[string]any{"sequence": step.Sequence, "name": step.Name}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

func (a *app) addEvidence(w http.ResponseWriter, r *http.Request) {
	principal, _ := middleware.PrincipalFromContext(r.Context())
	var request runs.Evidence
	if !decodeJSON(w, r, &request) {
		return
	}
	request.RunID = r.PathValue("run_id")
	evidence, err := a.runs.AddEvidence(r.Context(), request)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.recordAudit(r, principal.Actor, "run.evidence.add", "run_evidence", evidence.ID, "low", evidence.RunID, map[string]any{"kind": evidence.Kind}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, evidence)
}

func (a *app) addVerification(w http.ResponseWriter, r *http.Request) {
	principal, _ := middleware.PrincipalFromContext(r.Context())
	var request runs.Verification
	if !decodeJSON(w, r, &request) {
		return
	}
	request.RunID = r.PathValue("run_id")
	verification, err := a.runs.AddVerification(r.Context(), request)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.recordAudit(r, principal.Actor, "run.verification.add", "run_verification", verification.ID, "low", verification.RunID, map[string]any{"status": verification.Status}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, verification)
}

func (a *app) completeRun(w http.ResponseWriter, r *http.Request) {
	principal, _ := middleware.PrincipalFromContext(r.Context())
	var request struct {
		Status       runs.Status     `json:"status"`
		Output       json.RawMessage `json:"output"`
		ErrorCode    string          `json:"error_code"`
		ErrorMessage string          `json:"error_message"`
		Version      int64           `json:"version"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	run, err := a.runs.Complete(r.Context(), r.PathValue("run_id"), runs.CompleteInput{
		Status: request.Status, Output: request.Output,
		ErrorCode: request.ErrorCode, ErrorMessage: request.ErrorMessage, Version: request.Version,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.recordAudit(r, principal.Actor, "run.complete", "run", run.ID, "low", run.ID, map[string]any{"status": run.Status}); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
