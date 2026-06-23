package recall

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
	ErrInvalidPath        = errors.New("recall path must stay inside store directory")
	ErrConfirmationNeeded = errors.New("writing outside inbox requires confirmed=true")
	ErrFileExists         = errors.New("recall file exists; set overwrite=true to replace")
	ErrUnsupportedFile    = errors.New("recall path must be markdown or text")
	ErrDisallowedPath     = errors.New("recall path is outside allowed roots: profile.md, recall/docs/inbox/, recall/managed/notes/, recall/managed/cards/, recall/docs/projects/<project>/{project.md,environment.md,runbooks/}, recall/docs/devices/, recall/docs/ops/")
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

type Recall struct {
	Path        string            `json:"path"`
	Content     string            `json:"content"`
	Body        string            `json:"body"`
	Frontmatter map[string]string `json:"frontmatter"`
	SizeBytes   int               `json:"size_bytes"`
}

type SearchResult struct {
	Path          string            `json:"path"`
	Title         string            `json:"title,omitempty"`
	Snippet       string            `json:"snippet"`
	Frontmatter   map[string]string `json:"frontmatter"`
	MatchedTerms  []string          `json:"matched_terms,omitempty"`
	MatchedFields []string          `json:"matched_fields,omitempty"`
}

type RecallIndex struct {
	Path        string            `json:"path"`
	Title       string            `json:"title,omitempty"`
	Frontmatter map[string]string `json:"frontmatter,omitempty"`
	Aliases     []string          `json:"aliases,omitempty"`
	Keywords    []string          `json:"keywords,omitempty"`
	SizeBytes   int               `json:"size_bytes,omitempty"`
}

type WriteRequest struct {
	Path              string   `json:"path"`
	Content           string   `json:"content"`
	Type              string   `json:"type"`
	Scope             string   `json:"scope"`
	Status            string   `json:"status"`
	Project           string   `json:"project"`
	Device            string   `json:"device"`
	Agent             string   `json:"agent"`
	Skill             string   `json:"skill"`
	Source            string   `json:"source"`
	Confidence        string   `json:"confidence"`
	VerifiedAt        string   `json:"verified_at"`
	VerificationRunID string   `json:"verification_run_id"`
	SourceDevice      string   `json:"source_device"`
	SourceAgent       string   `json:"source_agent"`
	Tags              []string `json:"tags"`
	Confirmed         bool     `json:"confirmed"`
	Overwrite         bool     `json:"overwrite"`
}

type NoteRequest struct {
	Content string `json:"content"`
	Scope   string `json:"scope"`
	Name    string `json:"name"`
}

func NewStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		root = "recall"
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

func (s *Store) Read(path string) (Recall, error) {
	abs, err := s.resolve(path)
	if err != nil {
		return Recall{}, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Recall{}, err
	}
	if len(data) > MaxFileBytes {
		return Recall{}, fmt.Errorf("file too large: %d bytes", len(data))
	}
	if !utf8.Valid(data) {
		return Recall{}, errors.New("recall file must be utf-8 text")
	}
	rel, _ := filepath.Rel(s.root, abs)
	content := string(data)
	frontmatter, body := SplitFrontmatter(content)
	return Recall{Path: filepath.ToSlash(rel), Content: content, Body: body, Frontmatter: frontmatter, SizeBytes: len(data)}, nil
}

