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

	"github.com/uvwt/agentdock-nexus/internal/devices"
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

type opsTaskDetail struct {
	opsTaskSummary
	Path        string         `json:"path"`
	Content     string         `json:"content"`
	JSON        map[string]any `json:"json"`
	Conditions  []any          `json:"conditions,omitempty"`
	Steps       []any          `json:"steps,omitempty"`
	Attempts    []any          `json:"attempts,omitempty"`
	Events      []any          `json:"events,omitempty"`
	FinalReview map[string]any `json:"final_review,omitempty"`
}

type opsSkillSummary struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Source           string            `json:"source"`
	Path             string            `json:"path"`
	Description      string            `json:"description,omitempty"`
	UpdatedAt        string            `json:"updated_at"`
	FileCount        int               `json:"file_count"`
	Status           string            `json:"status"`
	ActiveVersion    string            `json:"active_version,omitempty"`
	Versions         []string          `json:"versions,omitempty"`
	Channels         map[string]string `json:"channels,omitempty"`
	RuntimeStatePath string            `json:"runtime_state_path,omitempty"`
	DocRoot          string            `json:"doc_root,omitempty"`
}

type opsSkillFile struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
	UpdatedAt string `json:"updated_at"`
}

type opsSkillDetail struct {
	opsSkillSummary
	Root         string         `json:"root"`
	SkillDoc     string         `json:"skill_doc,omitempty"`
	Files        []opsSkillFile `json:"files"`
	RuntimeState map[string]any `json:"runtime_state,omitempty"`
}

type opsSkillRuntimeState struct {
	ActiveVersion string            `json:"active_version"`
	Channels      map[string]string `json:"channels"`
	History       []string          `json:"history"`
	UpdatedAt     string            `json:"updated_at"`
}

type opsSkillStateRecord struct {
	ID      string
	Path    string
	ModTime string
	Raw     map[string]any
	State   opsSkillRuntimeState
}

