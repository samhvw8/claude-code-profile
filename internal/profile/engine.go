package profile

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/samhoang/ccp/internal/config"
)

// Engine represents an engine.toml file
type Engine struct {
	Name        string     `toml:"name"`
	Description string     `toml:"description,omitempty"`
	Hub         EngineHub  `toml:"hub"`
	Data        DataConfig `toml:"data"`
}

// EngineHub defines which hub items an engine links
type EngineHub struct {
	SettingFragments []string `toml:"setting-fragments,omitempty"`
	Hooks            []string `toml:"hooks,omitempty"`
}

// NewEngine creates a new engine with defaults
func NewEngine(name, description string) *Engine {
	defaults := config.DefaultDataConfig()
	return &Engine{
		Name:        name,
		Description: description,
		Hub:         EngineHub{},
		Data: DataConfig{
			Tasks:       defaults[config.DataTasks],
			Todos:       defaults[config.DataTodos],
			PasteCache:  defaults[config.DataPasteCache],
			History:     defaults[config.DataHistory],
			FileHistory: defaults[config.DataFileHistory],
			SessionEnv:  defaults[config.DataSessionEnv],
			Projects:    defaults[config.DataProjects],
			Plans:       defaults[config.DataPlans],
		},
	}
}

// LoadEngine reads an engine.toml from a directory
func LoadEngine(dir string) (*Engine, error) {
	path := filepath.Join(dir, "engine.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var e Engine
	if err := toml.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// Save writes the engine to engine.toml in the given directory
func (e *Engine) Save(dir string) error {
	data, err := toml.Marshal(e)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "engine.toml"), data, 0644)
}

// EngineManager handles engine CRUD operations
type EngineManager struct {
	paths *config.Paths
}

// NewEngineManager creates a new engine manager
func NewEngineManager(paths *config.Paths) *EngineManager {
	return &EngineManager{paths: paths}
}

// Create creates a new engine
func (m *EngineManager) Create(name string, engine *Engine) error {
	dir := m.paths.EngineDir(name)
	if _, err := os.Stat(dir); err == nil {
		return os.ErrExist
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	engine.Name = name
	return engine.Save(dir)
}

// Get retrieves an engine by name
func (m *EngineManager) Get(name string) (*Engine, error) {
	dir := m.paths.EngineDir(name)
	return LoadEngine(dir)
}

// List returns all engines
func (m *EngineManager) List() ([]*Engine, error) {
	entries, err := os.ReadDir(m.paths.EnginesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var engines []*Engine
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		engine, err := m.Get(entry.Name())
		if err != nil {
			continue
		}
		engines = append(engines, engine)
	}
	return engines, nil
}

// Delete removes an engine
func (m *EngineManager) Delete(name string) error {
	dir := m.paths.EngineDir(name)
	return os.RemoveAll(dir)
}

// Exists checks if an engine exists
func (m *EngineManager) Exists(name string) bool {
	dir := m.paths.EngineDir(name)
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ProfilesUsing returns profile names that reference this engine
func (m *EngineManager) ProfilesUsing(engineName string) ([]string, error) {
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
		if manifest.Engine == engineName {
			users = append(users, entry.Name())
		}
	}
	return users, nil
}