func (s *Store) Search(query, prefix string, maxResults int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 50
	}
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil, errors.New("query has no searchable terms")
	}
	base := s.root
	if strings.TrimSpace(prefix) != "" {
		resolved, err := s.resolve(prefix)
		if err != nil {
			return nil, err
		}
		base = resolved
	}
	type scoredResult struct {
		result SearchResult
		score  int
	}
	results := []scoredResult{}
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return []SearchResult{}, nil
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
		frontmatter, body := SplitFrontmatter(text)
		title := firstMarkdownTitle(body)

		pathText := strings.ToLower(rel)
		nameText := strings.ToLower(filepath.Base(rel))
		frontText := strings.ToLower(frontmatterText(frontmatter))
		titleText := strings.ToLower(title)
		bodyText := strings.ToLower(body)
		fullText := strings.ToLower(text)
		lowerQuery := strings.ToLower(query)

		matchedTerms := []string{}
		fieldSet := map[string]bool{}
		score := 0
		for _, term := range terms {
			matched := false
			if strings.Contains(pathText, term) {
				matched = true
				fieldSet["path"] = true
				score += 5
			}
			if strings.Contains(nameText, term) {
				matched = true
				fieldSet["filename"] = true
				score += 8
			}
			if strings.Contains(frontText, term) {
				matched = true
				fieldSet["frontmatter"] = true
				score += 7
			}
			if strings.Contains(titleText, term) {
				matched = true
				fieldSet["title"] = true
				score += 6
			}
			if strings.Contains(bodyText, term) {
				matched = true
				fieldSet["body"] = true
				score += 1
			}
			if matched {
				matchedTerms = append(matchedTerms, term)
			}
		}
		if strings.Contains(fullText, lowerQuery) || strings.Contains(pathText, lowerQuery) {
			score += 20
			fieldSet["exact_query"] = true
		}
		if len(matchedTerms) == 0 && !fieldSet["exact_query"] {
			return nil
		}
		matchedFields := sortedKeys(fieldSet)
		snippet := buildSnippet(rel, text, body, lowerQuery, matchedTerms)
		results = append(results, scoredResult{result: SearchResult{Path: rel, Title: title, Snippet: snippet, Frontmatter: frontmatter, MatchedTerms: matchedTerms, MatchedFields: matchedFields}, score: score})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].result.Path < results[j].result.Path
	})
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	out := make([]SearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, result.result)
	}
	return out, nil
}

func appendUniquePaths(paths []string, extras ...string) []string {
	seen := map[string]bool{}
	for _, path := range paths {
		seen[path] = true
	}
	for _, path := range extras {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func (s *Store) firstExistingPath(paths ...string) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		abs, err := s.resolve(path)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return path
		}
	}
	return ""
}

func (s *Store) listUnderAny(maxEntries int, bases ...string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, base := range bases {
		for _, path := range s.listUnder(base, maxEntries) {
			if seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
			if len(out) >= maxEntries {
				return out
			}
		}
	}
	return out
}

func (s *Store) RunbookIndex(project string, maxEntries int) ([]RecallIndex, error) {
	if maxEntries <= 0 || maxEntries > 200 {
		maxEntries = 50
	}
	if strings.TrimSpace(project) == "" {
		return []RecallIndex{}, nil
	}
	projectSegment := SafeSegment(project)
	paths := s.listUnderAny(maxEntries,
		"recall/docs/projects/"+projectSegment+"/runbooks",
	)
	indexes := make([]RecallIndex, 0, len(paths))
	for _, rel := range paths {
		mem, err := s.Read(rel)
		if err != nil {
			continue
		}
		indexes = append(indexes, RecallIndex{Path: mem.Path, Title: firstMarkdownTitle(mem.Body), Frontmatter: mem.Frontmatter, Aliases: frontmatterList(mem.Frontmatter, "aliases"), Keywords: frontmatterList(mem.Frontmatter, "keywords"), SizeBytes: mem.SizeBytes})
	}
	return indexes, nil
}

func (s *Store) Pack(project string, maxBytes int) ([]Recall, int, error) {
	if maxBytes <= 0 || maxBytes > 512000 {
		maxBytes = 120000
	}
	paths := []string{}
	paths = appendUniquePaths(paths, s.firstExistingPath("profile.md"))
	if strings.TrimSpace(project) != "" {
		projectSegment := SafeSegment(project)
		newBase := "recall/docs/projects/" + projectSegment
		paths = appendUniquePaths(paths,
			s.firstExistingPath(newBase+"/project.md"),
			s.firstExistingPath(newBase+"/conventions.md"),
			s.firstExistingPath(newBase+"/environment.md"),
			s.firstExistingPath(newBase+"/session-handoff.md"),
		)
		paths = appendUniquePaths(paths, s.listUnderAny(10, newBase+"/decisions")...)
		paths = appendUniquePaths(paths, s.listUnderAny(10, newBase+"/runbooks")...)
	}
	sections := []Recall{}
	total := 0
	seen := map[string]bool{}
	for _, rel := range paths {
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		section, err := s.Read(rel)
		if err != nil {
			continue
		}
		if total+len(section.Content) > maxBytes {
			remaining := maxBytes - total
			if remaining <= 0 {
				break
			}
			section.Content = section.Content[:remaining]
			section.Frontmatter, section.Body = SplitFrontmatter(section.Content)
			section.SizeBytes = len(section.Content)
		}
		sections = append(sections, section)
		total += len(section.Content)
		if total >= maxBytes {
			break
		}
	}
	return sections, total, nil
}

