package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	contracts "github.com/uvwt/agentdock-nexus/generated/nexuscontracts"
	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/devices"
)

func (s *Server) registerControlPlaneRoutes(mux *http.ServeMux) {
	admin := func(next http.HandlerFunc) http.HandlerFunc { return s.withAPIAccess(next) }
	mux.HandleFunc("GET /v1/devices", admin(s.listDevices))
	mux.HandleFunc("GET /v1/devices/{deviceId}", admin(s.getDevice))
	mux.HandleFunc("GET /v1/skills", admin(s.listSkills))
	mux.HandleFunc("GET /api/v1/skills", admin(s.listSkills))
	mux.HandleFunc("GET /api/skills", admin(s.listSkills))
	mux.HandleFunc("GET /v1/runs", admin(s.listRuns))
	mux.HandleFunc("GET /api/v1/runs", admin(s.listRuns))
	mux.HandleFunc("GET /api/runs", admin(s.listRuns))
	mux.HandleFunc("POST /v1/devices/enrollment-tokens", admin(s.createEnrollmentToken))
	mux.HandleFunc("POST /v1/devices/enroll", s.enrollDevice)
	mux.HandleFunc("POST /v1/devices/{deviceId}/approve", admin(s.approveDevice))
	mux.HandleFunc("POST /v1/devices/{deviceId}/revoke", admin(s.revokeDevice))
	mux.HandleFunc("POST /v1/devices/{deviceId}/heartbeat", s.reportDeviceHeartbeat)
	mux.HandleFunc("POST /v1/devices/{deviceId}/token/rotate", s.rotateDeviceToken)
	mux.HandleFunc("POST /v1/devices/{deviceId}/env/actions", admin(s.createDeviceEnvAction))
	mux.HandleFunc("POST /v1/devices/{deviceId}/commands", admin(s.createDeviceCommand))
	mux.HandleFunc("GET /v1/devices/{deviceId}/commands", admin(s.listDeviceCommands))
	mux.HandleFunc("POST /v1/devices/{deviceId}/commands/lease", s.leaseDeviceCommand)
	mux.HandleFunc("GET /v1/commands/{commandId}", admin(s.getCommand))
	mux.HandleFunc("POST /v1/commands/{commandId}/start", s.startCommand)
	mux.HandleFunc("POST /v1/commands/{commandId}/renew", s.renewCommandLease)
	mux.HandleFunc("POST /v1/commands/{commandId}/progress", s.reportCommandProgress)
	mux.HandleFunc("POST /v1/commands/{commandId}/result", s.completeCommand)
}

