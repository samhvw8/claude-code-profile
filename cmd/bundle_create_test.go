package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

// newBundleTestHub creates a hub on disk with one skill, one agent and one hook,
// and returns resolved paths plus the scanned hub.
func newBundleTestHub(t *testing.T) (*config.Paths, *hub.Hub) {
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
	writeFile(t, filepath.Join(paths.HubItemDir(config.HubSkills), "foo", "SKILL.md"), "# foo")
	writeFile(t, filepath.Join(paths.HubItemDir(config.HubAgents), "bar.md"), "# bar")
	writeFile(t, filepath.Join(paths.HubItemDir(config.HubHooks), "baz", "hooks.json"), `{"hooks":{}}`)

	h, err := hub.NewScanner().Scan(paths.HubDir)
	if err != nil {
		t.Fatalf("scan hub: %v", err)
	}
	return paths, h
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCreateBundleFromHub_CopiesMembersAndManifest(t *testing.T) {
	paths, h := newBundleTestHub(t)

	members := hub.ComponentList{
		Skills: []string{"foo"},
		Agents: []string{"bar.md"},
		Hooks:  []string{"baz"},
	}
	if err := createBundleFromHub(paths, h, "mybundle", "a test bundle", members); err != nil {
		t.Fatalf("createBundleFromHub: %v", err)
	}

	bundleDir := paths.BundleDir("mybundle")
	for _, rel := range []string{
		"bundle.yaml",
		"skills/foo/SKILL.md",
		"agents/bar.md",
		"hooks/baz/hooks.json",
	} {
		if _, err := os.Stat(filepath.Join(bundleDir, rel)); err != nil {
			t.Errorf("expected %s in bundle, got %v", rel, err)
		}
	}

	// Original standalone hub items must remain (copy, not move).
	if _, err := os.Stat(filepath.Join(paths.HubItemDir(config.HubSkills), "foo")); err != nil {
		t.Errorf("original skill should remain after bundling: %v", err)
	}

	loaded, err := hub.LoadBundle(paths.BundlesDir(), "mybundle")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if loaded.Description != "a test bundle" || loaded.Members.Count() != 3 {
		t.Errorf("manifest mismatch: %+v", loaded)
	}
}

func TestCreateBundleFromHub_RejectsDuplicate(t *testing.T) {
	paths, h := newBundleTestHub(t)
	members := hub.ComponentList{Skills: []string{"foo"}}
	if err := createBundleFromHub(paths, h, "dup", "", members); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := createBundleFromHub(paths, h, "dup", "", members); err == nil {
		t.Error("expected error creating a bundle that already exists")
	}
}

func TestCreateBundleFromHub_UnknownMemberRollsBack(t *testing.T) {
	paths, h := newBundleTestHub(t)
	members := hub.ComponentList{Skills: []string{"does-not-exist"}}
	if err := createBundleFromHub(paths, h, "broken", "", members); err == nil {
		t.Fatal("expected error for unknown member")
	}
	if _, err := os.Stat(paths.BundleDir("broken")); !os.IsNotExist(err) {
		t.Errorf("expected bundle dir to be rolled back, got err=%v", err)
	}
}

func TestSelectionsToComponentList(t *testing.T) {
	sel := map[string][]string{
		string(config.HubSkills): {"a", "b"},
		string(config.HubHooks):  {"h"},
	}
	cl := selectionsToComponentList(sel)
	if len(cl.Skills) != 2 || cl.Skills[0] != "a" {
		t.Errorf("skills mismatch: %v", cl.Skills)
	}
	if len(cl.Hooks) != 1 || cl.Hooks[0] != "h" {
		t.Errorf("hooks mismatch: %v", cl.Hooks)
	}
	if len(cl.Agents) != 0 {
		t.Errorf("agents should be empty: %v", cl.Agents)
	}
}
