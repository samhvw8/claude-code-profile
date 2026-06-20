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

func TestDiscoverItems_CodexAgentsDir(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Create .agents/skills/ structure (Codex cross-tool standard)
	createTestFile(t, sourceDir, ".agents/skills/codex-skill/SKILL.md", "# Codex Skill")
	createTestFile(t, sourceDir, ".agents/agents/codex-agent.md", "# Codex Agent")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(hubDir)
	installer := NewInstaller(paths, registry)

	items := installer.DiscoverItems(sourceDir)

	found := make(map[string]bool)
	for _, item := range items {
		found[item] = true
	}

	if !found["skills/codex-skill"] {
		t.Errorf("expected skills/codex-skill to be discovered from .agents/, got: %v", items)
	}
	if !found["agents/codex-agent"] {
		t.Errorf("expected agents/codex-agent to be discovered from .agents/, got: %v", items)
	}
}

func TestDiscoverItems_CodexDir(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Create .codex/skills/ structure (Codex project-level)
	createTestFile(t, sourceDir, ".codex/skills/project-skill/SKILL.md", "# Project Skill")
	createTestFile(t, sourceDir, ".codex/commands/build/SKILL.md", "# Build Command")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(hubDir)
	installer := NewInstaller(paths, registry)

	items := installer.DiscoverItems(sourceDir)

	found := make(map[string]bool)
	for _, item := range items {
		found[item] = true
	}

	if !found["skills/project-skill"] {
		t.Errorf("expected skills/project-skill from .codex/, got: %v", items)
	}
	if !found["commands/build"] {
		t.Errorf("expected commands/build from .codex/, got: %v", items)
	}
}

func TestDiscoverItems_CodexPluginJSON(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Create .codex-plugin/plugin.json with skills
	createTestFile(t, sourceDir, ".codex-plugin/plugin.json", `{
		"name": "test-plugin",
		"version": "1.0.0",
		"skills": "./custom-skills"
	}`)
	createTestFile(t, sourceDir, "custom-skills/my-codex-skill/SKILL.md", "# Codex Plugin Skill")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(hubDir)
	installer := NewInstaller(paths, registry)

	items := installer.DiscoverItems(sourceDir)

	found := make(map[string]bool)
	for _, item := range items {
		found[item] = true
	}

	if !found["skills/my-codex-skill"] {
		t.Errorf("expected skills/my-codex-skill from .codex-plugin/, got: %v", items)
	}
}

func TestDiscoverItems_NoDuplicates(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Create same skill in both root and .agents/ (should deduplicate)
	createTestFile(t, sourceDir, "skills/shared-skill/SKILL.md", "# Shared")
	createTestFile(t, sourceDir, ".agents/skills/shared-skill/SKILL.md", "# Shared")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(hubDir)
	installer := NewInstaller(paths, registry)

	items := installer.DiscoverItems(sourceDir)

	count := 0
	for _, item := range items {
		if item == "skills/shared-skill" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("expected 1 instance of skills/shared-skill, got %d (items: %v)", count, items)
	}
}

func TestResolveItemPaths_CodexFallback(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Only exists in .agents/ directory
	createTestFile(t, sourceDir, ".agents/skills/codex-only/SKILL.md", "# Codex Only")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(hubDir)
	installer := NewInstaller(paths, registry)

	srcPath, dstItem, err := installer.resolveItemPaths(sourceDir, "skills/codex-only")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(srcPath, ".agents/skills/codex-only") {
		t.Errorf("expected srcPath in .agents/, got: %s", srcPath)
	}
	if dstItem != "skills/codex-only" {
		t.Errorf("expected dstItem skills/codex-only, got: %s", dstItem)
	}
}

