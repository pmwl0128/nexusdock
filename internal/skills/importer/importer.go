package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/uvwt/memorydock/internal/skills/catalog"
	"github.com/uvwt/memorydock/internal/skills/provenance"
)

const (
	ErrorInvalidSource    = "SKILL_INVALID_SOURCE"
	ErrorInvalidManifest  = "SKILL_INVALID_MANIFEST"
	ErrorUnsafePackage    = "SKILL_UNSAFE_PACKAGE"
	ErrorPackageConflict  = "SKILL_PACKAGE_CONFLICT"
	ErrorImportIO         = "SKILL_IMPORT_IO"
	defaultMaxArchiveSize = int64(128 << 20)
)

type ImportError struct {
	Code    string
	Message string
	Cause   error
}

func (e *ImportError) Error() string {
	if e.Cause == nil {
		return e.Code + ": " + e.Message
	}
	return e.Code + ": " + e.Message + ": " + e.Cause.Error()
}

func (e *ImportError) Unwrap() error { return e.Cause }

type Request struct {
	SourceType      provenance.SourceType
	SourceURI       string
	Manifest        *catalog.Manifest
	UpstreamVersion string
	License         string
	AllowUnsafe     bool
}

type Result struct {
	PackagePath string
	Manifest    catalog.Manifest
	Provenance  provenance.Record
	Scan        ScanReport
}

type Importer struct {
	StoreRoot      string
	Now            func() time.Time
	MaxArchiveSize int64
	GitBinary      string
}

func (i *Importer) Import(ctx context.Context, request Request) (Result, error) {
	if i.StoreRoot == "" {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "store root is required"}
	}
	if err := os.MkdirAll(i.StoreRoot, 0o755); err != nil {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "create store root", Cause: err}
	}
	if request.SourceURI == "" {
		return Result{}, &ImportError{Code: ErrorInvalidSource, Message: "source URI is required"}
	}
	if i.Now == nil {
		i.Now = time.Now
	}
	if i.MaxArchiveSize <= 0 {
		i.MaxArchiveSize = defaultMaxArchiveSize
	}

	staging, err := os.MkdirTemp(i.StoreRoot, ".skill-import-")
	if err != nil {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "create import staging directory", Cause: err}
	}
	defer os.RemoveAll(staging)

	packageRoot := filepath.Join(staging, "package")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "create package staging directory", Cause: err}
	}

	commit, err := i.materialize(ctx, request, packageRoot)
	if err != nil {
		return Result{}, err
	}
	manifest, err := resolveManifest(packageRoot, request.Manifest, request.License)
	if err != nil {
		return Result{}, &ImportError{Code: ErrorInvalidManifest, Message: "resolve agentdock.yaml", Cause: err}
	}
	manifestBytes, err := catalog.MarshalManifest(manifest)
	if err != nil {
		return Result{}, &ImportError{Code: ErrorInvalidManifest, Message: "serialize agentdock.yaml", Cause: err}
	}
	manifestPath := filepath.Join(packageRoot, "agentdock.yaml")
	if _, statErr := os.Stat(manifestPath); errors.Is(statErr, os.ErrNotExist) {
		if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
			return Result{}, &ImportError{Code: ErrorImportIO, Message: "write agentdock.yaml", Cause: err}
		}
	}
	if _, err := os.Stat(filepath.Join(packageRoot, "SKILL.md")); err != nil {
		return Result{}, &ImportError{Code: ErrorInvalidSource, Message: "package must contain SKILL.md", Cause: err}
	}

	scan, err := Scan(packageRoot, manifest)
	if err != nil {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "scan package", Cause: err}
	}
	if scan.Blocked && !request.AllowUnsafe {
		return Result{}, &ImportError{Code: ErrorUnsafePackage, Message: "static scan blocked the package"}
	}
	digest, err := provenance.DigestDirectory(packageRoot)
	if err != nil {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "digest package", Cause: err}
	}
	record := provenance.Record{
		SourceType:      request.SourceType,
		SourceURI:       provenance.SanitizeURI(request.SourceURI),
		UpstreamVersion: request.UpstreamVersion,
		UpstreamCommit:  commit,
		Digest:          digest,
		License:         firstNonEmpty(request.License, manifest.Metadata.License),
		ImportedAt:      i.Now().UTC(),
	}
	if err := record.Validate(); err != nil {
		return Result{}, &ImportError{Code: ErrorInvalidSource, Message: "invalid provenance", Cause: err}
	}

	destination := filepath.Join(i.StoreRoot, manifest.Metadata.Name, manifest.Metadata.Version)
	if _, err := os.Stat(destination); err == nil {
		return Result{}, &ImportError{Code: ErrorPackageConflict, Message: "skill version already exists"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "inspect destination", Cause: err}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "create skill directory", Cause: err}
	}
	if err := os.Rename(packageRoot, destination); err != nil {
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "publish imported package", Cause: err}
	}
	if err := writeProvenance(i.StoreRoot, manifest.Metadata.Name, manifest.Metadata.Version, record); err != nil {
		_ = os.RemoveAll(destination)
		return Result{}, &ImportError{Code: ErrorImportIO, Message: "persist provenance", Cause: err}
	}
	return Result{PackagePath: destination, Manifest: manifest, Provenance: record, Scan: scan}, nil
}

func writeProvenance(storeRoot, name, version string, record provenance.Record) error {
	data, err := provenance.Marshal(record)
	if err != nil {
		return err
	}
	directory := filepath.Join(storeRoot, "_metadata", name, version)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".provenance-*.json")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filepath.Join(directory, "provenance.json"))
}

