package recall

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ProposeUpdateRequest struct {
	Path              string     `json:"path"`
	Content           string     `json:"content"`
	Scope             Scope      `json:"scope"`
	Status            Status     `json:"status"`
	Project           string     `json:"project,omitempty"`
	Device            string     `json:"device,omitempty"`
	Agent             string     `json:"agent,omitempty"`
	Skill             string     `json:"skill,omitempty"`
	Source            string     `json:"source"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	VerificationRunID string     `json:"verification_run_id,omitempty"`
	SourceDevice      string     `json:"source_device,omitempty"`
	SourceAgent       string     `json:"source_agent,omitempty"`
	Confidence        Confidence `json:"confidence"`
}

type UpdateProposal struct {
	ID              string    `json:"id"`
	Path            string    `json:"path"`
	PreviousDigest  string    `json:"previous_digest,omitempty"`
	ProposedDigest  string    `json:"proposed_digest"`
	ProposedContent string    `json:"proposed_content"`
	Metadata        Metadata  `json:"metadata"`
	Diff            string    `json:"diff"`
	CreatedAt       time.Time `json:"created_at"`
}

type ApplyUpdateRequest struct {
	Proposal UpdateProposal `json:"proposal"`
	Approved bool           `json:"approved"`
}

func digestString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func renderFrontmatter(body string, meta Metadata, existing map[string]string) string {
	values := map[string]string{}
	for key, value := range existing {
		values[key] = value
	}
	values["scope"] = string(meta.Scope)
	values["status"] = string(meta.Status)
	put := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			delete(values, key)
			return
		}
		values[key] = strings.TrimSpace(value)
	}
	put("project", meta.Project)
	put("device", meta.Device)
	put("agent", meta.Agent)
	put("skill", meta.Skill)
	put("source", meta.Source)
	put("verification_run_id", meta.Verification.VerificationRunID)
	put("source_device", meta.Verification.SourceDevice)
	put("source_agent", meta.Verification.SourceAgent)
	if meta.Verification.VerifiedAt != nil {
		values["verified_at"] = meta.Verification.VerifiedAt.UTC().Format(time.RFC3339)
	} else {
		delete(values, "verified_at")
	}
	if meta.Verification.Confidence.Valid() {
		values["confidence"] = string(meta.Verification.Confidence)
	}
	if _, ok := values["created_at"]; !ok {
		values["created_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	values["updated_at"] = time.Now().UTC().Format(time.RFC3339)

	ordered := []string{"type", "scope", "status", "project", "device", "agent", "skill", "source", "confidence", "verified_at", "verification_run_id", "source_device", "source_agent", "created_at", "updated_at", "tags"}
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range ordered {
		if value, ok := values[key]; ok && strings.TrimSpace(value) != "" {
			b.WriteString(key + ": " + value + "\n")
			seen[key] = true
		}
	}
	for _, key := range sortedMapKeys(values) {
		if seen[key] || strings.TrimSpace(values[key]) == "" {
			continue
		}
		b.WriteString(key + ": " + values[key] + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")
	return b.String()
}

func simpleUnifiedDiff(path, oldContent, newContent string) string {
	if oldContent == newContent {
		return ""
	}
	return fmt.Sprintf("--- a/%s\n+++ b/%s\n@@\n-%s\n+%s\n", path, path,
		strings.ReplaceAll(strings.TrimSuffix(oldContent, "\n"), "\n", "\n-"),
		strings.ReplaceAll(strings.TrimSuffix(newContent, "\n"), "\n", "\n+"))
}

func transientSource(source string) bool {
	s := strings.ToLower(strings.TrimSpace(source))
	return strings.Contains(s, "temporary") || strings.Contains(s, "temp-log") || strings.Contains(s, "diagnostic-log") || s == "log"
}

func validateProposal(req ProposeUpdateRequest) error {
	if strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.Content) == "" {
		return errors.New("path and content are required")
	}
	if !IsAllowedRecallPath(req.Path) {
		return ErrDisallowedPath
	}
	meta := Metadata{
		Scope: req.Scope, Status: req.Status, Project: req.Project, Device: req.Device, Agent: req.Agent,
		Skill: req.Skill, Source: req.Source,
		Verification: VerificationMetadata{VerifiedAt: req.VerifiedAt, VerificationRunID: req.VerificationRunID, SourceDevice: req.SourceDevice, SourceAgent: req.SourceAgent, Confidence: req.Confidence},
	}
	if err := validateMetadata(meta); err != nil {
		return err
	}
	if req.Scope != ScopeInbox && transientSource(req.Source) {
		return errors.New("temporary logs may only be written to inbox scope")
	}
	return nil
}

func proposalID(path, digest string) string {
	sum := sha256.Sum256([]byte(path + "\x00" + digest))
	return "mp_" + hex.EncodeToString(sum[:12])
}
