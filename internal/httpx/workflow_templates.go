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
	FileName     string    `json:"file_name"`
	Path         string    `json:"path"`
	SizeBytes    int64     `json:"size_bytes"`
	UpdatedAt    time.Time `json:"updated_at"`
	StepCount    int       `json:"step_count"`
	Keywords     []string  `json:"keywords,omitempty"`
	VersionCount int       `json:"version_count,omitempty"`
	ActiveCount  int       `json:"active_count,omitempty"`
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
	summary.Path = filepath.ToSlash(filepath.Join("workflow-templates", "published", summary.FileName))
	info, err := os.Stat(s.workflowTemplatePath("published", summary.ID, summary.Version))
	if err != nil {
		return
	}
	summary.SizeBytes = info.Size()
	summary.UpdatedAt = info.ModTime().UTC()
}

func templateSummaryMatches(summary workflowTemplateSummary, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{summary.ID, summary.Version, summary.Title, summary.Description, summary.Status, summary.FileName}, " "))
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
