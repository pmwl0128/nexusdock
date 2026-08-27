package recall

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestApplyMarkdownPatchSupportsLiteralSectionAndAppend(t *testing.T) {
	input := "# Root\n\nvalue old\n\n## Target\nold body\n\n## Tail\nkeep\n"
	got, changes, err := ApplyMarkdownPatch(input, "value old", "value new", "Target", "new body", "\nappended")
	if err != nil {
		t.Fatal(err)
	}
	if changes != 3 || !strings.Contains(got, "value new") || !strings.Contains(got, "## Target\nnew body\n## Tail") || !strings.HasSuffix(got, "appended") {
		t.Fatalf("patched content=%q changes=%d", got, changes)
	}
}

func TestUpdateMarkdownFactsHonorsSection(t *testing.T) {
	input := "# Root\n\nport: 1\n\n## Target\nport: 2\nmode: old\n\n## Tail\nport: 3\n"
	got, updates, err := UpdateMarkdownFacts(input, "Target", map[string]string{"port": "9", "new": "yes"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || !strings.Contains(got, "## Target\nnew：yes\nport:9") || !strings.Contains(got, "# Root\n\nport: 1") || !strings.Contains(got, "## Tail\nport: 3") {
		t.Fatalf("updated=%q updates=%#v", got, updates)
	}
}

func TestUnifiedDiffTruncatesAtUTF8Boundary(t *testing.T) {
	full := UnifiedDiff("note.md", "# 标题\n旧内容", "# 标题\n新内容", 0)
	if full == "" {
		t.Fatal("expected non-empty diff")
	}
	for maxBytes := 1; maxBytes < len(full); maxBytes++ {
		got := UnifiedDiff("note.md", "# 标题\n旧内容", "# 标题\n新内容", maxBytes)
		if !utf8.ValidString(got) || len(got) > maxBytes {
			t.Fatalf("max=%d len=%d valid=%v", maxBytes, len(got), utf8.ValidString(got))
		}
	}
}
