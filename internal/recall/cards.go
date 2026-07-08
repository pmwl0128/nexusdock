package recall

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CardType string

const (
	CardPreference   CardType = "preference"
	CardRunbook      CardType = "runbook"
	CardBugPattern   CardType = "bug_pattern"
	CardDeployNote   CardType = "deploy_note"
	CardProjectTrap  CardType = "project_trap"
	CardArchitecture CardType = "architecture"
	CardDecision     CardType = "decision"
	CardAntiPattern  CardType = "anti_pattern"
)

func (t CardType) Valid() bool {
	switch t {
	case CardPreference, CardRunbook, CardBugPattern, CardDeployNote, CardProjectTrap, CardArchitecture, CardDecision, CardAntiPattern:
		return true
	default:
		return false
	}
}

type CardRequest struct {
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Summary       string   `json:"summary"`
	Type          CardType `json:"type"`
	Scope         Scope    `json:"scope"`
	Project       string   `json:"project"`
	Status        Status   `json:"status"`
	Confidence    string   `json:"confidence"`
	Tags          []string `json:"tags"`
	Source        string   `json:"source"`
	Evidence      string   `json:"evidence"`
	Boundary      string   `json:"boundary"`
	Path          string   `json:"path"`
	Confirmed     bool     `json:"confirmed"`
	Overwrite     bool     `json:"overwrite"`
	AllowWarnings bool     `json:"allow_warnings"`
	MaxResults    int      `json:"max_results"`
}

type Card struct {
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Type       CardType `json:"type"`
	Scope      Scope    `json:"scope"`
	Project    string   `json:"project"`
	Status     Status   `json:"status"`
	Confidence string   `json:"confidence"`
	Tags       []string `json:"tags,omitempty"`
	Source     string   `json:"source"`
	Evidence   string   `json:"evidence,omitempty"`
	Boundary   string   `json:"boundary,omitempty"`
	Path       string   `json:"path"`
}

type CardCaptureResult struct {
	OK             bool           `json:"ok"`
	Card           Card           `json:"card"`
	Warnings       []string       `json:"warnings,omitempty"`
	CapturePlan    map[string]any `json:"capture_plan"`
	SimilarResults []SearchResult `json:"similar_results,omitempty"`
	SimilarCount   int            `json:"similar_count"`
}

type CardWriteResult struct {
	OK          bool     `json:"ok"`
	Card        Card     `json:"card"`
	Warnings    []string `json:"warnings,omitempty"`
	Recall      Recall   `json:"recall"`
	IndexPolicy string   `json:"index_policy"`
}

func (s *Store) CaptureCard(req CardRequest) (CardCaptureResult, error) {
	card, warnings, err := normalizeCard(req)
	if err != nil {
		return CardCaptureResult{}, err
	}
	maxResults := req.MaxResults
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 8
	}
	query := strings.Join(append([]string{card.Title, card.Content, card.Project, string(card.Type)}, card.Tags...), " ")
	similar, searchErr := s.Search(query, "recall/managed/cards", maxResults)
	if searchErr != nil {
		similar = []SearchResult{}
	}
	action := "create_card"
	reason := "no similar card found"
	if len(similar) > 0 {
		action = "review_similar_then_merge_or_supersede"
		reason = "similar cards found; review before writing to avoid duplicates"
	}
	if len(warnings) > 0 {
		action = "review_before_write"
		reason = "candidate has warnings"
	}
	plan := map[string]any{
		"recommended_action": action,
		"reason":             reason,
		"auto_write":         false,
		"needs_review":       true,
		"target_path":        card.Path,
		"write_endpoint":     "POST /v1/recall/cards",
		"write_status":       string(card.Status),
	}
	return CardCaptureResult{OK: true, Card: card, Warnings: warnings, CapturePlan: plan, SimilarResults: similar, SimilarCount: len(similar)}, nil
}

func (s *Store) WriteCard(req CardRequest) (CardWriteResult, error) {
	if !req.Confirmed {
		return CardWriteResult{}, ErrConfirmationNeeded
	}
	card, warnings, err := normalizeCard(req)
	if err != nil {
		return CardWriteResult{}, err
	}
	if len(warnings) > 0 && !req.AllowWarnings {
		return CardWriteResult{}, fmt.Errorf("card review required: %s", strings.Join(warnings, "; "))
	}
	mem, err := s.Write(WriteRequest{Path: card.Path, Content: renderCardMarkdown(card), Confirmed: true, Overwrite: req.Overwrite})
	if err != nil {
		return CardWriteResult{}, err
	}
	return CardWriteResult{OK: true, Card: card, Warnings: warnings, Recall: mem, IndexPolicy: "cards are indexed through Recall search over path, title, frontmatter and body; external embedding index may rebuild from recall/managed/cards/"}, nil
}

