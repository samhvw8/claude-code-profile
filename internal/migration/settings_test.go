package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain path", "/usr/bin/foo", "/usr/bin/foo"},
		{"tilde prefix", "~/foo/bar", filepath.Join(home, "foo/bar")},
		{"just tilde", "~", home},
		{"$HOME var", "$HOME/foo", filepath.Join(home, "foo")},
		{"$HOME in middle", "/prefix$HOME/suffix", "/prefix" + home + "/suffix"},
		{"no expansion needed", "/absolute/path", "/absolute/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandHome(tt.input)
			if result != tt.expected {
				t.Errorf("expandHome(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsInsideDir(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		path     string
		dir      string
		expected bool
	}{
		{"inside", "/home/user/.claude/hooks/test.sh", "/home/user/.claude", true},
		{"outside", "/home/user/scripts/test.sh", "/home/user/.claude", false},
		{"same dir", "/home/user/.claude", "/home/user/.claude", false}, // not inside, same level
		{"nested deep", "/home/user/.claude/a/b/c/d", "/home/user/.claude", true},
		{"$HOME inside", "$HOME/.claude/hooks/test.sh", "$HOME/.claude", true},
		{"~ inside", "~/.claude/hooks/test.sh", "~/.claude", true},
	}

	// Skip tests that require actual home dir if not available
	if home == "" {
		t.Skip("no home dir")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInsideDir(tt.path, tt.dir)
			if result != tt.expected {
				t.Errorf("isInsideDir(%q, %q) = %v, want %v", tt.path, tt.dir, result, tt.expected)
			}
		})
	}
}

