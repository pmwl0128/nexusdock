package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
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

type workflowTemplateDetail struct {
	workflowTemplateSummary
	Content string         `json:"content"`
	JSON    map[string]any `json:"json,omitempty"`
}

func (s *Server) listRuntimeWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	locationFilter := strings.TrimSpace(r.URL.Query().Get("location"))
	includeHistory := r.URL.Query().Get("include_history") == "true" || r.URL.Query().Get("view") == "history"
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	body, err := s.runtimeGet(r.Context(), "/internal/runtime/workflows", nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, runtimeUnavailablePayload(err))
		return
	}
	all := make([]workflowTemplateSummary, 0, len(opsArray(body["templates"])))
	for _, raw := range opsArray(body["templates"]) {
		item := workflowTemplateSummaryFromRuntime(opsMap(raw))
		if item.ID == "" {
			continue
		}
		if locationFilter != "" && item.Location != locationFilter {
			continue
		}
		all = append(all, item)
	}
	counters := workflowTemplateCounters(all)
	items := all
	mode := "history"
	if locationFilter == "" && !includeHistory {
		items = currentWorkflowTemplates(all)
		mode = "current"
	}
	if query != "" {
		filtered := make([]workflowTemplateSummary, 0, len(items))
		for _, item := range items {
			if templateSummaryMatches(item, query) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	sortWorkflowTemplates(items)
	conflicts := 0
	for _, counter := range counters {
		if counter.Active > 1 {
			conflicts++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "count": len(items), "total_count": len(all), "root": "agentdock-runtime-api", "source": "agentdock-runtime-api", "mode": mode, "conflict_count": conflicts, "version_summary": counters})
}

func (s *Server) runtimeWorkflowTemplateDetail(w http.ResponseWriter, r *http.Request) {
	_, fileName, err := workflowTemplateParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
		return
	}
	id, version, err := workflowTemplateIDVersion(fileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
		return
	}
	body, err := s.runtimeGet(r.Context(), "/internal/runtime/workflows/"+id+"/"+version, nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, runtimeUnavailablePayload(err))
		return
	}
	template := opsMap(body["template"])
	content, _ := json.MarshalIndent(template, "", "  ")
	detail := workflowTemplateDetail{workflowTemplateSummary: workflowTemplateSummaryFromRuntime(template), Content: string(content), JSON: template}
	allBody, _ := s.runtimeGet(r.Context(), "/internal/runtime/workflows", nil)
	all := workflowTemplateSummariesFromRuntime(allBody)
	attachWorkflowTemplateCounters(&detail.workflowTemplateSummary, all)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": detail, "source": "agentdock-runtime-api"})
}

func workflowTemplateSummariesFromRuntime(body map[string]any) []workflowTemplateSummary {
	items := make([]workflowTemplateSummary, 0, len(opsArray(body["templates"])))
	for _, raw := range opsArray(body["templates"]) {
		item := workflowTemplateSummaryFromRuntime(opsMap(raw))
		if item.ID != "" {
			items = append(items, item)
		}
	}
	return items
}

func workflowTemplateSummaryFromRuntime(item map[string]any) workflowTemplateSummary {
	id := opsString(item["id"])
	version := opsString(item["version"])
	status := opsString(item["status"])
	location := workflowLocationFromStatus(status)
	fileName := id + "@" + version + ".json"
	steps := opsArray(item["steps"])
	match := opsMap(item["match"])
	if len(steps) == 0 {
		steps = make([]any, opsInt(item["step_count"]))
	}
	return workflowTemplateSummary{ID: id, Version: version, Title: firstNonEmptyString(opsString(item["title"]), id), Description: opsString(item["description"]), Status: status, Location: location, FileName: fileName, Path: "agentdock-runtime-api/" + fileName, StepCount: len(steps), Keywords: opsStringArray(match["keywords"]), Current: status == "active" || status == "draft"}
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

func workflowTemplateIDVersion(fileName string) (string, string, error) {
	fileName = strings.TrimSuffix(strings.TrimSpace(fileName), ".json")
	parts := strings.SplitN(fileName, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("workflow template file name must be <id>@<version>.json")
	}
	return parts[0], parts[1], nil
}

func workflowTemplateParams(r *http.Request) (string, string, error) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/runtime/workflow-templates/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", "", errors.New("template location and file name are required")
	}
	location := strings.TrimSpace(parts[0])
	fileName := strings.TrimSpace(parts[1])
	if location == "" || fileName == "" || strings.Contains(fileName, "/") {
		return "", "", errors.New("template location and file name are required")
	}
	return location, fileName, nil
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
