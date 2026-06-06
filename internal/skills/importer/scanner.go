package importer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/uvwt/agentdock-nexus/internal/skills/catalog"
)

type FindingSeverity string

const (
	SeverityInfo     FindingSeverity = "info"
	SeverityWarning  FindingSeverity = "warning"
	SeverityHigh     FindingSeverity = "high"
	SeverityCritical FindingSeverity = "critical"
)

type Finding struct {
	Code     string          `json:"code"`
	Severity FindingSeverity `json:"severity"`
	Path     string          `json:"path"`
	Line     int             `json:"line,omitempty"`
	Message  string          `json:"message"`
}

type ScanReport struct {
	Findings []Finding `json:"findings"`
	Blocked  bool      `json:"blocked"`
}

var (
	dangerousShellPatterns = []struct {
		code    string
		pattern *regexp.Regexp
		message string
	}{
		{"DANGEROUS_SHELL", regexp.MustCompile(`(?i)(^|[;&|]\s*)(rm\s+-rf\s+/(?:\s|$)|mkfs(?:\.|\s)|dd\s+if=.*\s+of=/dev/)`), "destructive shell command detected"},
		{"DOWNLOAD_EXECUTE", regexp.MustCompile(`(?i)(curl|wget)[^\n|;]*(\||;|&&)\s*(sh|bash|zsh|python|node)(\s|$)`), "download-and-execute pipeline detected"},
		{"SECRET_LEAK", regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*["']?[A-Za-z0-9_./+=-]{12,}`), "possible embedded secret detected"},
	}
	executableExtensions = map[string]struct{}{".exe": {}, ".dll": {}, ".dylib": {}, ".so": {}, ".bin": {}, ".class": {}, ".jar": {}, ".wasm": {}}
)

func Scan(root string, manifest catalog.Manifest) (ScanReport, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return ScanReport{}, err
	}
	declaredHosts := make(map[string]struct{})
	for _, host := range manifest.Spec.Permissions.Network.Hosts {
		declaredHosts[strings.ToLower(strings.TrimPrefix(host, "*."))] = struct{}{}
	}
	var findings []Finding
	err = filepath.WalkDir(rootAbs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == rootAbs {
			return nil
		}
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(rel, "../") || rel == ".." || filepath.IsAbs(rel) {
			findings = append(findings, Finding{Code: "PATH_TRAVERSAL", Severity: SeverityCritical, Path: rel, Message: "path escapes package root"})
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				findings = append(findings, Finding{Code: "BROKEN_SYMLINK", Severity: SeverityHigh, Path: rel, Message: "symlink target cannot be resolved"})
				return nil
			}
			targetAbs, err := filepath.Abs(target)
			if err != nil || !withinRoot(rootAbs, targetAbs) {
				findings = append(findings, Finding{Code: "SYMLINK_ESCAPE", Severity: SeverityCritical, Path: rel, Message: "symlink target escapes package root"})
			}
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && entry.Name() != ".well-known" {
				findings = append(findings, Finding{Code: "HIDDEN_DIRECTORY", Severity: SeverityWarning, Path: rel, Message: "hidden directory requires review"})
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			findings = append(findings, Finding{Code: "SPECIAL_FILE", Severity: SeverityCritical, Path: rel, Message: "special files are not allowed"})
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			findings = append(findings, Finding{Code: "HIDDEN_FILE", Severity: SeverityWarning, Path: rel, Message: "hidden file requires review"})
		}
		if _, ok := executableExtensions[strings.ToLower(filepath.Ext(entry.Name()))]; ok {
			findings = append(findings, Finding{Code: "HIDDEN_BINARY", Severity: SeverityHigh, Path: rel, Message: "binary artifact requires explicit review"})
		}
		if info.Size() > 4<<20 {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		content, readErr := io.ReadAll(io.LimitReader(file, 4<<20))
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if bytes.IndexByte(content, 0) >= 0 {
			if info.Mode().Perm()&0o111 != 0 {
				findings = append(findings, Finding{Code: "HIDDEN_BINARY", Severity: SeverityHigh, Path: rel, Message: "executable binary artifact requires explicit review"})
			}
			return nil
		}
		findings = append(findings, scanText(rel, content, declaredHosts)...)
		return nil
	})
	if err != nil {
		return ScanReport{}, err
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			if findings[i].Line == findings[j].Line {
				return findings[i].Code < findings[j].Code
			}
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Path < findings[j].Path
	})
	report := ScanReport{Findings: findings}
	for _, finding := range findings {
		if finding.Severity == SeverityHigh || finding.Severity == SeverityCritical {
			report.Blocked = true
			break
		}
	}
	return report, nil
}

func scanText(path string, content []byte, declaredHosts map[string]struct{}) []Finding {
	var findings []Finding
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		for _, rule := range dangerousShellPatterns {
			if rule.pattern.MatchString(line) {
				severity := SeverityHigh
				if rule.code == "DANGEROUS_SHELL" || rule.code == "SECRET_LEAK" {
					severity = SeverityCritical
				}
				findings = append(findings, Finding{Code: rule.code, Severity: severity, Path: path, Line: lineNumber, Message: rule.message})
			}
		}
		for _, rawURL := range extractHTTPURLs(line) {
			parsed, err := url.Parse(rawURL)
			if err != nil || parsed.Hostname() == "" {
				continue
			}
			host := strings.ToLower(parsed.Hostname())
			if !hostDeclared(host, declaredHosts) {
				findings = append(findings, Finding{Code: "UNDECLARED_NETWORK", Severity: SeverityHigh, Path: path, Line: lineNumber, Message: fmt.Sprintf("network host %s is not declared", host)})
			}
		}
	}
	return findings
}

func extractHTTPURLs(line string) []string {
	matcher := regexp.MustCompile(`https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+`)
	return matcher.FindAllString(line, -1)
}

func hostDeclared(host string, declared map[string]struct{}) bool {
	for candidate := range declared {
		if host == candidate || strings.HasSuffix(host, "."+candidate) {
			return true
		}
	}
	return false
}

func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}