func TestResolveItemPaths_CodexDirFallback(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Only exists in .codex/ directory
	createTestFile(t, sourceDir, ".codex/skills/codex-project/SKILL.md", "# Codex Project")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(hubDir)
	installer := NewInstaller(paths, registry)

	srcPath, dstItem, err := installer.resolveItemPaths(sourceDir, "skills/codex-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(srcPath, ".codex/skills/codex-project") {
		t.Errorf("expected srcPath in .codex/, got: %s", srcPath)
	}
	if dstItem != "skills/codex-project" {
		t.Errorf("expected dstItem skills/codex-project, got: %s", dstItem)
	}
}

func TestResolveItemPaths_CodexFlatFile(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Flat .md file in .agents/
	createTestFile(t, sourceDir, ".agents/rules/codex-rule.md", "# Codex Rule")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(hubDir)
	installer := NewInstaller(paths, registry)

	srcPath, dstItem, err := installer.resolveItemPaths(sourceDir, "rules/codex-rule")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(srcPath, ".agents/rules/codex-rule.md") {
		t.Errorf("expected srcPath in .agents/ with .md extension, got: %s", srcPath)
	}
	if dstItem != "rules/codex-rule.md" {
		t.Errorf("expected dstItem rules/codex-rule.md, got: %s", dstItem)
	}
}

func TestDiscoverItems_RootLevelSkill(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	// Bare skill repo: SKILL.md at the repository root, no skills/<name>/ wrapper.
	createTestFile(t, sourceDir, "SKILL.md", "---\nname: frontend-audit\ndescription: A skill\n---\n\n# frontend-audit\n")
	createTestFile(t, sourceDir, "README.md", "# readme")
	createTestFile(t, sourceDir, "scripts/run.mjs", "// script")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(t.TempDir())
	installer := NewInstaller(paths, registry)
	items := installer.DiscoverItems(sourceDir)

	found := make(map[string]bool)
	for _, item := range items {
		found[item] = true
	}

	if !found["skills/frontend-audit"] {
		t.Errorf("expected skills/frontend-audit from root SKILL.md, got: %v", items)
	}
}

func TestDiscoverItems_RootLevelSkill_FallbackToDirName(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "my-skill")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	hubDir := t.TempDir()

	// Root SKILL.md with no name field -> falls back to directory base name.
	createTestFile(t, sourceDir, "SKILL.md", "---\ndescription: no name here\n---\n# x\n")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(t.TempDir())
	installer := NewInstaller(paths, registry)
	items := installer.DiscoverItems(sourceDir)

	found := make(map[string]bool)
	for _, item := range items {
		found[item] = true
	}
	if !found["skills/my-skill"] {
		t.Errorf("expected skills/my-skill (dir-name fallback), got: %v", items)
	}
}

func TestResolveItemPaths_RootLevelSkill(t *testing.T) {
	sourceDir := t.TempDir()
	hubDir := t.TempDir()

	createTestFile(t, sourceDir, "SKILL.md", "---\nname: frontend-audit\n---\n# x\n")

	paths := &config.Paths{HubDir: hubDir}
	registry := NewRegistry(hubDir)
	installer := NewInstaller(paths, registry)

	srcPath, dstItem, err := installer.resolveItemPaths(sourceDir, "skills/frontend-audit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srcPath != sourceDir {
		t.Errorf("expected srcPath to be repo root %s, got: %s", sourceDir, srcPath)
	}
	if dstItem != "skills/frontend-audit" {
		t.Errorf("expected dstItem skills/frontend-audit, got: %s", dstItem)
	}
}

