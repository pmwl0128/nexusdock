package security_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/memorydock/internal/memory"
)

func TestMemoryStoreRejectsTraversalAndHiddenPaths(t *testing.T) {
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	paths := []string{
		"../escape.md",
		"inbox/../../escape.md",
		"/tmp/escape.md",
		"inbox/.hidden.md",
		"projects/demo/../environment.md",
		"projects/demo/runbooks/../../escape.md",
	}
	for _, path := range paths {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			_, err := store.Write(memory.WriteRequest{Path: path, Content: "unsafe", Confirmed: true})
			if err == nil {
				t.Fatalf("unsafe path was accepted: %q", path)
			}
		})
	}
}

func TestEmbeddedWebDoesNotLeakPrivateMaterial(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "httpx", "web_dist")
	forbidden := []string{
		"/Users/" + "xx/",
		"/Volumes/" + "KIOXIA/",
		"BEGIN " + "PRIVATE KEY",
		"GITHUB_" + "TOKEN=",
		"AGENTDOCK_OAUTH_CLIENT_" + "SECRET=",
		"CF_API_" + "TOKEN=",
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, marker := range forbidden {
			if strings.Contains(text, marker) {
				t.Errorf("embedded web asset %s leaks forbidden marker %q", path, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan embedded web: %v", err)
	}
}

func TestSecurityFixtureDetectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "skill", "payload")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := rejectEscapingSymlinks(root); err == nil {
		t.Fatal("security harness did not detect a symlink escaping the package root")
	}
}

func rejectEscapingSymlinks(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, target)
		if err != nil {
			return err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fs.ErrPermission
		}
		return nil
	})
}
