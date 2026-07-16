package httpx

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

type workflowTemplateSummary struct {
	ID           string    `json:"id"`
	Version      string    `json:"version"`
	Title        string    `json:"title"`
	Description  string    `json:"description,omitempty"`
	Status       string    `json:"status"`
	Location     string    `json:"location"`
	FileName     string    `json:"file_name"`
	Path         string    `json:"path"`
	SizeBytes    int64     `json:"size_bytes"`
	UpdatedAt    time.Time `json:"updated_at"`
	StepCount    int       `json:"step_count"`
	Keywords     []string  `json:"keywords,omitempty"`
	Current      bool      `json:"current"`
	VersionCount int       `json:"version_count,omitempty"`
	ActiveCount  int       `json:"active_count,omitempty"`
	DraftCount   int       `json:"draft_count,omitempty"`
	RetiredCount int       `json:"retired_count,omitempty"`
	HasConflict  bool      `json:"has_conflict,omitempty"`
}

func (s *Server) workflowTemplateSummary(t workflowTemplate) workflowTemplateSummary {
	summary := workflowTemplateSummaryFromTemplate(t)
	s.attachWorkflowTemplateFileMetadata(&summary)
	return summary
}

func (s *Server) attachWorkflowTemplateFileMetadata(summary *workflowTemplateSummary) {
	if summary == nil || summary.ID == "" || summary.Version == "" {
		return
	}
	area := workflowTemplateStorageArea(summary.Status)
	summary.Path = filepath.ToSlash(filepath.Join("workflow-templates", area, summary.FileName))
	info, err := os.Stat(s.workflowTemplatePath(area, summary.ID, summary.Version))
	if err != nil {
		return
	}
	summary.SizeBytes = info.Size()
	summary.UpdatedAt = info.ModTime().UTC()
}

func workflowLocationFromStatus(status string) string {
	switch status {
	case "draft", "validated":
		return "drafts"
	case "retired":
		return "retired"
	default:
		return "published"
	}
}

func templateSummaryMatches(summary workflowTemplateSummary, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{summary.ID, summary.Version, summary.Title, summary.Description, summary.Status, summary.Location, summary.FileName}, " "))
	if strings.Contains(haystack, query) {
		return true
	}
	for _, keyword := range summary.Keywords {
		if strings.Contains(strings.ToLower(keyword), query) {
			return true
		}
	}
	return false
}
