package profile

import (
	"fmt"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

// GenerateSettings creates a complete settings map from the manifest.
// It loads the settings template (if any), then overlays hooks from the hub.
func GenerateSettings(manifest *Manifest, paths *config.Paths, profileDir string) (map[string]interface{}, error) {
	settings := make(map[string]interface{})

	// Load settings template
	if manifest.SettingsTemplate != "" {
		tmplMgr := hub.NewTemplateManager(paths.HubDir)
		tmpl, err := tmplMgr.Load(manifest.SettingsTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to load template %s: %w", manifest.SettingsTemplate, err)
		}
		for key, value := range tmpl.Settings {
			settings[key] = value
		}
	}

	// Collect hooks from hub
	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		return nil, err
	}
	if len(hooks) > 0 {
		settings["hooks"] = hooks
	}

	return settings, nil
}
