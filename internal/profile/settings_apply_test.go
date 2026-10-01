package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

func readSettingsForTest(t *testing.T, profileDir string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	return m
}

func addStopHook(t *testing.T, paths *config.Paths) {
	t.Helper()
	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubHooks), "okf-stop", "hooks.json"), `{
  "hooks": { "Stop": [ { "hooks": [ { "type": "command", "command": "bash ${CLAUDE_PLUGIN_ROOT}/stop.sh" } ] } ] }
}`)
}

// Linking or unlinking a hook must update settings.json's hooks right away,
// without touching any other key.
func TestLinkHook_UpdatesSettingsHooksOnly(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	addStopHook(t, paths)
	profileDir := paths.ProfileDir("p")
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"model": "edited-by-hand"}`)

	if err := mgr.LinkHubItem("p", config.HubHooks, "okf-stop"); err != nil {
		t.Fatalf("LinkHubItem: %v", err)
	}
	s := readSettingsForTest(t, profileDir)
	hooks, _ := s["hooks"].(map[string]interface{})
	if _, ok := hooks["Stop"]; !ok {
		t.Errorf("Stop hook not written to settings.json after link: %v", s)
	}
	if s["model"] != "edited-by-hand" {
		t.Errorf("link changed a non-hook key: model = %v", s["model"])
	}

	if err := mgr.UnlinkHubItem("p", config.HubHooks, "okf-stop"); err != nil {
		t.Fatalf("UnlinkHubItem: %v", err)
	}
	s = readSettingsForTest(t, profileDir)
	if _, ok := s["hooks"]; ok {
		t.Errorf("stale hooks left in settings.json after unlink: %v", s["hooks"])
	}
	if s["model"] != "edited-by-hand" {
		t.Errorf("unlink changed a non-hook key: model = %v", s["model"])
	}
}

// Regeneration must not silently drop edits that were never captured.
func TestApplySettings_KeepsUncapturedEdits(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	addStopHook(t, paths)
	profileDir := paths.ProfileDir("p")
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"model": "edited-by-hand"}`)
	if err := mgr.LinkHubItem("p", config.HubHooks, "okf-stop"); err != nil {
		t.Fatalf("LinkHubItem: %v", err)
	}
	p, _ := mgr.Get("p")

	uncaptured, err := ApplySettings(paths, profileDir, p.Manifest, false)
	if err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}
	if !reflect.DeepEqual(uncaptured, []string{"model"}) {
		t.Errorf("uncaptured = %v, want [model]", uncaptured)
	}
	s := readSettingsForTest(t, profileDir)
	if s["model"] != "edited-by-hand" {
		t.Errorf("uncaptured edit lost without force: %v", s)
	}
	if _, ok := s["hooks"]; !ok {
		t.Errorf("hooks not kept in sync: %v", s)
	}

	if _, err := ApplySettings(paths, profileDir, p.Manifest, true); err != nil {
		t.Fatalf("ApplySettings force: %v", err)
	}
	s = readSettingsForTest(t, profileDir)
	if _, ok := s["model"]; ok {
		t.Errorf("force should regenerate and drop uncaptured edits: %v", s)
	}
}

