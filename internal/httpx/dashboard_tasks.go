package httpx

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/devices"
)

type dashboardOverview struct {
	AgentTasks      int `json:"agent_tasks"`
	UserTasks       int `json:"user_tasks"`
	DeviceAlerts    int `json:"device_alerts"`
	SkillCandidates int `json:"skill_candidates"`
	MemoryConflicts int `json:"memory_conflicts"`
	RecentFailures  int `json:"recent_failures"`
}

type dashboardTask struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Source    string `json:"source,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func (s *Server) nexusOverview(w http.ResponseWriter, r *http.Request) {
	overview, _, err := s.dashboardState(r.Context())
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) listDashboardTasks(w http.ResponseWriter, r *http.Request) {
	_, tasks, err := s.dashboardState(r.Context())
	if err != nil {
		writeControlPlaneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tasks})
}

func (s *Server) dashboardState(ctx context.Context) (dashboardOverview, []dashboardTask, error) {
	var overview dashboardOverview
	tasks := []dashboardTask{}

	if s.devices != nil && s.commands != nil {
		deviceItems, err := s.devices.List(ctx)
		if err != nil {
			return overview, nil, err
		}
		for _, device := range deviceItems {
			snapshot, err := s.devices.Snapshot(ctx, device.ID)
			if err != nil {
				return overview, nil, err
			}
			memoryConflicts := snapshot.Heartbeat != nil && snapshot.Heartbeat.MemorySync.Conflicts > 0
			if memoryConflicts {
				overview.MemoryConflicts += snapshot.Heartbeat.MemorySync.Conflicts
				tasks = append(tasks, dashboardTask{
					ID:        "memory-conflict:" + device.ID,
					Title:     fmt.Sprintf("%s 有 %d 个记忆冲突", displayDeviceName(device), snapshot.Heartbeat.MemorySync.Conflicts),
					Type:      "needs_user",
					Status:    "awaiting_user",
					Source:    "memory_conflict",
					UpdatedAt: dashboardTime(device.UpdatedAt),
				})
			}
			if deviceNeedsAttention(device.Status) {
				overview.DeviceAlerts++
				tasks = append(tasks, deviceTask(device))
			}
			commandsForDevice, err := s.commands.ListByDevice(ctx, device.ID)
			if err != nil {
				return overview, nil, err
			}
			for _, command := range commandsForDevice {
				if command.Status == commands.StatusFailed {
					overview.RecentFailures++
					tasks = append(tasks, commandTask(command, device))
				}
			}
		}

		skills, err := s.skillItems(ctx)
		if err != nil {
			return overview, nil, err
		}
		for _, skill := range skills {
			if skill.Maturity != "active" {
				overview.SkillCandidates++
			}
		}
	}

	schedule := loadScheduleItem(scheduleStatusDir(), time.Now())
	if schedule.State == "failed" {
		overview.RecentFailures++
		tasks = append(tasks, scheduleTask(schedule))
	}

	for _, task := range tasks {
		switch task.Type {
		case "needs_user", "review":
			overview.UserTasks++
		default:
			overview.AgentTasks++
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		return taskSortTime(tasks[i].UpdatedAt).After(taskSortTime(tasks[j].UpdatedAt))
	})
	if len(tasks) > 50 {
		tasks = tasks[:50]
	}
	return overview, tasks, nil
}

func deviceNeedsAttention(status devices.Status) bool {
	switch status {
	case devices.StatusPending, devices.StatusDegraded, devices.StatusOffline, devices.StatusRevoked:
		return true
	default:
		return false
	}
}

func deviceTask(device devices.Device) dashboardTask {
	taskType := "needs_agent"
	status := "ready"
	source := "device_alert"
	title := fmt.Sprintf("处理设备状态: %s", displayDeviceName(device))
	if device.Status == devices.StatusPending {
		taskType = "needs_user"
		status = "awaiting_user"
		source = "device_approval"
		title = fmt.Sprintf("审批新设备: %s", displayDeviceName(device))
	}
	return dashboardTask{
		ID:        "device:" + device.ID,
		Title:     title,
		Type:      taskType,
		Status:    status,
		Source:    source,
		UpdatedAt: dashboardTime(device.UpdatedAt),
	}
}

func commandTask(command commands.Command, device devices.Device) dashboardTask {
	return dashboardTask{
		ID:        "run:" + command.ID,
		Title:     fmt.Sprintf("命令失败: %s / %s", displayDeviceName(device), commandTitle(command)),
		Type:      "needs_agent",
		Status:    "failed",
		Source:    "run_failure",
		UpdatedAt: dashboardTime(commandUpdatedAt(command)),
	}
}

func scheduleTask(schedule scheduleItem) dashboardTask {
	return dashboardTask{
		ID:        "schedule:" + schedule.ID,
		Title:     "计划任务失败: " + schedule.Title,
		Type:      "needs_agent",
		Status:    "failed",
		Source:    "schedule_failure",
		UpdatedAt: firstNonEmpty(schedule.LastCompletedAt, schedule.LastStartedAt),
	}
}

func commandUpdatedAt(command commands.Command) time.Time {
	if command.CompletedAt != nil {
		return *command.CompletedAt
	}
	if command.StartedAt != nil {
		return *command.StartedAt
	}
	if !command.UpdatedAt.IsZero() {
		return command.UpdatedAt
	}
	return command.CreatedAt
}

func dashboardTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func taskSortTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