func IsAllowedRecallPath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if hasHiddenSegment(path) {
		return false
	}
	if path == "profile.md" {
		return true
	}
	if strings.HasPrefix(path, "recall/docs/inbox/") {
		return IsTextFile(path)
	}
	if strings.HasPrefix(path, "recall/managed/notes/") {
		// notes 是受控个人知识库根目录，允许多级 Markdown/Text 笔记；
		// 后端只接受 recall/managed/notes/ 原生新路径，不再依赖旧根别名。
		return IsTextFile(path)
	}
	if strings.HasPrefix(path, "recall/managed/cards/") {
		parts := strings.Split(path, "/")
		return len(parts) == 7 && parts[3] != "" && parts[4] != "" && parts[5] != "" && IsTextFile(path)
	}
	if strings.HasPrefix(path, "recall/docs/devices/") {
		parts := strings.Split(path, "/")
		return len(parts) == 4 && IsTextFile(path)
	}
	if strings.HasPrefix(path, "recall/docs/ops/") {
		parts := strings.Split(path, "/")
		return len(parts) == 4 && IsTextFile(path)
	}
	if strings.HasPrefix(path, "recall/docs/projects/") {
		parts := strings.Split(path, "/")
		if len(parts) < 5 || parts[3] == "" {
			return false
		}
		if len(parts) == 5 {
			return (parts[4] == "project.md" || parts[4] == "environment.md") && IsTextFile(path)
		}
		return len(parts) == 6 && parts[4] == "runbooks" && IsTextFile(path)
	}
	return false
}

func (s *Store) Write(req WriteRequest) (Recall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return Recall{}, errors.New("content is required")
	}
	path := filepath.ToSlash(strings.TrimSpace(req.Path))
	if path == "" {
		path = DefaultPath(req)
	}
	if !strings.HasPrefix(path, "recall/docs/inbox/") && !req.Confirmed {
		return Recall{}, ErrConfirmationNeeded
	}
	if !IsTextFile(path) {
		return Recall{}, ErrUnsupportedFile
	}
	if !IsAllowedRecallPath(path) {
		return Recall{}, ErrDisallowedPath
	}
	if raw := strings.TrimSpace(req.Scope); raw != "" && !Scope(strings.ToLower(raw)).Valid() {
		return Recall{}, fmt.Errorf("invalid recall scope %q", raw)
	}
	if raw := strings.TrimSpace(req.Status); raw != "" && !Status(strings.ToLower(raw)).Valid() {
		return Recall{}, fmt.Errorf("invalid recall status %q", raw)
	}
	if raw := strings.TrimSpace(req.Confidence); raw != "" && !Confidence(strings.ToLower(raw)).Valid() {
		return Recall{}, fmt.Errorf("invalid recall confidence %q", raw)
	}
	abs, err := s.resolve(path)
	if err != nil {
		return Recall{}, err
	}
	if _, err := os.Stat(abs); err == nil && !req.Overwrite {
		return Recall{}, ErrFileExists
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Recall{}, err
	}
	if !strings.HasPrefix(content, "---\n") {
		content = BuildFrontmatter(req) + "\n" + content + "\n"
	}
	if err := atomicWriteFile(abs, []byte(content), 0o644); err != nil {
		return Recall{}, err
	}
	return s.Read(path)
}

func (s *Store) AppendNote(req NoteRequest) (Recall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return Recall{}, errors.New("content is required")
	}
	scope := SafeSegment(req.Scope)
	if scope == "" {
		scope = "inbox"
	}
	if scope != "inbox" {
		return Recall{}, errors.New("append_note only writes to inbox; use recall_write with an explicit allowed path for long-term recall")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = time.Now().Format("20060102-150405") + "-note.md"
	}
	name = SafeFilename(name)
	path := filepath.ToSlash(filepath.Join("recall", "docs", scope, name))
	abs, err := s.resolve(path)
	if err != nil {
		return Recall{}, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Recall{}, err
	}
	entry := fmt.Sprintf("---\ntype: note\nscope: %s\nsource: user-confirmed\ncreated_at: %s\nupdated_at: %s\n---\n\n%s\n", scope, now(), now(), content)
	if _, err := os.Stat(abs); err == nil {
		entry = "\n\n---\n\n" + content + "\n"
		file, err := os.OpenFile(abs, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return Recall{}, err
		}
		defer file.Close()
		if _, err := file.WriteString(entry); err != nil {
			return Recall{}, err
		}
	} else if err := os.WriteFile(abs, []byte(entry), 0o644); err != nil {
		return Recall{}, err
	}
	return s.Read(path)
}