func TestIsInlineScript(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{"heredoc", "cat <<EOF\ntest\nEOF", true},
		{"semicolon", "echo hello; echo world", true},
		{"and operator", "test -f file && echo exists", true},
		{"or operator", "test -f file || echo missing", true},
		{"echo command", "echo hello", true},
		{"cat command", "cat file.txt", true},
		{"printf command", "printf '%s' test", true},
		{"command substitution dollar", "test $(whoami)", true},
		{"command substitution backtick", "test `whoami`", true},
		{"variable with modifier", "${HOME:-/tmp}", true},
		{"simple file path", "/path/to/script.sh", false},
		{"relative path", "./script.sh", false},
		{"interpreter with path", "bash /path/to/script.sh", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInlineScript(tt.command)
			if result != tt.expected {
				t.Errorf("isInlineScript(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}

func TestExtractInterpreterAndPath(t *testing.T) {
	tests := []struct {
		name            string
		command         string
		wantInterpreter string
		wantPath        string
	}{
		{"simple path", "/path/to/script.sh", "", "/path/to/script.sh"},
		{"bash interpreter", "bash /path/to/script.sh", "bash", "/path/to/script.sh"},
		{"node interpreter", "node /path/to/script.js", "node", "/path/to/script.js"},
		{"python interpreter", "python /path/to/script.py", "python", "/path/to/script.py"},
		{"full path interpreter", "/bin/bash /path/to/script.sh", "bash", "/path/to/script.sh"},
		{"inline echo", "echo hello", "", ""},
		{"inline with semicolon", "echo a; echo b", "", ""},
		{"no path", "some-command", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interpreter, path := extractInterpreterAndPath(tt.command)
			if interpreter != tt.wantInterpreter {
				t.Errorf("interpreter = %q, want %q", interpreter, tt.wantInterpreter)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "my-hook", "my-hook"},
		{"spaces", "my hook", "my-hook"},
		{"underscores", "my_hook", "my-hook"},
		{"special chars", "my@hook#name!", "myhookname"},
		{"multiple hyphens", "my--hook", "my-hook"},
		{"leading hyphen", "-my-hook", "my-hook"},
		{"trailing hyphen", "my-hook-", "my-hook"},
		{"empty result", "@@@@", "hook"},
		{"uppercase", "MyHook", "myhook"},
		{"mixed", "My_Hook Name!", "my-hook-name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateHookName(t *testing.T) {
	tests := []struct {
		name     string
		hook     ExtractedHook
		expected string
	}{
		{
			name: "with file path",
			hook: ExtractedHook{
				FilePath: "/path/to/my_script.sh",
				HookType: "PreToolUse",
			},
			expected: "my-script",
		},
		{
			name: "inline with matcher",
			hook: ExtractedHook{
				HookType: "SessionStart",
				Matcher:  "Bash",
			},
			expected: "sessionstart-bash",
		},
		{
			name: "inline without matcher",
			hook: ExtractedHook{
				HookType: "PostToolUse",
			},
			expected: "posttooluse",
		},
		{
			name: "file with extension",
			hook: ExtractedHook{
				FilePath: "/path/to/validate.js",
			},
			expected: "validate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateHookName(tt.hook)
			if result != tt.expected {
				t.Errorf("GenerateHookName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseSettings(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid settings with hooks", func(t *testing.T) {
		settingsPath := filepath.Join(tmpDir, "settings1.json")
		content := `{
			"hooks": {
				"SessionStart": [
					{
						"hooks": [{"command": "bash /path/to/script.sh", "timeout": 5000, "type": "command"}]
					}
				]
			},
			"permissions": {"allow": ["Bash"]}
		}`
		if err := os.WriteFile(settingsPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		settings, err := ParseSettings(settingsPath)
		if err != nil {
			t.Fatalf("ParseSettings failed: %v", err)
		}

		if settings.Hooks == nil {
			t.Fatal("Hooks should not be nil")
		}
		if len(settings.Hooks["SessionStart"]) != 1 {
			t.Errorf("expected 1 SessionStart hook, got %d", len(settings.Hooks["SessionStart"]))
		}
	})

	t.Run("empty settings", func(t *testing.T) {
		settingsPath := filepath.Join(tmpDir, "settings2.json")
		if err := os.WriteFile(settingsPath, []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		settings, err := ParseSettings(settingsPath)
		if err != nil {
			t.Fatalf("ParseSettings failed: %v", err)
		}
		if settings.Hooks != nil && len(settings.Hooks) > 0 {
			t.Error("Hooks should be nil or empty for empty settings")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := ParseSettings(filepath.Join(tmpDir, "nonexistent.json"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		settingsPath := filepath.Join(tmpDir, "invalid.json")
		if err := os.WriteFile(settingsPath, []byte("{invalid}"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := ParseSettings(settingsPath)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestExtractHookPaths(t *testing.T) {
	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")

	settings := &SettingsFile{
		Hooks: map[string][]SettingsHook{
			"SessionStart": {
				{
					Hooks: []SettingsHookEntry{
						{Command: "bash " + claudeDir + "/hooks/start.sh", Timeout: 5000, Type: "command"},
					},
				},
			},
			"PreToolUse": {
				{
					Hooks: []SettingsHookEntry{
						{Command: "/usr/local/bin/external.sh", Timeout: 3000, Type: "command"},
					},
					Matcher: "Bash",
				},
			},
		},
	}

	hooks := ExtractHookPaths(settings, claudeDir)

	if len(hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(hooks))
	}

	// Find SessionStart hook
	var startHook *ExtractedHook
	for i := range hooks {
		if hooks[i].HookType == "SessionStart" {
			startHook = &hooks[i]
			break
		}
	}

	if startHook == nil {
		t.Fatal("SessionStart hook not found")
	}
	if !startHook.IsInside {
		t.Error("SessionStart hook should be marked as inside")
	}
	if startHook.Interpreter != "bash" {
		t.Errorf("expected interpreter 'bash', got %q", startHook.Interpreter)
	}
}

func TestExtractHookPaths_NilHooks(t *testing.T) {
	settings := &SettingsFile{}
	hooks := ExtractHookPaths(settings, "/tmp")
	if len(hooks) != 0 {
		t.Error("expected empty hooks for nil settings.Hooks")
	}
}