func (s *Server) createEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var request contracts.EnrollmentTokenCreateRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	policy := devices.DefaultPolicy()
	policy.AllowedCommandTypes = append([]string(nil), request.AllowedCommandTypes...)
	policy.MaxRisk = devices.RiskLevel(request.MaxRisk)
	result, err := s.devices.CreateEnrollmentToken(r.Context(), request.CreatedBy, time.Duration(request.TtlSeconds)*time.Second, policy)
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, contracts.EnrollmentTokenCreateResponse{
		Token:     result.PlainToken,
		ExpiresAt: result.Token.ExpiresAt.Format(time.RFC3339Nano),
	})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	items, err := s.devices.List(r.Context())
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.devices.Snapshot(r.Context(), r.PathValue("deviceId"))
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) listDeviceCommands(w http.ResponseWriter, r *http.Request) {
	items, err := s.commands.ListByDevice(r.Context(), r.PathValue("deviceId"))
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	itemsOut := make([]commands.Command, 0, len(items))
	for _, item := range items {
		itemsOut = append(itemsOut, redactCommandForAdmin(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": itemsOut})
}

func (s *Server) getCommand(w http.ResponseWriter, r *http.Request) {
	command, err := s.commands.Get(r.Context(), r.PathValue("commandId"))
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, redactCommandForAdmin(command))
}

func (s *Server) enrollDevice(w http.ResponseWriter, r *http.Request) {
	var request contracts.DeviceEnrollmentRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	labels := map[string]string{}
	if len(request.Labels) > 0 {
		if err := json.Unmarshal(request.Labels, &labels); err != nil {
			writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", "labels must be a string map")
			return
		}
	}
	result, err := s.devices.Enroll(r.Context(), devices.EnrollmentRequest{
		Token: request.EnrollmentToken, Name: request.Name, Platform: request.Platform,
		Arch: request.Arch, Labels: labels, AgentDockVersion: request.AgentdockVersion, PublicKey: request.PublicKey,
	})
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, contracts.DeviceEnrollmentResponse{
		DeviceId: result.Device.ID, DeviceToken: result.DeviceToken,
		TokenExpiresAt:           result.TokenExpiresAt.Format(time.RFC3339Nano),
		HeartbeatIntervalSeconds: int64(result.HeartbeatIntervalSeconds),
		ServerTime:               result.ServerTime.Format(time.RFC3339Nano),
	})
}

func (s *Server) approveDevice(w http.ResponseWriter, r *http.Request) {
	if _, err := s.devices.Approve(r.Context(), r.PathValue("deviceId")); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	var request contracts.DeviceRevokeRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if _, err := s.devices.Revoke(r.Context(), r.PathValue("deviceId"), request.Reason); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reportDeviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	var request contracts.DeviceHeartbeat
	if !decodeJSON(w, r, &request) {
		return
	}
	deviceID := r.PathValue("deviceId")
	if request.DeviceId != "" && request.DeviceId != deviceID {
		writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", "device_id does not match request path")
		return
	}
	var metrics devices.Metrics
	if err := json.Unmarshal(request.Metrics, &metrics); err != nil {
		writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", "metrics are invalid")
		return
	}
	capabilities := make([]devices.Capability, 0, len(request.Capabilities))
	for _, capability := range request.Capabilities {
		metadata := map[string]string{}
		if len(capability.Metadata) > 0 {
			_ = json.Unmarshal(capability.Metadata, &metadata)
		}
		capabilities = append(capabilities, devices.Capability{Name: capability.Name, Version: capability.Version, Enabled: capability.Enabled, Metadata: metadata})
	}
	sentAt, err := time.Parse(time.RFC3339, request.SentAt)
	if err != nil {
		writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", "sent_at must be RFC3339")
		return
	}
	heartbeat := devices.Heartbeat{SentAt: sentAt, UptimeSeconds: request.UptimeSeconds, AgentDockVersion: request.AgentdockVersion, Metrics: metrics, Capabilities: capabilities}
	if len(request.SkillSummary) > 0 {
		_ = json.Unmarshal(request.SkillSummary, &heartbeat.Skills)
	}
	if len(request.MemorySyncSummary) > 0 {
		_ = json.Unmarshal(request.MemorySyncSummary, &heartbeat.MemorySync)
	}
	if _, err := s.devices.Heartbeat(r.Context(), deviceID, bearerToken(r), heartbeat); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rotateDeviceToken(w http.ResponseWriter, r *http.Request) {
	result, err := s.devices.RotateDeviceCredential(r.Context(), r.PathValue("deviceId"), bearerToken(r))
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contracts.DeviceTokenRotationResponse{DeviceToken: result.DeviceToken, TokenExpiresAt: result.TokenExpiresAt.Format(time.RFC3339Nano)})
}

type envActionRequest struct {
	Action    string `json:"action"`
	Skill     string `json:"skill,omitempty"`
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Value     string `json:"value,omitempty"`
	Operation string `json:"operation,omitempty"`
	EnvFile   string `json:"env_file,omitempty"`
}

func (s *Server) createDeviceEnvAction(w http.ResponseWriter, r *http.Request) {
	var request envActionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	payload, risk, err := buildEnvActionPayload(request)
	if err != nil {
		writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	now := time.Now().UTC()
	command, created, err := s.commands.Enqueue(r.Context(), commands.EnqueueRequest{
		DeviceID: r.PathValue("deviceId"), Type: commands.TypeEnvManage, Risk: risk,
		Payload: payload, IdempotencyKey: fmt.Sprintf("env-%d", now.UnixNano()), Priority: 0,
		MaxAttempts: 1, NotBefore: now.Add(-time.Second), ExpiresAt: now.Add(5 * time.Minute), CreatedBy: "nexus-env-ui",
	})
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, contractCommand(command))
}

func buildEnvActionPayload(request envActionRequest) (json.RawMessage, devices.RiskLevel, error) {
	action := strings.TrimSpace(request.Action)
	if action == "" {
		return nil, "", errors.New("action is required")
	}
	allowed := map[string]devices.RiskLevel{
		"list":                       devices.RiskLow,
		"inspect":                    devices.RiskLow,
		"verify":                     devices.RiskLow,
		"set":                        devices.RiskMedium,
		"delete":                     devices.RiskMedium,
		"migrate-from-agentdock-env": devices.RiskMedium,
	}
	risk, ok := allowed[action]
	if !ok {
		return nil, "", fmt.Errorf("unsupported env action %q", action)
	}
	values := map[string]string{"action": action}
	optional := map[string]string{
		"skill": request.Skill, "name": request.Name, "kind": request.Kind,
		"value": request.Value, "operation": request.Operation, "env_file": request.EnvFile,
	}
	for key, value := range optional {
		if strings.TrimSpace(value) != "" {
			values[key] = value
		}
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return nil, "", err
	}
	return payload, risk, nil
}

func (s *Server) createDeviceCommand(w http.ResponseWriter, r *http.Request) {
	var request contracts.DeviceCommandCreateRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	notBefore, err := time.Parse(time.RFC3339, request.NotBefore)
	if err != nil {
		writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", "not_before must be RFC3339")
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, request.ExpiresAt)
	if err != nil {
		writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", "expires_at must be RFC3339")
		return
	}
	command, created, err := s.commands.Enqueue(r.Context(), commands.EnqueueRequest{
		DeviceID: r.PathValue("deviceId"), Type: commands.Type(request.Type), Risk: devices.RiskLevel(request.Risk),
		Payload: request.Payload, IdempotencyKey: request.IdempotencyKey, Priority: int(request.Priority),
		MaxAttempts: int(request.MaxAttempts), NotBefore: notBefore, ExpiresAt: expiresAt, CreatedBy: "api",
	})
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, contractCommand(command))
}

