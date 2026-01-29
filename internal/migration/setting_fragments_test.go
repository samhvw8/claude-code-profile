package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSettingFragments(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("extracts non-hook keys", func(t *testing.T) {
		settingsPath := filepath.Join(tmpDir, "settings1.json")
		content := `{
			"permissions": {"allow": ["Bash"]},
			"apiProvider": "anthropic",
			"hooks": {"SessionStart": []},
			"model": "claude-3-opus"
		}`
		if err := os.WriteFile(settingsPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		fragments, err := ExtractSettingFragments(settingsPath)
		if err != nil {
			t.Fatalf("ExtractSettingFragments failed: %v", err)
		}

		// Should have 3 fragments (permissions, apiProvider, model) - not hooks
		if len(fragments) != 3 {
			t.Errorf("expected 3 fragments, got %d", len(fragments))
		}

		// Verify hooks is excluded
		for _, f := range fragments {
			if f.Key == "hooks" {
				t.Error("hooks should be excluded from fragments")
			}
		}
	})

	t.Run("empty settings", func(t *testing.T) {
		settingsPath := filepath.Join(tmpDir, "settings2.json")
		if err := os.WriteFile(settingsPath, []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		fragments, err := ExtractSettingFragments(settingsPath)
		if err != nil {
			t.Fatalf("ExtractSettingFragments failed: %v", err)
		}
		if len(fragments) != 0 {
			t.Error("expected 0 fragments for empty settings")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := ExtractSettingFragments(filepath.Join(tmpDir, "nonexistent.json"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		settingsPath := filepath.Join(tmpDir, "invalid.json")
		if err := os.WriteFile(settingsPath, []byte("{invalid}"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := ExtractSettingFragments(settingsPath)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestSaveSettingFragments(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")

	fragments := []SettingFragment{
		{Name: "permissions", Key: "permissions", Value: map[string]interface{}{"allow": []string{"Bash"}}},
		{Name: "api-provider", Key: "apiProvider", Value: "anthropic"},
	}

	if err := SaveSettingFragments(hubDir, fragments); err != nil {
		t.Fatalf("SaveSettingFragments failed: %v", err)
	}

	// Verify files were created
	fragmentsDir := filepath.Join(hubDir, "setting-fragments")
	if _, err := os.Stat(filepath.Join(fragmentsDir, "permissions.yaml")); os.IsNotExist(err) {
		t.Error("permissions.yaml should exist")
	}
	if _, err := os.Stat(filepath.Join(fragmentsDir, "api-provider.yaml")); os.IsNotExist(err) {
		t.Error("api-provider.yaml should exist")
	}
}

func TestLoadSettingFragment(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	fragmentsDir := filepath.Join(hubDir, "setting-fragments")
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a fragment file
	fragmentContent := `name: permissions
key: permissions
value:
  allow:
    - Bash
    - Read
`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "permissions.yaml"), []byte(fragmentContent), 0644); err != nil {
		t.Fatal(err)
	}

	fragment, err := LoadSettingFragment(hubDir, "permissions")
	if err != nil {
		t.Fatalf("LoadSettingFragment failed: %v", err)
	}

	if fragment.Name != "permissions" {
		t.Errorf("Name = %q, want 'permissions'", fragment.Name)
	}
	if fragment.Key != "permissions" {
		t.Errorf("Key = %q, want 'permissions'", fragment.Key)
	}
}

func TestLoadSettingFragment_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := LoadSettingFragment(tmpDir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent fragment")
	}
}

func TestMergeSettingFragments(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	fragmentsDir := filepath.Join(hubDir, "setting-fragments")
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create fragment files
	frag1 := `name: permissions
key: permissions
value:
  allow:
    - Bash
`
	frag2 := `name: api-provider
key: apiProvider
value: anthropic
`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "permissions.yaml"), []byte(frag1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fragmentsDir, "api-provider.yaml"), []byte(frag2), 0644); err != nil {
		t.Fatal(err)
	}

	merged, err := MergeSettingFragments(hubDir, []string{"permissions", "api-provider"})
	if err != nil {
		t.Fatalf("MergeSettingFragments failed: %v", err)
	}

	if _, ok := merged["permissions"]; !ok {
		t.Error("merged should contain 'permissions' key")
	}
	if _, ok := merged["apiProvider"]; !ok {
		t.Error("merged should contain 'apiProvider' key")
	}
}

func TestMergeSettingFragments_MissingFragment(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := MergeSettingFragments(tmpDir, []string{"nonexistent"})
	if err == nil {
		t.Error("expected error for missing fragment")
	}
}

func TestKeyToFragmentName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"permissions", "permissions"},
		{"apiProvider", "api-provider"},
		{"customApiKey", "custom-api-key"},
		{"hasCompletedOnboarding", "has-completed-onboarding"},
		{"model", "model"},
		{"ABC", "abc"}, // All uppercase doesn't have lowercase-uppercase transitions
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := keyToFragmentName(tt.input)
			if result != tt.expected {
				t.Errorf("keyToFragmentName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetKeyDescription(t *testing.T) {
	// Known keys should have descriptions
	if desc := getKeyDescription("permissions"); desc == "" {
		t.Error("permissions should have a description")
	}
	if desc := getKeyDescription("apiProvider"); desc == "" {
		t.Error("apiProvider should have a description")
	}

	// Unknown keys return empty
	if desc := getKeyDescription("unknownKey"); desc != "" {
		t.Error("unknown keys should return empty description")
	}
}
