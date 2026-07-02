package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

const SettingsFragmentFile = "settings-fragment.json"

// GenerateSettings creates a complete settings map from the manifest.
// Pipeline: base template → deep merge fragment → overlay hooks.
func GenerateSettings(manifest *Manifest, paths *config.Paths, profileDir string) (map[string]interface{}, error) {
	settings := make(map[string]interface{})

	// Load settings template (base)
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

	// Load and merge per-profile fragment
	fragment, err := loadFragment(profileDir)
	if err != nil {
		return nil, err
	}
	if fragment != nil {
		settings = deepMerge(settings, fragment)
	}

	// Collect hub hooks and merge with any existing hooks from fragment
	hubHooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		return nil, err
	}
	if len(hubHooks) > 0 {
		mergeHubHooks(settings, hubHooks)
	}

	return settings, nil
}

// FragmentExists returns true if a settings fragment file exists in the profile directory.
func FragmentExists(profileDir string) bool {
	_, err := os.Stat(filepath.Join(profileDir, SettingsFragmentFile))
	return err == nil
}

// mergeHubHooks merges hub-generated hooks into settings.
// Fragment hooks take priority per hook type; hub hooks fill gaps.
func mergeHubHooks(settings map[string]interface{}, hubHooks map[config.HookType][]config.SettingsHookEntry) {
	existing, hasExisting := settings["hooks"].(map[string]interface{})
	if !hasExisting {
		settings["hooks"] = hubHooks
		return
	}

	for hookType, entries := range hubHooks {
		key := string(hookType)
		if _, exists := existing[key]; exists {
			continue
		}
		data, err := json.Marshal(entries)
		if err != nil {
			continue
		}
		var arr interface{}
		json.Unmarshal(data, &arr)
		existing[key] = arr
	}
	settings["hooks"] = existing
}

// loadFragment reads settings-fragment.json from the profile directory.
// Returns nil if the file doesn't exist.
func loadFragment(profileDir string) (map[string]interface{}, error) {
	path := filepath.Join(profileDir, SettingsFragmentFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read settings fragment: %w", err)
	}

	var fragment map[string]interface{}
	if err := json.Unmarshal(data, &fragment); err != nil {
		return nil, fmt.Errorf("failed to parse settings fragment: %w", err)
	}
	return fragment, nil
}

// DiffSettings returns keys in current that differ from base.
// Objects are compared recursively; arrays and scalars use reflect.DeepEqual.
func DiffSettings(base, current map[string]interface{}) map[string]interface{} {
	diff := make(map[string]interface{})
	for k, cv := range current {
		bv, exists := base[k]
		if !exists {
			diff[k] = cv
			continue
		}
		cm, cok := cv.(map[string]interface{})
		bm, bok := bv.(map[string]interface{})
		if cok && bok {
			sub := DiffSettings(bm, cm)
			if len(sub) > 0 {
				diff[k] = sub
			}
			continue
		}
		if !reflect.DeepEqual(bv, cv) {
			diff[k] = cv
		}
	}
	return diff
}

// PreviewFragment computes the diff without writing to disk.
func PreviewFragment(paths *config.Paths, profileDir string, manifest *Manifest) (map[string]interface{}, error) {
	return computeFragment(paths, profileDir, manifest)
}

// UpdateFragment computes the diff between current settings.json and the base
// template, strips hooks, and saves the result as settings-fragment.json.
func UpdateFragment(paths *config.Paths, profileDir string, manifest *Manifest) (map[string]interface{}, error) {
	fragment, err := computeFragment(paths, profileDir, manifest)
	if err != nil {
		return nil, err
	}

	fragmentPath := filepath.Join(profileDir, SettingsFragmentFile)
	if len(fragment) == 0 {
		os.Remove(fragmentPath)
		return fragment, nil
	}

	if err := writeJSONFile(fragmentPath, fragment); err != nil {
		return nil, fmt.Errorf("failed to write fragment: %w", err)
	}

	return fragment, nil
}

func computeFragment(paths *config.Paths, profileDir string, manifest *Manifest) (map[string]interface{}, error) {
	settingsPath := filepath.Join(profileDir, "settings.json")
	currentData, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read settings.json: %w", err)
	}

	var current map[string]interface{}
	if err := json.Unmarshal(currentData, &current); err != nil {
		return nil, fmt.Errorf("failed to parse settings.json: %w", err)
	}

	delete(current, "hooks")

	base := make(map[string]interface{})
	if manifest.SettingsTemplate != "" {
		tmplMgr := hub.NewTemplateManager(paths.HubDir)
		tmpl, err := tmplMgr.Load(manifest.SettingsTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to load template %s: %w", manifest.SettingsTemplate, err)
		}
		base = tmpl.Settings
	}

	return DiffSettings(base, current), nil
}

// deepMerge merges src into dst recursively.
// Objects merge recursively; arrays and scalars in src replace dst.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(dst))
	for k, v := range dst {
		result[k] = v
	}
	for k, srcVal := range src {
		dstVal, exists := result[k]
		if !exists {
			result[k] = srcVal
			continue
		}
		srcMap, srcOK := srcVal.(map[string]interface{})
		dstMap, dstOK := dstVal.(map[string]interface{})
		if srcOK && dstOK {
			result[k] = deepMerge(dstMap, srcMap)
		} else {
			result[k] = srcVal
		}
	}
	return result
}
