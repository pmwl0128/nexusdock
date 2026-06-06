package core

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Event struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Data       map[string]any `json:"data,omitempty"`
}

type EventBus interface {
	Publish(context.Context, Event) error
	Subscribe(buffer int) (<-chan Event, func())
}

type InMemoryEventBus struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan Event
	nextID      uint64
	closed      bool
}

func NewEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{subscribers: make(map[uint64]chan Event)}
}

func (b *InMemoryEventBus) Publish(ctx context.Context, event Event) error {
	if event.Type == "" {
		return NewError(CodeValidation, "event type is required", nil)
	}
	if event.ID == "" {
		id, err := NewID("evt")
		if err != nil {
			return err
		}
		event.ID = id
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return errors.New("event bus is closed")
	}
	for _, subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		case <-ctx.Done():
			return ctx.Err()
		default:
			// A slow subscriber must not block a write path. Consumers that need
			// durable delivery must use a persistent event adapter.
		}
	}
	return nil
}

func (b *InMemoryEventBus) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	channel := make(chan Event, buffer)
	if b.closed {
		close(channel)
		return channel, func() {}
	}
	id := b.nextID
	b.nextID++
	b.subscribers[id] = channel
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if existing, ok := b.subscribers[id]; ok {
				delete(b.subscribers, id)
				close(existing)
			}
		})
	}
	return channel, cancel
}

func (b *InMemoryEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, subscriber := range b.subscribers {
		delete(b.subscribers, id)
		close(subscriber)
	}
}
