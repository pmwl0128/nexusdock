package devices

import (
	"context"
	"sync"
	"time"
)

// Repository owns the atomicity boundary for enrollment consumption and heartbeats.
// T1 provides the persistent implementation and migration; this package ships a
// thread-safe implementation for tests and embedded deployments.
type Repository interface {
	CreateEnrollmentToken(context.Context, EnrollmentToken) error
	CommitEnrollment(context.Context, string, time.Time, func(EnrollmentToken) (Device, error)) (Device, error)
	ConsumeEnrollmentToken(context.Context, string, time.Time) (EnrollmentToken, error)
	CreateDevice(context.Context, Device) error
	GetDevice(context.Context, string) (Device, error)
	ListDevices(context.Context) ([]Device, error)
	UpdateDevice(context.Context, Device, int64) (Device, error)
	RecordHeartbeat(context.Context, string, int64, Heartbeat) (Device, error)
	LatestHeartbeat(context.Context, string) (Heartbeat, bool, error)
}

type MemoryRepository struct {
	mu         sync.RWMutex
	tokens     map[string]EnrollmentToken
	devices    map[string]Device
	heartbeats map[string]Heartbeat
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		tokens:     make(map[string]EnrollmentToken),
		devices:    make(map[string]Device),
		heartbeats: make(map[string]Heartbeat),
	}
}

func (r *MemoryRepository) CreateEnrollmentToken(_ context.Context, token EnrollmentToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tokens[token.Digest]; exists {
		return domainError(ErrVersionConflict, "enrollment token already exists")
	}
	r.tokens[token.Digest] = token
	return nil
}

// CommitEnrollment consumes an enrollment token and creates its device in one
// atomic repository operation. Persistent implementations must execute both
// writes in a single transaction.
func (r *MemoryRepository) CommitEnrollment(_ context.Context, digest string, now time.Time, build func(EnrollmentToken) (Device, error)) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, exists := r.tokens[digest]
	if !exists {
		return Device{}, domainError(ErrEnrollmentTokenInvalid, "enrollment token is invalid")
	}
	if token.UsedAt != nil {
		return Device{}, domainError(ErrEnrollmentTokenUsed, "enrollment token was already used")
	}
	if !now.Before(token.ExpiresAt) {
		return Device{}, domainError(ErrEnrollmentTokenExpired, "enrollment token expired")
	}
	device, err := build(token)
	if err != nil {
		return Device{}, err
	}
	if _, exists := r.devices[device.ID]; exists {
		return Device{}, domainError(ErrDeviceAlreadyExists, "device %q already exists", device.ID)
	}
	token.UsedAt = ptrTime(now)
	r.tokens[digest] = token
	r.devices[device.ID] = cloneDevice(device)
	return cloneDevice(device), nil
}

func (r *MemoryRepository) ConsumeEnrollmentToken(_ context.Context, digest string, now time.Time) (EnrollmentToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, exists := r.tokens[digest]
	if !exists {
		return EnrollmentToken{}, domainError(ErrEnrollmentTokenInvalid, "enrollment token is invalid")
	}
	if token.UsedAt != nil {
		return EnrollmentToken{}, domainError(ErrEnrollmentTokenUsed, "enrollment token was already used")
	}
	if !now.Before(token.ExpiresAt) {
		return EnrollmentToken{}, domainError(ErrEnrollmentTokenExpired, "enrollment token expired")
	}
	token.UsedAt = ptrTime(now)
	r.tokens[digest] = token
	return token, nil
}

func (r *MemoryRepository) CreateDevice(_ context.Context, device Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.devices[device.ID]; exists {
		return domainError(ErrDeviceAlreadyExists, "device %q already exists", device.ID)
	}
	r.devices[device.ID] = cloneDevice(device)
	return nil
}

func (r *MemoryRepository) GetDevice(_ context.Context, id string) (Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	device, exists := r.devices[id]
	if !exists {
		return Device{}, domainError(ErrDeviceNotFound, "device %q not found", id)
	}
	return cloneDevice(device), nil
}

func (r *MemoryRepository) ListDevices(_ context.Context) ([]Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Device, 0, len(r.devices))
	for _, device := range r.devices {
		result = append(result, cloneDevice(device))
	}
	return result, nil
}

func (r *MemoryRepository) UpdateDevice(_ context.Context, device Device, expectedVersion int64) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.devices[device.ID]
	if !exists {
		return Device{}, domainError(ErrDeviceNotFound, "device %q not found", device.ID)
	}
	if current.Version != expectedVersion {
		return Device{}, domainError(ErrVersionConflict, "device %q changed concurrently", device.ID)
	}
	device.Version = expectedVersion + 1
	r.devices[device.ID] = cloneDevice(device)
	return cloneDevice(device), nil
}

func (r *MemoryRepository) RecordHeartbeat(_ context.Context, id string, expectedVersion int64, heartbeat Heartbeat) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, exists := r.devices[id]
	if !exists {
		return Device{}, domainError(ErrDeviceNotFound, "device %q not found", id)
	}
	if device.Version != expectedVersion {
		return Device{}, domainError(ErrVersionConflict, "device %q changed concurrently", id)
	}
	device.AgentDockVersion = heartbeat.AgentDockVersion
	device.Capabilities = cloneCapabilities(heartbeat.Capabilities)
	device.LastSeen = ptrTime(heartbeat.ReceivedAt)
	device.Status = StatusOnline
	device.UpdatedAt = heartbeat.ReceivedAt
	device.Version++
	r.devices[id] = cloneDevice(device)
	r.heartbeats[id] = cloneHeartbeat(heartbeat)
	return cloneDevice(device), nil
}

func (r *MemoryRepository) LatestHeartbeat(_ context.Context, id string) (Heartbeat, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, exists := r.devices[id]; !exists {
		return Heartbeat{}, false, domainError(ErrDeviceNotFound, "device %q not found", id)
	}
	heartbeat, exists := r.heartbeats[id]
	return cloneHeartbeat(heartbeat), exists, nil
}

func ptrTime(value time.Time) *time.Time {
	copyValue := value
	return &copyValue
}