func normalizeCard(req CardRequest) (Card, []string, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = strings.TrimSpace(req.Summary)
	}
	card := Card{
		Title:      strings.TrimSpace(req.Title),
		Content:    content,
		Type:       req.Type,
		Scope:      req.Scope,
		Project:    SafeSegment(req.Project),
		Status:     req.Status,
		Confidence: strings.ToLower(strings.TrimSpace(req.Confidence)),
		Tags:       normalizeCardTags(req.Tags),
		Source:     strings.TrimSpace(req.Source),
		Evidence:   strings.TrimSpace(req.Evidence),
		Boundary:   strings.TrimSpace(req.Boundary),
	}
	if card.Title == "" {
		return Card{}, nil, errors.New("title is required")
	}
	if card.Content == "" {
		return Card{}, nil, errors.New("content or summary is required")
	}
	if card.Type == "" {
		card.Type = CardRunbook
	}
	if !card.Type.Valid() {
		return Card{}, nil, fmt.Errorf("invalid card type %q", card.Type)
	}
	if card.Scope == "" {
		card.Scope = ScopeProject
	}
	if !card.Scope.Valid() || card.Scope == ScopeNotes || card.Scope == ScopeInbox || card.Scope == ScopeProfile || card.Scope == ScopeOps || card.Scope == ScopeAgent {
		return Card{}, nil, fmt.Errorf("invalid card scope %q", card.Scope)
	}
	if card.Project == "" {
		card.Project = "global"
	}
	if card.Status == "" {
		card.Status = StatusInbox
	}
	if !card.Status.Valid() {
		return Card{}, nil, fmt.Errorf("invalid card status %q", card.Status)
	}
	if card.Confidence == "" {
		card.Confidence = string(ConfidenceMedium)
	}
	if !Confidence(card.Confidence).Valid() {
		return Card{}, nil, fmt.Errorf("invalid card confidence %q", card.Confidence)
	}
	if card.Source == "" {
		card.Source = "current conversation"
	}
	card.Path = filepath.ToSlash(strings.TrimSpace(req.Path))
	if card.Path == "" {
		card.Path = defaultCardPath(card)
	}
	if !IsAllowedRecallPath(card.Path) || !strings.HasPrefix(card.Path, "recall/managed/cards/") {
		return Card{}, nil, ErrDisallowedPath
	}
	warnings := cardWarnings(card)
	sort.Strings(warnings)
	return card, warnings, nil
}

func cardWarnings(card Card) []string {
	warnings := []string{}
	contentRunes := []rune(card.Content)
	if len(contentRunes) > 500 {
		warnings = append(warnings, "content is longer than 500 runes; split it into smaller cards")
	}
	if len(contentRunes) < 20 {
		warnings = append(warnings, "content is very short; make the reusable action explicit")
	}
	if card.Status == StatusActive || card.Status == StatusVerified {
		if card.Evidence == "" {
			warnings = append(warnings, "active/verified cards should include evidence")
		}
	}
	combined := strings.ToLower(card.Title + "\n" + card.Content + "\n" + card.Evidence)
	for _, marker := range []string{"private key", "access token", "password", "cookie", "credential"} {
		if strings.Contains(combined, marker) {
			warnings = append(warnings, "content may contain credential material")
			break
		}
	}
	for _, marker := range []string{"当前端口", "现在运行", "刚才日志", "临时", "一次性", "today", "now running"} {
		if strings.Contains(combined, strings.ToLower(marker)) {
			warnings = append(warnings, "content may describe temporary fact-layer state instead of reusable experience")
			break
		}
	}
	return uniqueCardWarnings(warnings)
}

func defaultCardPath(card Card) string {
	slug := SafeSegment(card.Title)
	if slug == "" {
		slug = time.Now().Format("20060102-150405")
	}
	return filepath.ToSlash(filepath.Join("recall", "managed", "cards", card.Project, string(card.Status), string(card.Type), slug+".md"))
}

func renderCardMarkdown(card Card) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: recall-card\n")
	b.WriteString("card_type: " + string(card.Type) + "\n")
	b.WriteString("scope: " + string(card.Scope) + "\n")
	b.WriteString("project: " + card.Project + "\n")
	b.WriteString("status: " + string(card.Status) + "\n")
	b.WriteString("confidence: " + card.Confidence + "\n")
	b.WriteString("source: " + quoteCardYAML(card.Source) + "\n")
	b.WriteString("created_at: " + now() + "\n")
	b.WriteString("updated_at: " + now() + "\n")
	if len(card.Tags) > 0 {
		b.WriteString("tags: " + strings.Join(card.Tags, ",") + "\n")
	}
	if card.Evidence != "" {
		b.WriteString("evidence: " + quoteCardYAML(card.Evidence) + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString("# " + card.Title + "\n\n")
	b.WriteString(card.Content + "\n")
	if card.Boundary != "" {
		b.WriteString("\n## 使用边界\n\n")
		b.WriteString(card.Boundary + "\n")
	}
	return b.String()
}

func normalizeCardTags(tags []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, tag := range tags {
		tag = strings.ToLower(strings.Trim(strings.TrimSpace(tag), "#，,;； "))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func uniqueCardWarnings(warnings []string) []string {
	seen := map[string]bool{}
	out := warnings[:0]
	for _, warning := range warnings {
		if warning == "" || seen[warning] {
			continue
		}
		seen[warning] = true
		out = append(out, warning)
	}
	return out
}

func quoteCardYAML(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ").Replace(value))
	if value == "" {
		return "\"\""
	}
	return fmt.Sprintf("%q", value)
}
