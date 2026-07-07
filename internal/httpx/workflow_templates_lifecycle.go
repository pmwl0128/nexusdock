package httpx

import (
	"sort"
	"strconv"
	"strings"
)

type workflowCounter struct {
	Versions int `json:"versions"`
	Active   int `json:"active"`
	Draft    int `json:"draft"`
	Retired  int `json:"retired"`
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
