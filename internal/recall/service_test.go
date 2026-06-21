package recall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFixture(t *testing.T, store *Store, path, content string) {
	t.Helper()
	abs := filepath.Join(store.Root(), filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataScopesAndVerification(t *testing.T) {
	store := newTestStore(t)
	writeFixture(t, store, "devices/dockmini.md", "---\nscope: device\nstatus: active\nsource_device: DockMini\nsource_agent: agent-1\nconfidence: high\nverified_at: 2026-06-05T12:00:00Z\nverification_run_id: run-1\n---\n\n# DockMini\n")
	svc, _ := NewService(store)
	record, err := svc.Read(context.Background(), "devices/dockmini.md")
	if err != nil {
		t.Fatal(err)
	}
	if record.Metadata.Scope != ScopeDevice || record.Metadata.Status != StatusActive {
		t.Fatalf("unexpected metadata: %#v", record.Metadata)
	}
	if record.Metadata.Verification.Confidence != ConfidenceHigh || record.Metadata.Verification.VerificationRunID != "run-1" {
		t.Fatalf("verification lost: %#v", record.Metadata.Verification)
	}
	if record.Metadata.Verification.VerifiedAt == nil {
		t.Fatal("verified_at was not parsed")
	}
}

func TestContextPackPriorityLimitAndDeprecatedExclusion(t *testing.T) {
	store := newTestStore(t)
	fixtures := map[string]string{
		"profile.md":                        "---\nscope: profile\nstatus: active\n---\n\n# Profile\nowner\n",
		"projects/nexus/project.md":         "---\nscope: project\nstatus: active\nproject: nexus\n---\n\n# Nexus\nproject facts\n",
		"projects/nexus/runbooks/deploy.md": "---\nscope: project\nstatus: active\nproject: nexus\n---\n\n# Deploy\nsteps\n",
		"devices/dockmini.md":               "---\nscope: device\nstatus: active\ndevice: dockmini\n---\n\n# Device\nstate\n",
		"ops/old.md":                        "---\nscope: ops\nstatus: deprecated\n---\n\n# Old\nignore\n",
	}
	for path, content := range fixtures {
		writeFixture(t, store, path, content)
	}
	svc, _ := NewService(store)
	pack, err := svc.BuildContextPack(context.Background(), ContextPackRequest{Project: "nexus", Device: "dockmini", MaxBytes: 180})
	if err != nil {
		t.Fatal(err)
	}
	if pack.TotalBytes > 180 {
		t.Fatalf("pack exceeded max: %d", pack.TotalBytes)
	}
	if len(pack.Sections) == 0 || pack.Sections[0].Kind != "profile" {
		t.Fatalf("unexpected priority: %#v", pack.Sections)
	}
	for _, section := range pack.Sections {
		if section.Path == "ops/old.md" {
			t.Fatal("deprecated recall entered context")
		}
	}
}

func TestConflictDetectionDeduplicatesAndIgnoresLowConfidence(t *testing.T) {
	store := newTestStore(t)
	writeFixture(t, store, "devices/dockmini.md", "# Device\nport: 18766\n")
	repo := NewInRecallConflictRepository()
	svc, _ := NewService(store, WithConflictRepository(repo))
	fact := ObservedFact{RecallPath: "devices/dockmini.md", Key: "port", RecallValue: "18766", ObservedValue: "18767", Source: ConflictSourceDeviceSnapshot, SourceID: "snapshot-1", Device: "dockmini", Confidence: ConfidenceHigh}
	first, err := svc.DetectConflict(context.Background(), DetectConflictRequest{Facts: []ObservedFact{fact}})
	if err != nil || len(first) != 1 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, _ := svc.DetectConflict(context.Background(), DetectConflictRequest{Facts: []ObservedFact{fact}})
	if len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("conflict id not stable: %#v %#v", first, second)
	}
	low := fact
	low.Confidence = ConfidenceLow
	low.ObservedValue = "18768"
	ignored, _ := svc.DetectConflict(context.Background(), DetectConflictRequest{Facts: []ObservedFact{low}})
	if len(ignored) != 0 {
		t.Fatalf("low confidence conflict accepted: %#v", ignored)
	}
	open, _ := repo.ListOpen(context.Background(), ContextPackRequest{Device: "dockmini"})
	if len(open) != 1 {
		t.Fatalf("repository did not deduplicate: %#v", open)
	}
}

func TestProposalApplyIsApprovedAndOptimistic(t *testing.T) {
	store := newTestStore(t)
	writeFixture(t, store, "projects/nexus/project.md", "---\nscope: project\nstatus: active\nproject: nexus\n---\n\n# Old\n")
	svc, _ := NewService(store)
	verified := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	proposal, err := svc.ProposeUpdate(context.Background(), ProposeUpdateRequest{Path: "projects/nexus/project.md", Content: "# New", Scope: ScopeProject, Status: StatusActive, Project: "nexus", Source: "user_edit", VerifiedAt: &verified, VerificationRunID: "run-2", Confidence: ConfidenceHigh})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Diff == "" || !strings.Contains(proposal.Diff, "# New") {
		t.Fatalf("missing diff: %s", proposal.Diff)
	}
	if _, err := svc.ApplyUpdate(context.Background(), ApplyUpdateRequest{Proposal: proposal}); err == nil {
		t.Fatal("unapproved update succeeded")
	}
	result, err := svc.ApplyUpdate(context.Background(), ApplyUpdateRequest{Proposal: proposal, Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metadata.Verification.VerificationRunID != "run-2" || !strings.Contains(result.Content, "# New") {
		t.Fatalf("bad applied record: %#v", result)
	}

	stale, _ := svc.ProposeUpdate(context.Background(), ProposeUpdateRequest{Path: "projects/nexus/project.md", Content: "# Later", Scope: ScopeProject, Status: StatusActive, Project: "nexus", Source: "user_edit", Confidence: ConfidenceMedium})
	writeFixture(t, store, "projects/nexus/project.md", "# concurrent edit\n")
	_, err = svc.ApplyUpdate(context.Background(), ApplyUpdateRequest{Proposal: stale, Approved: true})
	if err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("expected optimistic conflict, got %v", err)
	}
}

func TestTemporaryLogRejectedOutsideInbox(t *testing.T) {
	store := newTestStore(t)
	svc, _ := NewService(store)
	_, err := svc.ProposeUpdate(context.Background(), ProposeUpdateRequest{Path: "ops/log.md", Content: "temporary", Scope: ScopeOps, Status: StatusActive, Source: "diagnostic-log", Confidence: ConfidenceLow})
	if err == nil {
		t.Fatal("temporary log entered long-term recall")
	}
	if !errors.Is(err, ErrDisallowedPath) && !strings.Contains(err.Error(), "temporary logs") {
		t.Fatalf("unexpected error: %v", err)
	}
}
