package source

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// RegistryVersion is the current registry format version
const RegistryVersion = 1

// Registry represents the registry.toml file
type Registry struct {
	Version int               `toml:"version"`
	Sources map[string]Source `toml:"sources"`

	path string `toml:"-"`
}

// NewRegistry creates an empty registry
func NewRegistry(path string) *Registry {
	return &Registry{
		Version: RegistryVersion,
		Sources: make(map[string]Source),
		path:    path,
	}
}

// LoadRegistry reads registry.toml from path
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewRegistry(path), nil
		}
		return nil, err
	}

	var r Registry
	if err := toml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse registry.toml: %w", err)
	}

	r.path = path
	if r.Sources == nil {
		r.Sources = make(map[string]Source)
	}

	return &r, nil
}

// Save writes registry.toml to disk
func (r *Registry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(r)
	if err != nil {
		return err
	}

	return os.WriteFile(r.path, data, 0644)
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
