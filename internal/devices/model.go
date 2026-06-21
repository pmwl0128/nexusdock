package devices

import (
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusOnline   Status = "online"
	StatusDegraded Status = "degraded"
	StatusOffline  Status = "offline"
	StatusRevoked  Status = "revoked"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type ReleaseChannel string

const (
	ChannelStable ReleaseChannel = "stable"
	ChannelCanary ReleaseChannel = "canary"
)

type Policy struct {
	AllowedCommandTypes []string       `json:"allowed_command_types"`
	MaxRisk             RiskLevel      `json:"max_risk"`
	ReleaseChannel      ReleaseChannel `json:"release_channel"`
	AutoUpdate          bool           `json:"auto_update"`
	AllowedSkills       []string       `json:"allowed_skills,omitempty"`
}

func DefaultPolicy() Policy {
	return Policy{
		AllowedCommandTypes: []string{"health.check", "diagnostics.collect"},
		MaxRisk:             RiskLow,
		ReleaseChannel:      ChannelStable,
		AllowedSkills:       []string{},
	}
}

func (p Policy) Validate() error {
	if _, ok := riskRank[p.MaxRisk]; !ok {
		return domainError(ErrValidation, "unknown max risk %q", p.MaxRisk)
	}
	if p.ReleaseChannel != ChannelStable && p.ReleaseChannel != ChannelCanary {
		return domainError(ErrValidation, "unknown release channel %q", p.ReleaseChannel)
	}
	seen := make(map[string]struct{}, len(p.AllowedCommandTypes))
	for _, commandType := range p.AllowedCommandTypes {
		commandType = strings.TrimSpace(commandType)
		if commandType == "" {
			return domainError(ErrValidation, "allowed command type cannot be empty")
		}
		if _, duplicate := seen[commandType]; duplicate {
			return domainError(ErrValidation, "duplicate allowed command type %q", commandType)
		}
		seen[commandType] = struct{}{}
	}
	return nil
}

func (p Policy) AllowsCommand(commandType string, risk RiskLevel) bool {
	requestedRank, ok := riskRank[risk]
	if !ok {
		return false
	}
	maxRank, ok := riskRank[p.MaxRisk]
	if !ok || requestedRank > maxRank {
		return false
	}
	for _, allowed := range p.AllowedCommandTypes {
		if allowed == commandType {
			return true
		}
	}
	return false
}

func (p Policy) AllowsSkill(skill string) bool {
	if len(p.AllowedSkills) == 0 {
		return false
	}
	for _, allowed := range p.AllowedSkills {
		if allowed == "*" || allowed == skill {
			return true
		}
	}
	return false
}

var riskRank = map[RiskLevel]int{
	RiskLow: 1, RiskMedium: 2, RiskHigh: 3,
}

type Capability struct {
	Name     string            `json:"name"`
	Version  string            `json:"version,omitempty"`
	Enabled  bool              `json:"enabled"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Metrics struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskPercent   float64 `json:"disk_percent"`
}

type SkillSummary struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Channel string `json:"channel,omitempty"`
	Active  bool   `json:"active"`
}

type RecallSyncSummary struct {
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	Pending       int        `json:"pending"`
	Conflicts     int        `json:"conflicts"`
	LastError     string     `json:"last_error,omitempty"`
}

type Heartbeat struct {
	DeviceID         string            `json:"device_id"`
	SentAt           time.Time         `json:"sent_at"`
	ReceivedAt       time.Time         `json:"received_at"`
	UptimeSeconds    int64             `json:"uptime_seconds"`
	AgentDockVersion string            `json:"agentdock_version"`
	Metrics          Metrics           `json:"metrics"`
	Capabilities     []Capability      `json:"capabilities"`
	Skills           []SkillSummary    `json:"skills"`
	RecallSync       RecallSyncSummary `json:"recall_sync"`
}

type Device struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Platform             string            `json:"platform"`
	Arch                 string            `json:"arch"`
	PublicKey            string            `json:"public_key"`
	Labels               map[string]string `json:"labels"`
	Policy               Policy            `json:"policy"`
	Status               Status            `json:"status"`
	AgentDockVersion     string            `json:"agentdock_version,omitempty"`
	Capabilities         []Capability      `json:"capabilities,omitempty"`
	LastSeen             *time.Time        `json:"last_seen,omitempty"`
	ApprovedAt           *time.Time        `json:"approved_at,omitempty"`
	RevokedAt            *time.Time        `json:"revoked_at,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	Version              int64             `json:"version"`
	DeviceTokenDigest    string            `json:"-"`
	DeviceTokenExpiresAt time.Time         `json:"-"`
}

func (d Device) IsApproved() bool { return d.ApprovedAt != nil && d.RevokedAt == nil }

type EnrollmentToken struct {
	ID        string     `json:"id"`
	Digest    string     `json:"-"`
	Policy    Policy     `json:"policy"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

type EnrollmentRequest struct {
	Token            string            `json:"enrollment_token"`
	Name             string            `json:"name"`
	Platform         string            `json:"platform"`
	Arch             string            `json:"arch"`
	Labels           map[string]string `json:"labels,omitempty"`
	AgentDockVersion string            `json:"agentdock_version,omitempty"`
	PublicKey        string            `json:"public_key"`
}

type EnrollmentResult struct {
	Device                   Device    `json:"device"`
	DeviceToken              string    `json:"device_token"`
	TokenExpiresAt           time.Time `json:"token_expires_at"`
	HeartbeatIntervalSeconds int       `json:"heartbeat_interval_seconds"`
	ServerTime               time.Time `json:"server_time"`
}

type TokenResult struct {
	Token      EnrollmentToken `json:"enrollment"`
	PlainToken string          `json:"token"`
}

type DeviceCredential struct {
	DeviceToken    string    `json:"device_token"`
	TokenExpiresAt time.Time `json:"token_expires_at"`
}

type Snapshot struct {
	Device    Device     `json:"device"`
	Heartbeat *Heartbeat `json:"heartbeat,omitempty"`
}

func clonePolicy(p Policy) Policy {
	p.AllowedCommandTypes = append([]string(nil), p.AllowedCommandTypes...)
	p.AllowedSkills = append([]string(nil), p.AllowedSkills...)
	return p
}

func cloneCapabilities(values []Capability) []Capability {
	result := make([]Capability, len(values))
	for i, value := range values {
		result[i] = value
		if value.Metadata != nil {
			result[i].Metadata = make(map[string]string, len(value.Metadata))
			for key, item := range value.Metadata {
				result[i].Metadata[key] = item
			}
		}
	}
	return result
}

func cloneDevice(d Device) Device {
	d.Labels = cloneMap(d.Labels)
	d.Policy = clonePolicy(d.Policy)
	d.Capabilities = cloneCapabilities(d.Capabilities)
	return d
}

func cloneHeartbeat(h Heartbeat) Heartbeat {
	h.Capabilities = cloneCapabilities(h.Capabilities)
	h.Skills = append([]SkillSummary(nil), h.Skills...)
	return h
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func normalizeLabels(labels map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(labels))
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(labels[key])
		if trimmedKey == "" || len(trimmedKey) > 64 || len(trimmedValue) > 256 {
			return nil, domainError(ErrValidation, "invalid label %q", key)
		}
		result[trimmedKey] = trimmedValue
	}
	return result, nil
}
