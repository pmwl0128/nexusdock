package httpx

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultBackupStatusDir = "/backup-status"
const backupStatusID = "nexusdock-backup"

type backupHistory struct {
	SchemaVersion int    `json:"schema_version,omitempty"`
	State         string `json:"state"`
	Message       string `json:"message,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	Host          string `json:"host,omitempty"`
	Archive       string `json:"archive,omitempty"`
	ArchiveSize   int64  `json:"archive_size,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	RemotePath    string `json:"remote_path,omitempty"`
}

type backupStatus struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	Provider        string          `json:"provider"`
	Host            string          `json:"host"`
	Enabled         bool            `json:"enabled"`
	Schedule        string          `json:"schedule"`
	ScheduleType    string          `json:"schedule_type"`
	State           string          `json:"state"`
	LastStartedAt   string          `json:"last_started_at,omitempty"`
	LastCompletedAt string          `json:"last_completed_at,omitempty"`
	NextRunAt       string          `json:"next_run_at"`
	Message         string          `json:"message,omitempty"`
	Archive         string          `json:"archive,omitempty"`
	ArchiveSize     int64           `json:"archive_size,omitempty"`
	SHA256          string          `json:"sha256,omitempty"`
	RemotePath      string          `json:"remote_path,omitempty"`
	History         []backupHistory `json:"history"`
}

func backupStatusDir() string {
	if value := strings.TrimSpace(os.Getenv("NEXUS_BACKUP_STATUS_DIR")); value != "" {
		return filepath.Clean(value)
	}
	return defaultBackupStatusDir
}

func nextBackupRun(now time.Time) string {
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 30, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Format(time.RFC3339)
}

func loadBackupStatus(dir string, now time.Time) backupStatus {
	item := backupStatus{
		ID:           backupStatusID,
		Title:        "AgentDock + Nexus 云端备份",
		Description:  "备份 AgentDock、Nexus 源码和运行配置到天翼云盘",
		Provider:     "launchd",
		Host:         "DockMini",
		Enabled:      true,
		Schedule:     "每天 03:30",
		ScheduleType: "calendar",
		State:        "never_run",
		NextRunAt:    nextBackupRun(now),
		History:      []backupHistory{},
	}

	latestPath := filepath.Join(dir, "latest.json")
	if raw, err := os.ReadFile(latestPath); err == nil {
		var latest backupHistory
		if json.Unmarshal(raw, &latest) == nil {
			applyBackupStatus(&item, latest)
		} else {
			item.State = "unknown"
			item.Message = "latest.json 内容损坏"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		item.State = "unknown"
		item.Message = "无法读取备份状态"
	}

	historyPath := filepath.Join(dir, "history.jsonl")
	if file, err := os.Open(historyPath); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var entry backupHistory
			if json.Unmarshal(scanner.Bytes(), &entry) == nil {
				entry.State = normalizeBackupState(entry.State)
				item.History = append(item.History, entry)
			}
		}
		if len(item.History) > 50 {
			item.History = item.History[len(item.History)-50:]
		}
	}
	return item
}

func applyBackupStatus(item *backupStatus, status backupHistory) {
	item.State = normalizeBackupState(status.State)
	item.Message = status.Message
	item.LastStartedAt = status.StartedAt
	item.LastCompletedAt = status.CompletedAt
	item.Archive = status.Archive
	item.ArchiveSize = status.ArchiveSize
	item.SHA256 = status.SHA256
	item.RemotePath = status.RemotePath
}

func normalizeBackupState(state string) string {
	switch strings.TrimSpace(strings.ToLower(state)) {
	case "never_run", "queued", "running", "success", "failed", "unknown", "disabled":
		return strings.TrimSpace(strings.ToLower(state))
	case "succeeded", "completed":
		return "success"
	case "error", "errored":
		return "failed"
	default:
		return "unknown"
	}
}

func (s *Server) getBackupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, loadBackupStatus(backupStatusDir(), time.Now()))
}
