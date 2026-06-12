package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/devices"
)

type skillListItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	Trust         string `json:"trust,omitempty"`
	Maturity      string `json:"maturity,omitempty"`
	Installations int    `json:"installations"`
}

type runListItem struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status"`
	Device    string `json:"device,omitempty"`
	Skill     string `json:"skill,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
}

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	result, err := s.skillItems(r.Context())
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (s *Server) skillItems(ctx context.Context) ([]skillListItem, error) {
	deviceItems, err := s.devices.List(ctx)
	if err != nil {
		return nil, err
	}
	items := map[string]*skillListItem{}
	for _, device := range deviceItems {
		snapshot, err := s.devices.Snapshot(ctx, device.ID)
		if err != nil {
			return nil, err
		}
		if snapshot.Heartbeat != nil && len(snapshot.Heartbeat.Skills) > 0 {
			for _, skill := range snapshot.Heartbeat.Skills {
				addSkillSummary(items, skill)
			}
			continue
		}
		for _, capability := range snapshot.Device.Capabilities {
			addCapabilitySummary(items, capability)
		}
	}
	result := make([]skillListItem, 0, len(items))
	for _, item := range items {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func addSkillSummary(items map[string]*skillListItem, skill devices.SkillSummary) {
	name := strings.TrimSpace(skill.Name)
	if name == "" {
		return
	}
	key := "skill:" + name
	item, ok := items[key]
	if !ok {
		item = &skillListItem{ID: key, Name: name, Trust: "reported", Maturity: "active"}
		items[key] = item
	}
	if skill.Version != "" {
		item.Version = skill.Version
	}
	if !skill.Active {
		item.Maturity = "inactive"
	}
	item.Installations++
}

func addCapabilitySummary(items map[string]*skillListItem, capability devices.Capability) {
	name := strings.TrimSpace(capability.Name)
	if name == "" {
		return
	}
	key := "capability:" + name
	item, ok := items[key]
	if !ok {
		item = &skillListItem{ID: key, Name: name, Trust: "device", Maturity: "active"}
		items[key] = item
	}
	if capability.Version != "" {
		item.Version = capability.Version
	}
	if !capability.Enabled {
		item.Maturity = "inactive"
	}
	item.Installations++
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	deviceItems, err := s.devices.List(r.Context())
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	var result []runListItem
	for _, device := range deviceItems {
		items, err := s.commands.ListByDevice(r.Context(), device.ID)
		if err != nil {
			writeControlPlaneError(w, err)
			return
		}
		for _, command := range items {
			result = append(result, runItemFromCommand(command, device))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt > result[j].StartedAt
	})
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func runItemFromCommand(command commands.Command, device devices.Device) runListItem {
	startedAt := command.CreatedAt
	if command.StartedAt != nil {
		startedAt = *command.StartedAt
	}
	return runListItem{
		ID:        command.ID,
		Title:     commandTitle(command),
		Status:    string(command.Status),
		Device:    displayDeviceName(device),
		Skill:     commandSkillName(command),
		StartedAt: startedAt.UTC().Format(time.RFC3339Nano),
	}
}

func displayDeviceName(device devices.Device) string {
	if strings.TrimSpace(device.Name) != "" {
		return device.Name
	}
	return device.ID
}

func commandTitle(command commands.Command) string {
	if skill := commandSkillName(command); skill != "" {
		return string(command.Type) + " / " + skill
	}
	return string(command.Type)
}

func commandSkillName(command commands.Command) string {
	if command.Type != commands.TypeSkillInstall && command.Type != commands.TypeSkillRun && command.Type != commands.TypeSkillRollback {
		return ""
	}
	var payload struct {
		Skill string `json:"skill"`
	}
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Skill)
}