// Captured edits regenerate cleanly with nothing reported.
func TestApplySettings_RegeneratesWhenNothingIsLost(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{}`)
	mustWrite(t, filepath.Join(profileDir, SettingsFragmentFile), `{"model": "from-fragment"}`)
	p, _ := mgr.Get("p")

	uncaptured, err := ApplySettings(paths, profileDir, p.Manifest, false)
	if err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}
	if len(uncaptured) != 0 {
		t.Errorf("uncaptured = %v, want none", uncaptured)
	}
	if s := readSettingsForTest(t, profileDir); s["model"] != "from-fragment" {
		t.Errorf("fragment not applied: %v", s)
	}
}

func writeTemplate(t *testing.T, paths *config.Paths, name, settings string) {
	t.Helper()
	mustWrite(t, filepath.Join(paths.HubDir, "settings-templates", name, "settings.json"), settings)
}

// Switching template is not an edit: the new template must take effect.
func TestApplySettings_TemplateSwitchApplies(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	writeTemplate(t, paths, "a", `{"model": "opus", "effortLevel": "high"}`)
	writeTemplate(t, paths, "b", `{"model": "sonnet"}`)
	p, _ := mgr.Get("p")
	p.Manifest.SettingsTemplate = "a"
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}

	p.Manifest.SettingsTemplate = "b"
	uncaptured, err := ApplySettings(paths, profileDir, p.Manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncaptured) != 0 {
		t.Errorf("template switch reported as edits: %v", uncaptured)
	}
	s := readSettingsForTest(t, profileDir)
	if s["model"] != "sonnet" || s["effortLevel"] != nil {
		t.Errorf("template b not applied: %v", s)
	}
}

// Editing the fragment is not an edit of settings.json either.
func TestApplySettings_FragmentEditApplies(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	mustWrite(t, filepath.Join(profileDir, SettingsFragmentFile), `{"model": "opus"}`)
	p, _ := mgr.Get("p")
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}

	mustWrite(t, filepath.Join(profileDir, SettingsFragmentFile), `{"model": "fable"}`)
	uncaptured, err := ApplySettings(paths, profileDir, p.Manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncaptured) != 0 {
		t.Errorf("fragment edit reported as settings edits: %v", uncaptured)
	}
	if s := readSettingsForTest(t, profileDir); s["model"] != "fable" {
		t.Errorf("fragment change not applied: %v", s)
	}
}

// Deleting a generated key on disk is an edit too.
func TestApplySettings_DeletionIsUncaptured(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	writeTemplate(t, paths, "a", `{"model": "opus", "permissions": {"deny": ["b"]}}`)
	p, _ := mgr.Get("p")
	p.Manifest.SettingsTemplate = "a"
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"model": "opus"}`)

	uncaptured, err := ApplySettings(paths, profileDir, p.Manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(uncaptured, []string{"permissions"}) {
		t.Errorf("uncaptured = %v, want [permissions]", uncaptured)
	}
	if _, ok := readSettingsForTest(t, profileDir)["permissions"]; ok {
		t.Error("deleted key restored without --force")
	}
}

// Hooks ccp did not create survive hook link, unlink, and full regeneration.
func TestSettings_KeepForeignHooks(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	addStopHook(t, paths)
	profileDir := paths.ProfileDir("p")
	guard := `{"hooks": [{"type": "command", "command": "bash /opt/guard.sh"}], "matcher": "Bash"}`
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"hooks": {"PreToolUse": [`+guard+`]}}`)

	hasGuard := func(when string) {
		t.Helper()
		hooks, _ := readSettingsForTest(t, profileDir)["hooks"].(map[string]interface{})
		if list, _ := hooks["PreToolUse"].([]interface{}); len(list) != 1 {
			t.Errorf("%s: foreign PreToolUse hook lost or duplicated: %v", when, hooks)
		}
	}

	if err := mgr.LinkHubItem("p", config.HubHooks, "okf-stop"); err != nil {
		t.Fatal(err)
	}
	hasGuard("after link")
	p, _ := mgr.Get("p")
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	hasGuard("after regenerate")
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	hasGuard("after second regenerate")
	if err := mgr.UnlinkHubItem("p", config.HubHooks, "okf-stop"); err != nil {
		t.Fatal(err)
	}
	hasGuard("after unlink")
	hooks, _ := readSettingsForTest(t, profileDir)["hooks"].(map[string]interface{})
	if _, ok := hooks["Stop"]; ok {
		t.Errorf("unlinked hub hook still in settings: %v", hooks)
	}
}

// A settings.json ccp cannot parse must not fail a link that already happened.
func TestLinkHook_BadSettingsWarnsAndIsRetryable(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	addStopHook(t, paths)
	mustWrite(t, filepath.Join(paths.ProfileDir("p"), "settings.json"), `{not json`)

	for i := 0; i < 2; i++ {
		if err := mgr.LinkHubItem("p", config.HubHooks, "okf-stop"); err != nil {
			t.Fatalf("LinkHubItem attempt %d: %v", i+1, err)
		}
	}
}

// Every command that reconciles links keeps bundle members.
func TestSyncLinks_KeepsBundleMembers(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	if err := mgr.LinkHubBundle("p", "impeccable"); err != nil {
		t.Fatal(err)
	}
	member := filepath.Join(paths.ProfileDir("p"), "agents", "impeccable.md")
	p, _ := mgr.Get("p")
	if err := SyncLinks(paths, p, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(member); err != nil {
		t.Errorf("SyncLinks removed bundle member: %v", err)
	}
}

// The advertised way out — capture, then sync — must actually clear the edit.
func TestApplySettings_CapturedEditRegenerates(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	p, _ := mgr.Get("p")
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"model": "edited"}`)
	if _, err := UpdateFragment(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}

	uncaptured, err := ApplySettings(paths, profileDir, p.Manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncaptured) != 0 {
		t.Errorf("captured edit still reported: %v", uncaptured)
	}
	if s := readSettingsForTest(t, profileDir); s["model"] != "edited" {
		t.Errorf("captured value lost: %v", s)
	}
}

