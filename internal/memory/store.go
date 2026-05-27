package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const MaxFileBytes = 512 * 1024

var (
	ErrInvalidPath        = errors.New("memory path must stay inside store directory")
	ErrConfirmationNeeded = errors.New("writing outside inbox requires confirmed=true")
	ErrFileExists         = errors.New("memory file exists; set overwrite=true to replace")
	ErrUnsupportedFile    = errors.New("memory path must be markdown or text")
)

type Store struct {
	root string
	mu   sync.Mutex
}

type Entry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Modified  string `json:"modified,omitempty"`
}

type Memory struct {
	Path        string            `json:"path"`
	Content     string            `json:"content"`
	Body        string            `json:"body"`
	Frontmatter map[string]string `json:"frontmatter"`
	SizeBytes   int               `json:"size_bytes"`
}

type SearchResult struct {
	Path        string            `json:"path"`
	Snippet     string            `json:"snippet"`
	Frontmatter map[string]string `json:"frontmatter"`
}

type WriteRequest struct {
	Path       string   `json:"path"`
	Content    string   `json:"content"`
	Type       string   `json:"type"`
	Scope      string   `json:"scope"`
	Project    string   `json:"project"`
	Source     string   `json:"source"`
	Confidence string   `json:"confidence"`
	Tags       []string `json:"tags"`
	Confirmed  bool     `json:"confirmed"`
	Overwrite  bool     `json:"overwrite"`
}

type NoteRequest struct {
	Content string `json:"content"`
	Scope   string `json:"scope"`
	Name    string `json:"name"`
}

func NewStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		root = "memory"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: filepath.Clean(abs)}, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) List(prefix string, maxEntries int) ([]Entry, error) {
	if maxEntries <= 0 || maxEntries > 1000 {
		maxEntries = 200
	}
	base := s.root
	if strings.TrimSpace(prefix) != "" {
		resolved, err := s.resolve(prefix)
		if err != nil {
			return nil, err
		}
		base = resolved
	}
	entries := []Entry{}
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return entries, nil
	}
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && name == ".git" {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(s.root, path)
		entry := Entry{Path: filepath.ToSlash(rel), Name: name, Type: "file"}
		if d.IsDir() {
			entry.Type = "directory"
		}
		if info, err := d.Info(); err == nil {
			entry.SizeBytes = info.Size()
			entry.Modified = info.ModTime().Format(time.RFC3339)
		}
		entries = append(entries, entry)
		if len(entries) >= maxEntries {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (s *Store) Read(path string) (Memory, error) {
	abs, err := s.resolve(path)
	if err != nil {
		return Memory{}, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Memory{}, err
	}
	if len(data) > MaxFileBytes {
		return Memory{}, fmt.Errorf("file too large: %d bytes", len(data))
	}
	if !utf8.Valid(data) {
		return Memory{}, errors.New("memory file must be utf-8 text")
	}
	rel, _ := filepath.Rel(s.root, abs)
	content := string(data)
	frontmatter, body := SplitFrontmatter(content)
	return Memory{Path: filepath.ToSlash(rel), Content: content, Body: body, Frontmatter: frontmatter, SizeBytes: len(data)}, nil
}

func (s *Store) Search(query, prefix string, maxResults int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 50
	}
	base := s.root
	if strings.TrimSpace(prefix) != "" {
		resolved, err := s.resolve(prefix)
		if err != nil {
			return nil, err
		}
		base = resolved
	}
	lower := strings.ToLower(query)
	results := []SearchResult{}
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return results, nil
	}
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsTextFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > MaxFileBytes || !utf8.Valid(data) {
			return nil
		}
		rel, _ := filepath.Rel(s.root, path)
		rel = filepath.ToSlash(rel)
		text := string(data)
		idx := strings.Index(strings.ToLower(text), lower)
		if idx < 0 && !strings.Contains(strings.ToLower(rel), lower) {
			return nil
		}
		snippet := rel
		if idx >= 0 {
			start := idx - 120
			if start < 0 {
				start = 0
			}
			end := idx + len(query) + 180
			if end > len(text) {
				end = len(text)
			}
			snippet = strings.TrimSpace(text[start:end])
		}
		frontmatter, _ := SplitFrontmatter(text)
		results = append(results, SearchResult{Path: rel, Snippet: snippet, Frontmatter: frontmatter})
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		return nil
	})
	return results, err
}

func (s *Store) Pack(project string, maxBytes int) ([]Memory, int, error) {
	if maxBytes <= 0 || maxBytes > 512000 {
		maxBytes = 120000
	}
	paths := []string{"shared/profile.md"}
	if strings.TrimSpace(project) != "" {
		base := "shared/projects/" + SafeSegment(project)
		paths = append(paths,
			base+"/overview.md",
			base+"/conventions.md",
			base+"/environment.md",
			base+"/session-handoff.md",
		)
		paths = append(paths, s.listUnder(base+"/decisions", 10)...)
		paths = append(paths, s.listUnder(base+"/runbooks", 10)...)
	}
	sections := []Memory{}
	total := 0
	seen := map[string]bool{}
	for _, rel := range paths {
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		memory, err := s.Read(rel)
		if err != nil {
			continue
		}
		if total+len(memory.Content) > maxBytes {
			remaining := maxBytes - total
			if remaining <= 0 {
				break
			}
			memory.Content = memory.Content[:remaining]
			memory.Frontmatter, memory.Body = SplitFrontmatter(memory.Content)
			memory.SizeBytes = len(memory.Content)
		}
		sections = append(sections, memory)
		total += len(memory.Content)
		if total >= maxBytes {
			break
		}
	}
	return sections, total, nil
}

