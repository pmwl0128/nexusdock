package recall

import (
	"strings"
	"testing"
)

func TestCardCaptureIsReviewOnlyAndFindsSimilar(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.WriteCard(CardRequest{
		Title:         "ChatDock deploy check",
		Content:       "ChatDock deploy verification should check the final public page, not only source files.",
		Type:          CardProjectTrap,
		Scope:         ScopeProject,
		Project:       "chatdock",
		Status:        StatusActive,
		Confidence:    string(ConfidenceHigh),
		Evidence:      "unit test fixture",
		Confirmed:     true,
		AllowWarnings: true,
	}); err != nil {
		t.Fatalf("write fixture card: %v", err)
	}
	before, _ := store.List("cards", 20)
	capture, err := store.CaptureCard(CardRequest{
		Title:   "ChatDock deploy check",
		Content: "ChatDock deploy verification should check the final service page before considering the task done.",
		Type:    CardProjectTrap,
		Scope:   ScopeProject,
		Project: "chatdock",
		Status:  StatusInbox,
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := store.List("cards", 20)
	if len(after) != len(before) {
		t.Fatalf("CaptureCard must not write: before=%d after=%d", len(before), len(after))
	}
	if capture.SimilarCount == 0 || capture.CapturePlan["auto_write"] != false {
		t.Fatalf("bad capture plan: %#v", capture)
	}
}

func TestCardWriteUsesCardsPathAndReviewGate(t *testing.T) {
	store := newTestStore(t)
	_, err := store.WriteCard(CardRequest{
		Title:     "Temporary port",
		Content:   "当前端口是 1234。",
		Project:   "demo",
		Confirmed: true,
	})
	if err == nil || !strings.Contains(err.Error(), "review required") {
		t.Fatalf("expected warning review gate, got %v", err)
	}

	result, err := store.WriteCard(CardRequest{
		Title:      "Fact layer check",
		Content:    "When debugging a project, historical experience is only a reminder; current compose, config, database, and runtime output must still be checked live.",
		Type:       CardRunbook,
		Scope:      ScopeProject,
		Project:    "rss-monitor",
		Status:     StatusInbox,
		Confidence: string(ConfidenceHigh),
		Tags:       []string{"debugging", "recall"},
		Confirmed:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Card.Path, "cards/rss-monitor/inbox/runbook/") {
		t.Fatalf("unexpected path: %s", result.Card.Path)
	}
	for _, want := range []string{"type: recall-card", "card_type: runbook", "project: rss-monitor", "status: inbox"} {
		if !strings.Contains(result.Recall.Content, want) {
			t.Fatalf("written card missing %q: %s", want, result.Recall.Content)
		}
	}
}