// Old-style manifest.Hooks survive every rewrite.
func TestSettings_KeepLegacyManifestHooks(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	addStopHook(t, paths)
	profileDir := paths.ProfileDir("p")
	p, _ := mgr.Get("p")
	p.Manifest.Hooks = []config.HookConfig{{Name: "guard.sh", Type: config.HookPreToolUse}}
	if err := p.Manifest.Save(ManifestPath(profileDir)); err != nil {
		t.Fatal(err)
	}
	if err := NewSettingsManager(paths).SyncHooksFromManifest(profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}

	hasLegacy := func(when string) {
		t.Helper()
		hooks, _ := readSettingsForTest(t, profileDir)["hooks"].(map[string]interface{})
		if list, _ := hooks["PreToolUse"].([]interface{}); len(list) != 1 {
			t.Errorf("%s: legacy hook lost or duplicated: %v", when, hooks)
		}
	}
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	hasLegacy("after regenerate")
	if err := mgr.LinkHubItem("p", config.HubHooks, "okf-stop"); err != nil {
		t.Fatal(err)
	}
	hasLegacy("after link")
}

// A hub hook whose command does not point into the profile must still unlink
// cleanly on a profile an older ccp wrote (no snapshot yet).
func TestUnlinkHook_WithoutSnapshot(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubHooks), "npx-hook", "hooks.json"),
		`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "npx some-tool"}]}]}}`)
	profileDir := paths.ProfileDir("p")
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"model": "x"}`)

	if err := mgr.LinkHubItem("p", config.HubHooks, "npx-hook"); err != nil {
		t.Fatal(err)
	}
	// As if an older ccp had written settings.json: no snapshot
	if err := RemoveSettingsSnapshot(paths, profileDir); err != nil {
		t.Fatal(err)
	}
	if err := mgr.UnlinkHubItem("p", config.HubHooks, "npx-hook"); err != nil {
		t.Fatal(err)
	}
	if hooks, ok := readSettingsForTest(t, profileDir)["hooks"]; ok {
		t.Errorf("unlinked hook left behind: %v", hooks)
	}
}

// Removing a template key on disk can be captured, and stays removed.
func TestCapture_RecordsRemovedTemplateKeys(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	writeTemplate(t, paths, "a", `{"enabledPlugins": {"keep@x": true, "drop@x": true}, "effortLevel": "high"}`)
	p, _ := mgr.Get("p")
	p.Manifest.SettingsTemplate = "a"
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"enabledPlugins": {"keep@x": true}}`)
	if _, err := UpdateFragment(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}

	uncaptured, err := ApplySettings(paths, profileDir, p.Manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncaptured) != 0 {
		t.Errorf("captured removals still reported: %v", uncaptured)
	}
	s := readSettingsForTest(t, profileDir)
	plugins, _ := s["enabledPlugins"].(map[string]interface{})
	if _, ok := plugins["drop@x"]; ok {
		t.Errorf("removed plugin restored: %v", s)
	}
	if _, ok := s["effortLevel"]; ok {
		t.Errorf("removed top-level key restored: %v", s)
	}
}

