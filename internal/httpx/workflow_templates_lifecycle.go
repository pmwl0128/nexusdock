package httpx

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type workflowTemplateLifecycleResult struct {
	Template       workflowTemplateDetail     `json:"template"`
	Retired        []workflowTemplateSummary  `json:"retired,omitempty"`
	Current        []workflowTemplateSummary  `json:"current,omitempty"`
	ConflictCount  int                        `json:"conflict_count"`
	VersionSummary map[string]workflowCounter `json:"version_summary,omitempty"`
}

type workflowCounter struct {
	Versions int `json:"versions"`
	Active   int `json:"active"`
	Draft    int `json:"draft"`
	Retired  int `json:"retired"`
}

func (s *Server) readAllWorkflowTemplateSummaries(root, locationFilter string) ([]workflowTemplateSummary, error) {
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
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			summary, err := s.readWorkflowTemplateSummary(root, location, entry.Name())
			if err != nil {
				continue
			}
			items = append(items, summary)
		}
	}
	attachWorkflowTemplateCountersToList(items, items)
	return items, nil
}

func (s *Server) publishWorkflowTemplate(root, location, fileName string) (workflowTemplateLifecycleResult, error) {
	source := filepath.Join(root, location, fileName)
	content, err := os.ReadFile(source)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	var body map[string]any
	if err := json.Unmarshal(content, &body); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	if err := validateWorkflowTemplateBody(body, fileName); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	id := stringFromMap(body, "id")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	retired, err := s.retireOtherActiveWorkflowTemplates(root, id, fileName, now)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	body["status"] = "active"
	if stringFromMap(body, "published_at") == "" {
		body["published_at"] = now
	}
	delete(body, "retired_at")
	body["hash"] = workflowTemplateHash(body)
	data, err := marshalWorkflowTemplateBody(body)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(root, "published"), 0o755); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	destination := filepath.Join(root, "published", fileName)
	if err := os.WriteFile(destination, data, 0o644); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	if location != "published" {
		_ = os.Remove(source)
	}
	detail, err := s.readWorkflowTemplateDetail(root, "published", fileName)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	return s.workflowLifecycleResult(root, detail, retired)
}

func (s *Server) retireWorkflowTemplate(root, location, fileName string) (workflowTemplateLifecycleResult, error) {
	detail, err := s.readWorkflowTemplateDetail(root, location, fileName)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	body := detail.JSON
	if err := validateWorkflowTemplateBody(body, fileName); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	body["status"] = "retired"
	if stringFromMap(body, "published_at") == "" && location == "published" {
		body["published_at"] = detail.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	body["retired_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	body["hash"] = workflowTemplateHash(body)
	data, err := marshalWorkflowTemplateBody(body)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	writeLocation := location
	if location == "drafts" {
		writeLocation = "retired"
	}
	if err := os.MkdirAll(filepath.Join(root, writeLocation), 0o755); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	if err := os.WriteFile(filepath.Join(root, writeLocation, fileName), data, 0o644); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	if writeLocation != location {
		_ = os.Remove(filepath.Join(root, location, fileName))
	}
	next, err := s.readWorkflowTemplateDetail(root, writeLocation, fileName)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	return s.workflowLifecycleResult(root, next, nil)
}

func (s *Server) moveWorkflowTemplateToDrafts(root, location, fileName string) (workflowTemplateLifecycleResult, error) {
	detail, err := s.readWorkflowTemplateDetail(root, location, fileName)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	body := detail.JSON
	if err := validateWorkflowTemplateBody(body, fileName); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	body["status"] = "draft"
	delete(body, "published_at")
	delete(body, "retired_at")
	delete(body, "hash")
	data, err := marshalWorkflowTemplateBody(body)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(root, "drafts"), 0o755); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	if err := os.WriteFile(filepath.Join(root, "drafts", fileName), data, 0o644); err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	if location != "drafts" {
		_ = os.Remove(filepath.Join(root, location, fileName))
	}
	next, err := s.readWorkflowTemplateDetail(root, "drafts", fileName)
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	return s.workflowLifecycleResult(root, next, nil)
}

func (s *Server) workflowLifecycleResult(root string, detail workflowTemplateDetail, retired []workflowTemplateSummary) (workflowTemplateLifecycleResult, error) {
	all, err := s.readAllWorkflowTemplateSummaries(root, "")
	if err != nil {
		return workflowTemplateLifecycleResult{}, err
	}
	attachWorkflowTemplateCounters(&detail.workflowTemplateSummary, all)
	current := currentWorkflowTemplates(all)
	conflicts := 0
	for _, counter := range workflowTemplateCounters(all) {
		if counter.Active > 1 {
			conflicts++
		}
	}
	return workflowTemplateLifecycleResult{Template: detail, Retired: retired, Current: current, ConflictCount: conflicts, VersionSummary: workflowTemplateCounters(all)}, nil
}

func (s *Server) retireOtherActiveWorkflowTemplates(root, id, keepFileName, retiredAt string) ([]workflowTemplateSummary, error) {
	all, err := s.readAllWorkflowTemplateSummaries(root, "published")
	if err != nil {
		return nil, err
	}
	retired := make([]workflowTemplateSummary, 0)
	for _, item := range all {
		if item.ID != id || item.FileName == keepFileName || item.Status != "active" || item.Location != "published" {
			continue
		}
		detail, err := s.readWorkflowTemplateDetail(root, item.Location, item.FileName)
		if err != nil {
			return nil, err
		}
		body := detail.JSON
		body["status"] = "retired"
		body["retired_at"] = retiredAt
		if stringFromMap(body, "published_at") == "" {
			body["published_at"] = item.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
		body["hash"] = workflowTemplateHash(body)
		data, err := marshalWorkflowTemplateBody(body)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(root, item.Location, item.FileName), data, 0o644); err != nil {
			return nil, err
		}
		next, err := s.readWorkflowTemplateSummary(root, item.Location, item.FileName)
		if err != nil {
			return nil, err
		}
		retired = append(retired, next)
	}
	return retired, nil
}

func (s *Server) ensureSingleActive(root, id, keepFileName string) error {
	all, err := s.readAllWorkflowTemplateSummaries(root, "published")
	if err != nil {
		return err
	}
	for _, item := range all {
		if item.ID == id && item.FileName != keepFileName && item.Status == "active" && item.Location == "published" {
			return fmt.Errorf("template %s already has active version %s; use publish lifecycle action to replace it", id, item.Version)
		}
	}
	return nil
}

func workflowTemplateCounters(items []workflowTemplateSummary) map[string]workflowCounter {
	counters := make(map[string]workflowCounter)
	seen := make(map[string]map[string]bool)
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if seen[item.ID] == nil {
			seen[item.ID] = make(map[string]bool)
		}
		key := item.Location + "/" + item.FileName
		counter := counters[item.ID]
		if !seen[item.ID][key] {
			counter.Versions++
			seen[item.ID][key] = true
		}
		switch item.Status {
		case "active":
			counter.Active++
		case "draft", "validated":
			counter.Draft++
		case "retired":
			counter.Retired++
		}
		counters[item.ID] = counter
	}
	return counters
}