func (i *Importer) materialize(ctx context.Context, request Request, destination string) (string, error) {
	switch request.SourceType {
	case provenance.SourceLocal:
		if err := copyDirectory(request.SourceURI, destination); err != nil {
			return "", &ImportError{Code: ErrorInvalidSource, Message: "copy local package", Cause: err}
		}
		return "", nil
	case provenance.SourceGeneric:
		info, err := os.Stat(request.SourceURI)
		if err != nil {
			return "", &ImportError{Code: ErrorInvalidSource, Message: "read SKILL.md source", Cause: err}
		}
		if info.IsDir() {
			if err := copyDirectory(request.SourceURI, destination); err != nil {
				return "", &ImportError{Code: ErrorInvalidSource, Message: "copy generic package", Cause: err}
			}
			return "", nil
		}
		if filepath.Base(request.SourceURI) != "SKILL.md" {
			return "", &ImportError{Code: ErrorInvalidSource, Message: "generic source file must be named SKILL.md"}
		}
		if err := copyRegularFile(request.SourceURI, filepath.Join(destination, "SKILL.md"), info.Mode().Perm()); err != nil {
			return "", &ImportError{Code: ErrorInvalidSource, Message: "copy SKILL.md", Cause: err}
		}
		return "", nil
	case provenance.SourceZIP:
		if err := extractZIP(request.SourceURI, destination, i.MaxArchiveSize); err != nil {
			return "", &ImportError{Code: ErrorInvalidSource, Message: "extract zip package", Cause: err}
		}
		return "", nil
	case provenance.SourceGit:
		git := i.GitBinary
		if git == "" {
			git = "git"
		}
		cmd := exec.CommandContext(ctx, git, "clone", "--quiet", "--depth", "1", "--", request.SourceURI, destination)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", &ImportError{Code: ErrorInvalidSource, Message: "clone git package", Cause: fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))}
		}
		commitCmd := exec.CommandContext(ctx, git, "-C", destination, "rev-parse", "HEAD")
		output, err := commitCmd.Output()
		if err != nil {
			return "", &ImportError{Code: ErrorInvalidSource, Message: "read git commit", Cause: err}
		}
		if err := os.RemoveAll(filepath.Join(destination, ".git")); err != nil {
			return "", &ImportError{Code: ErrorImportIO, Message: "remove git metadata", Cause: err}
		}
		return strings.TrimSpace(string(output)), nil
	default:
		return "", &ImportError{Code: ErrorInvalidSource, Message: fmt.Sprintf("unsupported source type %q", request.SourceType)}
	}
}

func resolveManifest(root string, supplied *catalog.Manifest, license string) (catalog.Manifest, error) {
	manifestPath := filepath.Join(root, "agentdock.yaml")
	data, err := os.ReadFile(manifestPath)
	if err == nil {
		return catalog.ParseManifest(data)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return catalog.Manifest{}, err
	}
	if supplied != nil {
		if err := catalog.ValidateManifest(*supplied); err != nil {
			return catalog.Manifest{}, err
		}
		return *supplied, nil
	}
	return inferGenericManifest(root, license)
}

func inferGenericManifest(root, license string) (catalog.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return catalog.Manifest{}, fmt.Errorf("generic package requires SKILL.md: %w", err)
	}
	title := firstMarkdownTitle(string(data))
	if title == "" {
		title = filepath.Base(root)
	}
	name := normalizeSkillName(title)
	if name == "" {
		return catalog.Manifest{}, errors.New("cannot infer a valid skill name from SKILL.md")
	}
	manifest := catalog.Manifest{
		APIVersion: catalog.ManifestAPIVersionV1,
		Kind:       catalog.ManifestKindSkill,
		Metadata: catalog.ManifestMetadata{
			Name:        name,
			DisplayName: title,
			Version:     "0.1.0",
			License:     license,
		},
		Spec: catalog.ManifestSpec{Operations: []catalog.Operation{{
			ID:          "instructions",
			Description: "Read and follow SKILL.md",
			Runner:      "prompt",
			Entrypoint:  "SKILL.md",
		}}},
	}
	return manifest, catalog.ValidateManifest(manifest)
}

func firstMarkdownTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func normalizeSkillName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	if len(value) > 64 {
		value = strings.Trim(value[:64], "-._")
	}
	return value
}

func copyDirectory(source, destination string) error {
	root, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("source is not a directory")
	}
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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
		if rel == ".git" || strings.HasPrefix(filepath.ToSlash(rel), ".git/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, rel)
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		sourcePath := filepath.Join(root, rel)
		destinationPath := filepath.Join(destination, rel)
		info, err := os.Lstat(sourcePath)
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			if err := os.MkdirAll(destinationPath, info.Mode().Perm()); err != nil {
				return err
			}
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(sourcePath)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(target, destinationPath); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := copyRegularFile(sourcePath, destinationPath, info.Mode().Perm()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported file type: %s", rel)
		}
	}
	return nil
}

func copyRegularFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func extractZIP(source, destination string, maxBytes int64) error {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	var total int64
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			return fmt.Errorf("unsafe zip path %q", file.Name)
		}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("zip path traversal %q", file.Name)
		}
		if clean == ".git" || strings.HasPrefix(clean, ".git/") {
			continue
		}
		total += int64(file.UncompressedSize64)
		if total > maxBytes {
			return fmt.Errorf("archive exceeds %d bytes", maxBytes)
		}
		target := filepath.Join(destination, filepath.FromSlash(clean))
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, file.Mode().Perm()); err != nil {
				return err
			}
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip symlink %q is not allowed", file.Name)
		}
		if !file.Mode().IsRegular() {
			return fmt.Errorf("zip special file %q is not allowed", file.Name)
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			input.Close()
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, file.Mode().Perm())
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, int64(file.UncompressedSize64)+1))
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		if outputCloseErr != nil {
			return outputCloseErr
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
