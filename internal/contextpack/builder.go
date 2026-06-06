package contextpack

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/tasks"
)

const (
	DefaultMaxBytes = 128 * 1024
	MinimumMaxBytes = 1024
)

// The foreign module snapshots stay opaque here. T8 links and aggregates
// objects but never copies or owns T2/T3/T5/T1 domain fields.
type MemoryProvider interface {
	BuildTaskMemoryContext(context.Context, tasks.Task, int) (json.RawMessage, error)
}

type DeviceProvider interface {
	GetDeviceSnapshot(context.Context, string) (json.RawMessage, error)
}

type SkillProvider interface {
	GetSkillDetail(context.Context, string) (json.RawMessage, error)
}

type RunProvider interface {
	ListRecentRuns(context.Context, tasks.Task, int) ([]json.RawMessage, error)
	ListEvidence(context.Context, tasks.Task, int) ([]json.RawMessage, error)
}

type AccessChecker interface {
	CanReadLinkedObject(context.Context, tasks.Actor, tasks.Link) bool
}

type AccessCheckerFunc func(context.Context, tasks.Actor, tasks.Link) bool

func (f AccessCheckerFunc) CanReadLinkedObject(ctx context.Context, actor tasks.Actor, link tasks.Link) bool {
	return f(ctx, actor, link)
}

type Pack struct {
	Task        tasks.Task        `json:"task"`
	Memory      json.RawMessage   `json:"memory"`
	Device      json.RawMessage   `json:"device,omitempty"`
	Skill       json.RawMessage   `json:"skill,omitempty"`
	RecentRuns  []json.RawMessage `json:"recent_runs"`
	Evidence    []json.RawMessage `json:"evidence"`
	GeneratedAt string            `json:"generated_at"`
	Truncated   bool              `json:"truncated"`
}

type Builder struct {
	tasks   *tasks.Service
	memory  MemoryProvider
	devices DeviceProvider
	skills  SkillProvider
	runs    RunProvider
	access  AccessChecker
	now     func() time.Time
}

func NewBuilder(taskService *tasks.Service, memory MemoryProvider, devices DeviceProvider, skills SkillProvider, runs RunProvider, access AccessChecker) *Builder {
	if access == nil {
		access = AccessCheckerFunc(func(context.Context, tasks.Actor, tasks.Link) bool { return true })
	}
	return &Builder{
		tasks: taskService, memory: memory, devices: devices, skills: skills,
		runs: runs, access: access, now: time.Now,
	}
}

func (b *Builder) WithClock(now func() time.Time) *Builder {
	if now != nil {
		b.now = now
	}
	return b
}

// BuildTaskContext adapts Builder to the transport-neutral nexus_task handler.
func (b *Builder) BuildTaskContext(ctx context.Context, actor tasks.Actor, taskID string, maxBytes int) (any, error) {
	return b.Build(ctx, actor, taskID, maxBytes)
}

