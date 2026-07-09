package httpx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type workflowTemplateStatus string

const (
	workflowTemplateDraft   workflowTemplateStatus = "draft"
	workflowTemplateActive  workflowTemplateStatus = "active"
	workflowTemplateRetired workflowTemplateStatus = "retired"
)

type workflowMatchRule struct {
	Keywords []string `json:"keywords,omitempty"`
	Devices  []string `json:"devices,omitempty"`
	Type     string   `json:"type,omitempty"`
}

type workflowTemplateStep struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Phase string `json:"phase"`
}

type workflowTemplate struct {
	ID                   string                 `json:"id"`
	Version              string                 `json:"version"`
	Title                string                 `json:"title"`
	Description          string                 `json:"description,omitempty"`
	Status               workflowTemplateStatus `json:"status"`
	Match                workflowMatchRule      `json:"match,omitempty"`
	CompletionConditions []string               `json:"completion_conditions"`
	Steps                []workflowTemplateStep `json:"steps"`
	AllowLongTemplate    bool                   `json:"allow_long_template,omitempty"`
	LongTemplateReason   string                 `json:"long_template_reason,omitempty"`
	Hash                 string                 `json:"hash,omitempty"`
	PublishedAt          *time.Time             `json:"published_at,omitempty"`
	RetiredAt            *time.Time             `json:"retired_at,omitempty"`
}

type workflowTemplateCandidate struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Score   int    `json:"score"`
	Reason  string `json:"reason"`
}

var workflowRegistryMu sync.Mutex

func (s *Server) registerWorkflowTemplateRoutes(mux *http.ServeMux, protected func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /v1/workflow-templates", protected(s.workflowTemplatesList))
	mux.HandleFunc("POST /v1/workflow-templates/drafts", protected(s.workflowTemplateSaveDraft))
	mux.HandleFunc("POST /v1/workflow-templates/match", protected(s.workflowTemplatesMatch))
	mux.HandleFunc("POST /v1/workflow-templates/reindex", protected(s.workflowTemplatesReindex))
	mux.HandleFunc("GET /v1/workflow-templates/vector-index", protected(s.workflowTemplateVectorIndexRead))
	mux.HandleFunc("GET /v1/workflow-templates/", protected(s.workflowTemplateRead))
	mux.HandleFunc("POST /v1/workflow-templates/", protected(s.workflowTemplateAction))
}

func (s *Server) workflowRegistryRoot() string {
	root := strings.TrimSpace(s.cfg.NexusDataDir)
	if root == "" {
		root = filepath.Join(".", "nexus-data")
	}
	return filepath.Join(root, "workflow-templates")
}

