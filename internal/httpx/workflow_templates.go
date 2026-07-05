package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var workflowFileNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*@[0-9]+\.[0-9]+\.[0-9]+\.json$`)

type workflowTemplateSummary struct {
	ID          string    `json:"id"`
	Version     string    `json:"version"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Location    string    `json:"location"`
	FileName    string    `json:"file_name"`
	Path        string    `json:"path"`
	SizeBytes   int64     `json:"size_bytes"`
	UpdatedAt   time.Time `json:"updated_at"`
	StepCount   int       `json:"step_count"`
	Keywords    []string  `json:"keywords,omitempty"`
}

type workflowTemplateDetail struct {
	workflowTemplateSummary
	Content string         `json:"content"`
	JSON    map[string]any `json:"json,omitempty"`
}

type workflowTemplateWriteRequest struct {
	Location string `json:"location"`
	FileName string `json:"file_name"`
	Content  string `json:"content"`
}

type workflowTemplateMoveRequest struct {
	Location string `json:"location"`
	FileName string `json:"file_name"`
	Target   string `json:"target"`
}

func (s *Server) listWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	root, err := s.workflowTemplatesRoot()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "WORKFLOW_DIR_UNAVAILABLE", err.Error())
		return
	}
	locationFilter := strings.TrimSpace(r.URL.Query().Get("location"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	var items []workflowTemplateSummary
	for _, location := range workflowTemplateLocations() {
		if locationFilter != "" && location != locationFilter {
			continue
		}
		dir := filepath.Join(root, location)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			writeError(w, http.StatusInternalServerError, "WORKFLOW_LIST_FAILED", err.Error())
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			summary, err := s.readWorkflowTemplateSummary(root, location, entry.Name())
			if err != nil {
				continue
			}
			if query != "" && !templateSummaryMatches(summary, query) {
				continue
			}
			items = append(items, summary)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		if items[i].Version != items[j].Version {
			return items[i].Version > items[j].Version
		}
		return items[i].Location < items[j].Location
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "count": len(items), "root": root})
}

func (s *Server) workflowTemplateDetail(w http.ResponseWriter, r *http.Request) {
	root, err := s.workflowTemplatesRoot()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "WORKFLOW_DIR_UNAVAILABLE", err.Error())
		return
	}
	location, fileName, err := workflowTemplateParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
		return
	}
	detail, err := s.readWorkflowTemplateDetail(root, location, fileName)
	if err != nil {
		writeError(w, http.StatusNotFound, "WORKFLOW_TEMPLATE_NOT_FOUND", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": detail})
}

func (s *Server) saveWorkflowTemplate(w http.ResponseWriter, r *http.Request) {
	root, err := s.workflowTemplatesRoot()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "WORKFLOW_DIR_UNAVAILABLE", err.Error())
		return
	}
	location, fileName, err := workflowTemplateParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
		return
	}
	var req workflowTemplateWriteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "WORKFLOW_TEMPLATE_EMPTY", "template content is required")
		return
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		writeError(w, http.StatusBadRequest, "WORKFLOW_TEMPLATE_INVALID_JSON", err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Join(root, location), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_SAVE_FAILED", err.Error())
		return
	}
	path := filepath.Join(root, location, fileName)
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_SAVE_FAILED", err.Error())
		return
	}
	detail, err := s.readWorkflowTemplateDetail(root, location, fileName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_SAVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": detail})
}

func (s *Server) createWorkflowTemplate(w http.ResponseWriter, r *http.Request) {
	root, err := s.workflowTemplatesRoot()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "WORKFLOW_DIR_UNAVAILABLE", err.Error())
		return
	}
	var req workflowTemplateWriteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	location, err := cleanWorkflowLocation(req.Location)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_LOCATION", err.Error())
		return
	}
	fileName, err := cleanWorkflowFileName(req.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_FILE", err.Error())
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "WORKFLOW_TEMPLATE_EMPTY", "template content is required")
		return
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		writeError(w, http.StatusBadRequest, "WORKFLOW_TEMPLATE_INVALID_JSON", err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Join(root, location), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_SAVE_FAILED", err.Error())
		return
	}
	path := filepath.Join(root, location, fileName)
	if _, err := os.Stat(path); err == nil {
		writeError(w, http.StatusConflict, "WORKFLOW_TEMPLATE_EXISTS", "template file already exists")
		return
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_SAVE_FAILED", err.Error())
		return
	}
	detail, err := s.readWorkflowTemplateDetail(root, location, fileName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_SAVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": detail})
}

