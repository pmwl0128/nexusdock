package devices

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	defaultDegradedAfter            = 90 * time.Second
	defaultOfflineAfter             = 180 * time.Second
	defaultDeviceTokenTTL           = 90 * 24 * time.Hour
	defaultHeartbeatIntervalSeconds = 30
)

type Event struct {
	Type       string
	DeviceID   string
	OldStatus  Status
	NewStatus  Status
	OccurredAt time.Time
	Metadata   map[string]string
}

type EventSink interface {
	PublishDeviceEvent(context.Context, Event) error
}

type nopEventSink struct{}

func (nopEventSink) PublishDeviceEvent(context.Context, Event) error { return nil }

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type Option func(*Service)

func WithClock(clock Clock) Option {
	return func(service *Service) { service.clock = clock }
}

func WithStatusThresholds(degradedAfter, offlineAfter time.Duration) Option {
	return func(service *Service) {
		service.degradedAfter = degradedAfter
		service.offlineAfter = offlineAfter
	}
}

func WithEventSink(sink EventSink) Option {
	return func(service *Service) {
		if sink != nil {
			service.events = sink
		}
	}
}

type Service struct {
	repository    Repository
	clock         Clock
	events        EventSink
	degradedAfter time.Duration
	offlineAfter  time.Duration
}

func NewService(repository Repository, options ...Option) (*Service, error) {
	if repository == nil {
		return nil, domainError(ErrValidation, "repository is required")
	}
	service := &Service{
		repository:    repository,
		clock:         systemClock{},
		events:        nopEventSink{},
		degradedAfter: defaultDegradedAfter,
		offlineAfter:  defaultOfflineAfter,
	}
	for _, option := range options {
		option(service)
	}
	if service.clock == nil {
		return nil, domainError(ErrValidation, "clock is required")
	}
	if service.degradedAfter <= 0 || service.offlineAfter <= service.degradedAfter {
		return nil, domainError(ErrValidation, "status thresholds must satisfy 0 < degraded < offline")
	}
	return service, nil
}

