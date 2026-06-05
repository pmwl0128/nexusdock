package catalog_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/memorydock/internal/skills/catalog"
	"github.com/uvwt/memorydock/internal/skills/importer"
	"github.com/uvwt/memorydock/internal/skills/provenance"
)

type installationStub struct {
	items []catalog.Installation
}

func (s installationStub) ListSkillInstallations(string) ([]catalog.Installation, error) {
	return append([]catalog.Installation(nil), s.items...), nil
}

func TestFileCatalogReturnsLatestReleaseAndInstallations(t *testing.T) {
	store := t.TempDir()
	for _, version := range []string{"1.2.0", "1.10.0", "2.0.0-beta.1", "2.0.0"} {
		source := t.TempDir()
		if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("# Catalog Skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		manifest := catalog.Manifest{
			APIVersion: catalog.ManifestAPIVersionV1,
			Kind:       catalog.ManifestKindSkill,
			Metadata: catalog.ManifestMetadata{
				Name:        "catalog-skill",
				DisplayName: "Catalog Skill",
				Version:     version,
				Description: "catalog test",
			},
			Spec: catalog.ManifestSpec{Operations: []catalog.Operation{{
				ID: "run", Runner: "prompt", Entrypoint: "SKILL.md",
			}}},
		}
		_, err := (&importer.Importer{StoreRoot: store}).Import(context.Background(), importer.Request{
			SourceType: provenance.SourceLocal,
			SourceURI:  source,
			Manifest:   &manifest,
		})
		if err != nil {
			t.Fatalf("import %s: %v", version, err)
		}
	}

	service := catalog.NewFileCatalog(store, installationStub{items: []catalog.Installation{
		{DeviceID: "device-b", Version: "1.10.0", Status: "installed"},
		{DeviceID: "device-a", Version: "2.0.0", Status: "active"},
	}})
	detail, err := service.Get("catalog-skill")
	if err != nil {
		t.Fatal(err)
	}
	if detail.LatestVersion != "2.0.0" || detail.Maturity != catalog.MaturityStable {
		t.Fatalf("unexpected latest release: %#v", detail.Summary)
	}
	if len(detail.Releases) != 4 || detail.Releases[0].Version != "2.0.0" || detail.Releases[1].Version != "2.0.0-beta.1" {
		t.Fatalf("releases not sorted by semver: %#v", detail.Releases)
	}
	if len(detail.InstalledDevices) != 2 || detail.InstalledDevices[0].DeviceID != "device-a" {
		t.Fatalf("installations not sorted: %#v", detail.InstalledDevices)
	}
	service.SetVerification(detail.LatestProvenance.Digest, true)
	verified, err := service.Get("catalog-skill")
	if err != nil {
		t.Fatal(err)
	}
	if verified.Trust != catalog.TrustVerified {
		t.Fatalf("trust = %q", verified.Trust)
	}
	summaries, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].Name != "catalog-skill" {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
}
