package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type opsTaskSummary struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Goal            string `json:"goal"`
	Status          string `json:"status"`
	Phase           string `json:"phase"`
	ReviewStatus    string `json:"review_status"`
	Blocker         string `json:"blocker,omitempty"`
	UpdatedAt       string `json:"updated_at"`
	CreatedAt       string `json:"created_at"`
	TemplateID      string `json:"template_id,omitempty"`
	TemplateVersion string `json:"template_version,omitempty"`
	ConditionCount  int    `json:"condition_count"`
	StepCount       int    `json:"step_count"`
	AttemptCount    int    `json:"attempt_count"`
	EventCount      int    `json:"event_count"`
	Cleanable       bool   `json:"cleanable"`
	FileName        string `json:"file_name"`
}

type opsSkillSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	UpdatedAt   string `json:"updated_at"`
	FileCount   int    `json:"file_count"`
	Status      string `json:"status"`
}

type opsLogEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	UpdatedAt string `json:"updated_at"`
	Tail      string `json:"tail"`
}

func (s *Server) registerOpsRoutes(mux *http.ServeMux, protected func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /v1/ops/overview", protected(s.opsOverview))
	mux.HandleFunc("GET /v1/ops/tasks", protected(s.opsTasks))
	mux.HandleFunc("POST /v1/ops/tasks/cleanup", protected(s.opsCleanupTasks))
	mux.HandleFunc("GET /v1/ops/skills", protected(s.opsSkills))
	mux.HandleFunc("GET /v1/ops/capabilities", protected(s.opsCapabilities))
	mux.HandleFunc("GET /v1/ops/logs", protected(s.opsLogs))
	mux.HandleFunc("GET /v1/ops/deployment", protected(s.opsDeployment))
}

func (s *Server) opsOverview(w http.ResponseWriter, r *http.Request) {
	tasks := s.collectOpsTasks()
	counts := map[string]int{"active": 0, "completed": 0, "blocked": 0, "cleanable": 0}
	for _, task := range tasks {
		counts[task.Status]++
		if task.Cleanable {
			counts["cleanable"]++
		}
	}
	skills := s.collectOpsSkills()
	workflowCounts := s.workflowCounts()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"tasks":      counts,
		"skills":     map[string]any{"count": len(skills), "items": firstSkills(skills, 6)},
		"workflows":  workflowCounts,
		"paths":      s.opsPaths(),
		"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) opsTasks(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	limit := queryInt(r, "limit", 200)
	if limit > 800 {
		limit = 800
	}
	items := s.collectOpsTasks()
	filtered := make([]opsTaskSummary, 0, len(items))
	for _, item := range items {
		if status != "" && status != "all" && item.Status != status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(strings.Join([]string{item.ID, item.Title, item.Goal, item.Status, item.Phase, item.ReviewStatus, item.Blocker, item.TemplateID}, " ")), query) {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) >= limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": filtered, "count": len(filtered), "total": len(items), "root": s.agentDockPath("tasks")})
}

