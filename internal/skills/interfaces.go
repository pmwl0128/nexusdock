package skills

import (
	"context"

	"github.com/uvwt/agentdock-nexus/internal/skills/catalog"
	"github.com/uvwt/agentdock-nexus/internal/skills/exporter"
	"github.com/uvwt/agentdock-nexus/internal/skills/importer"
)

type SkillCatalog interface {
	List() ([]catalog.Summary, error)
	Get(name string) (catalog.Detail, error)
}

type SkillImporter interface {
	Import(context.Context, importer.Request) (importer.Result, error)
}

type SkillExporter interface {
	Export(context.Context, exporter.Request) (exporter.Result, error)
}

type SkillPackageValidator interface {
	ValidateManifest(catalog.Manifest) error
	Scan(root string, manifest catalog.Manifest) (importer.ScanReport, error)
}

type PackageValidator struct{}

func (PackageValidator) ValidateManifest(manifest catalog.Manifest) error {
	return catalog.ValidateManifest(manifest)
}

func (PackageValidator) Scan(root string, manifest catalog.Manifest) (importer.ScanReport, error) {
	return importer.Scan(root, manifest)
}
