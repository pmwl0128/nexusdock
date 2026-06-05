package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SourceType string

const (
	SourceLocal   SourceType = "local"
	SourceZIP     SourceType = "zip"
	SourceGit     SourceType = "git"
	SourceGeneric SourceType = "generic_skill_md"
)

type Record struct {
	SourceType      SourceType `json:"source_type"`
	SourceURI       string     `json:"source_uri"`
	UpstreamVersion string     `json:"upstream_version,omitempty"`
	UpstreamCommit  string     `json:"upstream_commit,omitempty"`
	Digest          string     `json:"digest"`
	License         string     `json:"license,omitempty"`
	ImportedAt      time.Time  `json:"imported_at"`
}

func (r Record) Validate() error {
	switch r.SourceType {
	case SourceLocal, SourceZIP, SourceGit, SourceGeneric:
	default:
		return fmt.Errorf("unsupported source type %q", r.SourceType)
	}
	if r.SourceURI == "" {
		return fmt.Errorf("source URI is required")
	}
	if len(r.Digest) != 64 {
		return fmt.Errorf("digest must be a sha256 hex string")
	}
	if _, err := hex.DecodeString(r.Digest); err != nil {
		return fmt.Errorf("invalid digest: %w", err)
	}
	if r.ImportedAt.IsZero() {
		return fmt.Errorf("imported_at is required")
	}
	return nil
}

// SanitizeURI strips credentials and query fragments before provenance is
// persisted. Local paths are reduced to a file URI without resolving symlinks.
func SanitizeURI(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "file://" + filepath.ToSlash(raw)
	}
	return "file://" + filepath.ToSlash(abs)
}

func DigestDirectory(root string) (string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
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
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, rel := range paths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%s\x00%o\x00", rel, info.Mode().Perm())
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return "", err
			}
			hash.Write([]byte("link\x00" + target + "\x00"))
		case info.IsDir():
			hash.Write([]byte("dir\x00"))
		case info.Mode().IsRegular():
			file, err := os.Open(path)
			if err != nil {
				return "", err
			}
			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil {
				return "", copyErr
			}
			if closeErr != nil {
				return "", closeErr
			}
		default:
			return "", fmt.Errorf("unsupported file type %s", rel)
		}
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func Marshal(record Record) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(record, "", "  ")
}