func (s *Server) moveWorkflowTemplate(w http.ResponseWriter, r *http.Request) {
	root, err := s.workflowTemplatesRoot()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "WORKFLOW_DIR_UNAVAILABLE", err.Error())
		return
	}
	var req workflowTemplateMoveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	location, err := cleanWorkflowLocation(req.Location)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_LOCATION", err.Error())
		return
	}
	target, err := cleanWorkflowLocation(req.Target)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TARGET", err.Error())
		return
	}
	fileName, err := cleanWorkflowFileName(req.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_FILE", err.Error())
		return
	}
	source := filepath.Join(root, location, fileName)
	content, err := os.ReadFile(source)
	if err != nil {
		writeError(w, http.StatusNotFound, "WORKFLOW_TEMPLATE_NOT_FOUND", err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Join(root, target), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_MOVE_FAILED", err.Error())
		return
	}
	destination := filepath.Join(root, target, fileName)
	if err := os.WriteFile(destination, content, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_MOVE_FAILED", err.Error())
		return
	}
	if target == "retired" && location != "retired" {
		_ = os.Remove(source)
	}
	detail, err := s.readWorkflowTemplateDetail(root, target, fileName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_TEMPLATE_MOVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": detail})
}

func (s *Server) workflowTemplatesRoot() (string, error) {
	s.mu.RLock()
	configured := strings.TrimSpace(s.cfg.WorkflowDir)
	s.mu.RUnlock()
	if configured == "" {
		return "", errors.New("NEXUS_WORKFLOW_DIR is not configured")
	}
	root, err := filepath.Abs(configured)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workflow root is not a directory: %s", root)
	}
	return root, nil
}

func (s *Server) readWorkflowTemplateSummary(root, location, fileName string) (workflowTemplateSummary, error) {
	detail, err := s.readWorkflowTemplateDetail(root, location, fileName)
	if err != nil {
		return workflowTemplateSummary{}, err
	}
	return detail.workflowTemplateSummary, nil
}

func (s *Server) readWorkflowTemplateDetail(root, location, fileName string) (workflowTemplateDetail, error) {
	location, err := cleanWorkflowLocation(location)
	if err != nil {
		return workflowTemplateDetail{}, err
	}
	fileName, err = cleanWorkflowFileName(fileName)
	if err != nil {
		return workflowTemplateDetail{}, err
	}
	path := filepath.Join(root, location, fileName)
	content, err := os.ReadFile(path)
	if err != nil {
		return workflowTemplateDetail{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return workflowTemplateDetail{}, err
	}
	var body map[string]any
	_ = json.Unmarshal(content, &body)
	id := stringFromMap(body, "id")
	version := stringFromMap(body, "version")
	status := stringFromMap(body, "status")
	if status == "" {
		status = location
	}
	steps := arrayFromMap(body, "steps")
	match := mapFromMap(body, "match")
	summary := workflowTemplateSummary{
		ID:          id,
		Version:     version,
		Title:       stringFromMap(body, "title"),
		Description: stringFromMap(body, "description"),
		Status:      status,
		Location:    location,
		FileName:    fileName,
		Path:        location + "/" + fileName,
		SizeBytes:   info.Size(),
		UpdatedAt:   info.ModTime(),
		StepCount:   len(steps),
		Keywords:    stringArrayFromMap(match, "keywords"),
	}
	return workflowTemplateDetail{workflowTemplateSummary: summary, Content: string(content), JSON: body}, nil
}

func workflowTemplateParams(r *http.Request) (string, string, error) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/workflow-templates/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", "", errors.New("template location and file name are required")
	}
	location, err := cleanWorkflowLocation(parts[0])
	if err != nil {
		return "", "", err
	}
	fileName, err := cleanWorkflowFileName(parts[1])
	if err != nil {
		return "", "", err
	}
	return location, fileName, nil
}

func cleanWorkflowLocation(value string) (string, error) {
	value = strings.TrimSpace(value)
	for _, allowed := range workflowTemplateLocations() {
		if value == allowed {
			return value, nil
		}
	}
	return "", errors.New("workflow location must be drafts, published, or retired")
}

func cleanWorkflowFileName(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = filepath.Base(value)
	if !workflowFileNamePattern.MatchString(value) {
		return "", errors.New("workflow template file must look like id@1.0.0.json")
	}
	return value, nil
}

func workflowTemplateLocations() []string {
	return []string{"drafts", "published", "retired"}
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

func stringFromMap(body map[string]any, key string) string {
	if value, ok := body[key].(string); ok {
		return value
	}
	return ""
}

func arrayFromMap(body map[string]any, key string) []any {
	if value, ok := body[key].([]any); ok {
		return value
	}
	return nil
}

func mapFromMap(body map[string]any, key string) map[string]any {
	if value, ok := body[key].(map[string]any); ok {
		return value
	}
	return nil
}

func stringArrayFromMap(body map[string]any, key string) []string {
	values := arrayFromMap(body, key)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}
