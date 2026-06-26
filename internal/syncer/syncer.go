package syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	RepoDir       string
	AutoSync      bool
	PullInterval  time.Duration
	PushDebounce  time.Duration
	CommitMessage string
}

type Status struct {
	OK              bool   `json:"ok"`
	GitRepo         bool   `json:"git_repo"`
	AutoSyncEnabled bool   `json:"auto_sync_enabled"`
	PendingPush     bool   `json:"pending_push"`
	LastPullAt      string `json:"last_pull_at,omitempty"`
	LastPushAt      string `json:"last_push_at,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	Conflict        bool   `json:"conflict"`
	Ahead           string `json:"ahead,omitempty"`
	Behind          string `json:"behind,omitempty"`
	Dirty           bool   `json:"dirty"`
}

type ChangedFile struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

type Diff struct {
	OK         bool          `json:"ok"`
	GitRepo    bool          `json:"git_repo"`
	Dirty      bool          `json:"dirty"`
	Status     string        `json:"status"`
	Stat       string        `json:"stat"`
	Diff       string        `json:"diff"`
	CachedDiff string        `json:"cached_diff"`
	Files      []ChangedFile `json:"files"`
}

type Commit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Date      string `json:"date"`
	Author    string `json:"author"`
	Subject   string `json:"subject"`
}

type Log struct {
	OK      bool     `json:"ok"`
	GitRepo bool     `json:"git_repo"`
	Commits []Commit `json:"commits"`
	Count   int      `json:"count"`
}

type CommitFile struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

type CommitDetail struct {
	OK      bool         `json:"ok"`
	GitRepo bool         `json:"git_repo"`
	Commit  Commit       `json:"commit"`
	Files   []CommitFile `json:"files"`
	Stat    string       `json:"stat"`
	Diff    string       `json:"diff"`
}

type Manager struct {
	cfg    Config
	logger *slog.Logger

	mu          sync.Mutex
	pendingPush bool
	lastPullAt  time.Time
	lastPushAt  time.Time
	lastError   string
	conflict    bool
	debounce    *time.Timer
}

func NewManager(cfg Config, logger *slog.Logger) *Manager {
	if cfg.PullInterval <= 0 {
		cfg.PullInterval = 120 * time.Second
	}
	if cfg.PushDebounce <= 0 {
		cfg.PushDebounce = 10 * time.Second
	}
	if strings.TrimSpace(cfg.CommitMessage) == "" {
		cfg.CommitMessage = "recall: 自动同步召回库"
	}
	return &Manager{cfg: cfg, logger: logger}
}

func (m *Manager) Start(ctx context.Context) {
	if !m.cfg.AutoSync {
		return
	}
	go func() {
		ticker := time.NewTicker(m.cfg.PullInterval)
		defer ticker.Stop()
		_ = m.Pull(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.Pull(ctx)
			}
		}
	}()
}

func (m *Manager) MarkChanged(ctx context.Context) {
	m.mu.Lock()
	m.pendingPush = true
	if !m.cfg.AutoSync || !m.isGitRepoLocked() {
		m.mu.Unlock()
		return
	}
	if m.debounce != nil {
		m.debounce.Stop()
	}
	delay := m.cfg.PushDebounce
	m.debounce = time.AfterFunc(delay, func() {
		_ = m.Sync(context.Background())
	})
	m.mu.Unlock()
	_ = ctx
}

func memoryGitArgs(args ...string) []string {
	return append(args, "--", ".", ":(exclude).nexus/**")
}

func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	status := Status{
		OK:              true,
		GitRepo:         m.isGitRepoLocked(),
		AutoSyncEnabled: m.cfg.AutoSync,
		PendingPush:     m.pendingPush,
		LastError:       m.lastError,
		Conflict:        m.conflict,
	}
	if !m.lastPullAt.IsZero() {
		status.LastPullAt = m.lastPullAt.Format(time.RFC3339)
	}
	if !m.lastPushAt.IsZero() {
		status.LastPushAt = m.lastPushAt.Format(time.RFC3339)
	}
	m.mu.Unlock()

	if status.GitRepo {
		if out, err := m.git(ctx, memoryGitArgs("status", "--porcelain")...); err == nil {
			status.Dirty = strings.TrimSpace(out) != ""
		}
		if out, err := m.git(ctx, "rev-list", "--left-right", "--count", "HEAD...@{upstream}"); err == nil {
			parts := strings.Fields(out)
			if len(parts) == 2 {
				status.Ahead = parts[0]
				status.Behind = parts[1]
			}
		}
	}
	return status
}

func (m *Manager) Diff(ctx context.Context) (Diff, error) {
	resp := Diff{OK: true, GitRepo: m.IsGitRepo()}
	if !resp.GitRepo {
		return resp, nil
	}
	status, err := m.git(ctx, memoryGitArgs("status", "--short", "--untracked-files=all")...)
	if err != nil {
		return resp, err
	}
	resp.Status = status
	resp.Dirty = strings.TrimSpace(status) != ""
	resp.Files = parseChangedFiles(status)
	if stat, err := m.git(ctx, memoryGitArgs("diff", "--stat")...); err == nil {
		resp.Stat = stat
	}
	if cachedDiff, err := m.git(ctx, memoryGitArgs("diff", "--cached", "--no-ext-diff")...); err == nil {
		resp.CachedDiff = cachedDiff
	}
	if diff, err := m.git(ctx, memoryGitArgs("diff", "--no-ext-diff")...); err == nil {
		resp.Diff = diff
	}
	return resp, nil
}

func parseChangedFiles(status string) []ChangedFile {
	seen := map[string]bool{}
	files := []ChangedFile{}
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = strings.TrimSpace(parts[len(parts)-1])
		}
		path = strings.Trim(path, `"`)
		path = filepath.ToSlash(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		if code == "" {
			code = strings.TrimSpace(line[:2])
		}
		files = append(files, ChangedFile{Status: code, Path: path})
	}
	return files
}