func (s *Server) leaseDeviceCommand(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	lease, err := s.commands.LeaseNext(r.Context(), deviceID)
	if commands.IsCode(err, commands.ErrCommandNotLeaseable) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contractLeaseForDevice(lease))
}

func (s *Server) startCommand(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("commandId")
	var request contracts.CommandLeaseAction
	if !decodeJSON(w, r, &request) || !s.authorizeCommandRequest(w, r, commandID) {
		return
	}
	if _, err := s.commands.Start(r.Context(), commandID, request.LeaseId); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) renewCommandLease(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("commandId")
	var request contracts.CommandLeaseAction
	if !decodeJSON(w, r, &request) || !s.authorizeCommandRequest(w, r, commandID) {
		return
	}
	lease, err := s.commands.Renew(r.Context(), commandID, request.LeaseId)
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contractLeaseForDevice(lease))
}

func (s *Server) reportCommandProgress(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("commandId")
	var request contracts.CommandProgress
	if !decodeJSON(w, r, &request) || !s.authorizeCommandRequest(w, r, commandID) {
		return
	}
	if request.CommandId != "" && request.CommandId != commandID {
		writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", "command_id does not match request path")
		return
	}
	percent := 0
	if request.Percent != nil {
		percent = int(*request.Percent)
	}
	message := ""
	if request.Message != nil {
		message = *request.Message
	}
	if _, err := s.commands.ReportProgress(r.Context(), commandID, request.LeaseId, commands.Progress{Percent: percent, Message: message}); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) completeCommand(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("commandId")
	var request contracts.CommandResult
	if !decodeJSON(w, r, &request) || !s.authorizeCommandRequest(w, r, commandID) {
		return
	}
	if request.CommandId != "" && request.CommandId != commandID {
		writeNexusError(w, http.StatusBadRequest, "VALIDATION_ERROR", "command_id does not match request path")
		return
	}
	result := commands.Result{Success: request.Status == string(commands.StatusSucceeded), Output: request.Output}
	if request.Error != nil {
		result.ErrorCode = request.Error.Code
		result.Error = request.Error.Message
	}
	if _, err := s.commands.Complete(r.Context(), commandID, request.LeaseId, result); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authorizeCommandRequest(w http.ResponseWriter, r *http.Request, commandID string) bool {
	command, err := s.commands.Get(r.Context(), commandID)
	if err != nil {
		writeControlPlaneError(w, err)
		return false
	}
	if _, err := s.devices.Authenticate(r.Context(), command.DeviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return false
	}
	return true
}

func contractCommand(command commands.Command) contracts.DeviceCommand {
	return contracts.DeviceCommand{
		Id: command.ID, DeviceId: command.DeviceID, Type: string(command.Type), Status: string(command.Status),
		Payload: redactCommandPayload(command.Type, command.Payload), Risk: string(command.Risk), IdempotencyKey: command.IdempotencyKey,
		CreatedAt: command.CreatedAt.Format(time.RFC3339Nano), ExpiresAt: command.ExpiresAt.Format(time.RFC3339Nano),
		Attempt: int64(command.Attempts), MaxAttempts: int64(command.MaxAttempts),
	}
}

func redactCommandPayload(commandType commands.Type, payload json.RawMessage) json.RawMessage {
	if commandType != commands.TypeEnvManage || len(payload) == 0 {
		return append(json.RawMessage(nil), payload...)
	}
	var values map[string]any
	if err := json.Unmarshal(payload, &values); err != nil {
		return append(json.RawMessage(nil), payload...)
	}
	if value, ok := values["value"].(string); ok && value != "" {
		values["value"] = "[REDACTED]"
	}
	redacted, err := json.Marshal(values)
	if err != nil {
		return append(json.RawMessage(nil), payload...)
	}
	return redacted
}

func redactCommandForAdmin(command commands.Command) commands.Command {
	command = commands.Command{
		ID: command.ID, DeviceID: command.DeviceID, Type: command.Type, Risk: command.Risk,
		Payload: redactCommandPayload(command.Type, command.Payload), Status: command.Status,
		IdempotencyKey: command.IdempotencyKey, Priority: command.Priority, MaxAttempts: command.MaxAttempts,
		Attempts: command.Attempts, NotBefore: command.NotBefore, ExpiresAt: command.ExpiresAt,
		LeaseID: command.LeaseID, LeaseExpiresAt: command.LeaseExpiresAt, Progress: command.Progress,
		Result: command.Result, CreatedBy: command.CreatedBy, CreatedAt: command.CreatedAt,
		UpdatedAt: command.UpdatedAt, StartedAt: command.StartedAt, CompletedAt: command.CompletedAt,
		Version: command.Version,
	}
	if command.Progress != nil {
		progress := *command.Progress
		progress.Details = append(json.RawMessage(nil), progress.Details...)
		command.Progress = &progress
	}
	if command.Result != nil {
		result := *command.Result
		result.Output = append(json.RawMessage(nil), result.Output...)
		result.EvidenceID = append([]string(nil), result.EvidenceID...)
		command.Result = &result
	}
	return command
}

func contractLease(lease commands.Lease) contracts.CommandLease {
	return contracts.CommandLease{
		Command: contractCommand(lease.Command), LeaseId: lease.ID,
		LeasedAt: lease.LeasedAt.Format(time.RFC3339Nano), ExpiresAt: lease.ExpiresAt.Format(time.RFC3339Nano),
		RenewAfterSeconds: int64(lease.RenewAfterSeconds),
	}
}

func contractLeaseForDevice(lease commands.Lease) contracts.CommandLease {
	return contracts.CommandLease{
		Command: contractCommandRaw(lease.Command), LeaseId: lease.ID,
		LeasedAt: lease.LeasedAt.Format(time.RFC3339Nano), ExpiresAt: lease.ExpiresAt.Format(time.RFC3339Nano),
		RenewAfterSeconds: int64(lease.RenewAfterSeconds),
	}
}

func contractCommandRaw(command commands.Command) contracts.DeviceCommand {
	return contracts.DeviceCommand{
		Id: command.ID, DeviceId: command.DeviceID, Type: string(command.Type), Status: string(command.Status),
		Payload: command.Payload, Risk: string(command.Risk), IdempotencyKey: command.IdempotencyKey,
		CreatedAt: command.CreatedAt.Format(time.RFC3339Nano), ExpiresAt: command.ExpiresAt.Format(time.RFC3339Nano),
		Attempt: int64(command.Attempts), MaxAttempts: int64(command.MaxAttempts),
	}
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < 7 || !strings.EqualFold(value[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func writeControlPlaneError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
	var deviceError *devices.Error
	var commandError *commands.Error
	switch {
	case errors.As(err, &deviceError):
		code = string(deviceError.Code)
		switch deviceError.Code {
		case devices.ErrDeviceNotFound:
			status = http.StatusNotFound
		case devices.ErrDeviceTokenInvalid, devices.ErrDeviceRevoked:
			status = http.StatusUnauthorized
		case devices.ErrPolicyDenied, devices.ErrDeviceNotApproved:
			status = http.StatusForbidden
		case devices.ErrValidation:
			status = http.StatusBadRequest
		default:
			status = http.StatusConflict
		}
	case errors.As(err, &commandError):
		code = string(commandError.Code)
		switch commandError.Code {
		case commands.ErrCommandNotFound:
			status = http.StatusNotFound
		case commands.ErrValidation, commands.ErrCommandTypeDenied:
			status = http.StatusBadRequest
		default:
			status = http.StatusConflict
		}
	}
	writeNexusError(w, status, code, err.Error())
}

func writeNexusError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, contracts.ErrorResponse{Code: code, Message: message, RequestId: "local"})
}
