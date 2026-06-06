package migration_test

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/uvwt/memorydock/internal/memory"
)

func TestLegacyMemoryDockDataIsOpenedInPlaceWithoutMutation(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string]string{
		"profile.md":                       "---\ntype: profile\nscope: profile\n---\n\n# 用户档案\n\n保留原内容。\n",
		"projects/demo/project.md":         "---\ntype: project-summary\nscope: project\nproject: demo\n---\n\n# Demo\n\n迁移兼容。\n",
		"projects/demo/runbooks/deploy.md": "# 部署\n\n执行 health 检查。\n",
		"devices/DockMini.md":              "# DockMini\n\n状态：active。\n",
		"ops/reverse-proxy.md":             "# 反代\n\n保留路径。\n",
	}
	for path, content := range fixtures {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	before := snapshot(t, root)
	store, err := memory.NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	entries, err := store.List("", 100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) < len(fixtures) {
		t.Fatalf("listed %d entries, want at least %d", len(entries), len(fixtures))
	}
	for path, content := range fixtures {
		entry, err := store.Read(path)
		if err != nil {
			t.Fatalf("Read(%s): %v", path, err)
		}
		if entry.Content != content {
			t.Fatalf("content changed for %s", path)
		}
	}
	results, err := store.Search("迁移兼容", "projects/demo", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Path != "projects/demo/project.md" {
		t.Fatalf("unexpected search result: %#v", results)
	}
	after := snapshot(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("opening legacy data mutated files\nbefore=%v\nafter=%v", before, after)
	}
}

func snapshot(t *testing.T, root string) []string {
	t.Helper()
	var result []string
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
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result = append(result, fmt.Sprintf("%s:%x", filepath.ToSlash(rel), sha256.Sum256(data)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(result)
	return result
}
