package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/source"
)

func TestProjectAllowedTypes(t *testing.T) {
	allowed := projectAllowedTypes()

	want := []string{"skills", "agents", "hooks", "rules", "commands"}
	for _, typ := range want {
		if !allowed[typ] {
			t.Errorf("expected %s to be allowed", typ)
		}
	}

	if allowed["settings-templates"] {
		t.Error("settings-templates should not be allowed")
	}
}

func setupTestSource(t *testing.T) (ccpDir string, paths *config.Paths, registry *source.Registry) {
	t.Helper()
	ccpDir = t.TempDir()
	ccpDir, _ = filepath.EvalSymlinks(ccpDir)

	hubDir := filepath.Join(ccpDir, "hub")
	os.MkdirAll(hubDir, 0755)

	sourceDir := filepath.Join(ccpDir, "sources", "test--repo")
	os.MkdirAll(sourceDir, 0755)

	paths = &config.Paths{
		CcpDir: ccpDir,
		HubDir: hubDir,
	}

	// Write ccp.toml with properly quoted TOML key
	ccpToml := `[sources]
[sources."test/repo"]
registry = "manual"
provider = "git"
url = "https://github.com/test/repo"
path = "` + filepath.Join("sources", "test--repo") + `"
`
	os.WriteFile(filepath.Join(ccpDir, "ccp.toml"), []byte(ccpToml), 0644)

	var err error
	registry, err = source.LoadRegistry(filepath.Join(ccpDir, "registry.toml"))
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	return ccpDir, paths, registry
}

func TestInstallToDir(t *testing.T) {
	ccpDir, paths, registry := setupTestSource(t)

	sourceDir := filepath.Join(ccpDir, "sources", "test--repo")
	skillDir := filepath.Join(sourceDir, "skills", "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# My Skill"), 0644)

	agentDir := filepath.Join(sourceDir, "agents", "my-agent")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte("# My Agent"), 0644)

	installer := source.NewInstaller(paths, registry)
	targetDir := filepath.Join(t.TempDir(), ".claude")

	allowed := map[string]bool{
		"skills": true, "agents": true, "hooks": true,
		"rules": true, "commands": true,
	}

	installed, err := installer.InstallToDir("test/repo", []string{"skills/my-skill", "agents/my-agent"}, targetDir, allowed)
	if err != nil {
		t.Fatalf("InstallToDir failed: %v", err)
	}

	if len(installed) != 2 {
		t.Fatalf("expected 2 installed items, got %d", len(installed))
	}

	skillFile := filepath.Join(targetDir, "skills", "my-skill", "skill.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("expected skill file at %s: %v", skillFile, err)
	}
	if string(data) != "# My Skill" {
		t.Errorf("skill content = %q, want %q", string(data), "# My Skill")
	}

	agentFile := filepath.Join(targetDir, "agents", "my-agent", "agent.md")
	data, err = os.ReadFile(agentFile)
	if err != nil {
		t.Fatalf("expected agent file at %s: %v", agentFile, err)
	}
	if string(data) != "# My Agent" {
		t.Errorf("agent content = %q, want %q", string(data), "# My Agent")
	}
}

func TestInstallToDir_DisallowedType(t *testing.T) {
	ccpDir, paths, registry := setupTestSource(t)

	sourceDir := filepath.Join(ccpDir, "sources", "test--repo")
	templateDir := filepath.Join(sourceDir, "settings-templates", "my-template")
	os.MkdirAll(templateDir, 0755)
	os.WriteFile(filepath.Join(templateDir, "settings.json"), []byte("{}"), 0644)

	installer := source.NewInstaller(paths, registry)
	targetDir := filepath.Join(t.TempDir(), ".claude")

	allowed := map[string]bool{
		"skills": true, "agents": true, "hooks": true,
		"rules": true, "commands": true,
	}

	_, err := installer.InstallToDir("test/repo", []string{"settings-templates/my-template"}, targetDir, allowed)
	if err == nil {
		t.Fatal("expected error for disallowed type settings-templates")
	}
}

func TestInstallToDir_OverwriteExisting(t *testing.T) {
	ccpDir, paths, registry := setupTestSource(t)

	sourceDir := filepath.Join(ccpDir, "sources", "test--repo")
	skillDir := filepath.Join(sourceDir, "skills", "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# Updated"), 0644)

	installer := source.NewInstaller(paths, registry)
	targetDir := filepath.Join(t.TempDir(), ".claude")

	// Pre-create existing item
	existingDir := filepath.Join(targetDir, "skills", "my-skill")
	os.MkdirAll(existingDir, 0755)
	os.WriteFile(filepath.Join(existingDir, "skill.md"), []byte("# Old"), 0644)

	allowed := map[string]bool{"skills": true}

	installed, err := installer.InstallToDir("test/repo", []string{"skills/my-skill"}, targetDir, allowed)
	if err != nil {
		t.Fatalf("InstallToDir failed: %v", err)
	}

	if len(installed) != 1 {
		t.Fatalf("expected 1 installed item, got %d", len(installed))
	}

	data, _ := os.ReadFile(filepath.Join(targetDir, "skills", "my-skill", "skill.md"))
	if string(data) != "# Updated" {
		t.Errorf("content = %q, want %q", string(data), "# Updated")
	}
}