func (b *Builder) Build(ctx context.Context, actor tasks.Actor, taskID string, maxBytes int) (Pack, error) {
	if b.tasks == nil {
		return Pack{}, fmt.Errorf("task service is not configured")
	}
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}
	if maxBytes < MinimumMaxBytes {
		return Pack{}, fmt.Errorf("max_bytes must be at least %d", MinimumMaxBytes)
	}
	task, err := b.tasks.Get(ctx, actor, taskID)
	if err != nil {
		return Pack{}, err
	}
	pack := Pack{
		Task:        task,
		Memory:      json.RawMessage(`{"entries":[],"conflicts":[],"truncated":false,"total_bytes":0,"generated_at":"` + b.now().UTC().Format(time.RFC3339Nano) + `"}`),
		RecentRuns:  make([]json.RawMessage, 0),
		Evidence:    make([]json.RawMessage, 0),
		GeneratedAt: b.now().UTC().Format(time.RFC3339Nano),
	}

	links := authorizedLinks(ctx, b.access, actor, task.Links)
	remaining := maxBytes - encodedSize(pack)
	if remaining < 0 {
		return Pack{}, fmt.Errorf("task metadata exceeds max_bytes")
	}
	if b.memory != nil {
		memory, buildErr := b.memory.BuildTaskMemoryContext(ctx, task, max(remaining, MinimumMaxBytes))
		if buildErr != nil {
			return Pack{}, fmt.Errorf("build memory context: %w", buildErr)
		}
		if validJSON(memory) {
			pack.Memory = cloneRaw(memory)
		}
	}

	if link, ok := firstLink(links, tasks.LinkDevice); ok && b.devices != nil {
		device, getErr := b.devices.GetDeviceSnapshot(ctx, link.ObjectID)
		if getErr != nil {
			return Pack{}, fmt.Errorf("get device snapshot: %w", getErr)
		}
		if validJSON(device) {
			pack.Device = cloneRaw(device)
		}
	}
	if link, ok := firstLink(links, tasks.LinkSkill); ok && b.skills != nil {
		skill, getErr := b.skills.GetSkillDetail(ctx, link.ObjectID)
		if getErr != nil {
			return Pack{}, fmt.Errorf("get skill detail: %w", getErr)
		}
		if validJSON(skill) {
			pack.Skill = cloneRaw(skill)
		}
	}
	if b.runs != nil {
		runs, listErr := b.runs.ListRecentRuns(ctx, task, 20)
		if listErr != nil {
			return Pack{}, fmt.Errorf("list recent runs: %w", listErr)
		}
		pack.RecentRuns = validRawItems(runs)
		evidence, evidenceErr := b.runs.ListEvidence(ctx, task, 50)
		if evidenceErr != nil {
			return Pack{}, fmt.Errorf("list run evidence: %w", evidenceErr)
		}
		pack.Evidence = validRawItems(evidence)
	}

	fitToBudget(&pack, maxBytes)
	if encodedSize(pack) > maxBytes {
		return Pack{}, fmt.Errorf("context pack cannot fit max_bytes=%d", maxBytes)
	}
	return pack, nil
}

func fitToBudget(pack *Pack, maxBytes int) {
	for encodedSize(*pack) > maxBytes && len(pack.Evidence) > 0 {
		pack.Evidence = pack.Evidence[:len(pack.Evidence)-1]
		pack.Truncated = true
	}
	for encodedSize(*pack) > maxBytes && len(pack.RecentRuns) > 0 {
		pack.RecentRuns = pack.RecentRuns[:len(pack.RecentRuns)-1]
		pack.Truncated = true
	}
	if encodedSize(*pack) > maxBytes && len(pack.Skill) > 0 {
		pack.Skill = nil
		pack.Truncated = true
	}
	if encodedSize(*pack) > maxBytes && len(pack.Device) > 0 {
		pack.Device = nil
		pack.Truncated = true
	}
	if encodedSize(*pack) > maxBytes {
		pack.Memory = json.RawMessage(`{"entries":[],"conflicts":[],"truncated":true,"total_bytes":0,"generated_at":"` + pack.GeneratedAt + `"}`)
		pack.Truncated = true
	}
}

func authorizedLinks(ctx context.Context, checker AccessChecker, actor tasks.Actor, links []tasks.Link) []tasks.Link {
	out := make([]tasks.Link, 0, len(links))
	for _, link := range links {
		if checker.CanReadLinkedObject(ctx, actor, link) {
			out = append(out, link)
		}
	}
	return out
}

func firstLink(links []tasks.Link, linkType tasks.LinkType) (tasks.Link, bool) {
	for _, link := range links {
		if link.Type == linkType && strings.TrimSpace(link.ObjectID) != "" {
			return link, true
		}
	}
	return tasks.Link{}, false
}

func validJSON(value json.RawMessage) bool {
	return len(value) > 0 && json.Valid(value)
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func validRawItems(values []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		if validJSON(value) {
			out = append(out, cloneRaw(value))
		}
	}
	return out
}

func encodedSize(value any) int {
	data, err := json.Marshal(value)
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return len(data)
}
