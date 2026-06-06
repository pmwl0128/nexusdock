package exporter

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/uvwt/agentdock-nexus/internal/skills/catalog"
)

type adapterStub struct{}

func (adapterStub) Name() string { return "stub" }
func (adapterStub) Files(context.Context, catalog.Manifest) (map[string][]byte, error) {
	return map[string][]byte{"adapters/stub.json": []byte(`{"enabled":true}`)}, nil
}

func TestExportFormats(t *testing.T) {
	root := validPackage(t)
	exporter := Exporter{}
	tests := []struct {
		format   Format
		adapter  Adapter
		contains []string
		excludes []string
	}{
		{FormatGeneric, nil, []string{"SKILL.md", "scripts/run.sh"}, []string{"agentdock.yaml"}},
		{FormatAgentDock, nil, []string{"SKILL.md", "scripts/run.sh", "agentdock.yaml"}, nil},
		{FormatAdapter, adapterStub{}, []string{"SKILL.md", "agentdock.yaml", "adapters/stub.json"}, nil},
	}
	for _, test := range tests {
		destination := filepath.Join(t.TempDir(), string(test.format)+".zip")
		result, err := exporter.Export(context.Background(), Request{
			PackageRoot: root,
			Destination: destination,
			Format:      test.format,
			Adapter:     test.adapter,
		})
		if err != nil {
			t.Fatalf("Export(%s): %v", test.format, err)
		}
		if result.FileCount == 0 || result.Bytes == 0 {
			t.Fatalf("empty result: %#v", result)
		}
		names := archiveNames(t, destination)
		for _, name := range test.contains {
			if !contains(names, name) {
				t.Fatalf("%s missing %s: %v", test.format, name, names)
			}
		}
		for _, name := range test.excludes {
			if contains(names, name) {
				t.Fatalf("%s unexpectedly contains %s", test.format, name)
			}
		}
	}
}

func TestExportRejectsSecretAndPrivatePath(t *testing.T) {
	tests := []string{
		"API_KEY=abcdefghijklmnop\n",
		"config=/Users/private/agentdock/config.json\n",
	}
	for _, content := range tests {
		root := validPackage(t)
		if err := os.WriteFile(filepath.Join(root, "references", "private.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := (Exporter{}).Export(context.Background(), Request{
			PackageRoot: root,
			Destination: filepath.Join(t.TempDir(), "out.zip"),
			Format:      FormatAgentDock,
		})
		var exportErr *ExportError
		if !errors.As(err, &exportErr) || exportErr.Code != ErrorUnsafePackage {
			t.Fatalf("expected unsafe package error, got %T %v", err, err)
		}
	}
}

func validPackage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# Export Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := catalog.Manifest{
		APIVersion: catalog.ManifestAPIVersionV1,
		Kind:       catalog.ManifestKindSkill,
		Metadata:   catalog.ManifestMetadata{Name: "export-skill", Version: "1.0.0"},
		Spec: catalog.ManifestSpec{Operations: []catalog.Operation{{
			ID: "run", Runner: "command", Entrypoint: "scripts/run.sh",
		}}},
	}
	data, err := catalog.MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agentdock.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func archiveNames(t *testing.T, path string) []string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		result = append(result, file.Name)
	}
	sort.Strings(result)
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