func (s *Server) ensureWorkflowRegistryDirs() error {
	for _, dir := range []string{filepath.Join(s.workflowRegistryRoot(), "drafts"), filepath.Join(s.workflowRegistryRoot(), "published")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) workflowTemplatePath(area, id, version string) string {
	return filepath.Join(s.workflowRegistryRoot(), area, id+"@"+version+".json")
}

func (s *Server) workflowTemplateSaveDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Template           workflowTemplate `json:"template"`
		AllowLongTemplate  bool             `json:"allow_long_template"`
		LongTemplateReason string           `json:"long_template_reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	workflowRegistryMu.Lock()
	defer workflowRegistryMu.Unlock()
	t := req.Template
	if req.AllowLongTemplate {
		t.AllowLongTemplate = true
	}
	if strings.TrimSpace(req.LongTemplateReason) != "" {
		t.LongTemplateReason = strings.TrimSpace(req.LongTemplateReason)
	}
	t.Status = workflowTemplateDraft
	t.Hash = ""
	t.PublishedAt = nil
	t.RetiredAt = nil
	if err := s.ensureWorkflowRegistryDirs(); err != nil {
		writeError(w, http.StatusConflict, "WORKFLOW_REGISTRY_FAILED", err.Error())
		return
	}
	if err := validateWorkflowTemplate(t); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
		return
	}
	if _, err := os.Stat(s.workflowTemplatePath("published", t.ID, t.Version)); err == nil {
		writeError(w, http.StatusConflict, "WORKFLOW_VERSION_IMMUTABLE", "published template version is immutable; create a new version")
		return
	}
	if err := writeWorkflowTemplateJSON(s.workflowTemplatePath("drafts", t.ID, t.Version), t); err != nil {
		writeError(w, http.StatusConflict, "WORKFLOW_SAVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": t, "template_summary": s.workflowTemplateSummary(t), "root": s.workflowRegistryRoot(), "source": "nexus-registry"})
}

func (s *Server) workflowTemplateAction(w http.ResponseWriter, r *http.Request) {
	id, version, action, err := workflowTemplateActionParams(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
		return
	}
	workflowRegistryMu.Lock()
	defer workflowRegistryMu.Unlock()
	if err := s.ensureWorkflowRegistryDirs(); err != nil {
		writeError(w, http.StatusConflict, "WORKFLOW_REGISTRY_FAILED", err.Error())
		return
	}
	switch action {
	case "validate":
		t, err := s.loadWorkflowTemplate("drafts", id, version)
		if err != nil {
			writeError(w, http.StatusNotFound, "WORKFLOW_TEMPLATE_NOT_FOUND", err.Error())
			return
		}
		if err := validateWorkflowTemplate(t); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "valid": true, "template": t, "template_summary": s.workflowTemplateSummary(t), "source": "nexus-registry"})
	case "publish":
		t, err := s.loadWorkflowTemplate("drafts", id, version)
		if err != nil {
			writeError(w, http.StatusNotFound, "WORKFLOW_TEMPLATE_NOT_FOUND", err.Error())
			return
		}
		if err := validateWorkflowTemplate(t); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
			return
		}
		if _, err := os.Stat(s.workflowTemplatePath("published", id, version)); err == nil {
			writeError(w, http.StatusConflict, "WORKFLOW_VERSION_IMMUTABLE", "published template version already exists and cannot be overwritten")
			return
		}
		now := time.Now().UTC()
		if err := s.retireActiveWorkflowTemplates(id, version, now); err != nil {
			writeError(w, http.StatusConflict, "WORKFLOW_RETIRE_OLD_FAILED", err.Error())
			return
		}
		t.Status = workflowTemplateActive
		t.PublishedAt = &now
		t.RetiredAt = nil
		t.Hash = workflowTemplateHash(t)
		if err := writeWorkflowTemplateJSON(s.workflowTemplatePath("published", id, version), t); err != nil {
			writeError(w, http.StatusConflict, "WORKFLOW_PUBLISH_FAILED", err.Error())
			return
		}
		_ = os.Remove(s.workflowTemplatePath("drafts", id, version))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": t, "template_summary": s.workflowTemplateSummary(t), "source": "nexus-registry"})
	case "retire":
		t, err := s.loadWorkflowTemplate("published", id, version)
		if err != nil {
			writeError(w, http.StatusNotFound, "WORKFLOW_TEMPLATE_NOT_FOUND", err.Error())
			return
		}
		if t.Status != workflowTemplateActive {
			writeError(w, http.StatusBadRequest, "WORKFLOW_TEMPLATE_NOT_ACTIVE", "only active templates can be retired")
			return
		}
		now := time.Now().UTC()
		t.Status = workflowTemplateRetired
		t.RetiredAt = &now
		if err := writeWorkflowTemplateJSON(s.workflowTemplatePath("published", id, version), t); err != nil {
			writeError(w, http.StatusConflict, "WORKFLOW_RETIRE_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": t, "template_summary": s.workflowTemplateSummary(t), "source": "nexus-registry"})
	default:
		writeError(w, http.StatusNotFound, "WORKFLOW_ACTION_NOT_FOUND", "unsupported workflow template action")
	}
}

func (s *Server) workflowTemplateRead(w http.ResponseWriter, r *http.Request) {
	id, version, err := workflowTemplatePathParams(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW_TEMPLATE", err.Error())
		return
	}
	workflowRegistryMu.Lock()
	defer workflowRegistryMu.Unlock()
	t, err := s.getWorkflowTemplate(id, version)
	if err != nil {
		writeError(w, http.StatusNotFound, "WORKFLOW_TEMPLATE_NOT_FOUND", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": t, "template_summary": s.workflowTemplateSummary(t), "source": "nexus-registry"})
}

func (s *Server) workflowTemplatesList(w http.ResponseWriter, r *http.Request) {
	status := workflowTemplateStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	includeHistory := r.URL.Query().Get("include_history") == "true" || r.URL.Query().Get("view") == "history"
	templates, err := s.listWorkflowTemplates(status)
	if err != nil {
		writeError(w, http.StatusConflict, "WORKFLOW_LIST_FAILED", err.Error())
		return
	}
	summaries := make([]workflowTemplateSummary, 0, len(templates))
	for _, t := range templates {
		item := s.workflowTemplateSummary(t)
		if query != "" && !templateSummaryMatches(item, query) {
			continue
		}
		summaries = append(summaries, item)
	}
	counters := workflowTemplateCounters(summaries)
	items := summaries
	mode := "history"
	if status == "" && !includeHistory {
		items = currentWorkflowTemplates(summaries)
		mode = "current"
	}
	conflicts := 0
	for _, counter := range counters {
		if counter.Active > 1 {
			conflicts++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "templates": workflowTemplateCompactList(templates), "count": len(items), "total_count": len(summaries), "root": s.workflowRegistryRoot(), "source": "nexus-registry", "mode": mode, "conflict_count": conflicts, "version_summary": counters})
}

func (s *Server) workflowTemplatesMatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Goal   string `json:"goal"`
		Device string `json:"device"`
		Type   string `json:"type"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	candidates, err := s.matchWorkflowTemplates(req.Goal, req.Device, req.Type)
	if err != nil {
		writeError(w, http.StatusConflict, "WORKFLOW_MATCH_FAILED", err.Error())
		return
	}
	vectorStatus, vectorItems := s.workflowTemplateVectorIndexInfo()
	result := map[string]any{"ok": true, "action": "match", "candidates": candidates, "count": len(candidates), "workflow_dir": s.workflowRegistryRoot(), "root": s.workflowRegistryRoot(), "source": "nexus-registry", "vector_search_enabled": s.workflowTemplateVectorEnabled(), "vector_index_status": vectorStatus, "vector_index_items": vectorItems, "embedding_model": s.cfg.EmbeddingModel}
	for k, v := range workflowMatchRecommendation(candidates) {
		result[k] = v
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) workflowTemplatesReindex(w http.ResponseWriter, r *http.Request) {
	result, err := s.reindexWorkflowTemplateVectors(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, "WORKFLOW_REINDEX_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) workflowTemplateVectorIndexRead(w http.ResponseWriter, r *http.Request) {
	if !s.workflowTemplateVectorEnabled() {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "available": false, "source": "nexus-registry", "vector_index_status": "not_configured"})
		return
	}
	data, err := os.ReadFile(s.workflowTemplateVectorIndexPath())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "available": false, "source": "nexus-registry", "vector_index_status": "missing"})
		return
	}
	var idx workflowTemplateVectorIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		writeError(w, http.StatusConflict, "WORKFLOW_VECTOR_INDEX_INVALID", err.Error())
		return
	}
	if idx.Model != s.cfg.EmbeddingModel || idx.Documents == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "available": false, "source": "nexus-registry", "vector_index_status": "stale", "embedding_model": s.cfg.EmbeddingModel})
		return
	}
	info, _ := os.Stat(s.workflowTemplateVectorIndexPath())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"available":           true,
		"source":              "nexus-registry",
		"file_name":           "vector-index.json",
		"path":                "workflow-templates/vector-index.json",
		"size_bytes":          fileSize(info),
		"updated_at":          modTime(info),
		"content":             string(data),
		"vector_index_status": "ready",
		"vector_index_items":  len(idx.Documents),
		"embedding_model":     idx.Model,
		"dimension":           idx.Dimension,
	})
}

