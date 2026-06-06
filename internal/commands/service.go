package commands

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/uvwt/memorydock/internal/devices"
)

const defaultLeaseDuration = 30 * time.Second

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type Authorizer interface {
	AuthorizeCommand(context.Context, string, string, devices.RiskLevel) error
}

type Event struct {
	Type       string
	CommandID  string
	DeviceID   string
	OldStatus  Status
	NewStatus  Status
	OccurredAt time.Time
}

type EventSink interface {
	PublishCommandEvent(context.Context, Event) error
}

type nopEventSink struct{}

func (nopEventSink) PublishCommandEvent(context.Context, Event) error { return nil }

type Option func(*Service)

func WithClock(clock Clock) Option { return func(service *Service) { service.clock = clock } }

func WithLeaseDuration(duration time.Duration) Option {
	return func(service *Service) { service.leaseDuration = duration }
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
	authorizer    Authorizer
	clock         Clock
	events        EventSink
	leaseDuration time.Duration
}

func (s *Service) Get(ctx context.Context, commandID string) (Command, error) {
	return s.repository.Get(ctx, strings.TrimSpace(commandID))
}

func NewService(repository Repository, authorizer Authorizer, options ...Option) (*Service, error) {
	if repository == nil || authorizer == nil {
		return nil, commandError(ErrValidation, "repository and authorizer are required")
	}
	service := &Service{repository: repository, authorizer: authorizer, clock: systemClock{}, events: nopEventSink{}, leaseDuration: defaultLeaseDuration}
	for _, option := range options {
		option(service)
	}
	if service.clock == nil || service.leaseDuration <= 0 {
		return nil, commandError(ErrValidation, "clock and positive lease duration are required")
	}
	return service, nil
}

