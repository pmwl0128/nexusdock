package evolution

import "time"

// Trigger is the frozen Nexus V1 evolution trigger enum.
type Trigger string

const (
	TriggerUserCorrection        Trigger = "user_correction"
	TriggerAgentRecovery         Trigger = "agent_recovery_success"
	TriggerFalseSuccess          Trigger = "false_success"
	TriggerFalseFailure          Trigger = "false_failure"
	TriggerRepeatedFailure       Trigger = "repeated_failure"
	TriggerRepeatedManualStep    Trigger = "repeated_manual_step"
	TriggerMissingValidation     Trigger = "missing_validation"
	TriggerEnvironmentDrift      Trigger = "environment_drift"
	TriggerPerformanceRegression Trigger = "performance_regression"
	TriggerSecurityViolation     Trigger = "security_violation"
	TriggerUpstreamUpdate        Trigger = "upstream_update"
)

var validTriggers = map[Trigger]struct{}{
	TriggerUserCorrection: {}, TriggerAgentRecovery: {}, TriggerFalseSuccess: {},
	TriggerFalseFailure: {}, TriggerRepeatedFailure: {}, TriggerRepeatedManualStep: {},
	TriggerMissingValidation: {}, TriggerEnvironmentDrift: {},
	TriggerPerformanceRegression: {}, TriggerSecurityViolation: {}, TriggerUpstreamUpdate: {},
}

func (t Trigger) Valid() bool { _, ok := validTriggers[t]; return ok }

// Status is shared by candidates and proposals and mirrors the frozen V1 contract.
type Status string

const (
	StatusObserved      Status = "observed"
	StatusWatching      Status = "watching"
	StatusCandidate     Status = "candidate"
	StatusProposalDraft Status = "proposal_draft"
	StatusReviewReady   Status = "review_ready"
	StatusApproved      Status = "approved"
	StatusTesting       Status = "testing"
	StatusCanary        Status = "canary"
	StatusReleased      Status = "released"
	StatusRejected      Status = "rejected"
	StatusDeferred      Status = "deferred"
	StatusRolledBack    Status = "rolled_back"
)

type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
	ScopeDevice  Scope = "device"
)

type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

// Observation matches contracts/components/schemas/Observation.
type Observation struct {
	ID           string    `json:"id"`
	SkillID      string    `json:"skill_id"`
	RunID        string    `json:"run_id"`
	DeviceID     *string   `json:"device_id,omitempty"`
	Trigger      Trigger   `json:"trigger"`
	Signature    string    `json:"signature"`
	Summary      string    `json:"summary"`
	EvidenceIDs  []string  `json:"evidence_ids"`
	PrivateScope bool      `json:"private_scope"`
	ObservedAt   time.Time `json:"observed_at"`
}

// EvolutionCandidate matches contracts/components/schemas/EvolutionCandidate.
type EvolutionCandidate struct {
	ID             string    `json:"id"`
	SkillID        string    `json:"skill_id"`
	Status         Status    `json:"status"`
	Signature      string    `json:"signature"`
	Trigger        Trigger   `json:"trigger"`
	ObservationIDs []string  `json:"observation_ids"`
	Score          float64   `json:"score"`
	Confidence     float64   `json:"confidence"`
	Reasoning      []string  `json:"reasoning"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ObjectReference struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// EvolutionProposal matches contracts/components/schemas/EvolutionProposal.
type EvolutionProposal struct {
	ID              string            `json:"id"`
	CandidateID     string            `json:"candidate_id"`
	SkillID         string            `json:"skill_id"`
	Status          Status            `json:"status"`
	Problem         string            `json:"problem"`
	Evidence        []ObjectReference `json:"evidence"`
	Scope           Scope             `json:"scope"`
	SuggestedFiles  []string          `json:"suggested_files"`
	Risk            Risk              `json:"risk"`
	Tests           []string          `json:"tests"`
	ExpectedBenefit string            `json:"expected_benefit"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type RunStatus string

const (
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

type Step struct {
	Name       string
	Action     string
	Outcome    string
	Error      string
	Manual     bool
	Validation bool
	Duration   time.Duration
}

type Evidence struct {
	ID      string
	Kind    string
	Summary string
	Private bool
}

type Verification struct {
	Passed  bool
	Summary string
}

type UserCorrectionEvent struct {
	ID      string
	Summary string
}

type UpstreamUpdateEvent struct {
	ID      string
	Summary string
}

// RunInput is an adapter-friendly input. T1/T6 can map generated contract DTOs into it.
type RunInput struct {
	ID               string
	SkillID          string
	DeviceID         *string
	ProjectID        string
	Status           RunStatus
	Error            string
	Steps            []Step
	Evidence         []Evidence
	Verification     *Verification
	Duration         time.Duration
	BaselineDuration time.Duration
	OriginalPlan     []PlanStep
	FinalPlan        []PlanStep
	SuggestedFiles   []string
	UserCorrections  []UserCorrectionEvent
	UpstreamUpdates  []UpstreamUpdateEvent
	CompletedAt      time.Time
}

// ObservationContext is stored internally and is never serialized as a public DTO.
type ObservationContext struct {
	ProjectID      string
	SuggestedFiles []string
	OriginalPlan   []PlanStep
	FinalPlan      []PlanStep
	Transient      bool
	ErrorCategory  string
}

type ObservationRecord struct {
	Observation Observation
	Context     ObservationContext
}

type PlanStep struct {
	Name       string `json:"name"`
	Action     string `json:"action"`
	Outcome    string `json:"outcome,omitempty"`
	Validation bool   `json:"validation"`
}

type PlanChange struct {
	Name   string    `json:"name"`
	Before *PlanStep `json:"before,omitempty"`
	After  *PlanStep `json:"after,omitempty"`
}

type PlanDelta struct {
	Added   []PlanStep   `json:"added"`
	Removed []PlanStep   `json:"removed"`
	Changed []PlanChange `json:"changed"`
	Summary string       `json:"summary"`
}

type ScoreResult struct {
	Score      float64
	Confidence float64
	Reasoning  []string
	Status     Status
}