func (s *Server) getWorkflowTemplate(id, version string) (workflowTemplate, error) {
	for _, area := range []string{"published", "drafts"} {
		t, err := s.loadWorkflowTemplate(area, id, version)
		if err == nil {
			return t, nil
		}
	}
	return workflowTemplate{}, fmt.Errorf("template %s@%s not found", id, version)
}

func (s *Server) loadWorkflowTemplate(area, id, version string) (workflowTemplate, error) {
	if !validWorkflowTemplateToken(id) || !validWorkflowTemplateToken(version) {
		return workflowTemplate{}, errors.New("invalid template id or version")
	}
	data, err := os.ReadFile(s.workflowTemplatePath(area, id, version))
	if err != nil {
		return workflowTemplate{}, err
	}
	var t workflowTemplate
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&t); err != nil {
		return workflowTemplate{}, err
	}
	return t, nil
}

func (s *Server) listWorkflowTemplates(status workflowTemplateStatus) ([]workflowTemplate, error) {
	workflowRegistryMu.Lock()
	defer workflowRegistryMu.Unlock()
	if err := s.ensureWorkflowRegistryDirs(); err != nil {
		return nil, err
	}
	out := []workflowTemplate{}
	for _, area := range []string{"drafts", "published"} {
		entries, err := os.ReadDir(filepath.Join(s.workflowRegistryRoot(), area))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(s.workflowRegistryRoot(), area, entry.Name()))
			if err != nil {
				return nil, err
			}
			var t workflowTemplate
			if err := json.Unmarshal(data, &t); err != nil {
				return nil, err
			}
			if status == "" || t.Status == status {
				out = append(out, t)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == out[j].ID {
			return compareWorkflowVersions(out[i].Version, out[j].Version) > 0
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Server) retireActiveWorkflowTemplates(id, exceptVersion string, retiredAt time.Time) error {
	entries, err := os.ReadDir(filepath.Join(s.workflowRegistryRoot(), "published"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.workflowRegistryRoot(), "published", entry.Name()))
		if err != nil {
			return err
		}
		var t workflowTemplate
		if err := json.Unmarshal(data, &t); err != nil {
			return err
		}
		if t.ID != id || t.Version == exceptVersion || t.Status != workflowTemplateActive {
			continue
		}
		t.Status = workflowTemplateRetired
		t.RetiredAt = &retiredAt
		if err := writeWorkflowTemplateJSON(s.workflowTemplatePath("published", t.ID, t.Version), t); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) matchWorkflowTemplates(goal, device, taskType string) ([]workflowTemplateCandidate, error) {
	templates, err := s.listWorkflowTemplates(workflowTemplateActive)
	if err != nil {
		return nil, err
	}
	templates = latestWorkflowTemplateVersions(templates)
	vectorScores := s.workflowTemplateVectorScores(context.Background(), goal, device, taskType)
	query := workflowTemplateMatchText(strings.Join([]string{goal, taskType, device}, " "))
	out := []workflowTemplateCandidate{}
	fallback := []workflowTemplateCandidate{}
	for _, t := range templates {
		score := 0
		semantic := false
		reasons := []string{}
		for _, keyword := range t.Match.Keywords {
			keyword = strings.TrimSpace(keyword)
			if keyword != "" && strings.Contains(query, workflowTemplateMatchText(keyword)) {
				if weakWorkflowTemplateKeyword(keyword) {
					score += 15
					reasons = append(reasons, "context_keyword:"+keyword)
					continue
				}
				score += 15
				semantic = true
				reasons = append(reasons, "keyword:"+keyword)
			}
		}
		if containsWorkflowTemplateHint([]string{t.Match.Type}, taskType) {
			score += 80
			semantic = true
			reasons = append(reasons, "type:"+taskType)
		}
		if vectorScore := vectorScores[t.ID+"@"+t.Version]; vectorScore >= 0.55 {
			score += workflowTemplateVectorBonus(vectorScore)
			semantic = true
			reasons = append(reasons, fmt.Sprintf("vector:%.2f", vectorScore))
		}
		if containsWorkflowTemplateHint(t.Match.Devices, device) {
			score += 5
			reasons = append(reasons, "device:"+device)
		}
		if score > 0 && len(reasons) > 0 {
			c := workflowTemplateCandidate{ID: t.ID, Version: t.Version, Score: score, Reason: strings.Join(reasons, ", ")}
			if semantic {
				out = append(out, c)
			} else {
				fallback = append(fallback, c)
			}
		}
	}
	if len(out) == 0 {
		out = fallback
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			if out[i].ID == out[j].ID {
				return compareWorkflowVersions(out[i].Version, out[j].Version) > 0
			}
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out, nil
}

type workflowTemplateVectorIndex struct {
	Model     string                            `json:"model"`
	Dimension int                               `json:"dimension,omitempty"`
	UpdatedAt time.Time                         `json:"updated_at"`
	Documents map[string]workflowTemplateVector `json:"documents"`
}

type workflowTemplateVector struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Hash      string    `json:"hash"`
	Text      string    `json:"text"`
	Vector    []float64 `json:"vector"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Server) workflowTemplateVectorEnabled() bool {
	return s.cfg.EmbeddingEnabled && strings.TrimSpace(s.cfg.EmbeddingEndpoint) != ""
}

func (s *Server) workflowTemplateVectorIndexPath() string {
	return filepath.Join(s.workflowRegistryRoot(), "vector-index.json")
}

func (s *Server) workflowTemplateVectorIndexInfo() (string, int) {
	if !s.workflowTemplateVectorEnabled() {
		return "not_configured", 0
	}
	idx, err := s.loadWorkflowTemplateVectorIndex()
	if err != nil {
		return "missing", 0
	}
	return "ready", len(idx.Documents)
}

func (s *Server) reindexWorkflowTemplateVectors(ctx context.Context) (map[string]any, error) {
	if !s.workflowTemplateVectorEnabled() {
		return nil, errors.New("workflow template vector search is disabled; enable RECALL_EMBEDDING_ENABLED and RECALL_EMBEDDING_ENDPOINT")
	}
	templates, err := s.listWorkflowTemplates(workflowTemplateActive)
	if err != nil {
		return nil, err
	}
	templates = latestWorkflowTemplateVersions(templates)
	texts := make([]string, 0, len(templates))
	for _, t := range templates {
		texts = append(texts, workflowTemplateVectorText(t))
	}
	vectors, err := s.embedWorkflowTemplateTexts(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(templates) {
		return nil, fmt.Errorf("embedding response count mismatch: got %d want %d", len(vectors), len(templates))
	}
	idx := workflowTemplateVectorIndex{Model: s.cfg.EmbeddingModel, UpdatedAt: time.Now().UTC(), Documents: map[string]workflowTemplateVector{}}
	for i, t := range templates {
		key := t.ID + "@" + t.Version
		idx.Documents[key] = workflowTemplateVector{ID: t.ID, Version: t.Version, Hash: t.Hash, Text: texts[i], Vector: vectors[i], UpdatedAt: time.Now().UTC()}
		if len(vectors[i]) > idx.Dimension {
			idx.Dimension = len(vectors[i])
		}
	}
	if err := writeWorkflowTemplateJSON(s.workflowTemplateVectorIndexPath(), idx); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "source": "nexus-registry", "count": len(idx.Documents), "vector_search_enabled": true, "vector_index_status": "ready", "embedding_model": s.cfg.EmbeddingModel, "dimension": idx.Dimension, "index_path": s.workflowTemplateVectorIndexPath()}, nil
}

func (s *Server) workflowTemplateVectorScores(ctx context.Context, goal, device, taskType string) map[string]float64 {
	if !s.workflowTemplateVectorEnabled() || strings.TrimSpace(goal) == "" {
		return nil
	}
	idx, err := s.loadWorkflowTemplateVectorIndex()
	if err != nil || len(idx.Documents) == 0 {
		return nil
	}
	vectors, err := s.embedWorkflowTemplateTexts(ctx, []string{strings.Join([]string{goal, taskType, device}, "\n")})
	if err != nil || len(vectors) != 1 {
		return nil
	}
	out := map[string]float64{}
	for key, doc := range idx.Documents {
		out[key] = cosineWorkflowVector(vectors[0], doc.Vector)
	}
	return out
}

func (s *Server) loadWorkflowTemplateVectorIndex() (workflowTemplateVectorIndex, error) {
	data, err := os.ReadFile(s.workflowTemplateVectorIndexPath())
	if err != nil {
		return workflowTemplateVectorIndex{}, err
	}
	var idx workflowTemplateVectorIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return workflowTemplateVectorIndex{}, err
	}
	if idx.Model != s.cfg.EmbeddingModel || idx.Documents == nil {
		return workflowTemplateVectorIndex{}, errors.New("workflow vector index is stale")
	}
	return idx, nil
}

func (s *Server) embedWorkflowTemplateTexts(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	endpoint := strings.TrimRight(strings.TrimSpace(s.cfg.EmbeddingEndpoint), "/")
	if endpoint == "" {
		return nil, errors.New("embedding endpoint is empty")
	}
	if !strings.HasSuffix(endpoint, "/v1/embeddings") {
		endpoint += "/v1/embeddings"
	}
	model := strings.TrimSpace(s.cfg.EmbeddingModel)
	if model == "" {
		model = "BAAI/bge-m3"
	}
	payload, _ := json.Marshal(map[string]any{"model": model, "input": texts})
	timeout := s.cfg.EmbeddingTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding endpoint returned %s", resp.Status)
	}
	return parseWorkflowEmbeddingResponse(data)
}

func parseWorkflowEmbeddingResponse(data []byte) ([][]float64, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if values, ok := raw["embeddings"].([]any); ok {
		return workflowVectorsFromArray(values)
	}
	if dataValues, ok := raw["data"].([]any); ok {
		vectors := make([][]float64, 0, len(dataValues))
		for _, item := range dataValues {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("embedding data item is not an object")
			}
			arr, ok := m["embedding"].([]any)
			if !ok {
				return nil, errors.New("embedding data item missing embedding")
			}
			vector, err := workflowVectorFromArray(arr)
			if err != nil {
				return nil, err
			}
			vectors = append(vectors, vector)
		}
		return vectors, nil
	}
	return nil, errors.New("embedding response missing data or embeddings")
}

func workflowVectorsFromArray(values []any) ([][]float64, error) {
	vectors := make([][]float64, 0, len(values))
	for _, item := range values {
		arr, ok := item.([]any)
		if !ok {
			return nil, errors.New("embedding item is not an array")
		}
		vector, err := workflowVectorFromArray(arr)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func workflowVectorFromArray(values []any) ([]float64, error) {
	vector := make([]float64, 0, len(values))
	for _, value := range values {
		n, ok := value.(float64)
		if !ok {
			return nil, errors.New("embedding value is not a number")
		}
		vector = append(vector, n)
	}
	return vector, nil
}

func workflowTemplateVectorText(t workflowTemplate) string {
	parts := []string{t.ID, t.Version, t.Title, t.Description, t.Match.Type}
	parts = append(parts, t.Match.Keywords...)
	parts = append(parts, t.Match.Devices...)
	parts = append(parts, t.CompletionConditions...)
	for _, step := range t.Steps {
		parts = append(parts, step.ID, step.Title, step.Phase)
	}
	return strings.Join(normalizeWorkflowTexts(parts), "\n")
}

func cosineWorkflowVector(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func workflowTemplateVectorBonus(score float64) int {
	if score >= 0.85 {
		return 50
	}
	if score >= 0.75 {
		return 35
	}
	if score >= 0.65 {
		return 25
	}
	return 15
}

func workflowTemplatePathParams(path string) (string, string, error) {
	tail := strings.Trim(strings.TrimPrefix(path, "/v1/workflow-templates/"), "/")
	parts := strings.Split(tail, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("template id and version are required")
	}
	return parts[0], parts[1], nil
}

func workflowTemplateActionParams(path string) (string, string, string, error) {
	tail := strings.Trim(strings.TrimPrefix(path, "/v1/workflow-templates/"), "/")
	parts := strings.Split(tail, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", errors.New("template id, version, and action are required")
	}
	return parts[0], parts[1], parts[2], nil
}

func workflowTemplateSummaryFromTemplate(t workflowTemplate) workflowTemplateSummary {
	fileName := t.ID + "@" + t.Version + ".json"
	status := string(t.Status)
	area := workflowTemplateStorageArea(status)
	return workflowTemplateSummary{ID: t.ID, Version: t.Version, Title: firstNonEmptyString(t.Title, t.ID), Description: t.Description, Status: status, Location: workflowLocationFromStatus(status), FileName: fileName, Path: filepath.ToSlash(filepath.Join("workflow-templates", area, fileName)), StepCount: len(t.Steps), Keywords: t.Match.Keywords, Current: t.Status == workflowTemplateActive || t.Status == workflowTemplateDraft}
}

func workflowTemplateStorageArea(status string) string {
	switch status {
	case "draft", "validated":
		return "drafts"
	default:
		// Retired templates are immutable history stored beside active published versions.
		return "published"
	}
}

func workflowTemplateCompactList(templates []workflowTemplate) []map[string]any {
	out := make([]map[string]any, 0, len(templates))
	for _, t := range templates {
		out = append(out, map[string]any{"id": t.ID, "version": t.Version, "title": t.Title, "description": t.Description, "status": t.Status, "match": t.Match, "completion_conditions": t.CompletionConditions, "steps": t.Steps, "step_count": len(t.Steps), "hash": t.Hash, "published_at": t.PublishedAt, "retired_at": t.RetiredAt})
	}
	return out
}

func validateWorkflowTemplate(t workflowTemplate) error {
	if !validWorkflowTemplateToken(t.ID) || !validWorkflowTemplateToken(t.Version) {
		return errors.New("template id and version must contain only letters, numbers, dot, dash, or underscore")
	}
	if strings.TrimSpace(t.Title) == "" {
		return errors.New("template title is required")
	}
	if len(normalizeWorkflowTexts(t.CompletionConditions)) == 0 {
		return errors.New("template requires at least one completion condition")
	}
	if len(t.Steps) == 0 {
		return errors.New("template requires at least one step")
	}
	if err := validateWorkflowTemplateGuardrails(t); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, step := range t.Steps {
		if !validWorkflowTemplateToken(step.ID) || strings.TrimSpace(step.Title) == "" {
			return fmt.Errorf("invalid template step %q", step.ID)
		}
		if !validWorkflowPhase(step.Phase) {
			return fmt.Errorf("step %s has invalid phase %q", step.ID, step.Phase)
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		seen[step.ID] = true
	}
	return nil
}

const maxWorkflowTemplateSteps = 8
const maxWorkflowTemplateConditions = 10

var sopWorkflowTemplateTerms = []string{"每条命令", "每个命令", "逐命令", "逐条命令", "记录证据", "补充证据", "再次记录", "逐项记录", "证据账本", "详细证据", "每一步", "每个步骤", "逐来源", "逐条", "分别记录"}

func validateWorkflowTemplateGuardrails(t workflowTemplate) error {
	if t.AllowLongTemplate && len([]rune(strings.TrimSpace(t.LongTemplateReason))) < 12 {
		return errors.New("long_template_reason is required when allow_long_template=true")
	}
	if !t.AllowLongTemplate && len(t.Steps) > maxWorkflowTemplateSteps {
		return fmt.Errorf("template has %d steps; max %d unless allow_long_template=true with long_template_reason", len(t.Steps), maxWorkflowTemplateSteps)
	}
	if !t.AllowLongTemplate && len(normalizeWorkflowTexts(t.CompletionConditions)) > maxWorkflowTemplateConditions {
		return fmt.Errorf("template has %d completion conditions; max %d unless allow_long_template=true with long_template_reason", len(normalizeWorkflowTexts(t.CompletionConditions)), maxWorkflowTemplateConditions)
	}
	texts := []string{t.Title, t.Description, t.LongTemplateReason}
	texts = append(texts, t.CompletionConditions...)
	for _, step := range t.Steps {
		texts = append(texts, step.ID, step.Title)
	}
	for _, text := range texts {
		for _, term := range sopWorkflowTemplateTerms {
			if strings.Contains(text, term) {
				return fmt.Errorf("template text looks like verbose SOP or evidence ledger; move details to script/Skill/runbook instead of using term %q", term)
			}
		}
	}
	return nil
}

func validWorkflowPhase(value string) bool {
	switch value {
	case "check", "execute", "verify", "closeout":
		return true
	default:
		return false
	}
}
func validWorkflowTemplateToken(v string) bool {
	if strings.TrimSpace(v) == "" {
		return false
	}
	for _, r := range v {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func workflowTemplateHash(t workflowTemplate) string {
	t.Hash = ""
	t.PublishedAt = nil
	t.RetiredAt = nil
	data, _ := json.Marshal(t)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func writeWorkflowTemplateJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
func normalizeWorkflowTexts(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
func latestWorkflowTemplateVersions(templates []workflowTemplate) []workflowTemplate {
	byID := map[string]workflowTemplate{}
	for _, template := range templates {
		current, exists := byID[template.ID]
		if !exists || compareWorkflowVersions(template.Version, current.Version) > 0 {
			byID[template.ID] = template
		}
	}
	out := make([]workflowTemplate, 0, len(byID))
	for _, template := range byID {
		out = append(out, template)
	}
	return out
}
func weakWorkflowTemplateKeyword(keyword string) bool {
	switch workflowTemplateMatchText(keyword) {
	case "agentdock", "nexus", "vitapulse":
		return true
	default:
		return false
	}
}
func workflowTemplateMatchText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if r <= ' ' || r == '个' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
func containsWorkflowTemplateHint(values []string, hint string) bool {
	hint = workflowTemplateMatchText(hint)
	if hint == "" {
		return false
	}
	for _, value := range values {
		if workflowTemplateMatchText(value) == hint {
			return true
		}
	}
	return false
}
func workflowMatchRecommendation(candidates []workflowTemplateCandidate) map[string]any {
	best := 0
	if len(candidates) > 0 {
		best = candidates[0].Score
	}
	recommended := "plain_task"
	reason := "no active template is specific enough; create a plain recoverable task"
	if best >= 85 {
		recommended = "use_template"
		reason = "top candidate score is strong enough to select by default"
	} else if best >= 60 {
		recommended = "consider_template"
		reason = "top candidate is plausible but should be checked against the user goal"
	}
	return map[string]any{"recommended": recommended, "recommendation_reason": reason, "best_candidate_score": best, "score_thresholds": map[string]any{"use_template": 85, "consider_template": 60, "plain_task_below": 60}}
}
