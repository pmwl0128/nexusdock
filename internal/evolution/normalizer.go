package evolution

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

type NormalizedError struct {
	Signature   string
	Category    string
	Transient   bool
	PrivatePath bool
}

type ErrorNormalizer struct{}

var (
	reUUID        = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	reHex         = regexp.MustCompile(`(?i)\b[0-9a-f]{16,}\b`)
	reTimestamp   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[tT ][0-9:.+-]+(?:[zZ])?\b`)
	reNumber      = regexp.MustCompile(`\b\d+\b`)
	reWhitespace  = regexp.MustCompile(`\s+`)
	reUnixPath    = regexp.MustCompile(`(?:^|\s|["'])((?:/Users|/home|/srv|/var|/opt|/tmp|/Volumes)/[^\s"']+)`)
	reWindowsPath = regexp.MustCompile(`(?i)\b[a-z]:\\[^\s"']+`)
)

func (ErrorNormalizer) Normalize(raw string) NormalizedError {
	original := strings.TrimSpace(raw)
	private := reUnixPath.MatchString(original) || reWindowsPath.MatchString(original)
	s := strings.ToLower((ErrorNormalizer{}).Redact(original))
	s = reUUID.ReplaceAllString(s, "<uuid>")
	s = reTimestamp.ReplaceAllString(s, "<time>")
	s = reHex.ReplaceAllString(s, "<hex>")
	s = reNumber.ReplaceAllString(s, "<n>")
	s = reWhitespace.ReplaceAllString(strings.TrimSpace(s), " ")

	category, transient := classifyError(s)
	if s == "" {
		s = "unspecified"
	}
	digest := sha256.Sum256([]byte(s))
	return NormalizedError{
		Signature: fmt.Sprintf("%s:%x", category, digest[:8]),
		Category:  category, Transient: transient, PrivatePath: private,
	}
}

// Redact removes device-local paths before summaries enter public DTOs,
// events, audit records, or proposals. Raw details remain in run evidence.
func (ErrorNormalizer) Redact(raw string) string {
	s := strings.TrimSpace(raw)
	s = reUnixPath.ReplaceAllString(s, " <path>")
	s = reWindowsPath.ReplaceAllString(s, "<path>")
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

func classifyError(s string) (string, bool) {
	cases := []struct {
		name      string
		transient bool
		terms     []string
	}{
		{"security_violation", false, []string{"security violation", "secret leaked", "path traversal", "symlink escape"}},
		{"network_timeout", true, []string{"timeout", "timed out", "deadline exceeded", "temporary failure"}},
		{"connection_refused", true, []string{"connection refused", "connection reset", "network unreachable", "dns"}},
		{"permission_denied", false, []string{"permission denied", "unauthorized", "forbidden"}},
		{"schema_validation", false, []string{"schema", "validation failed", "invalid input", "invalid output"}},
		{"not_found", false, []string{"not found", "no such file", "missing file"}},
		{"process_failure", false, []string{"exit status", "exit code", "signal: killed", "panic"}},
	}
	for _, c := range cases {
		for _, term := range c.terms {
			if strings.Contains(s, term) {
				return c.name, c.transient
			}
		}
	}
	return "unknown_failure", false
}