func (s *Service) CreateEnrollmentToken(ctx context.Context, createdBy string, ttl time.Duration, policy Policy) (TokenResult, error) {
	createdBy = strings.TrimSpace(createdBy)
	if createdBy == "" || ttl <= 0 {
		return TokenResult{}, domainError(ErrValidation, "created_by and positive ttl are required")
	}
	if err := policy.Validate(); err != nil {
		return TokenResult{}, err
	}
	plainToken, err := randomSecret(32)
	if err != nil {
		return TokenResult{}, fmt.Errorf("generate enrollment token: %w", err)
	}
	now := s.clock.Now().UTC()
	record := EnrollmentToken{
		ID:        newID(),
		Digest:    digestSecret(plainToken),
		Policy:    clonePolicy(policy),
		CreatedBy: createdBy,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.repository.CreateEnrollmentToken(ctx, record); err != nil {
		return TokenResult{}, err
	}
	return TokenResult{Token: record, PlainToken: plainToken}, nil
}

func (s *Service) Enroll(ctx context.Context, request EnrollmentRequest) (EnrollmentResult, error) {
	request.Token = strings.TrimSpace(request.Token)
	request.Name = strings.TrimSpace(request.Name)
	request.Platform = strings.TrimSpace(request.Platform)
	request.Arch = strings.TrimSpace(request.Arch)
	request.AgentDockVersion = strings.TrimSpace(request.AgentDockVersion)
	request.PublicKey = strings.TrimSpace(request.PublicKey)
	if request.Token == "" || request.Name == "" || request.Platform == "" || request.Arch == "" || request.AgentDockVersion == "" || request.PublicKey == "" {
		return EnrollmentResult{}, domainError(ErrValidation, "enrollment_token, name, platform, arch, agentdock_version, and public_key are required")
	}
	if request.Platform != "darwin" && request.Platform != "linux" {
		return EnrollmentResult{}, domainError(ErrValidation, "unsupported platform %q", request.Platform)
	}
	if request.Arch != "arm64" && request.Arch != "amd64" {
		return EnrollmentResult{}, domainError(ErrValidation, "unsupported arch %q", request.Arch)
	}
	labels, err := normalizeLabels(request.Labels)
	if err != nil {
		return EnrollmentResult{}, err
	}
	deviceToken, err := randomSecret(32)
	if err != nil {
		return EnrollmentResult{}, fmt.Errorf("generate device token: %w", err)
	}
	now := s.clock.Now().UTC()
	device, err := s.repository.CommitEnrollment(ctx, digestSecret(request.Token), now, func(token EnrollmentToken) (Device, error) {
		return Device{
			ID:                   newID(),
			Name:                 request.Name,
			Platform:             request.Platform,
			Arch:                 request.Arch,
			PublicKey:            request.PublicKey,
			Labels:               labels,
			Policy:               clonePolicy(token.Policy),
			Status:               StatusPending,
			AgentDockVersion:     request.AgentDockVersion,
			CreatedAt:            now,
			UpdatedAt:            now,
			Version:              1,
			DeviceTokenDigest:    digestSecret(deviceToken),
			DeviceTokenExpiresAt: now.Add(defaultDeviceTokenTTL),
		}, nil
	})
	if err != nil {
		return EnrollmentResult{}, err
	}
	return EnrollmentResult{
		Device:                   device,
		DeviceToken:              deviceToken,
		TokenExpiresAt:           device.DeviceTokenExpiresAt,
		HeartbeatIntervalSeconds: defaultHeartbeatIntervalSeconds,
		ServerTime:               now,
	}, nil
}

func (s *Service) Approve(ctx context.Context, deviceID string) (Device, error) {
	device, err := s.repository.GetDevice(ctx, deviceID)
	if err != nil {
		return Device{}, err
	}
	if device.RevokedAt != nil {
		return Device{}, domainError(ErrDeviceRevoked, "device %q is revoked", deviceID)
	}
	if device.ApprovedAt != nil {
		return device, nil
	}
	now := s.clock.Now().UTC()
	oldStatus := device.Status
	device.ApprovedAt = ptrTime(now)
	device.Status = StatusOffline
	device.UpdatedAt = now
	updated, err := s.repository.UpdateDevice(ctx, device, device.Version)
	if err != nil {
		return Device{}, err
	}
	_ = s.events.PublishDeviceEvent(ctx, Event{Type: "device.status.changed", DeviceID: device.ID, OldStatus: oldStatus, NewStatus: updated.Status, OccurredAt: now})
	return updated, nil
}

func (s *Service) Revoke(ctx context.Context, deviceID string, reason string) (Device, error) {
	device, err := s.repository.GetDevice(ctx, deviceID)
	if err != nil {
		return Device{}, err
	}
	if device.RevokedAt != nil {
		return device, nil
	}
	now := s.clock.Now().UTC()
	oldStatus := device.Status
	device.RevokedAt = ptrTime(now)
	device.Status = StatusRevoked
	device.UpdatedAt = now
	updated, err := s.repository.UpdateDevice(ctx, device, device.Version)
	if err != nil {
		return Device{}, err
	}
	_ = s.events.PublishDeviceEvent(ctx, Event{Type: "device.status.changed", DeviceID: device.ID, OldStatus: oldStatus, NewStatus: StatusRevoked, OccurredAt: now, Metadata: map[string]string{"reason": strings.TrimSpace(reason)}})
	return updated, nil
}

func (s *Service) UpdateLabels(ctx context.Context, deviceID string, labels map[string]string, expectedVersion int64) (Device, error) {
	device, err := s.repository.GetDevice(ctx, deviceID)
	if err != nil {
		return Device{}, err
	}
	if device.Version != expectedVersion {
		return Device{}, domainError(ErrVersionConflict, "device %q version is %d, expected %d", deviceID, device.Version, expectedVersion)
	}
	normalized, err := normalizeLabels(labels)
	if err != nil {
		return Device{}, err
	}
	device.Labels = normalized
	device.UpdatedAt = s.clock.Now().UTC()
	return s.repository.UpdateDevice(ctx, device, expectedVersion)
}

func (s *Service) UpdatePolicy(ctx context.Context, deviceID string, policy Policy, expectedVersion int64) (Device, error) {
	if err := policy.Validate(); err != nil {
		return Device{}, err
	}
	device, err := s.repository.GetDevice(ctx, deviceID)
	if err != nil {
		return Device{}, err
	}
	if device.Version != expectedVersion {
		return Device{}, domainError(ErrVersionConflict, "device %q version is %d, expected %d", deviceID, device.Version, expectedVersion)
	}
	device.Policy = clonePolicy(policy)
	device.UpdatedAt = s.clock.Now().UTC()
	return s.repository.UpdateDevice(ctx, device, expectedVersion)
}

func (s *Service) Heartbeat(ctx context.Context, deviceID, deviceToken string, heartbeat Heartbeat) (Device, error) {
	device, err := s.Authenticate(ctx, deviceID, deviceToken)
	if err != nil {
		return Device{}, err
	}
	if !device.IsApproved() {
		return Device{}, domainError(ErrDeviceNotApproved, "device %q has not been approved", deviceID)
	}
	if err := validateHeartbeat(heartbeat); err != nil {
		return Device{}, err
	}
	now := s.clock.Now().UTC()
	heartbeat.DeviceID = deviceID
	heartbeat.ReceivedAt = now
	oldStatus := device.Status
	updated, err := s.repository.RecordHeartbeat(ctx, deviceID, device.Version, heartbeat)
	if err != nil {
		return Device{}, err
	}
	if oldStatus != StatusOnline {
		_ = s.events.PublishDeviceEvent(ctx, Event{Type: "device.status.changed", DeviceID: deviceID, OldStatus: oldStatus, NewStatus: StatusOnline, OccurredAt: now})
	}
	return updated, nil
}

func (s *Service) RotateDeviceCredential(ctx context.Context, deviceID, currentCredential string) (DeviceCredential, error) {
	device, err := s.Authenticate(ctx, deviceID, currentCredential)
	if err != nil {
		return DeviceCredential{}, err
	}
	if !device.IsApproved() {
		return DeviceCredential{}, domainError(ErrDeviceNotApproved, "device %q has not been approved", deviceID)
	}
	credential, err := randomSecret(32)
	if err != nil {
		return DeviceCredential{}, fmt.Errorf("generate device credential: %w", err)
	}
	now := s.clock.Now().UTC()
	device.DeviceTokenDigest = digestSecret(credential)
	device.DeviceTokenExpiresAt = now.Add(defaultDeviceTokenTTL)
	device.UpdatedAt = now
	updated, err := s.repository.UpdateDevice(ctx, device, device.Version)
	if err != nil {
		return DeviceCredential{}, err
	}
	return DeviceCredential{DeviceToken: credential, TokenExpiresAt: updated.DeviceTokenExpiresAt}, nil
}

func (s *Service) Authenticate(ctx context.Context, deviceID, deviceToken string) (Device, error) {
	device, err := s.repository.GetDevice(ctx, strings.TrimSpace(deviceID))
	if err != nil {
		return Device{}, err
	}
	if device.RevokedAt != nil {
		return Device{}, domainError(ErrDeviceRevoked, "device %q is revoked", deviceID)
	}
	if !s.clock.Now().UTC().Before(device.DeviceTokenExpiresAt) {
		return Device{}, domainError(ErrDeviceTokenInvalid, "device token expired")
	}
	want, err := hex.DecodeString(device.DeviceTokenDigest)
	if err != nil {
		return Device{}, domainError(ErrDeviceTokenInvalid, "device token is invalid")
	}
	gotSum := sha256.Sum256([]byte(strings.TrimSpace(deviceToken)))
	if subtle.ConstantTimeCompare(want, gotSum[:]) != 1 {
		return Device{}, domainError(ErrDeviceTokenInvalid, "device token is invalid")
	}
	return device, nil
}

// AuthorizeCommand implements the server-side policy check consumed by internal/commands.
func (s *Service) AuthorizeCommand(ctx context.Context, deviceID, commandType string, risk RiskLevel) error {
	device, err := s.repository.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if device.RevokedAt != nil {
		return domainError(ErrDeviceRevoked, "device %q is revoked", deviceID)
	}
	if !device.IsApproved() {
		return domainError(ErrDeviceNotApproved, "device %q has not been approved", deviceID)
	}
	if !device.Policy.AllowsCommand(commandType, risk) {
		return domainError(ErrPolicyDenied, "policy denies %s at risk %s", commandType, risk)
	}
	return nil
}

func (s *Service) RefreshStatuses(ctx context.Context) ([]Device, error) {
	now := s.clock.Now().UTC()
	items, err := s.repository.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	changed := make([]Device, 0)
	for _, device := range items {
		if !device.IsApproved() || device.RevokedAt != nil {
			continue
		}
		status := statusAt(device.LastSeen, now, s.degradedAfter, s.offlineAfter)
		if status == device.Status {
			continue
		}
		oldStatus := device.Status
		device.Status = status
		device.UpdatedAt = now
		updated, updateErr := s.repository.UpdateDevice(ctx, device, device.Version)
		if updateErr != nil {
			return changed, updateErr
		}
		changed = append(changed, updated)
		_ = s.events.PublishDeviceEvent(ctx, Event{Type: "device.status.changed", DeviceID: device.ID, OldStatus: oldStatus, NewStatus: status, OccurredAt: now})
	}
	return changed, nil
}

func (s *Service) Snapshot(ctx context.Context, deviceID string) (Snapshot, error) {
	device, err := s.repository.GetDevice(ctx, deviceID)
	if err != nil {
		return Snapshot{}, err
	}
	heartbeat, ok, err := s.repository.LatestHeartbeat(ctx, deviceID)
	if err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{Device: device}
	if ok {
		result.Heartbeat = &heartbeat
	}
	return result, nil
}

func statusAt(lastSeen *time.Time, now time.Time, degradedAfter, offlineAfter time.Duration) Status {
	if lastSeen == nil {
		return StatusOffline
	}
	age := now.Sub(*lastSeen)
	if age >= offlineAfter {
		return StatusOffline
	}
	if age >= degradedAfter {
		return StatusDegraded
	}
	return StatusOnline
}

func validateHeartbeat(heartbeat Heartbeat) error {
	if strings.TrimSpace(heartbeat.AgentDockVersion) == "" {
		return domainError(ErrValidation, "heartbeat agentdock_version is required")
	}
	if heartbeat.SentAt.IsZero() || heartbeat.UptimeSeconds < 0 {
		return domainError(ErrValidation, "heartbeat sent_at and non-negative uptime_seconds are required")
	}
	if heartbeat.Metrics.CPUPercent < 0 || heartbeat.Metrics.CPUPercent > 100 {
		return domainError(ErrValidation, "cpu_percent must be between 0 and 100")
	}
	if heartbeat.Metrics.MemoryPercent < 0 || heartbeat.Metrics.MemoryPercent > 100 {
		return domainError(ErrValidation, "memory_percent must be between 0 and 100")
	}
	if heartbeat.Metrics.DiskPercent < 0 || heartbeat.Metrics.DiskPercent > 100 {
		return domainError(ErrValidation, "disk_percent must be between 0 and 100")
	}
	return nil
}

func randomSecret(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func newID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("00000000-0000-4000-8000-%012x", time.Now().UnixNano()&0xffffffffffff)
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", buffer[0:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:16])
}

func digestSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
