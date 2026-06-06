package catalog

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseManifestRoundTrip(t *testing.T) {
	source := []byte(`apiVersion: agentdock.io/v1
kind: Skill
metadata:
  name: cloudflare-dns
  displayName: "Cloudflare DNS"
  version: 1.2.3
  description: Manage DNS records
  license: MIT
  tags: [dns, cloudflare]
spec:
  operations:
    - id: list-records
      runner: command
      entrypoint: scripts/list.sh
      args:
        - --json
      timeoutSeconds: 30
      inputSchema: {"type":"object"}
      outputSchema: {"type":"object"}
  compatibility:
    os: [darwin, linux]
    arch: [arm64, amd64]
    agentdock: ">=1.0.0"
  permissions:
    network:
      mode: declared
      hosts:
        - api.cloudflare.com
    filesystem:
      read: [references/config.json]
    secrets:
      - name: CLOUDFLARE_API_TOKEN
        required: true
`)

	manifest, err := ParseManifest(source)
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	if manifest.Metadata.Name != "cloudflare-dns" || len(manifest.Spec.Operations) != 1 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	encoded, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatalf("MarshalManifest() error = %v", err)
	}
	roundTripped, err := ParseManifest(encoded)
	if err != nil {
		t.Fatalf("round trip ParseManifest() error = %v\n%s", err, encoded)
	}
	if !reflect.DeepEqual(manifest, roundTripped) {
		t.Fatalf("round trip mismatch\nwant: %#v\ngot:  %#v", manifest, roundTripped)
	}
}

func TestValidateManifestReturnsStableIssues(t *testing.T) {
	manifest := Manifest{
		APIVersion: "v2",
		Kind:       "Plugin",
		Metadata:   ManifestMetadata{Name: "Bad Name", Version: "latest"},
		Spec: ManifestSpec{Operations: []Operation{
			{ID: "Run", Runner: "shell", Entrypoint: "../run.sh", TimeoutSeconds: 90000},
			{ID: "Run", Runner: "command", Entrypoint: "/tmp/run.sh"},
		}},
	}
	err := ValidateManifest(manifest)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if len(validationErr.Issues) < 8 {
		t.Fatalf("expected multiple issues, got %#v", validationErr.Issues)
	}
}

func TestParseManifestRejectsUnknownAndDuplicateKeys(t *testing.T) {
	tests := [][]byte{
		[]byte("apiVersion: agentdock.io/v1\napiVersion: agentdock.io/v1\n"),
		[]byte("apiVersion: agentdock.io/v1\nkind: Skill\nmetadata: {}\nspec: {}\nunknown: true\n"),
		[]byte("apiVersion: &version agentdock.io/v1\n"),
	}
	for _, source := range tests {
		if _, err := ParseManifest(source); err == nil {
			t.Fatalf("ParseManifest(%q) expected error", source)
		}
	}
}

func TestParseManifestAllowsAmpersandAndWildcardHost(t *testing.T) {
	source := []byte(`apiVersion: agentdock.io/v1
kind: Skill
metadata:
  name: research-skill
  displayName: "R&D Skill"
  version: 1.0.0
spec:
  operations:
    - id: run
      runner: prompt
      entrypoint: SKILL.md
  permissions:
    network:
      mode: declared
      hosts:
        - "*.example.com"
`)
	manifest, err := ParseManifest(source)
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	if manifest.Metadata.DisplayName != "R&D Skill" || manifest.Spec.Permissions.Network.Hosts[0] != "*.example.com" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}
