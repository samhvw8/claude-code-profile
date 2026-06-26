package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

// setupBundleTest builds a hub containing one bundle ("impeccable") with a
// skill, an agent, and a hook member, plus an empty profile to link it to.
func setupBundleTest(t *testing.T) (*config.Paths, *Manager) {
	t.Helper()
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:          testDir,
		ClaudeDir:       filepath.Join(testDir, "claude"),
		GlobalClaudeDir: filepath.Join(testDir, "claude"),
		HubDir:          filepath.Join(testDir, "hub"),
		ProfilesDir:     filepath.Join(testDir, "profiles"),
		SharedDir:       filepath.Join(testDir, "profiles", "shared"),
		StoreDir:        filepath.Join(testDir, "store"),
	}
	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	// Materialize the bundle on disk: hub/bundles/impeccable/{skills,agents,hooks}
	bundleDir := paths.BundleDir("impeccable")
	mustMkdir(t, filepath.Join(bundleDir, "skills", "impeccable"))
	mustWrite(t, filepath.Join(bundleDir, "skills", "impeccable", "SKILL.md"), "# impeccable")
	mustWrite(t, filepath.Join(bundleDir, "agents", "impeccable.md"), "# agent")
	mustMkdir(t, filepath.Join(bundleDir, "hooks", "impeccable"))
	mustWrite(t, filepath.Join(bundleDir, "hooks", "impeccable", "hooks.json"), `{
  "hooks": {
    "PostToolUse": [
      { "matcher": "Edit|Write|MultiEdit",
        "hooks": [ { "type": "command", "command": "node ${CLAUDE_PLUGIN_ROOT}/scripts/hook.mjs", "timeout": 5 } ] }
    ]
  }
}`)

	bundle := &hub.Bundle{
		Name:    "impeccable",
		Version: "1.0.0",
		Members: hub.ComponentList{
			Skills: []string{"impeccable"},
			Agents: []string{"impeccable.md"},
			Hooks:  []string{"impeccable"},
		},
	}
	if err := bundle.Save(paths.BundlesDir()); err != nil {
		t.Fatalf("bundle.Save: %v", err)
	}

	mgr := NewManager(paths)
	if _, err := mgr.Create("p", NewManifest("p", "")); err != nil {
		t.Fatalf("Create profile: %v", err)
	}
	return paths, mgr
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLinkHubBundle_ExpandsMembers(t *testing.T) {
	paths, mgr := setupBundleTest(t)

	if err := mgr.LinkHubBundle("p", "impeccable"); err != nil {
		t.Fatalf("LinkHubBundle: %v", err)
	}

	// All three members must be symlinked into the profile's leaf dirs.
	profileDir := paths.ProfileDir("p")
	for _, rel := range []string{"skills/impeccable", "agents/impeccable.md", "hooks/impeccable"} {
		info, err := os.Lstat(filepath.Join(profileDir, rel))
		if err != nil {
			t.Fatalf("member %s not materialized: %v", rel, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("member %s is not a symlink", rel)
		}
	}

	// Manifest records ONLY the bundle name — members must not leak into leaf lists.
	p, _ := mgr.Get("p")
	if got := p.Manifest.Hub.Bundles; len(got) != 1 || got[0] != "impeccable" {
		t.Errorf("manifest bundles = %v, want [impeccable]", got)
	}
	if len(p.Manifest.Hub.Skills) != 0 || len(p.Manifest.Hub.Agents) != 0 || len(p.Manifest.Hub.Hooks) != 0 {
		t.Errorf("members leaked into leaf lists: skills=%v agents=%v hooks=%v",
			p.Manifest.Hub.Skills, p.Manifest.Hub.Agents, p.Manifest.Hub.Hooks)
	}
}

func TestBundleHook_AppearsInSettings(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	if err := mgr.LinkHubBundle("p", "impeccable"); err != nil {
		t.Fatalf("LinkHubBundle: %v", err)
	}

	p, _ := mgr.Get("p")
	hooks, err := GenerateSettingsHooks(paths, paths.ProfileDir("p"), p.Manifest)
	if err != nil {
		t.Fatalf("GenerateSettingsHooks: %v", err)
	}
	data, _ := json.Marshal(hooks)
	if !strings.Contains(string(data), "hook.mjs") {
		t.Errorf("bundle hook not merged into settings; got %s", data)
	}
	if len(hooks) == 0 {
		t.Error("expected at least one hook type from the bundle")
	}
}

func TestUnlinkHubBundle_RemovesMembers(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	if err := mgr.LinkHubBundle("p", "impeccable"); err != nil {
		t.Fatalf("LinkHubBundle: %v", err)
	}
	if err := mgr.UnlinkHubBundle("p", "impeccable"); err != nil {
		t.Fatalf("UnlinkHubBundle: %v", err)
	}

	profileDir := paths.ProfileDir("p")
	for _, rel := range []string{"skills/impeccable", "agents/impeccable.md", "hooks/impeccable"} {
		if _, err := os.Lstat(filepath.Join(profileDir, rel)); !os.IsNotExist(err) {
			t.Errorf("member %s still present after unlink (err=%v)", rel, err)
		}
	}
	p, _ := mgr.Get("p")
	if len(p.Manifest.Hub.Bundles) != 0 {
		t.Errorf("manifest still lists bundle: %v", p.Manifest.Hub.Bundles)
	}
}

func TestBundleDrift_MembersNotExtra(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	if err := mgr.LinkHubBundle("p", "impeccable"); err != nil {
		t.Fatalf("LinkHubBundle: %v", err)
	}

	p, _ := mgr.Get("p")
	det := NewDetector(paths)
	report, err := det.Detect(p)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	// Bundle member symlinks must NOT be flagged as extra/unknown.
	for _, issue := range report.Issues {
		if issue.Type == DriftExtra {
			t.Errorf("bundle member wrongly flagged as extra: %+v", issue)
		}
	}

	// Now break one member symlink and expect a missing-bundle-member issue.
	os.Remove(filepath.Join(paths.ProfileDir("p"), "skills", "impeccable"))
	report, err = det.Detect(p)
	if err != nil {
		t.Fatalf("Detect after break: %v", err)
	}
	foundMissing := false
	for _, issue := range report.Issues {
		if issue.Type == DriftMissing && issue.ItemType == config.HubBundles {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Errorf("expected a missing bundle-member drift issue, got %+v", report.Issues)
	}
}