func (s *Store) Move(fromPath, toPath string, confirmed, overwrite bool) (Recall, error) {
	if !confirmed {
		return Recall{}, ErrConfirmationNeeded
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	fromPath = filepath.ToSlash(strings.TrimSpace(fromPath))
	toPath = filepath.ToSlash(strings.TrimSpace(toPath))
	if fromPath == "" || toPath == "" {
		return Recall{}, errors.New("from_path and to_path are required")
	}
	if hasHiddenSegment(fromPath) || hasHiddenSegment(toPath) {
		return Recall{}, ErrInvalidPath
	}
	if !IsAllowedRecallPath(toPath) {
		return Recall{}, ErrDisallowedPath
	}
	fromAbs, err := s.resolve(fromPath)
	if err != nil {
		return Recall{}, err
	}
	toAbs, err := s.resolve(toPath)
	if err != nil {
		return Recall{}, err
	}
	if fromAbs == toAbs {
		rel, _ := filepath.Rel(s.root, toAbs)
		return Recall{Path: filepath.ToSlash(rel), Frontmatter: map[string]string{}}, nil
	}
	info, err := os.Stat(fromAbs)
	if err != nil {
		return Recall{}, err
	}
	if info.IsDir() {
		if strings.HasPrefix(toAbs, fromAbs+string(filepath.Separator)) {
			return Recall{}, errors.New("cannot move a directory inside itself")
		}
		if _, err := os.Stat(toAbs); err == nil {
			return Recall{}, ErrFileExists
		}
		if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
			return Recall{}, err
		}
		if err := os.Rename(fromAbs, toAbs); err != nil {
			return Recall{}, err
		}
		removeEmptyParents(filepath.Dir(fromAbs), s.root)
		rel, _ := filepath.Rel(s.root, toAbs)
		return Recall{Path: filepath.ToSlash(rel), Frontmatter: map[string]string{}}, nil
	}
	if !IsTextFile(fromPath) || !IsTextFile(toPath) {
		return Recall{}, ErrUnsupportedFile
	}
	if _, err := os.Stat(toAbs); err == nil && !overwrite {
		return Recall{}, ErrFileExists
	}
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		return Recall{}, err
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		return Recall{}, err
	}
	removeEmptyParents(filepath.Dir(fromAbs), s.root)
	return s.Read(toPath)
}

func (s *Store) Delete(path string, confirmed bool) error {
	if !confirmed {
		return ErrConfirmationNeeded
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" || hasHiddenSegment(path) {
		return ErrInvalidPath
	}
	abs, err := s.resolve(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if abs == s.root || strings.HasSuffix(abs, string(filepath.Separator)+".git") {
			return ErrInvalidPath
		}
		if err := os.RemoveAll(abs); err != nil {
			return err
		}
		removeEmptyParents(filepath.Dir(abs), s.root)
		return nil
	}
	if !IsTextFile(abs) {
		return ErrUnsupportedFile
	}
	if err := os.Remove(abs); err != nil {
		return err
	}
	removeEmptyParents(filepath.Dir(abs), s.root)
	return nil
}

func hasHiddenSegment(rel string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}

func removeEmptyParents(dir, root string) {
	dir = filepath.Clean(dir)
	root = filepath.Clean(root)
	for dir != root && strings.HasPrefix(dir, root+string(filepath.Separator)) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func (s *Store) resolve(rel string) (string, error) {
	raw := strings.TrimSpace(rel)
	if raw == "" {
		return "", ErrInvalidPath
	}
	converted := filepath.FromSlash(raw)
	if filepath.IsAbs(converted) || hasParentSegment(raw) {
		return "", ErrInvalidPath
	}
	clean := filepath.Clean(converted)
	if clean == "." || clean == "" || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrInvalidPath
	}
	if hasHiddenSegment(clean) {
		return "", ErrInvalidPath
	}
	if isLegacyRecallRoot(filepath.ToSlash(clean)) {
		return "", ErrDisallowedPath
	}
	abs := filepath.Clean(filepath.Join(s.root, clean))
	rootWithSep := s.root + string(filepath.Separator)
	if abs != s.root && !strings.HasPrefix(abs, rootWithSep) {
		return "", ErrInvalidPath
	}
	return abs, nil
}