func attachWorkflowTemplateCountersToList(items []workflowTemplateSummary, all []workflowTemplateSummary) {
	for i := range items {
		attachWorkflowTemplateCounters(&items[i], all)
	}
}

func attachWorkflowTemplateCounters(summary *workflowTemplateSummary, all []workflowTemplateSummary) {
	counter := workflowTemplateCounters(all)[summary.ID]
	summary.VersionCount = counter.Versions
	summary.ActiveCount = counter.Active
	summary.DraftCount = counter.Draft
	summary.RetiredCount = counter.Retired
	summary.HasConflict = counter.Active > 1
	summary.Current = summary.Status == "active" && summary.Location == "published"
}

func currentWorkflowTemplates(all []workflowTemplateSummary) []workflowTemplateSummary {
	byID := make(map[string][]workflowTemplateSummary)
	for _, item := range all {
		if item.ID == "" {
			continue
		}
		byID[item.ID] = append(byID[item.ID], item)
	}
	items := make([]workflowTemplateSummary, 0, len(byID))
	for _, versions := range byID {
		current := versions[0]
		for _, item := range versions {
			if workflowTemplateRank(item, current) {
				current = item
			}
		}
		attachWorkflowTemplateCounters(&current, all)
		items = append(items, current)
	}
	sortWorkflowTemplates(items)
	return items
}

func workflowTemplateRank(candidate, current workflowTemplateSummary) bool {
	candidateRank := workflowTemplateStatusRank(candidate)
	currentRank := workflowTemplateStatusRank(current)
	if candidateRank != currentRank {
		return candidateRank > currentRank
	}
	if cmp := compareWorkflowVersions(candidate.Version, current.Version); cmp != 0 {
		return cmp > 0
	}
	return candidate.UpdatedAt.After(current.UpdatedAt)
}

func workflowTemplateStatusRank(item workflowTemplateSummary) int {
	if item.Location == "published" && item.Status == "active" {
		return 4
	}
	if item.Location == "drafts" && (item.Status == "validated" || item.Status == "draft") {
		return 3
	}
	if item.Location == "published" && item.Status == "retired" {
		return 2
	}
	return 1
}

func sortWorkflowTemplates(items []workflowTemplateSummary) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		if cmp := compareWorkflowVersions(items[i].Version, items[j].Version); cmp != 0 {
			return cmp > 0
		}
		return items[i].Location < items[j].Location
	})
}

func compareWorkflowVersions(a, b string) int {
	pa := parseWorkflowVersion(a)
	pb := parseWorkflowVersion(b)
	for i := 0; i < len(pa); i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

func parseWorkflowVersion(value string) [3]int {
	var result [3]int
	parts := strings.Split(value, ".")
	for i := 0; i < len(result) && i < len(parts); i++ {
		parsed, _ := strconv.Atoi(parts[i])
		result[i] = parsed
	}
	return result
}

func validateWorkflowTemplateBody(body map[string]any, fileName string) error {
	id := stringFromMap(body, "id")
	version := stringFromMap(body, "version")
	if id == "" || version == "" {
		return errors.New("template JSON must include id and version")
	}
	if fileNameForWorkflowTemplate(id, version) != fileName {
		return fmt.Errorf("file name must match template id/version: %s", fileNameForWorkflowTemplate(id, version))
	}
	if len(arrayFromMap(body, "steps")) == 0 {
		return errors.New("template JSON must include at least one step")
	}
	if len(arrayFromMap(body, "completion_conditions")) == 0 {
		return errors.New("template JSON must include completion_conditions")
	}
	return nil
}

func fileNameForWorkflowTemplate(id, version string) string {
	return id + "@" + version + ".json"
}

func workflowTemplateHash(body map[string]any) string {
	clone := make(map[string]any, len(body))
	for key, value := range body {
		if key == "hash" {
			continue
		}
		clone[key] = value
	}
	data, _ := json.Marshal(clone)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum[:])
}

func marshalWorkflowTemplateBody(body map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func workflowLifecycleHTTPStatus(err error) int {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
