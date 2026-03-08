package profile

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/samhoang/ccp/internal/config"
)

// Context represents a context.toml file
type Context struct {
	Name        string     `toml:"name"`
	Description string     `toml:"description,omitempty"`
	Hub         ContextHub `toml:"hub"`
}

// ContextHub defines which hub items a context links
type ContextHub struct {
	Skills   []string `toml:"skills,omitempty"`
	Agents   []string `toml:"agents,omitempty"`
	Rules    []string `toml:"rules,omitempty"`
	Commands []string `toml:"commands,omitempty"`
	Hooks    []string `toml:"hooks,omitempty"`
}

// NewContext creates a new context with defaults
func NewContext(name, description string) *Context {
	return &Context{
		Name:        name,
		Description: description,
		Hub:         ContextHub{},
	}
}

// LoadContext reads a context.toml from a directory
func LoadContext(dir string) (*Context, error) {
	path := filepath.Join(dir, "context.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c Context
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save writes the context to context.toml in the given directory
func (c *Context) Save(dir string) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "context.toml"), data, 0644)
}

// ContextManager handles context CRUD operations
type ContextManager struct {
	paths *config.Paths
}

// NewContextManager creates a new context manager
func NewContextManager(paths *config.Paths) *ContextManager {
	return &ContextManager{paths: paths}
}

// Create creates a new context
func (m *ContextManager) Create(name string, ctx *Context) error {
	dir := m.paths.ContextDir(name)
	if _, err := os.Stat(dir); err == nil {
		return os.ErrExist
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	ctx.Name = name
	return ctx.Save(dir)
}

// Get retrieves a context by name
func (m *ContextManager) Get(name string) (*Context, error) {
	dir := m.paths.ContextDir(name)
	return LoadContext(dir)
}

// List returns all contexts
func (m *ContextManager) List() ([]*Context, error) {
	entries, err := os.ReadDir(m.paths.ContextsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var contexts []*Context
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ctx, err := m.Get(entry.Name())
		if err != nil {
			continue
		}
		contexts = append(contexts, ctx)
	}
	return contexts, nil
}

// Delete removes a context
func (m *ContextManager) Delete(name string) error {
	dir := m.paths.ContextDir(name)
	return os.RemoveAll(dir)
}

// Exists checks if a context exists
func (m *ContextManager) Exists(name string) bool {
	dir := m.paths.ContextDir(name)
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ProfilesUsing returns profile names that reference this context
func (m *ContextManager) ProfilesUsing(contextName string) ([]string, error) {
	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var users []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		profileDir := m.paths.ProfileDir(entry.Name())
		manifestPath := ManifestPath(profileDir)
		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			continue
		}
		if manifest.Context == contextName {
			users = append(users, entry.Name())
		}
	}
	return users, nil
}
