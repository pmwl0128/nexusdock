package tasks

import (
	"context"
	"fmt"
	"strings"
)

type SourceKind string

const (
	SourceDeviceAlert       SourceKind = "device_alert"
	SourceSkillFailure      SourceKind = "skill_failure"
	SourceEvolutionProposal SourceKind = "evolution_proposal"
	SourceMemoryConflict    SourceKind = "memory_conflict"
	SourceUpstreamConflict  SourceKind = "upstream_conflict"
	SourceUnfinishedRun     SourceKind = "unfinished_run"
)

type SourceEvent struct {
	Kind               SourceKind
	SourceID           string
	ObjectID           string
	Title              string
	Description        string
	Priority           Priority
	Links              []Link
	CompletionCriteria []string
	RiskConstraints    []string
	Reason             string
}

type Inbox struct {
	tasks *Service
}

func NewInbox(service *Service) *Inbox { return &Inbox{tasks: service} }

func (i *Inbox) Ingest(ctx context.Context, actor Actor, event SourceEvent) (CreateResult, error) {
	if i.tasks == nil {
		return CreateResult{}, taskError(CodeRepository, "task service is not configured", nil)
	}
	input, err := event.CreateInput()
	if err != nil {
		return CreateResult{}, err
	}
	return i.tasks.Create(ctx, actor, input)
}

func (e SourceEvent) CreateInput() (CreateInput, error) {
	if strings.TrimSpace(e.SourceID) == "" || strings.TrimSpace(e.ObjectID) == "" {
		return CreateInput{}, taskError(CodeValidation, "source_id and object_id are required", nil)
	}
	input := CreateInput{
		Title: strings.TrimSpace(e.Title), Description: strings.TrimSpace(e.Description),
		SourceType: string(e.Kind), SourceID: e.SourceID, ObjectID: e.ObjectID,
		Priority: e.Priority, Links: e.Links,
		CompletionCriteria: e.CompletionCriteria, RiskConstraints: e.RiskConstraints,
		CreationReason: strings.TrimSpace(e.Reason),
	}
	switch e.Kind {
	case SourceDeviceAlert:
		input.Type = TypeNeedsAgent
		input.Category = "device_alert"
		input.Links = ensureLink(input.Links, Link{Type: LinkDevice, ObjectID: e.ObjectID, Relation: "affected_device"})
	case SourceSkillFailure:
		input.Type = TypeNeedsAgent
		input.Category = "skill_failure"
		input.Links = ensureLink(input.Links, Link{Type: LinkRun, ObjectID: e.ObjectID, Relation: "failed_run"})
	case SourceEvolutionProposal:
		input.Type = TypeReview
		input.Category = "evolution_review"
		input.Links = ensureLink(input.Links, Link{Type: LinkProposal, ObjectID: e.ObjectID, Relation: "review_target"})
	case SourceMemoryConflict:
		input.Type = TypeNeedsUser
		input.Category = "memory_conflict"
		input.Links = ensureLink(input.Links, Link{Type: LinkMemory, ObjectID: e.ObjectID, Relation: "conflicted_memory"})
	case SourceUpstreamConflict:
		input.Type = TypeNeedsAgent
		input.Category = "upstream_conflict"
		input.Links = ensureLink(input.Links, Link{Type: LinkProject, ObjectID: e.ObjectID, Relation: "conflicted_project"})
	case SourceUnfinishedRun:
		input.Type = TypeNeedsAgent
		input.Category = "unfinished_run"
		input.Links = ensureLink(input.Links, Link{Type: LinkRun, ObjectID: e.ObjectID, Relation: "unfinished_run"})
	default:
		return CreateInput{}, taskError(CodeValidation, fmt.Sprintf("unsupported source kind %q", e.Kind), nil)
	}
	if input.Title == "" {
		input.Title = defaultSourceTitle(e.Kind)
	}
	if len(normalizeStrings(input.CompletionCriteria)) == 0 {
		input.CompletionCriteria = []string{"处理根因并记录验证摘要"}
	}
	if input.CreationReason == "" {
		input.CreationReason = fmt.Sprintf("%s emitted source event %s", e.Kind, e.SourceID)
	}
	return input, nil
}

func ensureLink(links []Link, required Link) []Link {
	for _, link := range links {
		if link.Type == required.Type && link.ObjectID == required.ObjectID {
			return links
		}
	}
	return append(links, required)
}

func defaultSourceTitle(kind SourceKind) string {
	switch kind {
	case SourceDeviceAlert:
		return "处理设备异常"
	case SourceSkillFailure:
		return "处理 Skill 运行失败"
	case SourceEvolutionProposal:
		return "审查 Skill 进化提案"
	case SourceMemoryConflict:
		return "解决 Memory 冲突"
	case SourceUpstreamConflict:
		return "解决上游同步冲突"
	case SourceUnfinishedRun:
		return "恢复未完成的 Run"
	default:
		return "处理 Nexus 事件"
	}
}