func (s *Store) Write(req WriteRequest) (Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return Memory{}, errors.New("content is required")
	}
	path := filepath.ToSlash(strings.TrimSpace(req.Path))
	if path == "" {
		path = DefaultPath(req)
	}
	if !strings.HasPrefix(path, "inbox/") && !req.Confirmed {
		return Memory{}, ErrConfirmationNeeded
	}
	if !IsTextFile(path) {
		return Memory{}, ErrUnsupportedFile
	}
	abs, err := s.resolve(path)
	if err != nil {
		return Memory{}, err
	}
	if _, err := os.Stat(abs); err == nil && !req.Overwrite {
		return Memory{}, ErrFileExists
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Memory{}, err
	}
	if !strings.HasPrefix(content, "---\n") {
		content = BuildFrontmatter(req) + "\n" + content + "\n"
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return Memory{}, err
	}
	return s.Read(path)
}

func (s *Store) AppendNote(req NoteRequest) (Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return Memory{}, errors.New("content is required")
	}
	scope := SafeSegment(req.Scope)
	if scope == "" {
		scope = "inbox"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = time.Now().Format("20060102-150405") + "-note.md"
	}
	name = SafeFilename(name)
	path := filepath.ToSlash(filepath.Join(scope, name))
	abs, err := s.resolve(path)
	if err != nil {
		return Memory{}, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Memory{}, err
	}
	entry := fmt.Sprintf("---\ntype: note\nscope: %s\nsource: user-confirmed\ncreated_at: %s\nupdated_at: %s\n---\n\n%s\n", scope, now(), now(), content)
	if _, err := os.Stat(abs); err == nil {
		entry = "\n\n---\n\n" + content + "\n"
		file, err := os.OpenFile(abs, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return Memory{}, err
		}
		defer file.Close()
		if _, err := file.WriteString(entry); err != nil {
			return Memory{}, err
		}
	} else if err := os.WriteFile(abs, []byte(entry), 0o644); err != nil {
		return Memory{}, err
	}
	return s.Read(path)
}

func (s *Store) Delete(path string, confirmed bool) error {
	if !confirmed {
		return ErrConfirmationNeeded
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	abs, err := s.resolve(path)
	if err != nil {
		return err
	}
	if !IsTextFile(abs) {
		return ErrUnsupportedFile
	}
	return os.Remove(abs)
}

func (s *Store) resolve(rel string) (string, error) {
	rel = filepath.Clean(strings.TrimPrefix(filepath.FromSlash(strings.TrimSpace(rel)), string(filepath.Separator)))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", ErrInvalidPath
	}
	abs := filepath.Clean(filepath.Join(s.root, rel))
	rootWithSep := s.root + string(filepath.Separator)
	if abs != s.root && !strings.HasPrefix(abs, rootWithSep) {
		return "", ErrInvalidPath
	}
	return abs, nil
}

func (s *Store) listUnder(rel string, max int) []string {
	abs, err := s.resolve(rel)
	if err != nil {
		return nil
	}
	files := []string{}
	_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !IsTextFile(path) {
			return nil
		}
		relPath, _ := filepath.Rel(s.root, path)
		files = append(files, filepath.ToSlash(relPath))
		return nil
	})
	sort.Strings(files)
	if len(files) > max {
		files = files[len(files)-max:]
	}
	return files
}

func IsTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || ext == ".txt"
}

func SplitFrontmatter(content string) (map[string]string, string) {
	meta := map[string]string{}
	if !strings.HasPrefix(content, "---\n") {
		return meta, content
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return meta, content
	}
	front := content[4 : 4+end]
	body := content[4+end+5:]
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		meta[key] = value
	}
	return meta, body
}

func BuildFrontmatter(req WriteRequest) string {
	typeName := strings.TrimSpace(req.Type)
	if typeName == "" {
		typeName = "memory"
	}
	scope := SafeSegment(req.Scope)
	if scope == "" {
		if strings.HasPrefix(req.Path, "shared/") {
			scope = "shared"
		} else {
			scope = "inbox"
		}
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "user-confirmed"
	}
	confidence := strings.TrimSpace(req.Confidence)
	if confidence == "" {
		confidence = "medium"
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: " + typeName + "\n")
	b.WriteString("scope: " + scope + "\n")
	if project := SafeSegment(req.Project); project != "" {
		b.WriteString("project: " + project + "\n")
	}
	b.WriteString("source: " + source + "\n")
	b.WriteString("confidence: " + confidence + "\n")
	b.WriteString("created_at: " + now() + "\n")
	b.WriteString("updated_at: " + now() + "\n")
	if len(req.Tags) > 0 {
		b.WriteString("tags: " + strings.Join(req.Tags, ",") + "\n")
	}
	b.WriteString("---\n")
	return b.String()
}

func DefaultPath(req WriteRequest) string {
	scope := SafeSegment(req.Scope)
	if scope == "" {
		scope = "inbox"
	}
	typeName := SafeSegment(req.Type)
	if typeName == "" {
		typeName = "memory"
	}
	stamp := time.Now().Format("20060102-150405")
	if project := SafeSegment(req.Project); project != "" && scope == "shared" {
		return filepath.ToSlash(filepath.Join("shared", "projects", project, typeName+"s", stamp+"-"+typeName+".md"))
	}
	return filepath.ToSlash(filepath.Join(scope, stamp+"-"+typeName+".md"))
}

func SafeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-_")
}

func SafeFilename(value string) string {
	value = filepath.Base(filepath.ToSlash(value))
	if !IsTextFile(value) {
		value += ".md"
	}
	clean := SafeSegment(strings.TrimSuffix(value, filepath.Ext(value)))
	if clean == "" {
		clean = time.Now().Format("20060102-150405") + "-note"
	}
	return clean + strings.ToLower(filepath.Ext(value))
}

func now() string { return time.Now().Format(time.RFC3339) }