func (s *Server) opsCleanupTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun bool `json:"dry_run"`
		Limit  int  `json:"limit"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Limit <= 0 || req.Limit > 200 {
		req.Limit = 200
	}
	root := s.agentDockPath("tasks")
	entries, err := os.ReadDir(root)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "TASK_DIR_UNAVAILABLE", err.Error())
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	changed := []opsTaskSummary{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		body, summary, err := readOpsTask(path)
		if err != nil || !summary.Cleanable {
			continue
		}
		changed = append(changed, summary)
		if !req.DryRun {
			body["status"] = "completed"
			body["completed_at"] = now
			body["updated_at"] = now
			body["summary"] = firstNonEmptyString(opsString(body["summary"]), "任务已通过 final_review，已由 Nexus 任务清理标记完成。")
			events := opsArray(body["events"])
			events = append(events, map[string]any{"type": "completed", "summary": "Nexus 任务清理：review pass 的 active 任务标记完成。", "created_at": now})
			body["events"] = events
			if err := writeJSONFile(path, body); err != nil {
				writeError(w, http.StatusInternalServerError, "TASK_CLEANUP_FAILED", err.Error())
				return
			}
		}
		if len(changed) >= req.Limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dry_run": req.DryRun, "changed": changed, "count": len(changed)})
}

func (s *Server) opsSkills(w http.ResponseWriter, r *http.Request) {
	items := s.collectOpsSkills()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "count": len(items), "root": s.agentDockPath("skills")})
}

func (s *Server) opsCapabilities(w http.ResponseWriter, r *http.Request) {
	tasks := s.collectOpsTasks()
	skills := s.collectOpsSkills()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"tools": []map[string]any{
			{"name": "task_manage", "category": "task", "status": "available", "description": "持久化任务、模板匹配、final_review 和完成流"},
			{"name": "recall_bootstrap", "category": "memory", "status": "available", "description": "读取高优先级 RecallDock 上下文"},
			{"name": "workflow_templates", "category": "template", "status": "available", "description": "Nexus 任务模板 API 和管理页"},
			{"name": "device_commands", "category": "nexus", "status": "available", "description": "设备注册、命令队列、环境变量动作和 Artifact Relay"},
		},
		"counts": map[string]any{"tasks": len(tasks), "skills": len(skills), "workflows": s.workflowCounts()},
		"paths":  s.opsPaths(),
	})
}

func (s *Server) opsLogs(w http.ResponseWriter, r *http.Request) {
	items := s.collectOpsLogs()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "count": len(items), "roots": s.logRoots()})
}

func (s *Server) opsDeployment(w http.ResponseWriter, r *http.Request) {
	compose := readSmallText(filepath.Join(strings.TrimSpace(s.cfg.DeployDir), "docker-compose.yml"), 16000)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"service":    "recalldock",
		"health":     map[string]any{"ok": true, "addr": s.cfg.Addr()},
		"paths":      s.opsPaths(),
		"compose":    compose,
		"source":     map[string]any{"dir": s.cfg.SourceDir, "commit": readGitCommit(s.cfg.SourceDir)},
		"image":      strings.TrimSpace(os.Getenv("NEXUS_IMAGE_NAME")),
		"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) collectOpsTasks() []opsTaskSummary {
	root := s.agentDockPath("tasks")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	items := make([]opsTaskSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		_, summary, err := readOpsTask(filepath.Join(root, entry.Name()))
		if err == nil {
			items = append(items, summary)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return items
}

func readOpsTask(path string) (map[string]any, opsTaskSummary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, opsTaskSummary{}, err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, opsTaskSummary{}, err
	}
	finalReview := opsMap(body["final_review"])
	review := firstNonEmptyString(opsString(body["review_status"]), opsString(finalReview["status"]), "not_started")
	template := opsMap(body["template"])
	summary := opsTaskSummary{
		ID: opsString(body["id"]), Title: opsString(body["title"]), Goal: opsString(body["goal"]),
		Status: firstNonEmptyString(opsString(body["status"]), "unknown"), Phase: opsString(body["phase"]), ReviewStatus: review,
		Blocker: opsString(body["blocker"]), UpdatedAt: opsString(body["updated_at"]), CreatedAt: opsString(body["created_at"]),
		TemplateID: opsString(template["id"]), TemplateVersion: opsString(template["version"]),
		ConditionCount: len(opsArray(body["conditions"])), StepCount: len(opsArray(body["steps"])), AttemptCount: len(opsArray(body["attempts"])), EventCount: len(opsArray(body["events"])),
		FileName: filepath.Base(path),
	}
	summary.Cleanable = summary.Status == "active" && summary.Phase == "closeout" && summary.ReviewStatus == "pass"
	return body, summary, nil
}

func (s *Server) collectOpsSkills() []opsSkillSummary {
	roots := []struct{ label, path string }{{"runtime", s.agentDockPath("skills")}, {"agents", s.agentDockPath(".agents/skills")}}
	seen := map[string]bool{}
	items := []opsSkillSummary{}
	for _, root := range roots {
		entries, err := os.ReadDir(root.path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || seen[root.label+":"+entry.Name()] {
				continue
			}
			seen[root.label+":"+entry.Name()] = true
			path := filepath.Join(root.path, entry.Name())
			info, _ := entry.Info()
			title, desc := readSkillDoc(filepath.Join(path, "SKILL.md"))
			items = append(items, opsSkillSummary{ID: entry.Name(), Title: firstNonEmptyString(title, entry.Name()), Description: desc, Source: root.label, Path: filepath.ToSlash(filepath.Join(root.label, entry.Name())), UpdatedAt: modTime(info), FileCount: countFiles(path, 4), Status: "installed"})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (s *Server) collectOpsLogs() []opsLogEntry {
	items := []opsLogEntry{}
	for _, root := range s.logRoots() {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if !(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".out") || strings.HasSuffix(name, ".err")) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			items = append(items, opsLogEntry{Name: name, Path: trimKnownRoot(path, root), SizeBytes: info.Size(), UpdatedAt: modTime(info), Tail: tailText(path, 80, 12000)})
			if len(items) >= 30 {
				return fs.SkipAll
			}
			return nil
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	if len(items) > 30 {
		items = items[:30]
	}
	return items
}

func (s *Server) workflowCounts() map[string]int {
	root := strings.TrimSpace(s.cfg.WorkflowDir)
	counts := map[string]int{"drafts": 0, "published": 0, "retired": 0}
	for key := range counts {
		entries, err := os.ReadDir(filepath.Join(root, key))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				counts[key]++
			}
		}
	}
	return counts
}

func (s *Server) opsPaths() map[string]string {
	return map[string]string{"agentdock": s.cfg.AgentDockDir, "workspace": s.cfg.WorkspaceDir, "workflows": s.cfg.WorkflowDir, "deploy": s.cfg.DeployDir, "source": s.cfg.SourceDir}
}

func (s *Server) agentDockPath(parts ...string) string {
	return filepath.Join(append([]string{strings.TrimSpace(s.cfg.AgentDockDir)}, parts...)...)
}
func (s *Server) logRoots() []string {
	if strings.TrimSpace(s.cfg.LogDirs) != "" {
		return opsSplitCSV(s.cfg.LogDirs)
	}
	roots := []string{}
	if s.cfg.AgentDockDir != "" {
		roots = append(roots, s.cfg.AgentDockDir)
	}
	if s.cfg.WorkspaceDir != "" {
		roots = append(roots, filepath.Join(s.cfg.WorkspaceDir, ".npm/_logs"), filepath.Join(s.cfg.WorkspaceDir, ".cc-switch/logs"))
	}
	return roots
}

func opsSplitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func firstSkills(items []opsSkillSummary, n int) []opsSkillSummary {
	if len(items) <= n {
		return items
	}
	return items[:n]
}
func opsString(v any) string { s, _ := v.(string); return s }
func opsMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
func opsArray(v any) []any { a, _ := v.([]any); return a }
func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func modTime(info fs.FileInfo) string {
	if info == nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339Nano)
}

func writeJSONFile(path string, body map[string]any) error {
	content, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0o644)
}

func readSkillDoc(path string) (string, string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	lines := strings.Split(string(raw), "\n")
	title, desc := "", ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if title == "" && strings.HasPrefix(line, "#") {
			title = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		}
		if title != "" && desc == "" && line != "" && !strings.HasPrefix(line, "#") {
			desc = line
			break
		}
	}
	return title, desc
}

func countFiles(root string, maxDepth int) int {
	root = filepath.Clean(root)
	baseDepth := strings.Count(root, string(os.PathSeparator))
	count := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.Count(filepath.Clean(path), string(os.PathSeparator))-baseDepth > maxDepth {
			return fs.SkipDir
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	return count
}

func readSmallText(path string, limit int) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	if len(raw) > limit {
		raw = raw[:limit]
	}
	return string(raw)
}

func tailText(path string, maxLines, maxBytes int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	if len(raw) > maxBytes {
		raw = raw[len(raw)-maxBytes:]
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func trimKnownRoot(path, root string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

func readGitCommit(root string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	head := strings.TrimSpace(readSmallText(filepath.Join(root, ".git", "HEAD"), 200))
	if strings.HasPrefix(head, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(head, "ref:"))
		return strings.TrimSpace(readSmallText(filepath.Join(root, ".git", filepath.FromSlash(ref)), 200))
	}
	if strings.Contains(head, " ") || strings.Contains(head, "no such") {
		return ""
	}
	return head
}

var _ = errors.Is
var _ = fmt.Sprintf