func (s *Service) Enqueue(ctx context.Context, request EnqueueRequest) (Command, bool, error) {
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.DeviceID == "" || request.IdempotencyKey == "" || request.CreatedBy == "" {
		return Command{}, false, commandError(ErrValidation, "device_id, idempotency_key, and created_by are required")
	}
	if len(request.IdempotencyKey) < 8 || len(request.IdempotencyKey) > 128 {
		return Command{}, false, commandError(ErrValidation, "idempotency_key must contain 8..128 characters")
	}
	if !request.Type.Valid() {
		return Command{}, false, commandError(ErrCommandTypeDenied, "command type %q is not allowed", request.Type)
	}
	if !json.Valid(request.Payload) {
		return Command{}, false, commandError(ErrValidation, "payload must be valid JSON")
	}
	if request.MaxAttempts <= 0 {
		request.MaxAttempts = 1
	}
	if request.MaxAttempts > 20 || request.Priority < -100 || request.Priority > 100 {
		return Command{}, false, commandError(ErrValidation, "max_attempts or priority is outside allowed range")
	}
	now := s.clock.Now().UTC()
	if request.NotBefore.IsZero() {
		request.NotBefore = now
	}
	if request.ExpiresAt.IsZero() || !request.ExpiresAt.After(request.NotBefore) || !request.ExpiresAt.After(now) {
		return Command{}, false, commandError(ErrValidation, "expires_at must be after now and not_before")
	}
	if err := s.authorizer.AuthorizeCommand(ctx, request.DeviceID, string(request.Type), request.Risk); err != nil {
		return Command{}, false, err
	}
	command := Command{
		ID: newCommandID(), DeviceID: request.DeviceID, Type: request.Type,
		Risk: request.Risk, Payload: append(json.RawMessage(nil), request.Payload...),
		Status: StatusQueued, IdempotencyKey: request.IdempotencyKey,
		Priority: request.Priority, MaxAttempts: request.MaxAttempts,
		NotBefore: request.NotBefore.UTC(), ExpiresAt: request.ExpiresAt.UTC(),
		CreatedBy: request.CreatedBy, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	createdCommand, created, err := s.repository.Enqueue(ctx, command)
	if err != nil {
		return Command{}, false, err
	}
	if created {
		_ = s.events.PublishCommandEvent(ctx, Event{Type: "command.status.changed", CommandID: command.ID, DeviceID: command.DeviceID, NewStatus: StatusQueued, OccurredAt: now})
	}
	return createdCommand, created, nil
}

func (s *Service) LeaseNext(ctx context.Context, deviceID string) (Lease, error) {
	now := s.clock.Now().UTC()
	command, err := s.repository.LeaseNext(ctx, strings.TrimSpace(deviceID), now, s.leaseDuration)
	if err != nil {
		return Lease{}, err
	}
	if err := s.authorizer.AuthorizeCommand(ctx, command.DeviceID, string(command.Type), command.Risk); err != nil {
		// Policy may have changed after enqueue. Cancel rather than expose a forbidden command.
		oldStatus := command.Status
		command.Status = StatusCancelled
		command.LeaseID = ""
		command.LeaseExpiresAt = nil
		command.CompletedAt = ptrTime(now)
		command.UpdatedAt = now
		updated, updateErr := s.repository.Update(ctx, command, command.Version)
		if updateErr == nil {
			_ = s.events.PublishCommandEvent(ctx, Event{Type: "command.status.changed", CommandID: updated.ID, DeviceID: updated.DeviceID, OldStatus: oldStatus, NewStatus: StatusCancelled, OccurredAt: now})
		}
		return Lease{}, err
	}
	_ = s.events.PublishCommandEvent(ctx, Event{Type: "command.status.changed", CommandID: command.ID, DeviceID: command.DeviceID, OldStatus: StatusQueued, NewStatus: StatusLeased, OccurredAt: now})
	return Lease{
		ID: command.LeaseID, Command: command, LeasedAt: now,
		ExpiresAt:         *command.LeaseExpiresAt,
		RenewAfterSeconds: maxInt(1, int(s.leaseDuration.Seconds()/2)),
	}, nil
}

func (s *Service) Renew(ctx context.Context, commandID, leaseID string) (Lease, error) {
	command, err := s.repository.Get(ctx, commandID)
	if err != nil {
		return Lease{}, err
	}
	now := s.clock.Now().UTC()
	if err := validateLease(command, leaseID, now); err != nil {
		return Lease{}, err
	}
	if command.Status != StatusLeased && command.Status != StatusRunning {
		return Lease{}, commandError(ErrInvalidTransition, "cannot renew command in status %s", command.Status)
	}
	command.LeaseExpiresAt = ptrTime(now.Add(s.leaseDuration))
	command.UpdatedAt = now
	updated, err := s.repository.Update(ctx, command, command.Version)
	if err != nil {
		return Lease{}, err
	}
	return Lease{
		ID: updated.LeaseID, Command: updated, LeasedAt: now,
		ExpiresAt:         *updated.LeaseExpiresAt,
		RenewAfterSeconds: maxInt(1, int(s.leaseDuration.Seconds()/2)),
	}, nil
}

func (s *Service) Start(ctx context.Context, commandID, leaseID string) (Command, error) {
	command, err := s.repository.Get(ctx, commandID)
	if err != nil {
		return Command{}, err
	}
	now := s.clock.Now().UTC()
	if err := validateLease(command, leaseID, now); err != nil {
		return Command{}, err
	}
	if command.Status != StatusLeased {
		return Command{}, commandError(ErrInvalidTransition, "cannot start command in status %s", command.Status)
	}
	oldStatus := command.Status
	command.Status = StatusRunning
	command.StartedAt = ptrTime(now)
	command.UpdatedAt = now
	updated, err := s.repository.Update(ctx, command, command.Version)
	if err == nil {
		_ = s.events.PublishCommandEvent(ctx, Event{Type: "command.status.changed", CommandID: updated.ID, DeviceID: updated.DeviceID, OldStatus: oldStatus, NewStatus: StatusRunning, OccurredAt: now})
	}
	return updated, err
}

func (s *Service) ReportProgress(ctx context.Context, commandID, leaseID string, progress Progress) (Command, error) {
	command, err := s.repository.Get(ctx, commandID)
	if err != nil {
		return Command{}, err
	}
	now := s.clock.Now().UTC()
	if err := validateLease(command, leaseID, now); err != nil {
		return Command{}, err
	}
	if command.Status != StatusRunning || progress.Percent < 0 || progress.Percent > 100 {
		return Command{}, commandError(ErrInvalidTransition, "progress requires running status and percent 0..100")
	}
	progress.Message = strings.TrimSpace(progress.Message)
	progress.UpdatedAt = now
	command.Progress = &progress
	command.UpdatedAt = now
	return s.repository.Update(ctx, command, command.Version)
}

func (s *Service) Complete(ctx context.Context, commandID, leaseID string, result Result) (Command, error) {
	command, err := s.repository.Get(ctx, commandID)
	if err != nil {
		return Command{}, err
	}
	now := s.clock.Now().UTC()
	if err := validateLease(command, leaseID, now); err != nil {
		return Command{}, err
	}
	if command.Status != StatusRunning && command.Status != StatusLeased {
		return Command{}, commandError(ErrInvalidTransition, "cannot complete command in status %s", command.Status)
	}
	oldStatus := command.Status
	result.FinishedAt = now
	command.Result = &result
	command.Status = StatusSucceeded
	if !result.Success {
		command.Status = StatusFailed
	}
	command.LeaseID = ""
	command.LeaseExpiresAt = nil
	command.CompletedAt = ptrTime(now)
	command.UpdatedAt = now
	updated, err := s.repository.Update(ctx, command, command.Version)
	if err == nil {
		_ = s.events.PublishCommandEvent(ctx, Event{Type: "command.status.changed", CommandID: updated.ID, DeviceID: updated.DeviceID, OldStatus: oldStatus, NewStatus: updated.Status, OccurredAt: now})
	}
	return updated, err
}

// Retry requeues a failed command only when the reported result was retryable and attempts remain.
func (s *Service) Retry(ctx context.Context, commandID string, delay time.Duration) (Command, error) {
	command, err := s.repository.Get(ctx, commandID)
	if err != nil {
		return Command{}, err
	}
	if command.Status != StatusFailed || command.Result == nil || !command.Result.Retryable || command.Attempts >= command.MaxAttempts {
		return Command{}, commandError(ErrInvalidTransition, "command is not eligible for retry")
	}
	now := s.clock.Now().UTC()
	if !now.Before(command.ExpiresAt) {
		return Command{}, commandError(ErrCommandExpired, "command expired")
	}
	command.Status = StatusQueued
	command.Result = nil
	command.Progress = nil
	command.CompletedAt = nil
	command.NotBefore = now.Add(maxDuration(delay, 0))
	command.UpdatedAt = now
	updated, err := s.repository.Update(ctx, command, command.Version)
	if err == nil {
		_ = s.events.PublishCommandEvent(ctx, Event{Type: "command.status.changed", CommandID: updated.ID, DeviceID: updated.DeviceID, OldStatus: StatusFailed, NewStatus: StatusQueued, OccurredAt: now})
	}
	return updated, err
}

func (s *Service) Cancel(ctx context.Context, commandID string) (Command, error) {
	command, err := s.repository.Get(ctx, commandID)
	if err != nil {
		return Command{}, err
	}
	if command.Status.Terminal() {
		if command.Status == StatusCancelled {
			return command, nil
		}
		return Command{}, commandError(ErrCommandTerminal, "command is already %s", command.Status)
	}
	now := s.clock.Now().UTC()
	oldStatus := command.Status
	command.Status = StatusCancelled
	command.LeaseID = ""
	command.LeaseExpiresAt = nil
	command.CompletedAt = ptrTime(now)
	command.UpdatedAt = now
	updated, err := s.repository.Update(ctx, command, command.Version)
	if err == nil {
		_ = s.events.PublishCommandEvent(ctx, Event{Type: "command.status.changed", CommandID: updated.ID, DeviceID: updated.DeviceID, OldStatus: oldStatus, NewStatus: StatusCancelled, OccurredAt: now})
	}
	return updated, err
}

func validateLease(command Command, leaseID string, now time.Time) error {
	if command.Status.Terminal() {
		return commandError(ErrCommandTerminal, "command is %s", command.Status)
	}
	if command.LeaseID == "" || strings.TrimSpace(leaseID) == "" {
		return commandError(ErrLeaseNotFound, "command has no active lease")
	}
	if command.LeaseID != leaseID {
		return commandError(ErrLeaseMismatch, "lease does not own command")
	}
	if command.LeaseExpiresAt == nil || !now.Before(*command.LeaseExpiresAt) {
		return commandError(ErrLeaseExpired, "lease expired")
	}
	if !now.Before(command.ExpiresAt) {
		return commandError(ErrCommandExpired, "command expired")
	}
	return nil
}

func newLeaseID() string { return newUUID() }

func newCommandID() string { return newUUID() }

func newUUID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("00000000-0000-4000-8000-%012x", time.Now().UnixNano()&0xffffffffffff)
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", buffer[0:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:16])
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
