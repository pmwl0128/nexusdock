package commands

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Repository is the persistence boundary T1 can implement using its transaction layer.
type Repository interface {
	Enqueue(context.Context, Command) (Command, bool, error)
	Get(context.Context, string) (Command, error)
	ListByDevice(context.Context, string) ([]Command, error)
	Update(context.Context, Command, int64) (Command, error)
	LeaseNext(context.Context, string, time.Time, time.Duration) (Command, error)
}

type MemoryRepository struct {
	mu          sync.Mutex
	commands    map[string]Command
	idempotency map[string]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{commands: make(map[string]Command), idempotency: make(map[string]string)}
}

func idempotencyIndex(deviceID, key string) string { return deviceID + "\x00" + key }

func (r *MemoryRepository) Enqueue(_ context.Context, command Command) (Command, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := idempotencyIndex(command.DeviceID, command.IdempotencyKey)
	if existingID, ok := r.idempotency[index]; ok {
		return cloneCommand(r.commands[existingID]), false, nil
	}
	r.commands[command.ID] = cloneCommand(command)
	r.idempotency[index] = command.ID
	return cloneCommand(command), true, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	command, exists := r.commands[id]
	if !exists {
		return Command{}, commandError(ErrCommandNotFound, "command %q not found", id)
	}
	return cloneCommand(command), nil
}

func (r *MemoryRepository) ListByDevice(_ context.Context, deviceID string) ([]Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Command, 0)
	for _, command := range r.commands {
		if command.DeviceID == deviceID {
			result = append(result, cloneCommand(command))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (r *MemoryRepository) Update(_ context.Context, command Command, expectedVersion int64) (Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.commands[command.ID]
	if !exists {
		return Command{}, commandError(ErrCommandNotFound, "command %q not found", command.ID)
	}
	if current.Version != expectedVersion {
		return Command{}, commandError(ErrVersionConflict, "command %q changed concurrently", command.ID)
	}
	command.Version = expectedVersion + 1
	r.commands[command.ID] = cloneCommand(command)
	return cloneCommand(command), nil
}

func (r *MemoryRepository) LeaseNext(_ context.Context, deviceID string, now time.Time, leaseDuration time.Duration) (Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var candidate *Command
	for id, stored := range r.commands {
		command := stored
		if command.DeviceID != deviceID || command.Status.Terminal() || now.Before(command.NotBefore) {
			continue
		}
		if !now.Before(command.ExpiresAt) {
			command.Status = StatusExpired
			command.LeaseID = ""
			command.LeaseExpiresAt = nil
			command.CompletedAt = ptrTime(now)
			command.UpdatedAt = now
			command.Version++
			r.commands[id] = command
			continue
		}
		leaseable := command.Status == StatusQueued ||
			((command.Status == StatusLeased || command.Status == StatusRunning) && command.LeaseExpiresAt != nil && !now.Before(*command.LeaseExpiresAt))
		if leaseable && command.Attempts >= command.MaxAttempts {
			command.Status = StatusFailed
			command.LeaseID = ""
			command.LeaseExpiresAt = nil
			command.Result = &Result{
				Success:    false,
				ErrorCode:  "MAX_ATTEMPTS_EXCEEDED",
				Error:      "command lease expired after maximum attempts",
				Retryable:  false,
				FinishedAt: now,
			}
			command.CompletedAt = ptrTime(now)
			command.UpdatedAt = now
			command.Version++
			r.commands[id] = command
			continue
		}
		if !leaseable {
			continue
		}
		if candidate == nil || command.Priority > candidate.Priority ||
			(command.Priority == candidate.Priority && command.CreatedAt.Before(candidate.CreatedAt)) {
			copyValue := command
			candidate = &copyValue
		}
	}
	if candidate == nil {
		return Command{}, commandError(ErrCommandNotLeaseable, "no leaseable command for device %q", deviceID)
	}
	candidate.Status = StatusLeased
	candidate.LeaseID = newLeaseID()
	candidate.LeaseExpiresAt = ptrTime(now.Add(leaseDuration))
	candidate.Attempts++
	candidate.UpdatedAt = now
	candidate.Version++
	r.commands[candidate.ID] = cloneCommand(*candidate)
	return cloneCommand(*candidate), nil
}