type opsToolSummary struct {
	Name        string         `json:"name"`
	Category    string         `json:"category"`
	Status      string         `json:"status"`
	Description string         `json:"description,omitempty"`
	Source      string         `json:"source"`
	DeviceID    string         `json:"device_id,omitempty"`
	DeviceName  string         `json:"device_name,omitempty"`
	Version     string         `json:"version,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
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
	mux.HandleFunc("GET /v1/ops/tasks/{fileName}", protected(s.opsTaskDetail))
	mux.HandleFunc("POST /v1/ops/tasks/cleanup", protected(s.opsCleanupTasks))
	mux.HandleFunc("GET /v1/ops/skills", protected(s.opsSkills))
	mux.HandleFunc("GET /v1/ops/skills/{source}/{skillID}", protected(s.opsSkillDetail))
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

func (s *Server) opsTaskDetail(w http.ResponseWriter, r *http.Request) {
	fileName, err := cleanOpsFileName(r.PathValue("fileName"), ".json")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TASK_FILE", err.Error())
		return
	}
	path := filepath.Join(s.agentDockPath("tasks"), fileName)
	body, summary, err := readOpsTask(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TASK_READ_FAILED", err.Error())
		return
	}
	detail := opsTaskDetail{
		opsTaskSummary: summary,
		Path:           filepath.ToSlash(path),
		Content:        string(raw),
		JSON:           body,
		Conditions:     opsArray(body["conditions"]),
		Steps:          opsArray(body["steps"]),
		Attempts:       opsArray(body["attempts"]),
		Events:         opsArray(body["events"]),
		FinalReview:    opsMap(body["final_review"]),
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": detail})
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "count": len(items), "root": s.opsSkillStateDir(), "fallback_roots": s.opsSkillDocRoots()})
}

func (s *Server) opsSkillDetail(w http.ResponseWriter, r *http.Request) {
	source := strings.TrimSpace(r.PathValue("source"))
	skillID, err := cleanOpsName(r.PathValue("skillID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_SKILL_ID", err.Error())
		return
	}
	detail, err := s.opsSkillDetailModel(source, skillID)
	if err != nil {
		writeError(w, http.StatusNotFound, "SKILL_NOT_FOUND", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skill": detail})
}

func (s *Server) opsCapabilities(w http.ResponseWriter, r *http.Request) {
	tasks := s.collectOpsTasks()
	skills := s.collectOpsSkills()
	tools, a, b := s.collectOpsCapabilityTools(r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tools": tools, "counts": map[string]any{"tasks": len(tasks), "skills": len(skills), "workflows": s.workflowCounts(), "tools": len(tools), "devices": a, "heartbeats": b}, "paths": s.opsPaths()})
}

func (s *Server) collectOpsCapabilityTools(r *http.Request) ([]opsToolSummary, int, int) {
	items := []opsToolSummary{}
	seen := map[string]bool{}
	add := func(item opsToolSummary) {
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			return
		}
		key := item.Source + ":" + item.Category + ":" + item.Name + ":" + item.DeviceID
		if seen[key] {
			return
		}
		seen[key] = true
		if item.Status == "" {
			item.Status = "available"
		}
		items = append(items, item)
	}
	dc, hc := 0, 0
	if s.devices != nil {
		devs, err := s.devices.List(r.Context())
		if err == nil {
			dc = len(devs)
			for _, dev := range devs {
				caps := dev.Capabilities
				skills := []devices.SkillSummary{}
				src := "device-registry"
				if snap, err := s.devices.Snapshot(r.Context(), dev.ID); err == nil && snap.Heartbeat != nil {
					hc++
					caps = snap.Heartbeat.Capabilities
					skills = snap.Heartbeat.Skills
					src = "device-heartbeat"
				}
				for _, cap := range caps {
					st := "disabled"
					if cap.Enabled {
						st = "available"
					}
					md := map[string]any{}
					for k, v := range cap.Metadata {
						md[k] = v
					}
					add(opsToolSummary{Name: cap.Name, Category: "device-capability", Status: st, Description: "AgentDock device reported capability", Source: src, DeviceID: dev.ID, DeviceName: dev.Name, Version: cap.Version, Metadata: md})
				}
				for _, sk := range skills {
					st := "installed"
					if sk.Active {
						st = "active"
					}
					add(opsToolSummary{Name: sk.Name, Category: "runtime-skill", Status: st, Description: "AgentDock heartbeat reported skill", Source: src, DeviceID: dev.ID, DeviceName: dev.Name, Version: sk.Version, Metadata: map[string]any{"channel": sk.Channel}})
				}
			}
		}
	}
	for _, sk := range s.collectOpsSkills() {
		add(opsToolSummary{Name: sk.ID, Category: "runtime-skill", Status: sk.Status, Description: firstNonEmptyString(sk.Description, "AgentDock Runtime state skill"), Source: "runtime-state", Version: sk.ActiveVersion, Metadata: map[string]any{"channels": sk.Channels, "versions": sk.Versions, "doc_root": sk.DocRoot}})
	}
	for _, item := range opsCommandContracts() {
		add(item)
	}
	add(opsToolSummary{Name: "workflow_templates", Category: "workflow", Status: "available", Description: "Nexus mediated AgentDock workflow templates", Source: "workflow-runtime-dir", Metadata: map[string]any{"counts": s.workflowCounts(), "root": strings.TrimSpace(s.cfg.WorkflowDir)}})
	add(opsToolSummary{Name: "task_state", Category: "task", Status: "available", Description: "AgentDock persistent task state", Source: "task-runtime-dir", Metadata: map[string]any{"root": s.agentDockPath("tasks"), "count": len(s.collectOpsTasks())}})
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		if items[i].Source != items[j].Source {
			return items[i].Source < items[j].Source
		}
		return items[i].Name < items[j].Name
	})
	return items, dc, hc
}

func opsCommandContracts() []opsToolSummary {
	data := []struct{ name, risk, desc string }{
		{"health.check", "low", "Health check command"},
		{"recall.sync", "low", "Recall sync command"},
		{"service.inspect", "low", "Service inspect command"},
		{"diagnostics.collect", "low", "Diagnostics command"},
		{"env.manage", "medium", "Env registry command"},
		{"service.restart", "high", "Service restart command"},
		{"agentdock.reload", "high", "AgentDock reload command"},
	}
	items := make([]opsToolSummary, 0, len(data))
	for _, row := range data {
		items = append(items, opsToolSummary{Name: row.name, Category: "command", Status: "contract", Description: row.desc, Source: "nexus-command-contract", Metadata: map[string]any{"risk": row.risk}})
	}
	return items
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

func (s *Server) opsSkillStateDir() string {
	return s.agentDockPath("nexus/skills/state")
}

func (s *Server) opsSkillDocRoots() []string {
	roots := []string{
		s.agentDockPath("skill-sources"),
		filepath.Join(strings.TrimSpace(s.cfg.WorkspaceDir), "skills"),
		s.agentDockPath("skills"),
		filepath.Join(strings.TrimSpace(s.cfg.WorkspaceDir), ".agents/skills"),
		s.agentDockPath(".agents/skills"),
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		result = append(result, root)
	}
	return result
}

func (s *Server) collectOpsSkillStates() map[string]opsSkillStateRecord {
	root := s.opsSkillStateDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	items := map[string]opsSkillStateRecord{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if _, err := cleanOpsName(id); err != nil {
			continue
		}
		path := filepath.Join(root, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var state opsSkillRuntimeState
		if err := json.Unmarshal(raw, &state); err != nil {
			continue
		}
		var rawState map[string]any
		_ = json.Unmarshal(raw, &rawState)
		info, _ := entry.Info()
		items[id] = opsSkillStateRecord{ID: id, Path: path, ModTime: modTime(info), Raw: rawState, State: state}
	}
	return items
}

func (s *Server) collectOpsSkills() []opsSkillSummary {
	states := s.collectOpsSkillStates()
	if len(states) > 0 {
		items := make([]opsSkillSummary, 0, len(states))
		for id, record := range states {
			items = append(items, s.opsSkillSummaryFromState(id, record))
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		return items
	}
	return s.collectOpsSkillDirectoryFallback()
}

func (s *Server) opsSkillSummaryFromState(skillID string, record opsSkillStateRecord) opsSkillSummary {
	docRoot := s.findOpsSkillDocRoot(skillID)
	title, desc, updatedAt, fileCount := "", "", firstNonEmptyString(record.State.UpdatedAt, record.ModTime), 0
	if docRoot != "" {
		info, _ := os.Stat(docRoot)
		title, desc = readSkillDoc(filepath.Join(docRoot, "SKILL.md"))
		fileCount = countFiles(docRoot, 4)
		updatedAt = firstNonEmptyString(record.State.UpdatedAt, modTime(info), record.ModTime)
	}
	return opsSkillSummary{
		ID:               skillID,
		Title:            firstNonEmptyString(title, skillID),
		Description:      desc,
		Source:           "runtime",
		Path:             filepath.ToSlash(filepath.Join("runtime", skillID)),
		UpdatedAt:        updatedAt,
		FileCount:        fileCount,
		Status:           "installed",
		ActiveVersion:    record.State.ActiveVersion,
		Versions:         opsSkillVersions(record.State),
		Channels:         record.State.Channels,
		RuntimeStatePath: filepath.ToSlash(record.Path),
		DocRoot:          filepath.ToSlash(docRoot),
	}
}

func (s *Server) collectOpsSkillDirectoryFallback() []opsSkillSummary {
	items := []opsSkillSummary{}
	seen := map[string]bool{}
	for _, root := range s.opsSkillDocRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		label := s.opsSkillRootLabel(root)
		for _, entry := range entries {
			if !entry.IsDir() || seen[entry.Name()] {
				continue
			}
			seen[entry.Name()] = true
			path := filepath.Join(root, entry.Name())
			info, _ := entry.Info()
			title, desc := readSkillDoc(filepath.Join(path, "SKILL.md"))
			items = append(items, opsSkillSummary{ID: entry.Name(), Title: firstNonEmptyString(title, entry.Name()), Description: desc, Source: label, Path: filepath.ToSlash(filepath.Join(label, entry.Name())), UpdatedAt: modTime(info), FileCount: countFiles(path, 4), Status: "source-only", DocRoot: filepath.ToSlash(path)})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (s *Server) opsSkillDetailModel(source, skillID string) (opsSkillDetail, error) {
	states := s.collectOpsSkillStates()
	record, hasState := states[skillID]
	root := s.findOpsSkillDocRoot(skillID)
	if !hasState && root == "" {
		return opsSkillDetail{}, fmt.Errorf("skill %s not found", skillID)
	}
	if source != "" && source != "runtime" && root != "" && s.opsSkillRootLabel(filepath.Dir(root)) != source {
		// Keep old links usable, but reject clearly unrelated source labels when no runtime state exists.
		if !hasState {
			return opsSkillDetail{}, fmt.Errorf("skill %s not found in source %s", skillID, source)
		}
	}
	summary := opsSkillSummary{ID: skillID, Title: skillID, Source: firstNonEmptyString(source, "runtime"), Path: filepath.ToSlash(filepath.Join(firstNonEmptyString(source, "runtime"), skillID)), Status: "installed"}
	if hasState {
		summary = s.opsSkillSummaryFromState(skillID, record)
	} else if root != "" {
		info, _ := os.Stat(root)
		title, desc := readSkillDoc(filepath.Join(root, "SKILL.md"))
		summary = opsSkillSummary{ID: skillID, Title: firstNonEmptyString(title, skillID), Description: desc, Source: s.opsSkillRootLabel(filepath.Dir(root)), Path: filepath.ToSlash(filepath.Join(s.opsSkillRootLabel(filepath.Dir(root)), skillID)), UpdatedAt: modTime(info), FileCount: countFiles(root, 4), Status: "source-only", DocRoot: filepath.ToSlash(root)}
	}
	detail := opsSkillDetail{opsSkillSummary: summary, Root: filepath.ToSlash(root), RuntimeState: record.Raw}
	if root != "" {
		detail.SkillDoc = readSmallText(filepath.Join(root, "SKILL.md"), 32000)
		detail.Files = collectOpsSkillFiles(root, 4, 160)
	} else {
		detail.Files = []opsSkillFile{}
	}
	return detail, nil
}

func (s *Server) findOpsSkillDocRoot(skillID string) string {
	for _, root := range s.opsSkillDocRoots() {
		candidate := filepath.Join(root, skillID)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (s *Server) opsSkillRootLabel(root string) string {
	clean := filepath.Clean(root)
	switch clean {
	case filepath.Clean(s.agentDockPath("skill-sources")):
		return "source"
	case filepath.Clean(filepath.Join(strings.TrimSpace(s.cfg.WorkspaceDir), "skills")):
		return "workspace"
	case filepath.Clean(s.agentDockPath("skills")):
		return "legacy"
	case filepath.Clean(filepath.Join(strings.TrimSpace(s.cfg.WorkspaceDir), ".agents/skills")), filepath.Clean(s.agentDockPath(".agents/skills")):
		return "agents"
	default:
		return "source"
	}
}

func opsSkillVersions(state opsSkillRuntimeState) []string {
	seen := map[string]bool{}
	versions := []string{}
	for i := len(state.History) - 1; i >= 0; i-- {
		version := strings.TrimSpace(state.History[i])
		if version != "" && !seen[version] {
			seen[version] = true
			versions = append(versions, version)
		}
	}
	if version := strings.TrimSpace(state.ActiveVersion); version != "" && !seen[version] {
		versions = append(versions, version)
	}
	return versions
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

func cleanOpsFileName(value, suffix string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value != filepath.Base(value) || strings.Contains(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, "..") {
		return "", fmt.Errorf("invalid file name")
	}
	if suffix != "" && !strings.HasSuffix(value, suffix) {
		return "", fmt.Errorf("file name must end with %s", suffix)
	}
	return value, nil
}

func cleanOpsName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value != filepath.Base(value) || strings.Contains(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, "..") {
		return "", fmt.Errorf("invalid name")
	}
	return value, nil
}

func collectOpsSkillFiles(root string, maxDepth, maxItems int) []opsSkillFile {
	root = filepath.Clean(root)
	baseDepth := strings.Count(root, string(os.PathSeparator))
	items := []opsSkillFile{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - baseDepth
		if d.IsDir() && depth > maxDepth {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel := trimKnownRoot(path, root)
		items = append(items, opsSkillFile{Path: rel, Kind: skillFileKind(rel), SizeBytes: info.Size(), UpdatedAt: modTime(info)})
		if len(items) >= maxItems {
			return fs.SkipAll
		}
		return nil
	})
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Path < items[j].Path
	})
	return items
}

func skillFileKind(path string) string {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case name == "skill.md" || name == "readme.md":
		return "doc"
	case name == "manifest.json" || name == "package.json" || name == "skill.json":
		return "manifest"
	case ext == ".go" || ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".py" || ext == ".sh":
		return "code"
	case ext == ".json" || ext == ".yaml" || ext == ".yml" || ext == ".toml":
		return "config"
	default:
		return "asset"
	}
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
