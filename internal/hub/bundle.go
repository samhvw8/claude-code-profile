package hub

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// BundleManifestFile is the metadata file stored at the root of every bundle.
const BundleManifestFile = "bundle.yaml"

// Bundle is an atomic, non-separable group of hub items (skills, agents,
// hooks, rules, commands) stored together under hub/bundles/<name>/ and
// installed/linked/removed as a single unit. Because members live inside the
// bundle directory — not in the top-level hub/<type>/ dirs — they can never be
// linked on their own. The member set is modeled with the existing
// ComponentList type (reused from plugin.go).
type Bundle struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Version     string        `yaml:"version,omitempty"`
	Members     ComponentList `yaml:"members"`
}

// LoadBundle reads bundle.yaml from hub/bundles/<name>/.
func LoadBundle(bundlesDir, name string) (*Bundle, error) {
	path := filepath.Join(bundlesDir, name, BundleManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b Bundle
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	if b.Name == "" {
		b.Name = name
	}
	return &b, nil
}

// Save writes bundle.yaml into hub/bundles/<name>/.
func (b *Bundle) Save(bundlesDir string) error {
	dir := filepath.Join(bundlesDir, b.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(b)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, BundleManifestFile), data, 0644)
}

// ListBundles returns all bundles found in bundlesDir. A missing directory is
// not an error — it simply means no bundles have been created yet.
func ListBundles(bundlesDir string) ([]*Bundle, error) {
	entries, err := os.ReadDir(bundlesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var bundles []*Bundle
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		b, err := LoadBundle(bundlesDir, e.Name())
		if err != nil {
			continue // skip entries without a valid bundle.yaml
		}
		bundles = append(bundles, b)
	}
	return bundles, nil
}
