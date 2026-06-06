package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/uvwt/memorydock/internal/skills/provenance"
)

type Trust string

const (
	TrustBlocked  Trust = "blocked"
	TrustUnknown  Trust = "unknown"
	TrustImported Trust = "imported"
	TrustVerified Trust = "verified"
)

type Maturity string

const (
	MaturityExperimental Maturity = "experimental"
	MaturityDevelopment  Maturity = "development"
	MaturityStable       Maturity = "stable"
)

type Release struct {
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Channel string `json:"channel,omitempty"`
}

type Installation struct {
	DeviceID string `json:"device_id"`
	Version  string `json:"version"`
	Status   string `json:"status"`
}

type Summary struct {
	Name          string        `json:"name"`
	DisplayName   string        `json:"display_name,omitempty"`
	Description   string        `json:"description,omitempty"`
	LatestVersion string        `json:"latest_version"`
	Trust         Trust         `json:"trust"`
	Maturity      Maturity      `json:"maturity"`
	Compatibility Compatibility `json:"compatibility,omitempty"`
}

type Detail struct {
	Summary
	Operations         []Operation                  `json:"operations"`
	Releases           []Release                    `json:"releases"`
	InstalledDevices   []Installation               `json:"installed_devices"`
	LatestProvenance   provenance.Record            `json:"latest_provenance"`
	ReleaseProvenances map[string]provenance.Record `json:"release_provenances"`
}

type InstallationProvider interface {
	ListSkillInstallations(skillName string) ([]Installation, error)
}

type Catalog interface {
	List() ([]Summary, error)
	Get(name string) (Detail, error)
}

type FileCatalog struct {
	Root          string
	Installations InstallationProvider

	mu                   sync.RWMutex
	verificationByDigest map[string]bool
	blockedByDigest      map[string]bool
}

func NewFileCatalog(root string, installations InstallationProvider) *FileCatalog {
	return &FileCatalog{
		Root:                 root,
		Installations:        installations,
		verificationByDigest: make(map[string]bool),
		blockedByDigest:      make(map[string]bool),
	}
}

func (c *FileCatalog) SetVerification(digest string, verified bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.verificationByDigest[digest] = verified
}

func (c *FileCatalog) SetBlocked(digest string, blocked bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blockedByDigest[digest] = blocked
}

func (c *FileCatalog) List() ([]Summary, error) {
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		return nil, err
	}
	var result []Summary
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "_metadata" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		detail, err := c.Get(entry.Name())
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, detail.Summary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (c *FileCatalog) Get(name string) (Detail, error) {
	if !skillNamePattern.MatchString(name) {
		return Detail{}, fmt.Errorf("invalid skill name %q", name)
	}
	versionsRoot := filepath.Join(c.Root, name)
	entries, err := os.ReadDir(versionsRoot)
	if err != nil {
		return Detail{}, err
	}
	var releases []Release
	provenances := make(map[string]provenance.Record)
	manifests := make(map[string]Manifest)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		manifestData, err := os.ReadFile(filepath.Join(versionsRoot, entry.Name(), "agentdock.yaml"))
		if err != nil {
			return Detail{}, fmt.Errorf("read %s/%s manifest: %w", name, entry.Name(), err)
		}
		manifest, err := ParseManifest(manifestData)
		if err != nil {
			return Detail{}, fmt.Errorf("parse %s/%s manifest: %w", name, entry.Name(), err)
		}
		if manifest.Metadata.Name != name || manifest.Metadata.Version != entry.Name() {
			return Detail{}, fmt.Errorf("catalog path does not match manifest for %s/%s", name, entry.Name())
		}
		record, err := readProvenance(c.Root, name, entry.Name())
		if err != nil {
			return Detail{}, err
		}
		manifests[entry.Name()] = manifest
		provenances[entry.Name()] = record
		releases = append(releases, Release{Version: entry.Name(), Digest: record.Digest})
	}
	if len(releases) == 0 {
		return Detail{}, os.ErrNotExist
	}
	sort.Slice(releases, func(i, j int) bool {
		return compareSemver(releases[i].Version, releases[j].Version) > 0
	})
	latest := releases[0]
	manifest := manifests[latest.Version]
	record := provenances[latest.Version]
	var installations []Installation
	if c.Installations != nil {
		installations, err = c.Installations.ListSkillInstallations(name)
		if err != nil {
			return Detail{}, err
		}
		sort.Slice(installations, func(i, j int) bool { return installations[i].DeviceID < installations[j].DeviceID })
	}
	return Detail{
		Summary: Summary{
			Name:          name,
			DisplayName:   manifest.Metadata.DisplayName,
			Description:   manifest.Metadata.Description,
			LatestVersion: latest.Version,
			Trust:         c.trust(record.Digest),
			Maturity:      maturityForVersion(latest.Version),
			Compatibility: manifest.Spec.Compatibility,
		},
		Operations:         append([]Operation(nil), manifest.Spec.Operations...),
		Releases:           releases,
		InstalledDevices:   installations,
		LatestProvenance:   record,
		ReleaseProvenances: provenances,
	}, nil
}

func (c *FileCatalog) trust(digest string) Trust {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.blockedByDigest[digest] {
		return TrustBlocked
	}
	if c.verificationByDigest[digest] {
		return TrustVerified
	}
	if digest != "" {
		return TrustImported
	}
	return TrustUnknown
}

func readProvenance(root, name, version string) (provenance.Record, error) {
	data, err := os.ReadFile(filepath.Join(root, "_metadata", name, version, "provenance.json"))
	if err != nil {
		return provenance.Record{}, fmt.Errorf("read provenance for %s/%s: %w", name, version, err)
	}
	var record provenance.Record
	if err := json.Unmarshal(data, &record); err != nil {
		return provenance.Record{}, fmt.Errorf("decode provenance for %s/%s: %w", name, version, err)
	}
	if err := record.Validate(); err != nil {
		return provenance.Record{}, fmt.Errorf("validate provenance for %s/%s: %w", name, version, err)
	}
	return record, nil
}

func maturityForVersion(version string) Maturity {
	if strings.Contains(version, "-") {
		return MaturityExperimental
	}
	parts := strings.SplitN(version, ".", 3)
	if len(parts) == 3 && parts[0] == "0" {
		return MaturityDevelopment
	}
	return MaturityStable
}

func compareSemver(left, right string) int {
	leftCore, leftPre, _ := strings.Cut(left, "-")
	rightCore, rightPre, _ := strings.Cut(right, "-")
	leftParts := strings.Split(leftCore, ".")
	rightParts := strings.Split(rightCore, ".")
	for index := 0; index < 3; index++ {
		leftNumber, _ := strconv.Atoi(leftParts[index])
		rightNumber, _ := strconv.Atoi(rightParts[index])
		if leftNumber > rightNumber {
			return 1
		}
		if leftNumber < rightNumber {
			return -1
		}
	}
	if leftPre == "" && rightPre != "" {
		return 1
	}
	if leftPre != "" && rightPre == "" {
		return -1
	}
	return strings.Compare(leftPre, rightPre)
}
