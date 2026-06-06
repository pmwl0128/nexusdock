package tasks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// MemoryRepository is a race-safe reference implementation used by unit tests,
// local development and as the behavioral specification for T1's SQL adapter.
type MemoryRepository struct {
	mu         sync.RWMutex
	tasks      map[string]Task
	dedup      map[string]string
	activities map[string][]Activity
	idempotent map[string]Task
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		tasks:      make(map[string]Task),
		dedup:      make(map[string]string),
		activities: make(map[string][]Activity),
		idempotent: make(map[string]Task),
	}
}

func (r *MemoryRepository) CreateOrGet(_ context.Context, task Task, dedupKey string, activity Activity) (Task, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.dedup[dedupKey]; ok {
		return cloneTask(r.tasks[id]), false, nil
	}
	if _, exists := r.tasks[task.ID]; exists {
		return Task{}, false, fmt.Errorf("task id already exists: %s", task.ID)
	}
	r.tasks[task.ID] = cloneTask(task)
	r.dedup[dedupKey] = task.ID
	r.activities[task.ID] = append(r.activities[task.ID], activity)
	return cloneTask(task), true, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[id]
	if !ok {
		return Task{}, taskError(CodeNotFound, "task does not exist", nil)
	}
	return cloneTask(task), nil
}

func (r *MemoryRepository) List(_ context.Context, filter Filter) (Page, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Task, 0, len(r.tasks))
	for _, task := range r.tasks {
		if !matchesFilter(task, filter) {
			continue
		}
		items = append(items, cloneTask(task))
	}
	sort.Slice(items, func(i, j int) bool {
		pi, pj := priorityRank(items[i].Priority), priorityRank(items[j].Priority)
		if pi != pj {
			return pi > pj
		}
		ci, cj := parseTime(items[i].CreatedAt), parseTime(items[j].CreatedAt)
		if !ci.Equal(cj) {
			return ci.After(cj)
		}
		return items[i].ID < items[j].ID
	})
	total := len(items)
	start := 0
	if cursor := strings.TrimSpace(filter.Cursor); cursor != "" {
		for i := range items {
			if items[i].ID == cursor {
				start = i + 1
				break
			}
		}
	}
	limit := filter.Limit
	if limit < 1 || limit > 200 {
		limit = 50
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) && end > start {
		next = items[end-1].ID
	}
	return Page{Items: append([]Task(nil), items[start:end]...), NextCursor: next, Total: total}, nil
}

func (r *MemoryRepository) Update(_ context.Context, task Task, expectedVersion int64, activity Activity) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.tasks[task.ID]
	if !ok {
		return Task{}, taskError(CodeNotFound, "task does not exist", nil)
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return Task{}, taskError(CodeVersionConflict, "task version changed", nil)
	}
	task.Version = current.Version + 1
	r.tasks[task.ID] = cloneTask(task)
	r.activities[task.ID] = append(r.activities[task.ID], activity)
	return cloneTask(task), nil
}

func (r *MemoryRepository) Activities(_ context.Context, taskID string) ([]Activity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.tasks[taskID]; !ok {
		return nil, taskError(CodeNotFound, "task does not exist", nil)
	}
	items := r.activities[taskID]
	out := make([]Activity, len(items))
	copy(out, items)
	return out, nil
}

func (r *MemoryRepository) GetIdempotency(_ context.Context, scope, key string) (Task, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.idempotent[scope+"\x00"+key]
	return cloneTask(task), ok, nil
}

func (r *MemoryRepository) PutIdempotency(_ context.Context, scope, key string, task Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	mapKey := scope + "\x00" + key
	if existing, ok := r.idempotent[mapKey]; ok && existing.ID != task.ID {
		return taskError(CodeVersionConflict, "idempotency key already belongs to another task", nil)
	}
	r.idempotent[mapKey] = cloneTask(task)
	return nil
}

func matchesFilter(task Task, filter Filter) bool {
	if len(filter.Statuses) > 0 {
		matched := false
		for _, status := range filter.Statuses {
			matched = matched || task.Status == status
		}
		if !matched {
			return false
		}
	}
	if len(filter.Types) > 0 {
		matched := false
		for _, taskType := range filter.Types {
			matched = matched || task.Type == taskType
		}
		if !matched {
			return false
		}
	}
	if filter.Category != "" && task.Category != filter.Category {
		return false
	}
	if filter.AssignedActor != nil {
		if task.AssignedActor == nil || task.AssignedActor.Type != filter.AssignedActor.Type || task.AssignedActor.ID != filter.AssignedActor.ID {
			return false
		}
	}
	if filter.LinkType != "" || filter.LinkObjectID != "" {
		matched := false
		for _, link := range task.Links {
			if (filter.LinkType == "" || link.Type == filter.LinkType) &&
				(filter.LinkObjectID == "" || link.ObjectID == filter.LinkObjectID) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func priorityRank(priority Priority) int {
	switch priority {
	case PriorityCritical:
		return 4
	case PriorityHigh:
		return 3
	case PriorityNormal:
		return 2
	case PriorityLow:
		return 1
	default:
		return 0
	}
}
