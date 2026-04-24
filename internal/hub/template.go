package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
)

// Template represents a settings.json template in the hub
type Template struct {
	Name     string                 // Directory name
	Settings map[string]interface{} // Raw settings content (hooks excluded)
}

// TemplateManager handles settings template CRUD operations
type TemplateManager struct {
	hubDir string
}

// NewTemplateManager creates a new template manager
func NewTemplateManager(hubDir string) *TemplateManager {
	return &TemplateManager{hubDir: hubDir}
}

// templatesDir returns the base directory for all templates
func (m *TemplateManager) templatesDir() string {
	return filepath.Join(m.hubDir, string(config.HubSettingsTemplates))
}

// templateDir returns the directory for a specific template
func (m *TemplateManager) templateDir(name string) string {
	return filepath.Join(m.templatesDir(), name)
}

// settingsPath returns the path to a template's settings.json
func (m *TemplateManager) settingsPath(name string) string {
	return filepath.Join(m.templateDir(name), "settings.json")
}

// Load reads a template by name
func (m *TemplateManager) Load(name string) (*Template, error) {
	data, err := os.ReadFile(m.settingsPath(name))
	if err != nil {
		return nil, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return &Template{Name: name, Settings: settings}, nil
}

// Save writes a template to disk
func (m *TemplateManager) Save(t *Template) error {
	dir := m.templateDir(t.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(t.Settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.settingsPath(t.Name), append(data, '\n'), 0644)
}

// List returns all available templates
func (m *TemplateManager) List() ([]*Template, error) {
	entries, err := os.ReadDir(m.templatesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var templates []*Template
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t, err := m.Load(entry.Name())
		if err != nil {
			continue
		}
		templates = append(templates, t)
	}
	return templates, nil
}

// Delete removes a template
func (m *TemplateManager) Delete(name string) error {
	return os.RemoveAll(m.templateDir(name))
}

// Exists checks if a template exists
func (m *TemplateManager) Exists(name string) bool {
	_, err := os.Stat(m.settingsPath(name))
	return err == nil
}

// ExtractFromSettings creates a template from a settings.json file,
// excluding hooks (which are managed separately by the hub hooks system).
// If keys is non-empty, only those top-level keys are included; any key
// not present in the settings produces an error.
func ExtractFromSettings(settingsPath string, keys ...string) (map[string]interface{}, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	delete(settings, "hooks")

	if len(keys) == 0 {
		return settings, nil
	}

	filtered := make(map[string]interface{}, len(keys))
	for _, k := range keys {
		if k == "hooks" {
			continue
		}
		v, ok := settings[k]
		if !ok {
			return nil, fmt.Errorf("key not found in settings: %s", k)
		}
		filtered[k] = v
	}
	return filtered, nil
}
