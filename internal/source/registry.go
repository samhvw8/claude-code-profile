package source

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/samhoang/ccp/internal/config"
)

// RegistryVersion is the current registry format version
const RegistryVersion = 1

// Registry represents the sources in ccp.toml
type Registry struct {
	Sources map[string]Source `toml:"sources"`

	ccpDir string `toml:"-"`
}

// NewRegistry creates an empty registry
func NewRegistry(ccpDir string) *Registry {
	return &Registry{
		Sources: make(map[string]Source),
		ccpDir:  ccpDir,
	}
}

// LoadRegistry reads sources from ccp.toml
// Falls back to legacy registry.toml if sources not in ccp.toml
func LoadRegistry(path string) (*Registry, error) {
	// path is ~/.ccp/registry.toml, derive ccpDir
	ccpDir := filepath.Dir(path)

	// Try to load from ccp.toml first
	ccpConfig, err := config.LoadCcpConfig(ccpDir)
	if err != nil {
		return nil, fmt.Errorf("load ccp.toml: %w", err)
	}

	// If sources exist in ccp.toml, use them
	if len(ccpConfig.Sources) > 0 {
		r := &Registry{
			Sources: make(map[string]Source),
			ccpDir:  ccpDir,
		}
		for id, src := range ccpConfig.Sources {
			r.Sources[id] = Source{
				Registry:  src.Registry,
				Provider:  src.Provider,
				URL:       src.URL,
				Path:      src.Path,
				Ref:       src.Ref,
				Commit:    src.Commit,
				Checksum:  src.Checksum,
				Updated:   src.Updated,
				Installed: src.Installed,
			}
		}
		return r, nil
	}

	// Fall back to legacy registry.toml
	legacyPath := filepath.Join(ccpDir, "registry.toml")
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewRegistry(ccpDir), nil
		}
		return nil, err
	}

	// Parse legacy format
	var legacy struct {
		Version int               `toml:"version"`
		Sources map[string]Source `toml:"sources"`
	}
	if err := toml.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse registry.toml: %w", err)
	}

	r := &Registry{
		Sources: legacy.Sources,
		ccpDir:  ccpDir,
	}
	if r.Sources == nil {
		r.Sources = make(map[string]Source)
	}

	return r, nil
}

// Save writes sources to ccp.toml
func (r *Registry) Save() error {
	if err := os.MkdirAll(r.ccpDir, 0755); err != nil {
		return err
	}

	// Load existing ccp.toml config
	ccpConfig, err := config.LoadCcpConfig(r.ccpDir)
	if err != nil {
		ccpConfig = config.DefaultCcpConfig()
	}

	// Update sources
	ccpConfig.Sources = make(map[string]config.SourceConfig)
	for id, src := range r.Sources {
		ccpConfig.Sources[id] = config.SourceConfig{
			Registry:  src.Registry,
			Provider:  src.Provider,
			URL:       src.URL,
			Path:      src.Path,
			Ref:       src.Ref,
			Commit:    src.Commit,
			Checksum:  src.Checksum,
			Updated:   src.Updated,
			Installed: src.Installed,
		}
	}

	// Save to ccp.toml
	if err := ccpConfig.Save(r.ccpDir); err != nil {
		return err
	}

	// Remove legacy registry.toml if it exists
	legacyPath := filepath.Join(r.ccpDir, "registry.toml")
	if _, err := os.Stat(legacyPath); err == nil {
		os.Remove(legacyPath)
	}

	return nil
}

// AddSource adds a new source to the registry
func (r *Registry) AddSource(id string, source Source) error {
	if _, exists := r.Sources[id]; exists {
		return &SourceError{Op: "add source", Source: id, Err: ErrSourceExists}
	}

	source.Updated = time.Now()
	r.Sources[id] = source
	return nil
}

// UpdateSource updates an existing source
func (r *Registry) UpdateSource(id string, source Source) error {
	if _, exists := r.Sources[id]; !exists {
		return &SourceError{Op: "update source", Source: id, Err: ErrSourceNotFound}
	}

	source.Updated = time.Now()
	r.Sources[id] = source
	return nil
}

// RemoveSource removes a source from the registry
func (r *Registry) RemoveSource(id string) error {
	if _, exists := r.Sources[id]; !exists {
		return &SourceError{Op: "remove source", Source: id, Err: ErrSourceNotFound}
	}

	delete(r.Sources, id)
	return nil
}

// GetSource returns a source by ID
func (r *Registry) GetSource(id string) (*Source, error) {
	source, exists := r.Sources[id]
	if !exists {
		return nil, &SourceError{Op: "get source", Source: id, Err: ErrSourceNotFound}
	}
	return &source, nil
}

// ListSources returns all sources
func (r *Registry) ListSources() []SourceEntry {
	entries := make([]SourceEntry, 0, len(r.Sources))
	for id, source := range r.Sources {
		entries = append(entries, SourceEntry{
			ID:     id,
			Source: source,
		})
	}
	return entries
}

// SourceEntry is a source with its ID
type SourceEntry struct {
	ID     string
	Source Source
}

// AddInstalled adds an installed item to a source
func (r *Registry) AddInstalled(sourceID, item string) error {
	source, exists := r.Sources[sourceID]
	if !exists {
		return &SourceError{Op: "add installed", Source: sourceID, Err: ErrSourceNotFound}
	}

	for _, i := range source.Installed {
		if i == item {
			return nil
		}
	}

	source.Installed = append(source.Installed, item)
	source.Updated = time.Now()
	r.Sources[sourceID] = source
	return nil
}

// RemoveInstalled removes an installed item from a source
func (r *Registry) RemoveInstalled(sourceID, item string) error {
	source, exists := r.Sources[sourceID]
	if !exists {
		return &SourceError{Op: "remove installed", Source: sourceID, Err: ErrSourceNotFound}
	}

	for i, inst := range source.Installed {
		if inst == item {
			source.Installed = append(source.Installed[:i], source.Installed[i+1:]...)
			source.Updated = time.Now()
			r.Sources[sourceID] = source
			return nil
		}
	}

	return &SourceError{Op: "remove installed", Source: sourceID, Err: ErrItemNotFound}
}

// FindSourceByItem finds which source an item belongs to
func (r *Registry) FindSourceByItem(item string) *SourceEntry {
	for id, source := range r.Sources {
		for _, inst := range source.Installed {
			if inst == item {
				return &SourceEntry{ID: id, Source: source}
			}
		}
	}
	return nil
}

// HasSource checks if a source exists
func (r *Registry) HasSource(id string) bool {
	_, exists := r.Sources[id]
	return exists
}

// SourceCount returns the number of sources
func (r *Registry) SourceCount() int {
	return len(r.Sources)
}

// InstalledCount returns total number of installed items
func (r *Registry) InstalledCount() int {
	count := 0
	for _, source := range r.Sources {
		count += len(source.Installed)
	}
	return count
}
