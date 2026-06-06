package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	EventTaskCreated = "task.created"
	EventTaskUpdated = "task.updated"
)

type IDGenerator func() (string, error)

type Service struct {
	repo   Repository
	authz  Authorizer
	audit  AuditSink
	events EventSink
	now    func() time.Time
	newID  IDGenerator
}

func NewService(repo Repository, authz Authorizer, audit AuditSink, events EventSink) *Service {
	if authz == nil {
		authz = AllowAllAuthorizer()
	}
	if audit == nil {
		audit = discardAudit{}
	}
	if events == nil {
		events = discardEvents{}
	}
	return &Service{
		repo: repo, authz: authz, audit: audit, events: events,
		now: time.Now, newID: NewUUID,
	}
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) WithIDGenerator(generator IDGenerator) *Service {
	if generator != nil {
		s.newID = generator
	}
	return s
}

func (s *Service) Create(ctx context.Context, actor Actor, input CreateInput) (CreateResult, error) {
	if s.repo == nil {
		return CreateResult{}, taskError(CodeRepository, "task repository is not configured", nil)
	}
	if !actor.Valid() {
		return CreateResult{}, taskError(CodeForbidden, "valid actor is required", nil)
	}
	if err := validateCreate(input); err != nil {
		return CreateResult{}, err
	}
	id, err := s.newID()
	if err != nil {
		return CreateResult{}, taskError(CodeRepository, "could not allocate task id", err)
	}
	activityID, err := s.newID()
	if err != nil {
		return CreateResult{}, taskError(CodeRepository, "could not allocate activity id", err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	priority := input.Priority
	if priority == "" {
		priority = PriorityNormal
	}
	task := Task{
		ID:                 id,
		Type:               input.Type,
		Status:             StatusInbox,
		Title:              strings.TrimSpace(input.Title),
		Description:        strings.TrimSpace(input.Description),
		Category:           strings.TrimSpace(input.Category),
		SourceType:         strings.TrimSpace(input.SourceType),
		SourceID:           strings.TrimSpace(input.SourceID),
		ObjectID:           strings.TrimSpace(input.ObjectID),
		Priority:           priority,
		Links:              normalizeLinks(input.Links),
		CompletionCriteria: normalizeStrings(input.CompletionCriteria),
		RiskConstraints:    normalizeStrings(input.RiskConstraints),
		CreatedAt:          now,
		UpdatedAt:          now,
		Version:            1,
	}
	if !s.authz.Can(ctx, actor, "task.create", task) {
		return CreateResult{}, taskError(CodeForbidden, "actor cannot create this task", nil)
	}
	reason := strings.TrimSpace(input.CreationReason)
	if reason == "" {
		reason = fmt.Sprintf("created from %s %s", task.SourceType, task.SourceID)
	}
	activity := Activity{
		ID: activityID, TaskID: task.ID, Actor: actor, Action: "create",
		To: task.Status, Reason: reason, CreatedAt: now,
		Metadata: map[string]any{
			"source_type": task.SourceType,
			"source_id":   task.SourceID,
			"category":    task.Category,
			"object_id":   task.ObjectID,
		},
	}
	stored, created, err := s.repo.CreateOrGet(ctx, task, DedupKey(task.SourceType, task.SourceID, task.Category, task.ObjectID), activity)
	if err != nil {
		return CreateResult{}, taskError(CodeRepository, "create task", err)
	}
	if !created {
		if !s.authz.Can(ctx, actor, "task.inspect", stored) {
			return CreateResult{}, taskError(CodeForbidden, "actor cannot inspect deduplicated task", nil)
		}
		return CreateResult{Task: stored, Created: false}, nil
	}
	if err := s.recordMutation(ctx, actor, "task.create", stored, "created", map[string]any{"dedup_key": DedupKey(task.SourceType, task.SourceID, task.Category, task.ObjectID)}); err != nil {
		return CreateResult{Task: stored, Created: true}, err
	}
	if err := s.events.Publish(ctx, Event{Type: EventTaskCreated, Data: taskEventData(stored, "create")}); err != nil {
		return CreateResult{Task: stored, Created: true}, taskError(CodeRepository, "publish task.created", err)
	}
	return CreateResult{Task: stored, Created: true}, nil
}

func (s *Service) Get(ctx context.Context, actor Actor, id string) (Task, error) {
	task, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Task{}, err
	}
	if !s.authz.Can(ctx, actor, "task.inspect", task) {
		return Task{}, taskError(CodeForbidden, "actor cannot inspect task", nil)
	}
	return task, nil
}

func (s *Service) Inspect(ctx context.Context, actor Actor, id string) (Inspection, error) {
	task, err := s.Get(ctx, actor, id)
	if err != nil {
		return Inspection{}, err
	}
	activities, err := s.repo.Activities(ctx, task.ID)
	if err != nil {
		return Inspection{}, taskError(CodeRepository, "read task activities", err)
	}
	reason := ""
	for _, activity := range activities {
		if activity.Action == "create" {
			reason = activity.Reason
			break
		}
	}
	return Inspection{Task: task, CreationReason: reason, Activities: activities}, nil
}

func (s *Service) List(ctx context.Context, actor Actor, filter Filter) (Page, error) {
	page, err := s.repo.List(ctx, filter)
	if err != nil {
		return Page{}, taskError(CodeRepository, "list tasks", err)
	}
	allowed := page.Items[:0]
	for _, task := range page.Items {
		if s.authz.Can(ctx, actor, "task.inspect", task) {
			allowed = append(allowed, task)
		}
	}
	page.Items = allowed
	return page, nil
}

func (s *Service) Claim(ctx context.Context, actor Actor, id string, expectedVersion int64) (Task, error) {
	if actor.Type != "agent" && actor.Type != "user" {
		return Task{}, taskError(CodeForbidden, "only user or agent actors can claim tasks", nil)
	}
	return s.mutate(ctx, actor, id, "claim", expectedVersion, func(task *Task) (string, map[string]any, error) {
		if task.Status != StatusInbox && task.Status != StatusReady {
			if task.AssignedActor != nil {
				return "", nil, taskError(CodeAlreadyClaimed, "task is already claimed", nil)
			}
			return "", nil, invalidTransition(task.Status, StatusInProgress)
		}
		task.Status = StatusInProgress
		assigned := actor
		task.AssignedActor = &assigned
		return "task claimed", map[string]any{"assigned_actor_type": actor.Type, "assigned_actor_id": actor.ID}, nil
	})
}

func (s *Service) Update(ctx context.Context, actor Actor, id string, input UpdateInput) (Task, error) {
	return s.mutate(ctx, actor, id, "update", input.ExpectedVersion, func(task *Task) (string, map[string]any, error) {
		changed := map[string]any{}
		if input.Title != nil {
			value := strings.TrimSpace(*input.Title)
			if value == "" || len(value) > 200 {
				return "", nil, taskError(CodeValidation, "title must be 1..200 characters", nil)
			}
			task.Title = value
			changed["title"] = true
		}
		if input.Description != nil {
			value := strings.TrimSpace(*input.Description)
			if len(value) > 20000 {
				return "", nil, taskError(CodeValidation, "description is too long", nil)
			}
			task.Description = value
			changed["description"] = true
		}
		if input.Priority != nil {
			if !validPriority(*input.Priority) {
				return "", nil, taskError(CodeValidation, "invalid priority", nil)
			}
			task.Priority = *input.Priority
			changed["priority"] = true
		}
		if input.CompletionCriteria != nil {
			criteria := normalizeStrings(*input.CompletionCriteria)
			if len(criteria) == 0 {
				return "", nil, taskError(CodeValidation, "completion criteria cannot be empty", nil)
			}
			task.CompletionCriteria = criteria
			changed["completion_criteria"] = true
		}
		if input.RiskConstraints != nil {
			task.RiskConstraints = normalizeStrings(*input.RiskConstraints)
			changed["risk_constraints"] = true
		}
		if len(changed) == 0 {
			return "", nil, taskError(CodeValidation, "no fields to update", nil)
		}
		return "task fields updated", changed, nil
	})
}

func (s *Service) Progress(ctx context.Context, actor Actor, id, note string, expectedVersion int64) (Task, error) {
	return s.mutate(ctx, actor, id, "progress", expectedVersion, func(task *Task) (string, map[string]any, error) {
		if task.Status != StatusInProgress {
			return "", nil, invalidTransition(task.Status, StatusInProgress)
		}
		if strings.TrimSpace(note) == "" {
			return "", nil, taskError(CodeValidation, "progress note is required", nil)
		}
		return strings.TrimSpace(note), nil, nil
	})
}

func (s *Service) Block(ctx context.Context, actor Actor, id, reason string, expectedVersion int64) (Task, error) {
	return s.transition(ctx, actor, id, "block", StatusBlocked, reason, expectedVersion)
}

func (s *Service) AwaitUser(ctx context.Context, actor Actor, id, reason string, expectedVersion int64) (Task, error) {
	return s.transition(ctx, actor, id, "await_user", StatusAwaitingUser, reason, expectedVersion)
}

func (s *Service) AwaitAgent(ctx context.Context, actor Actor, id, reason string, expectedVersion int64) (Task, error) {
	return s.transition(ctx, actor, id, "await_agent", StatusAwaitingAgent, reason, expectedVersion)
}

func (s *Service) Ready(ctx context.Context, actor Actor, id, reason string, expectedVersion int64) (Task, error) {
	return s.transition(ctx, actor, id, "ready", StatusReady, reason, expectedVersion)
}

func (s *Service) Fail(ctx context.Context, actor Actor, id, reason string, expectedVersion int64) (Task, error) {
	return s.transition(ctx, actor, id, "fail", StatusFailed, reason, expectedVersion)
}

func (s *Service) Cancel(ctx context.Context, actor Actor, id, reason string, expectedVersion int64) (Task, error) {
	return s.transition(ctx, actor, id, "cancel", StatusCancelled, reason, expectedVersion)
}

func (s *Service) Complete(ctx context.Context, actor Actor, id string, input CompletionInput) (Task, error) {
	if strings.TrimSpace(input.IdempotencyKey) != "" {
		if cached, ok, err := s.repo.GetIdempotency(ctx, "task.complete:"+id, input.IdempotencyKey); err != nil {
			return Task{}, taskError(CodeRepository, "read idempotency result", err)
		} else if ok {
			if !s.authz.Can(ctx, actor, "task.inspect", cached) {
				return Task{}, taskError(CodeForbidden, "actor cannot inspect completed task", nil)
			}
			return cached, nil
		}
	}
	if strings.TrimSpace(input.VerificationSummary) == "" {
		return Task{}, taskError(CodeVerification, "verification summary is required", nil)
	}
	if strings.TrimSpace(input.Summary) == "" {
		return Task{}, taskError(CodeValidation, "completion summary is required", nil)
	}
	task, err := s.mutate(ctx, actor, id, "complete", input.ExpectedVersion, func(task *Task) (string, map[string]any, error) {
		if !canTransition(task.Status, StatusCompleted) {
			return "", nil, invalidTransition(task.Status, StatusCompleted)
		}
		task.Status = StatusCompleted
		task.Completion = &Completion{
			Summary:             strings.TrimSpace(input.Summary),
			VerificationSummary: strings.TrimSpace(input.VerificationSummary),
			RunID:               input.RunID,
			EvidenceIDs:         normalizeStrings(input.EvidenceIDs),
			CompletedAt:         s.now().UTC().Format(time.RFC3339Nano),
		}
		return task.Completion.Summary, map[string]any{
			"verification_summary": task.Completion.VerificationSummary,
			"evidence_count":       len(task.Completion.EvidenceIDs),
		}, nil
	})
	if err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(input.IdempotencyKey) != "" {
		if err := s.repo.PutIdempotency(ctx, "task.complete:"+id, input.IdempotencyKey, task); err != nil {
			return task, taskError(CodeRepository, "store idempotency result", err)
		}
	}
	return task, nil
}

func (s *Service) transition(ctx context.Context, actor Actor, id, action string, target Status, reason string, expectedVersion int64) (Task, error) {
	return s.mutate(ctx, actor, id, action, expectedVersion, func(task *Task) (string, map[string]any, error) {
		if strings.TrimSpace(reason) == "" {
			return "", nil, taskError(CodeValidation, "transition reason is required", nil)
		}
		if !canTransition(task.Status, target) {
			return "", nil, invalidTransition(task.Status, target)
		}
		task.Status = target
		return strings.TrimSpace(reason), nil, nil
	})
}

func (s *Service) mutate(ctx context.Context, actor Actor, id, action string, expectedVersion int64, apply func(*Task) (string, map[string]any, error)) (Task, error) {
	current, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Task{}, err
	}
	if !s.authz.Can(ctx, actor, "task."+action, current) {
		return Task{}, taskError(CodeForbidden, "actor cannot perform task action", nil)
	}
	before := current.Status
	reason, metadata, err := apply(&current)
	if err != nil {
		return Task{}, err
	}
	activityID, err := s.newID()
	if err != nil {
		return Task{}, taskError(CodeRepository, "could not allocate activity id", err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	current.UpdatedAt = now
	activity := Activity{
		ID: activityID, TaskID: current.ID, Actor: actor, Action: action,
		From: before, To: current.Status, Reason: reason, Metadata: metadata, CreatedAt: now,
	}
	stored, err := s.repo.Update(ctx, current, expectedVersion, activity)
	if err != nil {
		if IsCode(err, CodeVersionConflict) || IsCode(err, CodeNotFound) {
			return Task{}, err
		}
		return Task{}, taskError(CodeRepository, "update task", err)
	}
	if err := s.recordMutation(ctx, actor, "task."+action, stored, "succeeded", metadata); err != nil {
		return stored, err
	}
	if err := s.events.Publish(ctx, Event{Type: EventTaskUpdated, Data: taskEventData(stored, action)}); err != nil {
		return stored, taskError(CodeRepository, "publish task.updated", err)
	}
	return stored, nil
}

func (s *Service) recordMutation(ctx context.Context, actor Actor, action string, task Task, result string, metadata map[string]any) error {
	if err := s.audit.Record(ctx, AuditRecord{
		Actor: actor, Action: action, ObjectType: "task", ObjectID: task.ID,
		Result: result, Risk: auditRisk(task.Priority), Metadata: metadata,
	}); err != nil {
		return taskError(CodeAudit, "record task audit event", err)
	}
	return nil
}

func taskEventData(task Task, action string) map[string]any {
	return map[string]any{
		"task_id": task.ID, "status": task.Status, "type": task.Type,
		"category": task.Category, "action": action, "version": task.Version,
	}
}

func auditRisk(priority Priority) string {
	switch priority {
	case PriorityCritical:
		return "critical"
	case PriorityHigh:
		return "high"
	default:
		return "low"
	}
}

func validateCreate(input CreateInput) error {
	if !validType(input.Type) {
		return taskError(CodeValidation, "invalid task type", nil)
	}
	if title := strings.TrimSpace(input.Title); title == "" || len(title) > 200 {
		return taskError(CodeValidation, "title must be 1..200 characters", nil)
	}
	if len(strings.TrimSpace(input.Description)) > 20000 {
		return taskError(CodeValidation, "description is too long", nil)
	}
	if strings.TrimSpace(input.Category) == "" || strings.TrimSpace(input.SourceType) == "" || strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.ObjectID) == "" {
		return taskError(CodeValidation, "category, source_type, source_id and object_id are required", nil)
	}
	if input.Priority != "" && !validPriority(input.Priority) {
		return taskError(CodeValidation, "invalid priority", nil)
	}
	if len(normalizeStrings(input.CompletionCriteria)) == 0 {
		return taskError(CodeValidation, "at least one completion criterion is required", nil)
	}
	for _, link := range input.Links {
		if !validLinkType(link.Type) || strings.TrimSpace(link.ObjectID) == "" || strings.TrimSpace(link.Relation) == "" {
			return taskError(CodeValidation, "invalid task link", nil)
		}
	}
	return nil
}

func validType(value Type) bool {
	switch value {
	case TypeNeedsAgent, TypeNeedsUser, TypeAutomatic, TypeScheduled, TypeReview:
		return true
	default:
		return false
	}
}

func validPriority(value Priority) bool {
	switch value {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical:
		return true
	default:
		return false
	}
}

func validLinkType(value LinkType) bool {
	switch value {
	case LinkDevice, LinkMemory, LinkSkill, LinkRun, LinkProposal, LinkProject:
		return true
	default:
		return false
	}
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeLinks(values []Link) []Link {
	seen := make(map[string]struct{}, len(values))
	out := make([]Link, 0, len(values))
	for _, value := range values {
		value.ObjectID = strings.TrimSpace(value.ObjectID)
		value.Relation = strings.TrimSpace(value.Relation)
		key := string(value.Type) + "\x00" + value.ObjectID + "\x00" + value.Relation
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func canTransition(from, to Status) bool {
	allowed := map[Status]map[Status]bool{
		StatusInbox:         {StatusReady: true, StatusInProgress: true, StatusCancelled: true},
		StatusReady:         {StatusInProgress: true, StatusBlocked: true, StatusAwaitingUser: true, StatusAwaitingAgent: true, StatusCancelled: true, StatusFailed: true},
		StatusInProgress:    {StatusReady: true, StatusBlocked: true, StatusAwaitingUser: true, StatusAwaitingAgent: true, StatusCompleted: true, StatusCancelled: true, StatusFailed: true},
		StatusBlocked:       {StatusReady: true, StatusInProgress: true, StatusCancelled: true, StatusFailed: true},
		StatusAwaitingUser:  {StatusReady: true, StatusInProgress: true, StatusCancelled: true, StatusFailed: true},
		StatusAwaitingAgent: {StatusReady: true, StatusInProgress: true, StatusCancelled: true, StatusFailed: true},
	}
	return allowed[from][to]
}

func invalidTransition(from, to Status) error {
	return taskError(CodeInvalidTransition, fmt.Sprintf("cannot transition from %s to %s", from, to), nil)
}
