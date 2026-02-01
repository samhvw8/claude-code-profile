package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestIsValidItemFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		// Valid files
		{"markdown file", "skill.md", true},
		{"yaml file", "config.yaml", true},
		{"yml file", "config.yml", true},
		{"json file", "plugin.json", true},
		{"toml file", "manifest.toml", true},
		{"uppercase extension", "SKILL.MD", true},

		// Invalid files
		{"hidden file", ".gitkeep", false},
		{"hidden md file", ".hidden.md", false},
		{"shell script", "script.sh", false},
		{"go file", "main.go", false},
		{"text file", "readme.txt", false},
		{"no extension", "README", false},
		{"typescript", "index.ts", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidItemFile(tt.filename)
			if got != tt.want {
				t.Errorf("isValidItemFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestDiscoverItems_WithFlatFiles(t *testing.T) {
	// Create temp directory structure simulating plugins with flat files
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Create plugins/test-plugin/agents/ with flat .md files
	createTestFile(t, sourceDir, "plugins/test-plugin/agents/agent1.md", "# Agent 1")
	createTestFile(t, sourceDir, "plugins/test-plugin/agents/agent2.md", "# Agent 2")

	// Create plugins/test-plugin/commands/ with flat .md files
	createTestFile(t, sourceDir, "plugins/test-plugin/commands/cmd1.md", "# Command 1")

	// Create plugins/test-plugin/skills/ with subdirectories (traditional structure)
	createTestFile(t, sourceDir, "plugins/test-plugin/skills/skill1/SKILL.md", "# Skill 1")
	createTestFile(t, sourceDir, "plugins/test-plugin/skills/skill2/SKILL.md", "# Skill 2")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(t.TempDir())
	installer := NewInstaller(paths, registry)
	items := installer.DiscoverItems(sourceDir)

	// Should find 2 agents, 1 command, 2 skills
	counts := map[string]int{}
	for _, item := range items {
		if strings.Contains(item, "/agents/") {
			counts["agents"]++
		} else if strings.Contains(item, "/commands/") {
			counts["commands"]++
		} else if strings.Contains(item, "/skills/") {
			counts["skills"]++
		}
	}

	if counts["agents"] != 2 {
		t.Errorf("expected 2 agents, got %d (items: %v)", counts["agents"], items)
	}
	if counts["commands"] != 1 {
		t.Errorf("expected 1 command, got %d (items: %v)", counts["commands"], items)
	}
	if counts["skills"] != 2 {
		t.Errorf("expected 2 skills, got %d (items: %v)", counts["skills"], items)
	}
}

func TestDiscoverItems_IgnoresHiddenFiles(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Create valid and hidden files
	createTestFile(t, sourceDir, "plugins/test/agents/valid.md", "# Valid")
	createTestFile(t, sourceDir, "plugins/test/agents/.hidden.md", "# Hidden")
	createTestFile(t, sourceDir, "plugins/test/agents/.gitkeep", "")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(t.TempDir())
	installer := NewInstaller(paths, registry)
	items := installer.DiscoverItems(sourceDir)

	// Should only find 1 agent (valid.md), not hidden files
	agentCount := 0
	for _, item := range items {
		if strings.Contains(item, "/agents/") {
			agentCount++
		}
	}

	if agentCount != 1 {
		t.Errorf("expected 1 agent (hidden excluded), got %d (items: %v)", agentCount, items)
	}
}

func createTestFile(t *testing.T, base, path, content string) {
	t.Helper()
	fullPath := filepath.Join(base, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