func isLegacyRecallRoot(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(strings.TrimPrefix(rel, "/")))
	for _, root := range []string{"cards", "notes", "projects", "devices", "ops", "inbox"} {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

func hasParentSegment(rel string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.TrimSpace(segment) == ".." {
			return true
		}
	}
	return false
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

func queryTerms(query string) []string {
	seen := map[string]bool{}
	terms := []string{}
	for _, raw := range strings.Fields(query) {
		term := strings.ToLower(strings.TrimSpace(raw))
		term = strings.Trim(term, "\"'`.,;:!?()[]{}<>，。；：！？（）【】《》")
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

func frontmatterText(frontmatter map[string]string) string {
	if len(frontmatter) == 0 {
		return ""
	}
	keys := sortedMapKeys(frontmatter)
	parts := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		parts = append(parts, key, frontmatter[key])
	}
	return strings.Join(parts, " ")
}

func firstMarkdownTitle(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}

func buildSnippet(path, content, body, lowerQuery string, terms []string) string {
	lowerContent := strings.ToLower(content)
	idx := -1
	needleLen := len(lowerQuery)
	if lowerQuery != "" {
		idx = strings.Index(lowerContent, lowerQuery)
	}
	if idx < 0 {
		for _, term := range terms {
			if term == "" {
				continue
			}
			if found := strings.Index(lowerContent, term); found >= 0 {
				idx = found
				needleLen = len(term)
				break
			}
		}
	}
	if idx < 0 {
		trimmed := strings.TrimSpace(body)
		if trimmed == "" {
			return path
		}
		if len(trimmed) > 260 {
			trimmed = trimmed[:260]
		}
		return trimmed
	}
	start := idx - 120
	if start < 0 {
		start = 0
	}
	end := idx + needleLen + 180
	if end > len(content) {
		end = len(content)
	}
	return strings.TrimSpace(content[start:end])
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, ok := range values {
		if ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func frontmatterList(frontmatter map[string]string, key string) []string {
	value := strings.TrimSpace(frontmatter[key])
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == '，' || r == '；' })
	out := []string{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
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
		typeName = "recall"
	}
	scope := SafeSegment(req.Scope)
	if scope == "" {
		scope = string(inferScope(req.Path))
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = string(StatusActive)
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
	b.WriteString("status: " + status + "\n")
	if project := SafeSegment(req.Project); project != "" {
		b.WriteString("project: " + project + "\n")
	}
	if device := strings.TrimSpace(req.Device); device != "" {
		b.WriteString("device: " + device + "\n")
	}
	if agent := strings.TrimSpace(req.Agent); agent != "" {
		b.WriteString("agent: " + agent + "\n")
	}
	if skill := strings.TrimSpace(req.Skill); skill != "" {
		b.WriteString("skill: " + skill + "\n")
	}
	b.WriteString("source: " + source + "\n")
	b.WriteString("confidence: " + confidence + "\n")
	if verifiedAt := strings.TrimSpace(req.VerifiedAt); verifiedAt != "" {
		b.WriteString("verified_at: " + verifiedAt + "\n")
	}
	if runID := strings.TrimSpace(req.VerificationRunID); runID != "" {
		b.WriteString("verification_run_id: " + runID + "\n")
	}
	if device := strings.TrimSpace(req.SourceDevice); device != "" {
		b.WriteString("source_device: " + device + "\n")
	}
	if agent := strings.TrimSpace(req.SourceAgent); agent != "" {
		b.WriteString("source_agent: " + agent + "\n")
	}
	b.WriteString("created_at: " + now() + "\n")
	b.WriteString("updated_at: " + now() + "\n")
	if len(req.Tags) > 0 {
		b.WriteString("tags: " + strings.Join(req.Tags, ",") + "\n")
	}
	b.WriteString("---\n")
	return b.String()
}

func DefaultPath(req WriteRequest) string {
	typeName := SafeSegment(req.Type)
	if typeName == "" {
		typeName = "recall"
	}
	stamp := time.Now().Format("20060102-150405")
	return filepath.ToSlash(filepath.Join("recall", "docs", "inbox", stamp+"-"+typeName+".md"))
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

func atomicWriteFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".recalldock-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	defer cleanup()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