func (m *Manager) Discard(ctx context.Context, path string, confirmed bool) (Status, error) {
	if !confirmed {
		return m.Status(ctx), errors.New("confirmation required")
	}
	if !m.IsGitRepo() {
		return m.Status(ctx), nil
	}
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path != "" {
		if strings.HasPrefix(path, "/") || strings.Contains(path, "..") || strings.HasPrefix(path, ".") || strings.Contains(path, "//") {
			return m.Status(ctx), errors.New("invalid path")
		}
		if _, err := m.git(ctx, "restore", "--staged", "--worktree", "--", path); err != nil {
			// Untracked files cannot be restored; git clean below handles them.
			m.logger.Debug("git restore path failed before clean", "path", path, "error", err)
		}
		if _, err := m.git(ctx, "clean", "-fd", "--", path); err != nil {
			return m.Status(ctx), err
		}
	} else {
		if _, err := m.git(ctx, memoryGitArgs("restore", "--staged", "--worktree")...); err != nil {
			return m.Status(ctx), err
		}
		if _, err := m.git(ctx, "clean", "-fd", "-e", ".nexus/", "--", "."); err != nil {
			return m.Status(ctx), err
		}
	}

	if dirty, err := m.isDirty(ctx); err == nil && !dirty {
		m.mu.Lock()
		m.pendingPush = false
		m.lastError = ""
		m.conflict = false
		m.mu.Unlock()
	}
	return m.Status(ctx), nil
}

func (m *Manager) Log(ctx context.Context, limit int) (Log, error) {
	resp := Log{OK: true, GitRepo: m.IsGitRepo()}
	if !resp.GitRepo {
		return resp, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	format := "%H%x1f%h%x1f%ad%x1f%an%x1f%s%x1e"
	out, err := m.git(ctx, "log", "--date=iso-strict", "-n", strconv.Itoa(limit), "--pretty=format:"+format)
	if err != nil {
		return resp, err
	}
	for _, record := range strings.Split(out, "\x1e") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		fields := strings.Split(record, "\x1f")
		if len(fields) < 5 {
			continue
		}
		resp.Commits = append(resp.Commits, Commit{
			Hash:      fields[0],
			ShortHash: fields[1],
			Date:      fields[2],
			Author:    fields[3],
			Subject:   fields[4],
		})
	}
	resp.Count = len(resp.Commits)
	return resp, nil
}

