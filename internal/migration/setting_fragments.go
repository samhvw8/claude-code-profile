package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
)

// SettingFragment represents a single setting fragment
type SettingFragment struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description,omitempty"`
	Key         string      `yaml:"key"`
	Value       interface{} `yaml:"value"`
}

// ExtractSettingFragments extracts each top-level key from settings.json into fragments
func ExtractSettingFragments(settingsPath string) ([]SettingFragment, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	var fragments []SettingFragment

	for key, value := range settings {
		// Skip hooks - they're handled separately
		if key == "hooks" {
			continue
		}

		fragment := SettingFragment{
			Name:        keyToFragmentName(key),
			Description: getKeyDescription(key),
			Key:         key,
			Value:       value,
		}
		fragments = append(fragments, fragment)
	}

	return fragments, nil
}

// SaveSettingFragments saves extracted fragments to hub/setting-fragments/
func SaveSettingFragments(hubDir string, fragments []SettingFragment) error {
	fragmentsDir := filepath.Join(hubDir, string(config.HubSettingFragments))

	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		return err
	}

	for _, fragment := range fragments {
		filename := fragment.Name + ".yaml"
		fragmentPath := filepath.Join(fragmentsDir, filename)

		data, err := yaml.Marshal(fragment)
		if err != nil {
			return fmt.Errorf("failed to marshal fragment %s: %w", fragment.Name, err)
		}

		if err := os.WriteFile(fragmentPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write fragment %s: %w", fragment.Name, err)
		}
	}

	return nil
}

// LoadSettingFragment loads a single setting fragment from hub
func LoadSettingFragment(hubDir, fragmentName string) (*SettingFragment, error) {
	fragmentPath := filepath.Join(hubDir, string(config.HubSettingFragments), fragmentName+".yaml")

	data, err := os.ReadFile(fragmentPath)
	if err != nil {
		return nil, err
	}

	var fragment SettingFragment
	if err := yaml.Unmarshal(data, &fragment); err != nil {
		return nil, err
	}

	return &fragment, nil
}

// MergeSettingFragments merges multiple fragments into a settings map
func MergeSettingFragments(hubDir string, fragmentNames []string) (map[string]interface{}, error) {
	settings := make(map[string]interface{})

	for _, name := range fragmentNames {
		fragment, err := LoadSettingFragment(hubDir, name)
		if err != nil {
			return nil, fmt.Errorf("failed to load fragment %s: %w", name, err)
		}

		settings[fragment.Key] = fragment.Value
	}

	return settings, nil
}

// keyToFragmentName converts a settings.json key to a fragment filename
func keyToFragmentName(key string) string {
	// Convert camelCase to kebab-case
	re := regexp.MustCompile("([a-z])([A-Z])")
	kebab := re.ReplaceAllString(key, "${1}-${2}")
	return strings.ToLower(kebab)
}

// getKeyDescription returns a description for known settings keys
func getKeyDescription(key string) string {
	descriptions := map[string]string{
		"permissions":          "Permission settings for Claude Code",
		"apiProvider":          "API provider configuration",
		"model":                "Model selection settings",
		"customApiKey":         "Custom API key configuration",
		"preferredNotifChannel": "Notification channel preference",
		"autoUpdaterStatus":    "Auto-updater configuration",
		"hasCompletedOnboarding": "Onboarding completion status",
		"hasDismissedWelcome":  "Welcome message dismissal status",
		"primaryApiKey":        "Primary API key for Claude",
		"projects":             "Project-specific settings",
	}

	if desc, ok := descriptions[key]; ok {
		return desc
	}
	return ""
}