func TestParseFrontmatterName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"basic", "---\nname: foo\ndescription: bar\n---\nbody", "foo"},
		{"quoted", "---\nname: \"foo-bar\"\n---\n", "foo-bar"},
		{"no frontmatter", "# just markdown\nname: foo", ""},
		{"no name field", "---\ndescription: bar\n---\n", ""},
		{"unterminated", "---\nname: foo\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseFrontmatterName([]byte(tt.in)); got != tt.want {
				t.Errorf("parseFrontmatterName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestInstallPath_RootSkill(t *testing.T) {
	ccpDir := t.TempDir()
	hubDir := t.TempDir()
	paths := &config.Paths{CcpDir: ccpDir, HubDir: hubDir}
	registry := NewRegistry(ccpDir)

	sourceID := "owner/frontend-audit-skill"
	srcDir := paths.SourceDir(sourceID)
	// Bare skill repo cloned: SKILL.md at the source root, plus siblings.
	createTestFile(t, srcDir, "SKILL.md", "---\nname: frontend-audit\n---\n# x\n")
	createTestFile(t, srcDir, "README.md", "# readme")
	createTestFile(t, srcDir, "scripts/run.mjs", "// s")

	if err := registry.AddSource(sourceID, Source{Provider: "git"}); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(paths, registry)
	item, err := installer.InstallPath(sourceID, ".")
	if err != nil {
		t.Fatalf("InstallPath: %v", err)
	}
	if item != "skills/frontend-audit" {
		t.Errorf("item = %q, want skills/frontend-audit", item)
	}

	// Everything at SKILL.md's level was copied.
	for _, f := range []string{"SKILL.md", "README.md", "scripts/run.mjs"} {
		if _, err := os.Stat(filepath.Join(hubDir, "skills", "frontend-audit", f)); err != nil {
			t.Errorf("expected %s copied: %v", f, err)
		}
	}

	// Registry tracks it as installed.
	src, _ := registry.GetSource(sourceID)
	if len(src.Installed) != 1 || src.Installed[0] != "skills/frontend-audit" {
		t.Errorf("registry Installed = %v, want [skills/frontend-audit]", src.Installed)
	}
}

func TestInstallPath_SubdirSkill(t *testing.T) {
	ccpDir := t.TempDir()
	hubDir := t.TempDir()
	paths := &config.Paths{CcpDir: ccpDir, HubDir: hubDir}
	registry := NewRegistry(ccpDir)

	sourceID := "owner/monorepo"
	srcDir := paths.SourceDir(sourceID)
	// Skill lives in a subdirectory; name comes from frontmatter.
	createTestFile(t, srcDir, "skills/debug/SKILL.md", "---\nname: debug\n---\n# d\n")
	createTestFile(t, srcDir, "skills/debug/helper.py", "# h")

	if err := registry.AddSource(sourceID, Source{Provider: "git"}); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(paths, registry)
	item, err := installer.InstallPath(sourceID, "skills/debug")
	if err != nil {
		t.Fatalf("InstallPath: %v", err)
	}
	if item != "skills/debug" {
		t.Errorf("item = %q, want skills/debug", item)
	}
	if _, err := os.Stat(filepath.Join(hubDir, "skills", "debug", "helper.py")); err != nil {
		t.Errorf("expected helper.py copied: %v", err)
	}
}

func TestInstallPath_NoSkillMd(t *testing.T) {
	ccpDir := t.TempDir()
	hubDir := t.TempDir()
	paths := &config.Paths{CcpDir: ccpDir, HubDir: hubDir}
	registry := NewRegistry(ccpDir)

	sourceID := "owner/empty"
	srcDir := paths.SourceDir(sourceID)
	createTestFile(t, srcDir, "README.md", "# no skill here")
	if err := registry.AddSource(sourceID, Source{Provider: "git"}); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(paths, registry)
	if _, err := installer.InstallPath(sourceID, "."); err == nil {
		t.Error("expected error when no SKILL.md is present, got nil")
	}
}

func TestCopyDir_SkipsGit(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")

	createTestFile(t, src, "SKILL.md", "# skill")
	createTestFile(t, src, ".git/config", "[core]")
	createTestFile(t, src, ".git/HEAD", "ref: refs/heads/main")

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Errorf("expected SKILL.md to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); !os.IsNotExist(err) {
		t.Errorf("expected .git to be skipped, but it exists (err=%v)", err)
	}
}
