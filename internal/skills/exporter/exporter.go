package exporter

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/uvwt/agentdock-nexus/internal/skills/catalog"
	"github.com/uvwt/agentdock-nexus/internal/skills/importer"
)

type Format string

const (
	FormatGeneric   Format = "generic"
	FormatAgentDock Format = "agentdock"
	FormatAdapter   Format = "adapter"
)

const (
	ErrorInvalidPackage = "SKILL_EXPORT_INVALID_PACKAGE"
	ErrorUnsafePackage  = "SKILL_EXPORT_UNSAFE_PACKAGE"
	ErrorAdapterMissing = "SKILL_EXPORT_ADAPTER_MISSING"
	ErrorExportIO       = "SKILL_EXPORT_IO"
)

type ExportError struct {
	Code    string
	Message string
	Cause   error
}

func (e *ExportError) Error() string {
	if e.Cause == nil {
		return e.Code + ": " + e.Message
	}
	return e.Code + ": " + e.Message + ": " + e.Cause.Error()
}

func (e *ExportError) Unwrap() error { return e.Cause }

type Adapter interface {
	Name() string
	Files(context.Context, catalog.Manifest) (map[string][]byte, error)
}

type Request struct {
	PackageRoot string
	Destination string
	Format      Format
	Adapter     Adapter
}

type Result struct {
	Destination string
	FileCount   int
	Bytes       int64
}

type Exporter struct{}

var (
	secretPattern      = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|password|client[_-]?secret|private[_-]?key)\s*[:=]\s*["']?[A-Za-z0-9_./+=-]{12,}`)
	privatePathPattern = regexp.MustCompile(`(?m)(/Users/[^/\s]+/|/home/[^/\s]+/|[A-Za-z]:\\Users\\[^\\\s]+\\)`)
)

func (Exporter) Export(ctx context.Context, request Request) (Result, error) {
	if request.PackageRoot == "" || request.Destination == "" {
		return Result{}, &ExportError{Code: ErrorInvalidPackage, Message: "package root and destination are required"}
	}
	manifestData, err := os.ReadFile(filepath.Join(request.PackageRoot, "agentdock.yaml"))
	if err != nil {
		return Result{}, &ExportError{Code: ErrorInvalidPackage, Message: "read agentdock.yaml", Cause: err}
	}
	manifest, err := catalog.ParseManifest(manifestData)
	if err != nil {
		return Result{}, &ExportError{Code: ErrorInvalidPackage, Message: "validate agentdock.yaml", Cause: err}
	}
	if _, err := os.Stat(filepath.Join(request.PackageRoot, "SKILL.md")); err != nil {
		return Result{}, &ExportError{Code: ErrorInvalidPackage, Message: "SKILL.md is required", Cause: err}
	}
	report, err := importer.Scan(request.PackageRoot, manifest)
	if err != nil {
		return Result{}, &ExportError{Code: ErrorExportIO, Message: "scan package", Cause: err}
	}
	if report.Blocked {
		return Result{}, &ExportError{Code: ErrorUnsafePackage, Message: "static scan blocked export"}
	}

	files, err := collectFiles(request.PackageRoot, request.Format)
	if err != nil {
		return Result{}, err
	}
	if request.Format == FormatAdapter {
		if request.Adapter == nil {
			return Result{}, &ExportError{Code: ErrorAdapterMissing, Message: "target adapter is required"}
		}
		adapterFiles, err := request.Adapter.Files(ctx, manifest)
		if err != nil {
			return Result{}, &ExportError{Code: ErrorExportIO, Message: "generate adapter files", Cause: err}
		}
		for path, data := range adapterFiles {
			clean, err := validateArchivePath(path)
			if err != nil {
				return Result{}, &ExportError{Code: ErrorInvalidPackage, Message: "invalid adapter path", Cause: err}
			}
			if _, exists := files[clean]; exists {
				return Result{}, &ExportError{Code: ErrorInvalidPackage, Message: "adapter file conflicts with package: " + clean}
			}
			files[clean] = data
		}
	}
	for path, data := range files {
		if secretPattern.Match(data) {
			return Result{}, &ExportError{Code: ErrorUnsafePackage, Message: "possible secret in " + path}
		}
		if privatePathPattern.Match(data) {
			return Result{}, &ExportError{Code: ErrorUnsafePackage, Message: "private absolute path in " + path}
		}
	}
	return writeZIPAtomically(request.Destination, files)
}

func collectFiles(root string, format Format) (map[string][]byte, error) {
	switch format {
	case FormatGeneric, FormatAgentDock, FormatAdapter:
	default:
		return nil, &ExportError{Code: ErrorInvalidPackage, Message: fmt.Sprintf("unsupported format %q", format)}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, &ExportError{Code: ErrorExportIO, Message: "resolve package root", Cause: err}
	}
	files := make(map[string][]byte)
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
		if rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == "_metadata" || strings.HasPrefix(rel, "_metadata/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink export is not allowed: %s", rel)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file export is not allowed: %s", rel)
		}
		if format == FormatGeneric && rel == "agentdock.yaml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = data
		return nil
	})
	if err != nil {
		return nil, &ExportError{Code: ErrorInvalidPackage, Message: "collect package files", Cause: err}
	}
	if _, ok := files["SKILL.md"]; !ok {
		return nil, &ExportError{Code: ErrorInvalidPackage, Message: "SKILL.md is required"}
	}
	if format != FormatGeneric {
		if _, ok := files["agentdock.yaml"]; !ok {
			return nil, &ExportError{Code: ErrorInvalidPackage, Message: "agentdock.yaml is required"}
		}
	}
	return files, nil
}

func validateArchivePath(path string) (string, error) {
	path = strings.ReplaceAll(path, "\\", "/")
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, ":") {
		return "", errors.New("path must be relative")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path escapes archive root")
	}
	return clean, nil
}

func writeZIPAtomically(destination string, files map[string][]byte) (Result, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return Result{}, &ExportError{Code: ErrorExportIO, Message: "create export directory", Cause: err}
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".skill-export-*.zip")
	if err != nil {
		return Result{}, &ExportError{Code: ErrorExportIO, Message: "create export file", Cause: err}
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	archive := zip.NewWriter(temporary)
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var total int64
	for _, path := range paths {
		header := &zip.FileHeader{Name: path, Method: zip.Deflate}
		header.SetMode(0o644)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			archive.Close()
			temporary.Close()
			return Result{}, &ExportError{Code: ErrorExportIO, Message: "create archive entry", Cause: err}
		}
		written, err := io.Copy(writer, bytes.NewReader(files[path]))
		if err != nil {
			archive.Close()
			temporary.Close()
			return Result{}, &ExportError{Code: ErrorExportIO, Message: "write archive entry", Cause: err}
		}
		total += written
	}
	if err := archive.Close(); err != nil {
		temporary.Close()
		return Result{}, &ExportError{Code: ErrorExportIO, Message: "finalize archive", Cause: err}
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return Result{}, &ExportError{Code: ErrorExportIO, Message: "sync archive", Cause: err}
	}
	if err := temporary.Close(); err != nil {
		return Result{}, &ExportError{Code: ErrorExportIO, Message: "close archive", Cause: err}
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return Result{}, &ExportError{Code: ErrorExportIO, Message: "publish archive", Cause: err}
	}
	return Result{Destination: destination, FileCount: len(files), Bytes: total}, nil
}