func validCommitRef(ref string) bool {
	if len(ref) < 7 || len(ref) > 64 {
		return false
	}
	for _, ch := range ref {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return true
}

func (m *Manager) CommitDetail(ctx context.Context, hash string) (CommitDetail, error) {
	resp := CommitDetail{OK: true, GitRepo: m.IsGitRepo()}
	if !resp.GitRepo {
		return resp, nil
	}
	hash = strings.TrimSpace(hash)
	if !validCommitRef(hash) {
		return resp, errors.New("invalid commit hash")
	}
	format := "%H%x1f%h%x1f%ad%x1f%an%x1f%s"
	meta, err := m.git(ctx, "show", "--no-patch", "--date=iso-strict", "--pretty=format:"+format, hash)
	if err != nil {
		return resp, err
	}
	fields := strings.Split(meta, "\x1f")
	if len(fields) >= 5 {
		resp.Commit = Commit{Hash: fields[0], ShortHash: fields[1], Date: fields[2], Author: fields[3], Subject: fields[4]}
	}
	if stat, err := m.git(ctx, "show", "--stat", "--format=", "--summary", hash, "--"); err == nil {
		resp.Stat = stat
	}
	if diff, err := m.git(ctx, "show", "--no-ext-diff", "--format=", "--patch", hash, "--"); err == nil {
		resp.Diff = diff
	}
	if files, err := m.git(ctx, "show", "--name-status", "--format=", hash, "--"); err == nil {
		for _, line := range strings.Split(files, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			path := parts[len(parts)-1]
			resp.Files = append(resp.Files, CommitFile{Status: parts[0], Path: filepath.ToSlash(path)})
		}
	}
	return resp, nil
}

func (m *Manager) guardSafeMarkdownSync(ctx context.Context) error {
	out, err := m.git(ctx, "ls-files", "*.md")
	if err != nil {
		return err
	}
	zero := []string{}
	for _, rel := range strings.Split(out, "\n") {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		info, err := os.Stat(filepath.Join(m.cfg.RepoDir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() && info.Size() == 0 {
			zero = append(zero, rel)
			if len(zero) >= 20 {
				break
			}
		}
	}
	if len(zero) > 0 {
		return fmt.Errorf("refusing to sync zero-byte tracked markdown files: %s", strings.Join(zero, ", "))
	}
	return nil
}

func (m *Manager) guardSafeMarkdownStagedDiff(ctx context.Context) error {
	out, err := m.git(ctx, "diff", "--cached", "--numstat", "--", "*.md")
	if err != nil {
		return err
	}
	changedFiles, additions, deletions := 0, 0, 0
	deleteOnlyFiles := []string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		added, addErr := strconv.Atoi(fields[0])
		deleted, delErr := strconv.Atoi(fields[1])
		if addErr != nil || delErr != nil {
			continue
		}
		changedFiles++
		additions += added
		deletions += deleted
		if added == 0 && deleted > 0 {
			deleteOnlyFiles = append(deleteOnlyFiles, strings.Join(fields[2:], " "))
		}
	}
	if len(deleteOnlyFiles) >= 10 && deletions >= 100 {
		return fmt.Errorf("refusing suspicious bulk markdown delete-only change: files=%d additions=%d deletions=%d examples=%s", changedFiles, additions, deletions, strings.Join(limitStrings(deleteOnlyFiles, 20), ", "))
	}
	if changedFiles >= 20 && deletions >= 300 && deletions > additions*3 {
		return fmt.Errorf("refusing suspicious bulk markdown deletion: files=%d additions=%d deletions=%d", changedFiles, additions, deletions)
	}
	return nil
}

func limitStrings(values []string, max int) []string {
	if len(values) <= max {
		return values
	}
	limited := append([]string{}, values[:max]...)
	limited = append(limited, fmt.Sprintf("...and %d more", len(values)-max))
	return limited
}

func (m *Manager) Pull(ctx context.Context) error {
	if !m.IsGitRepo() {
		return nil
	}
	_, err := m.git(ctx, "pull", "--rebase", "--autostash")
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.lastError = err.Error()
		m.conflict = true
		m.logger.Warn("git pull failed", "error", err)
		return err
	}
	m.lastPullAt = time.Now()
	m.lastError = ""
	m.conflict = false
	return nil
}

func (m *Manager) Push(ctx context.Context) error {
	if !m.IsGitRepo() {
		return nil
	}
	dirty, err := m.isDirty(ctx)
	if err != nil {
		m.setError(err)
		return err
	}
	if !dirty {
		m.mu.Lock()
		m.pendingPush = false
		m.mu.Unlock()
		return nil
	}
	if err := m.guardSafeMarkdownSync(ctx); err != nil {
		m.setError(err)
		return err
	}
	if _, err := m.git(ctx, memoryGitArgs("add", "-A")...); err != nil {
		m.setError(err)
		return err
	}
	if err := m.guardSafeMarkdownStagedDiff(ctx); err != nil {
		m.setError(err)
		return err
	}
	if _, err := m.git(ctx, "commit", "-m", m.cfg.CommitMessage); err != nil {
		// git commit exits non-zero when there is nothing to commit. Re-check before treating it as fatal.
		dirty, dirtyErr := m.isDirty(ctx)
		if dirtyErr != nil || dirty {
			m.setError(err)
			return err
		}
	}
	if _, err := m.git(ctx, "push"); err != nil {
		m.setError(err)
		return err
	}
	m.mu.Lock()
	m.pendingPush = false
	m.lastPushAt = time.Now()
	m.lastError = ""
	m.conflict = false
	m.mu.Unlock()
	return nil
}

func (m *Manager) Sync(ctx context.Context) error {
	if !m.IsGitRepo() {
		return nil
	}
	if err := m.Pull(ctx); err != nil {
		return err
	}
	return m.Push(ctx)
}

func (m *Manager) IsGitRepo() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isGitRepoLocked()
}

func (m *Manager) isGitRepoLocked() bool {
	info, err := os.Stat(filepath.Join(m.cfg.RepoDir, ".git"))
	return err == nil && info.IsDir()
}

func (m *Manager) isDirty(ctx context.Context) (bool, error) {
	out, err := m.git(ctx, memoryGitArgs("status", "--porcelain")...)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func (m *Manager) git(ctx context.Context, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.Dir = m.cfg.RepoDir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, errors.New(text)
	}
	return text, nil
}

func (m *Manager) setError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastError = err.Error()
	m.conflict = true
}