// 'ccp init' and migrations point hook commands at the hub; those entries are
// ccp's and must be replaced, not kept beside the new ones.
func TestLinkHook_ReplacesHubPathEntry(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	addStopHook(t, paths)
	profileDir := paths.ProfileDir("p")
	hubCmd := filepath.Join(paths.HubItemDir(config.HubHooks), "okf-stop", "stop.sh")
	mustWrite(t, filepath.Join(profileDir, "settings.json"),
		`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "bash `+hubCmd+`"}]}]}}`)

	if err := mgr.LinkHubItem("p", config.HubHooks, "okf-stop"); err != nil {
		t.Fatal(err)
	}
	hooks, _ := readSettingsForTest(t, profileDir)["hooks"].(map[string]interface{})
	if list, _ := hooks["Stop"].([]interface{}); len(list) != 1 {
		t.Errorf("Stop hook duplicated: %v", hooks)
	}
}

// A fragment null deep inside a new map is never written to settings.json.
func TestRegenerate_NeverWritesNulls(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	mustWrite(t, filepath.Join(profileDir, SettingsFragmentFile), `{"x": {"y": 1, "z": null}, "w": null}`)
	p, _ := mgr.Get("p")
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if strings.Contains(string(data), "null") {
		t.Errorf("null written to settings.json: %s", data)
	}
}

// An explicit null on disk can be captured and then stops counting.
func TestCapture_UserNullClears(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	writeTemplate(t, paths, "a", `{"x": {"y": 1}}`)
	p, _ := mgr.Get("p")
	p.Manifest.SettingsTemplate = "a"
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"x": {"y": null}}`)
	if _, err := UpdateFragment(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	uncaptured, err := ApplySettings(paths, profileDir, p.Manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncaptured) != 0 {
		t.Errorf("captured null still reported: %v", uncaptured)
	}
}

// --force repairs an unparseable settings.json, keeping the old file; without
// --force ccp refuses instead of guessing.
func TestApplySettings_CorruptSettings(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{broken`)
	p, _ := mgr.Get("p")

	if _, err := ApplySettings(paths, profileDir, p.Manifest, false); err == nil {
		t.Error("expected an error without --force")
	}
	if _, err := ApplySettings(paths, profileDir, p.Manifest, true); err != nil {
		t.Fatalf("--force did not repair: %v", err)
	}
	readSettingsForTest(t, profileDir) // parses now
	if data, err := os.ReadFile(filepath.Join(profileDir, "settings.json.bak")); err != nil || string(data) != "{broken" {
		t.Errorf("old file not kept as settings.json.bak: %q, %v", data, err)
	}
}

// The snapshot lives outside the profile, so dotfiles never carry it.
func TestSnapshot_OutsideProfileDir(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	profileDir := paths.ProfileDir("p")
	p, _ := mgr.Get("p")
	if err := RegenerateSettings(paths, profileDir, p.Manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snapshotPath(paths, profileDir)); err != nil {
		t.Errorf("snapshot not written: %v", err)
	}
	if strings.HasPrefix(snapshotPath(paths, profileDir), profileDir) {
		t.Errorf("snapshot inside the profile: %s", snapshotPath(paths, profileDir))
	}
	if err := mgr.Delete("p"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snapshotPath(paths, profileDir)); !os.IsNotExist(err) {
		t.Errorf("snapshot left behind after delete: %v", err)
	}
}
