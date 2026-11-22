package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/hub"
)

func setupHarnessTestEnv(t *testing.T) (*config.Paths, *Profile, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	ccpDir := filepath.Join(root, "ccp")
	t.Setenv("HOME", home)
	t.Setenv("CCP_DIR", ccpDir)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	paths, err := config.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range config.AllHubItemTypes() {
		if err := os.MkdirAll(paths.HubItemDir(it), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Populate hub items: skill, rule, agent, command, output style.
	skillDir := filepath.Join(paths.HubItemDir(config.HubSkills), "alpha")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: alpha\ndescription: Alpha skill\n---\nbody\n"), 0644)
	os.WriteFile(filepath.Join(paths.HubItemDir(config.HubRules), "r1.md"), []byte("# Rule 1\nBe brief.\n"), 0644)
	os.WriteFile(filepath.Join(paths.HubItemDir(config.HubRules), "r2.md"), []byte("# Rule 2\nVerify.\n"), 0644)
	os.WriteFile(filepath.Join(paths.HubItemDir(config.HubAgents), "reviewer.md"), []byte("---\nname: reviewer\ndescription: Reviews code\n---\nReview carefully.\n"), 0644)
	os.WriteFile(filepath.Join(paths.HubItemDir(config.HubCommands), "deploy.md"), []byte("---\ndescription: Deploy app\n---\nRun deploy.\n"), 0644)
	os.WriteFile(filepath.Join(paths.HubItemDir(config.HubOutputStyles), "concise.md"), []byte("---\nname: Concise\n---\nKeep replies short.\n"), 0644)

	mgr := NewManager(paths)
	manifest := NewManifest("dev", "")
	p, err := mgr.Create("dev", manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		t    config.HubItemType
		name string
	}{
		{config.HubSkills, "alpha"},
		{config.HubRules, "r1.md"},
		{config.HubRules, "r2.md"},
		{config.HubAgents, "reviewer.md"},
		{config.HubCommands, "deploy.md"},
		{config.HubOutputStyles, "concise.md"},
	} {
		if err := mgr.LinkHubItem("dev", item.t, item.name); err != nil {
			t.Fatalf("LinkHubItem %s/%s: %v", item.t, item.name, err)
		}
	}
	os.WriteFile(filepath.Join(p.Path, "CLAUDE.md"), []byte("# Dev Instructions\n"), 0644)
	os.WriteFile(filepath.Join(p.Path, "settings.json"), []byte(`{"outputStyle":"Concise"}`), 0644)

	p, err = mgr.Get("dev")
	if err != nil {
		t.Fatal(err)
	}
	return paths, p, home
}

func TestExpandHarnessPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CUSTOM_HARNESS_DIR", "")
	t.Setenv("SET_HARNESS_DIR", filepath.Join(home, "custom-set"))

	got, err := ExpandHarnessPath("${env:CUSTOM_HARNESS_DIR:-~/.codex}")
	if err != nil || got != filepath.Join(home, ".codex") {
		t.Fatalf("fallback = %q (%v)", got, err)
	}
	got, err = ExpandHarnessPath("${env:SET_HARNESS_DIR:-~/.codex}")
	if err != nil || got != filepath.Join(home, "custom-set") {
		t.Fatalf("env set = %q (%v)", got, err)
	}
	if _, err := ExpandHarnessPath("${bad"); err == nil {
		t.Error("expected error on unclosed var expression")
	}
}

func TestValidateHarnessID(t *testing.T) {
	for _, good := range []string{"codex", "cursor", "my-agent-2"} {
		if err := ValidateHarnessID(good); err != nil {
			t.Errorf("ValidateHarnessID(%q) = %v", good, err)
		}
	}
	for _, bad := range []string{"", "Codex", "1abc", "../evil", "agy", "omp", "opencode", "pi", "claude"} {
		if err := ValidateHarnessID(bad); err == nil {
			t.Errorf("ValidateHarnessID(%q) succeeded, want error", bad)
		}
	}
}

func TestTOMLHarnessSync_CheckFollowAndRemove(t *testing.T) {
	paths, p, home := setupHarnessTestEnv(t)
	targetRoot := filepath.Join(home, ".codex")
	t.Setenv("CODEX_HOME", targetRoot)

	if err := os.MkdirAll(paths.HarnessesDir(), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `
id = "codex"
name = "Codex"
root = "${env:CODEX_HOME:-~/.codex}"

[context]
link = "AGENTS.md"

[skills]
mode = "link"
dir = "skills"

[commands]
mode = "copy"
dir = "commands"

[agents]
mode = "copy"
dir = "agents"

[rules]
mode = "merge"
file = "RULES.md"

[output_style]
file = "STYLE.md"
`
	if err := os.WriteFile(filepath.Join(paths.HarnessesDir(), "codex.toml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	// Dry-run check writes nothing.
	plan, err := HarnessCheck(paths, "codex", p)
	if err != nil {
		t.Fatalf("HarnessCheck: %v", err)
	}
	if plan.Root != targetRoot || len(plan.Links) != 2 || len(plan.Copies) != 3 || plan.StyleFile != "STYLE.md" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := os.Stat(targetRoot); !os.IsNotExist(err) {
		t.Fatalf("HarnessCheck created target dir: %v", err)
	}

	// Sync applies links, copies, merged rules, and style carrier.
	res, err := HarnessSync(paths, "codex", p)
	if err != nil {
		t.Fatalf("HarnessSync: %v", err)
	}
	if len(res.Linked) != 2 || len(res.Written) != 3 || len(res.Style) == 0 {
		t.Fatalf("unexpected sync result: %+v", res)
	}
	if !HarnessOptedIn(paths, "codex") {
		t.Fatal("expected codex to be opted in after sync")
	}

	rulesData, err := os.ReadFile(filepath.Join(targetRoot, "RULES.md"))
	if err != nil || !strings.Contains(string(rulesData), "Rule 1") || !strings.Contains(string(rulesData), "Rule 2") {
		t.Fatalf("RULES.md = %q (%v)", rulesData, err)
	}
	styleData, err := os.ReadFile(filepath.Join(targetRoot, "STYLE.md"))
	if err != nil || !strings.Contains(string(styleData), "Keep replies short.") {
		t.Fatalf("STYLE.md = %q (%v)", styleData, err)
	}

	// Hand-edited copy is preserved on follow.
	agentPath := filepath.Join(targetRoot, "agents", "reviewer.md")
	os.WriteFile(agentPath, []byte("hand edited\n"), 0644)
	os.WriteFile(filepath.Join(paths.HubItemDir(config.HubAgents), "reviewer.md"), []byte("updated upstream\n"), 0644)

	follows := HarnessFollowAll(paths, p)
	if len(follows) != 1 || follows[0].Err != nil || !slices.Contains(follows[0].Result.Edited, "agents/reviewer.md") {
		t.Fatalf("HarnessFollowAll = %+v", follows)
	}
	if got, _ := os.ReadFile(agentPath); string(got) != "hand edited\n" {
		t.Fatalf("hand-edited file was overwritten: %q", got)
	}

	// Managed links are reported for reset.
	links := HarnessManagedLinks(paths)
	if len(links[targetRoot]) != 2 {
		t.Fatalf("HarnessManagedLinks = %+v", links)
	}

	// Remove cleans unmodified copies, links, style carrier, and state, while keeping hand-edited file.
	rmRes, err := HarnessRemove(paths, "codex")
	if err != nil {
		t.Fatalf("HarnessRemove: %v", err)
	}
	if HarnessOptedIn(paths, "codex") {
		t.Fatal("expected codex opted out after remove")
	}
	if !slices.Contains(rmRes.Edited, "agents/reviewer.md") {
		t.Errorf("expected hand-edited agent reported in remove: %+v", rmRes)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "RULES.md")); !os.IsNotExist(err) {
		t.Errorf("RULES.md still exists after remove")
	}
	if _, err := os.Lstat(filepath.Join(targetRoot, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md link still exists after remove")
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "STYLE.md")); !os.IsNotExist(err) {
		t.Errorf("STYLE.md still exists after remove")
	}
}

func TestTOMLHarness_RejectsUnknownFieldsAndSymlinkDir(t *testing.T) {
	paths, p, home := setupHarnessTestEnv(t)
	if err := os.MkdirAll(paths.HarnessesDir(), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "bad.toml"), []byte("root = \"~/.bad\"\nbogus_key = true\n"), 0644)
	if _, err := FindHarness(paths, "bad"); err == nil {
		t.Fatal("expected unknown TOML field to fail")
	}

	// Symlinked skills/ dir is refused, not written through.
	targetRoot := filepath.Join(home, ".safe")
	foreignDir := filepath.Join(home, "foreign-skills")
	os.MkdirAll(targetRoot, 0755)
	os.MkdirAll(foreignDir, 0755)
	os.Symlink(foreignDir, filepath.Join(targetRoot, "skills"))

	os.WriteFile(filepath.Join(paths.HarnessesDir(), "safe.toml"), []byte("root = \""+targetRoot+"\"\n[skills]\nmode = \"link\"\ndir = \"skills\"\n"), 0644)
	res, err := HarnessSync(paths, "safe", p)
	if err != nil {
		t.Fatalf("HarnessSync: %v", err)
	}
	if len(res.Linked) != 0 || len(res.Notes) == 0 || !strings.Contains(res.Notes[0], "is a symlink") {
		t.Fatalf("expected symlink dir refusal, got %+v", res)
	}
	if entries, _ := os.ReadDir(foreignDir); len(entries) != 0 {
		t.Fatalf("wrote through symlinked dir: %v", entries)
	}
}

func TestExecHarnessPlugin_SyncAndPathGuards(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script exec plugin test requires Unix")
	}
	paths, p, home := setupHarnessTestEnv(t)
	targetRoot := filepath.Join(home, ".mytool")
	binDir := filepath.Join(home, "bin")
	os.MkdirAll(binDir, 0755)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Plugin on PATH: ccp-harness-mytool reads stdin JSON and outputs a valid SyncPlan.
	skillHub := filepath.Join(paths.HubItemDir(config.HubSkills), "alpha")
	script := "#!/bin/sh\n" +
		"cat >/dev/null\n" +
		"printf '%s\\n' '{\"api_version\":1,\"root\":\"" + targetRoot + "\",\"links\":{\"skills/alpha\":\"" + skillHub + "\"},\"copies\":{\"converted/a.md\":\"hello\\n\"},\"notes\":[\"custom ok\"]}'\n"
	pluginPath := filepath.Join(binDir, "ccp-harness-mytool")
	if err := os.WriteFile(pluginPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	plugins, err := DiscoverHarnesses(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].ID != "mytool" || plugins[0].Kind != HarnessKindExec {
		t.Fatalf("DiscoverHarnesses = %+v", plugins)
	}

	res, err := HarnessSync(paths, "mytool", p)
	if err != nil {
		t.Fatalf("HarnessSync exec: %v", err)
	}
	if !slices.Contains(res.Linked, "skills/alpha") || !slices.Contains(res.Written, "converted/a.md") {
		t.Fatalf("exec sync result = %+v", res)
	}
	if got, _ := os.ReadFile(filepath.Join(targetRoot, "converted", "a.md")); string(got) != "hello\n" {
		t.Fatalf("converted/a.md = %q", got)
	}

	// Path traversal and outside-ccp link targets are rejected before any write.
	os.MkdirAll(paths.HarnessesDir(), 0755)
	evilTraversal := "#!/bin/sh\nprintf '{\"api_version\":1,\"root\":\"" + targetRoot + "\",\"copies\":{\"../escape.md\":\"nope\"}}\\n'\n"
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "evil-copy"), []byte(evilTraversal), 0755)
	if _, err := HarnessSync(paths, "evil-copy", p); err == nil || !strings.Contains(err.Error(), "escapes harness root") {
		t.Errorf("traversal err = %v", err)
	}

	evilLink := "#!/bin/sh\nprintf '{\"api_version\":1,\"root\":\"" + targetRoot + "\",\"links\":{\"x\":\"/etc/passwd\"}}\\n'\n"
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "evil-link"), []byte(evilLink), 0755)
	if _, err := HarnessSync(paths, "evil-link", p); err == nil || !strings.Contains(err.Error(), "must resolve strictly inside") {
		t.Errorf("outside link err = %v", err)
	}
}

func TestCouncilRegressions_RulesLinkSkillsCopyAndEnvGuards(t *testing.T) {
	paths, p, home := setupHarnessTestEnv(t)
	os.MkdirAll(paths.HarnessesDir(), 0755)

	// 1. [rules] mode = "link" links all profile rules.
	rulesRoot := filepath.Join(home, ".rules-link")
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "rlink.toml"), []byte(
		"root = \""+rulesRoot+"\"\n[rules]\nmode = \"link\"\ndir = \"rules\"\n",
	), 0644)
	res, err := HarnessSync(paths, "rlink", p)
	if err != nil {
		t.Fatalf("rlink sync: %v", err)
	}
	if !slices.Contains(res.Linked, "rules/r1.md") || !slices.Contains(res.Linked, "rules/r2.md") {
		t.Fatalf("expected rules linked, got %+v", res)
	}

	// 2. [skills] mode = "copy" is rejected during validation.
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "badskills.toml"), []byte(
		"root = \""+filepath.Join(home, ".bs")+"\"\n[skills]\nmode = \"copy\"\ndir = \"skills\"\n",
	), 0644)
	if _, err := FindHarness(paths, "badskills"); err == nil {
		t.Fatal("expected [skills] mode = \"copy\" to fail validation")
	}

	// 3. Unset ${env:VAR} without default, relative root, and dangerous roots fail.
	t.Setenv("UNSET_HARNESS_VAR", "")
	for _, bad := range []string{"${env:UNSET_HARNESS_VAR}", "./relative", "codex"} {
		if _, err := ExpandHarnessPath(bad); err == nil {
			t.Errorf("ExpandHarnessPath(%q) succeeded, want error", bad)
		}
	}
	for _, badRoot := range []string{"/", home, paths.CcpDir, filepath.Join(paths.CcpDir, "sub")} {
		plan := HarnessPlan{APIVersion: 1, Root: badRoot}
		if err := ValidateHarnessPlan(paths, &plan); err == nil {
			t.Errorf("ValidateHarnessPlan(root=%q) succeeded, want error", badRoot)
		}
	}
}

func TestCouncilRegressions_LinkToCopyRootChangeStyleRetireAndNestedSymlink(t *testing.T) {
	paths, p, home := setupHarnessTestEnv(t)
	os.MkdirAll(paths.HarnessesDir(), 0755)
	rootA := filepath.Join(home, ".tool-a")
	rootB := filepath.Join(home, ".tool-b")
	manifestPath := filepath.Join(paths.HarnessesDir(), "evo.toml")

	// Sync 1: rules as links + STYLE.md.
	os.WriteFile(manifestPath, []byte(
		"root = \""+rootA+"\"\n[rules]\nmode = \"link\"\ndir = \"rules\"\n[output_style]\nfile = \"STYLE.md\"\n",
	), 0644)
	if _, err := HarnessSync(paths, "evo", p); err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(rootA, "rules", "r1.md")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink after sync 1: %v %v", info, err)
	}

	// Sync 2: switch rules from link -> copy, and remove [output_style].
	os.WriteFile(manifestPath, []byte(
		"root = \""+rootA+"\"\n[rules]\nmode = \"copy\"\ndir = \"rules\"\n",
	), 0644)
	if _, err := HarnessSync(paths, "evo", p); err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(rootA, "rules", "r1.md")); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected regular copied file after link->copy: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(rootA, "STYLE.md")); !os.IsNotExist(err) {
		t.Fatalf("expected STYLE.md retired when [output_style] removed, err=%v", err)
	}

	// Sync 3: change root from rootA -> rootB; old files in rootA must be cleaned up.
	os.WriteFile(manifestPath, []byte(
		"root = \""+rootB+"\"\n[rules]\nmode = \"copy\"\ndir = \"rules\"\n",
	), 0644)
	if _, err := HarnessSync(paths, "evo", p); err != nil {
		t.Fatalf("sync 3: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootA, "rules", "r1.md")); !os.IsNotExist(err) {
		t.Fatalf("old root file stranded after root change")
	}
	if _, err := os.Stat(filepath.Join(rootB, "rules", "r1.md")); err != nil {
		t.Fatalf("new root file missing: %v", err)
	}

	// Nested symlinked parent dir (a/b -> outside) is refused.
	outside := filepath.Join(home, "outside-nested")
	os.MkdirAll(outside, 0755)
	os.MkdirAll(filepath.Join(rootB, "a"), 0755)
	os.Symlink(outside, filepath.Join(rootB, "a", "b"))
	plan := HarnessPlan{
		APIVersion: 1,
		Root:       rootB,
		Copies:     map[string]string{"a/b/x.md": "should not write"},
		StyleFile:  "a/b/STYLE.md",
	}
	res, err := applyHarnessPlan(paths, "evo", p, plan)
	if err != nil {
		t.Fatalf("nested symlink sync: %v", err)
	}
	if len(res.Written) != 0 || len(res.Style) != 0 {
		t.Fatalf("wrote through nested symlink: %+v", res)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("files leaked into outside dir: %v", entries)
	}
}

func TestCouncilRegressions_ExecSecurityAndTargetEvalSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script exec plugin test requires Unix")
	}
	paths, p, home := setupHarnessTestEnv(t)
	os.MkdirAll(paths.HarnessesDir(), 0755)
	targetRoot := filepath.Join(home, ".sec")

	// 1. Symlink inside ~/.ccp escaping to outside dir must be rejected by targetInsideCcp.
	outsideFile := filepath.Join(home, "secret.txt")
	os.WriteFile(outsideFile, []byte("secret"), 0600)
	escapeLink := filepath.Join(paths.HubDir, "escape")
	os.Symlink(outsideFile, escapeLink)
	badPlan := HarnessPlan{APIVersion: 1, Root: targetRoot, Links: map[string]string{"leak": escapeLink}}
	if err := ValidateHarnessPlan(paths, &badPlan); err == nil {
		t.Fatal("expected symlink inside ~/.ccp escaping outside to be rejected")
	}
	// Linking ~/.ccp itself must also be rejected.
	selfPlan := HarnessPlan{APIVersion: 1, Root: targetRoot, Links: map[string]string{"all": paths.CcpDir}}
	if err := ValidateHarnessPlan(paths, &selfPlan); err == nil {
		t.Fatal("expected linking CcpDir itself to be rejected")
	}

	// 2. Unknown JSON field in exec plugin output must fail.
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "typo"), []byte(
		"#!/bin/sh\nprintf '%s\\n' '{\"api_version\":1,\"root\":\""+targetRoot+"\",\"link\":{}}'\n",
	), 0755)
	if _, err := HarnessSync(paths, "typo", p); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}

	// 3. Ancestor/descendant collision and style_file collision must fail.
	nestPlan := HarnessPlan{
		APIVersion: 1,
		Root:       targetRoot,
		Links:      map[string]string{"skills/alpha": filepath.Join(paths.HubItemDir(config.HubSkills), "alpha")},
		Copies:     map[string]string{"skills/alpha/extra.md": "x"},
	}
	if err := ValidateHarnessPlan(paths, &nestPlan); err == nil {
		t.Fatal("expected nested path collision error")
	}
	stylePlan := HarnessPlan{
		APIVersion: 1,
		Root:       targetRoot,
		Copies:     map[string]string{"STYLE.md": "x"},
		StyleFile:  "STYLE.md",
	}
	if err := ValidateHarnessPlan(paths, &stylePlan); err == nil {
		t.Fatal("expected style_file collision error")
	}

	// 4. Relative PATH entry "." must not be discovered.
	relDir := filepath.Join(home, "cwd-rel")
	os.MkdirAll(relDir, 0755)
	os.WriteFile(filepath.Join(relDir, "ccp-harness-sneaky"), []byte("#!/bin/sh\necho '{}'\n"), 0755)
	origWD, _ := os.Getwd()
	os.Chdir(relDir)
	t.Cleanup(func() { os.Chdir(origWD) })
	t.Setenv("PATH", ".")
	plugins, err := DiscoverHarnesses(paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range plugins {
		if pl.ID == "sneaky" {
			t.Fatalf("discovered plugin from relative PATH '.': %+v", pl)
		}
	}
}

func TestVerifyRegressions_NestedSiblingStyleMoveAndSymlinkRetire(t *testing.T) {
	paths, p, home := setupHarnessTestEnv(t)
	os.MkdirAll(paths.HarnessesDir(), 0755)
	targetRoot := filepath.Join(home, ".verify")

	// 1. Sibling sorting between ancestor and descendant ("skills", "skills.md", "skills/alpha").
	siblingPlan := HarnessPlan{
		APIVersion: 1,
		Root:       targetRoot,
		Links: map[string]string{
			"skills":       filepath.Join(paths.HubDir, "skills"),
			"skills/alpha": filepath.Join(paths.HubItemDir(config.HubSkills), "alpha"),
		},
		Copies: map[string]string{"skills.md": "x"},
	}
	if err := ValidateHarnessPlan(paths, &siblingPlan); err == nil {
		t.Fatal("expected ancestor/descendant overlap to fail even with 'skills.md' sorting between them")
	}

	// 2. Moving a path from style_file -> copies replaces it cleanly without Occupied.
	p1 := HarnessPlan{APIVersion: 1, Root: targetRoot, StyleFile: "STYLE.md"}
	if _, err := applyHarnessPlan(paths, "vtest", p, p1); err != nil {
		t.Fatalf("style sync: %v", err)
	}
	p2 := HarnessPlan{APIVersion: 1, Root: targetRoot, Copies: map[string]string{"STYLE.md": "new copy\n"}}
	res, err := applyHarnessPlan(paths, "vtest", p, p2)
	if err != nil {
		t.Fatalf("style->copy sync: %v", err)
	}
	if len(res.Occupied) != 0 {
		t.Fatalf("expected no Occupied on style->copy move, got %+v", res)
	}
	if got, _ := os.ReadFile(filepath.Join(targetRoot, "STYLE.md")); string(got) != "new copy\n" {
		t.Fatalf("STYLE.md = %q, want new copy", got)
	}

	// 3. Replacing style_file parent dir with a symlink must NOT delete outside file on sync or remove.
	subRoot := filepath.Join(home, ".verify-sub")
	s1 := HarnessPlan{APIVersion: 1, Root: subRoot, StyleFile: "sub/STYLE.md"}
	if _, err := applyHarnessPlan(paths, "vsub", p, s1); err != nil {
		t.Fatalf("sub style sync: %v", err)
	}
	origBytes, err := os.ReadFile(filepath.Join(subRoot, "sub", "STYLE.md"))
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside-style")
	os.MkdirAll(outside, 0755)
	os.WriteFile(filepath.Join(outside, "STYLE.md"), origBytes, 0644)
	os.RemoveAll(filepath.Join(subRoot, "sub"))
	os.Symlink(outside, filepath.Join(subRoot, "sub"))

	if _, err := applyHarnessPlan(paths, "vsub", p, HarnessPlan{APIVersion: 1, Root: subRoot}); err != nil {
		t.Fatalf("retire with symlinked parent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "STYLE.md")); err != nil {
		t.Fatalf("outside STYLE.md was deleted through symlinked parent: %v", err)
	}
}

func TestStyleCarrier_CrossHarnessGroupIsolation(t *testing.T) {
	paths, p, home := setupHarnessTestEnv(t)
	if _, err := AgySync(paths, p); err != nil {
		t.Fatalf("AgySync: %v", err)
	}
	geminiPath := filepath.Join(home, ".gemini", "config", "GEMINI.md")
	origData, err := os.ReadFile(geminiPath)
	if err != nil {
		t.Fatalf("agy GEMINI.md missing: %v", err)
	}

	os.MkdirAll(paths.HarnessesDir(), 0755)
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "hijack.toml"), []byte(
		"root = \""+filepath.Join(home, ".gemini", "config")+"\"\n[output_style]\nfile = \"GEMINI.md\"\n",
	), 0644)

	res, err := HarnessSync(paths, "hijack", p)
	if err != nil {
		t.Fatalf("HarnessSync hijack: %v", err)
	}
	if len(res.Style) == 0 || !strings.Contains(res.Style[0], "occupied") {
		t.Fatalf("expected occupied style report, got %+v", res.Style)
	}

	ledger, err := loadCarrierLedger(paths)
	if err != nil {
		t.Fatalf("loadCarrierLedger: %v", err)
	}
	if rec, ok := ledger.Carriers[geminiPath]; !ok || rec.Group != carrierGroupAgy {
		t.Fatalf("carrier ownership hijacked or lost: ok=%v rec=%+v", ok, rec)
	}

	// Even if output style is deselected before HarnessRemove, hijack cannot
	// remove agy's carrier.
	os.WriteFile(filepath.Join(p.Path, "settings.json"), []byte(`{}`), 0644)
	if _, err := HarnessSync(paths, "hijack", p); err != nil {
		t.Fatalf("HarnessSync hijack (deselected): %v", err)
	}
	if _, err := HarnessRemove(paths, "hijack"); err != nil {
		t.Fatalf("HarnessRemove hijack: %v", err)
	}
	got, err := os.ReadFile(geminiPath)
	if err != nil || string(got) != string(origData) {
		t.Fatalf("agy GEMINI.md altered or deleted by HarnessRemove: err=%v data=%q", err, got)
	}
	ledger, err = loadCarrierLedger(paths)
	if err != nil {
		t.Fatalf("loadCarrierLedger after remove: %v", err)
	}
	if rec, ok := ledger.Carriers[geminiPath]; !ok || rec.Group != carrierGroupAgy {
		t.Fatalf("agy ledger entry lost after HarnessRemove: ok=%v rec=%+v", ok, rec)
	}
}

func TestBuildHarnessInput_Completeness(t *testing.T) {
	paths, p, _ := setupHarnessTestEnv(t)

	// Direct hub hook + bundle containing agent, rule, command, and hook.
	directHookDir := filepath.Join(paths.HubItemDir(config.HubHooks), "z-direct")
	os.MkdirAll(directHookDir, 0755)
	os.WriteFile(filepath.Join(directHookDir, "hooks.json"), []byte(`{}`), 0644)

	bundleDir := paths.BundleDir("pack")
	os.MkdirAll(filepath.Join(bundleDir, "agents"), 0755)
	os.MkdirAll(filepath.Join(bundleDir, "rules"), 0755)
	os.MkdirAll(filepath.Join(bundleDir, "commands"), 0755)
	os.MkdirAll(filepath.Join(bundleDir, "hooks", "a-bundle-hook"), 0755)
	os.WriteFile(filepath.Join(bundleDir, "agents", "b-agent.md"), []byte("---\ndescription: Bundle agent\n---\n"), 0644)
	os.WriteFile(filepath.Join(bundleDir, "rules", "b-rule.md"), []byte("# Bundle rule\n"), 0644)
	os.WriteFile(filepath.Join(bundleDir, "commands", "b-cmd.md"), []byte("---\ndescription: Bundle cmd\n---\n"), 0644)
	os.WriteFile(filepath.Join(bundleDir, "hooks", "a-bundle-hook", "hooks.json"), []byte(`{}`), 0644)

	bundle := &hub.Bundle{
		Name:    "pack",
		Version: "1.0.0",
		Members: hub.ComponentList{
			Agents:   []string{"b-agent.md"},
			Rules:    []string{"b-rule.md"},
			Commands: []string{"b-cmd.md"},
			Hooks:    []string{"a-bundle-hook"},
		},
	}
	if err := bundle.Save(paths.BundlesDir()); err != nil {
		t.Fatalf("bundle.Save: %v", err)
	}

	mgr := NewManager(paths)
	if err := mgr.LinkHubItem("dev", config.HubHooks, "z-direct"); err != nil {
		t.Fatalf("LinkHubItem hook: %v", err)
	}
	if err := mgr.LinkHubBundle("dev", "pack"); err != nil {
		t.Fatalf("LinkHubBundle: %v", err)
	}
	// Profile-local rule not backed by the hub should have empty HubPath.
	os.WriteFile(filepath.Join(p.Path, "rules", "local-only.md"), []byte("# Local\n"), 0644)

	p, err := mgr.Get("dev")
	if err != nil {
		t.Fatalf("mgr.Get: %v", err)
	}

	in := buildHarnessInput(paths, "custom", p)
	if in.OutputStyleUnknown || in.OutputStyle == nil {
		t.Fatalf("expected known OutputStyle, got unknown=%v style=%+v", in.OutputStyleUnknown, in.OutputStyle)
	}
	if wantHooks := []string{"a-bundle-hook", "z-direct"}; !slices.Equal(in.Hooks, wantHooks) {
		t.Fatalf("Hooks = %v, want %v", in.Hooks, wantHooks)
	}

	hubPathByName := func(items []HarnessFileInput) map[string]string {
		m := map[string]string{}
		for _, it := range items {
			m[it.Name] = it.HubPath
		}
		return m
	}

	agents := hubPathByName(in.Agents)
	if agents["reviewer.md"] != filepath.Join(paths.HubItemDir(config.HubAgents), "reviewer.md") {
		t.Errorf("direct agent HubPath = %q", agents["reviewer.md"])
	}
	if agents["b-agent.md"] != filepath.Join(bundleDir, "agents", "b-agent.md") {
		t.Errorf("bundle agent HubPath = %q", agents["b-agent.md"])
	}

	rules := hubPathByName(in.Rules)
	if rules["r1.md"] != filepath.Join(paths.HubItemDir(config.HubRules), "r1.md") {
		t.Errorf("direct rule HubPath = %q", rules["r1.md"])
	}
	if rules["b-rule.md"] != filepath.Join(bundleDir, "rules", "b-rule.md") {
		t.Errorf("bundle rule HubPath = %q", rules["b-rule.md"])
	}
	if rules["local-only.md"] != "" {
		t.Errorf("local-only rule HubPath = %q, want empty", rules["local-only.md"])
	}

	cmds := hubPathByName(in.Commands)
	if cmds["deploy.md"] != filepath.Join(paths.HubItemDir(config.HubCommands), "deploy.md") {
		t.Errorf("direct command HubPath = %q", cmds["deploy.md"])
	}
	if cmds["b-cmd.md"] != filepath.Join(bundleDir, "commands", "b-cmd.md") {
		t.Errorf("bundle command HubPath = %q", cmds["b-cmd.md"])
	}

	// Corrupt settings.json sets OutputStyleUnknown = true.
	os.WriteFile(filepath.Join(p.Path, "settings.json"), []byte(`{"outputStyle": `), 0644)
	inCorrupt := buildHarnessInput(paths, "custom", p)
	if !inCorrupt.OutputStyleUnknown || inCorrupt.OutputStyle != nil {
		t.Fatalf("corrupt settings.json: OutputStyleUnknown=%v OutputStyle=%+v", inCorrupt.OutputStyleUnknown, inCorrupt.OutputStyle)
	}
}

func TestCouncilRound2Regressions(t *testing.T) {
	paths, p, home := setupHarnessTestEnv(t)
	os.MkdirAll(paths.HarnessesDir(), 0755)

	// 1. Symlinked root pointing to ~/.ccp must be rejected by validateHarnessRoot.
	alias := filepath.Join(home, "ccp-alias")
	os.Symlink(paths.CcpDir, alias)
	for _, badRoot := range []string{alias, filepath.Join(alias, "sub")} {
		plan := HarnessPlan{APIVersion: 1, Root: badRoot}
		if err := ValidateHarnessPlan(paths, &plan); err == nil {
			t.Errorf("ValidateHarnessPlan(symlink root=%q) succeeded, want error", badRoot)
		}
	}

	// 2. Deleted/edited style carrier is forgotten on HarnessRemove so another harness can use the path.
	sharedRoot := filepath.Join(home, ".r2-style")
	if _, err := applyHarnessPlan(paths, "h1", p, HarnessPlan{APIVersion: 1, Root: sharedRoot, StyleFile: "STYLE.md"}); err != nil {
		t.Fatalf("h1 sync: %v", err)
	}
	os.WriteFile(filepath.Join(sharedRoot, "STYLE.md"), []byte("user edit\n"), 0644)
	rmRes, err := HarnessRemove(paths, "h1")
	if err != nil {
		t.Fatalf("h1 remove: %v", err)
	}
	if len(rmRes.Edited) == 0 {
		t.Fatalf("expected edited style carrier reported on remove, got %+v", rmRes)
	}
	os.Remove(filepath.Join(sharedRoot, "STYLE.md"))
	// Now h2 can take ownership of STYLE.md without hitting foreignOwner.
	h2Res, err := applyHarnessPlan(paths, "h2", p, HarnessPlan{APIVersion: 1, Root: sharedRoot, StyleFile: "STYLE.md"})
	if err != nil {
		t.Fatalf("h2 sync: %v", err)
	}
	if strings.Contains(strings.Join(h2Res.Style, "\n"), "occupied by another harness") {
		t.Fatalf("h2 blocked by stale h1 ledger entry: %+v", h2Res.Style)
	}

	// 3. Switching a directory link ("skills") to child link ("skills/alpha") succeeds in one sync.
	swRoot := filepath.Join(home, ".r2-switch")
	p1 := HarnessPlan{APIVersion: 1, Root: swRoot, Links: map[string]string{"skills": filepath.Join(paths.HubDir, "skills")}}
	if _, err := applyHarnessPlan(paths, "sw", p, p1); err != nil {
		t.Fatalf("sw sync 1: %v", err)
	}
	p2 := HarnessPlan{APIVersion: 1, Root: swRoot, Links: map[string]string{"skills/alpha": filepath.Join(paths.HubItemDir(config.HubSkills), "alpha")}}
	res, err := applyHarnessPlan(paths, "sw", p, p2)
	if err != nil {
		t.Fatalf("sw sync 2: %v", err)
	}
	if !slices.Contains(res.Linked, "skills/alpha") {
		t.Fatalf("expected skills/alpha linked on first sync after dir->child switch, got %+v", res)
	}

	// 4. Missing hub item in TOML link mode is skipped instead of aborting the whole sync.
	p.Manifest.Hub.Skills = append(p.Manifest.Hub.Skills, "deleted-skill")
	if err := p.Manifest.Save(ManifestPath(p.Path)); err != nil {
		t.Fatal(err)
	}
	missingRoot := filepath.Join(home, ".r2-missing")
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "ms.toml"), []byte(
		"root = \""+missingRoot+"\"\n[skills]\nmode = \"link\"\ndir = \"skills\"\n",
	), 0644)
	msRes, err := HarnessSync(paths, "ms", p)
	if err != nil {
		t.Fatalf("ms sync with missing hub item failed: %v", err)
	}
	if !slices.Contains(msRes.Linked, "skills/alpha") || len(msRes.Skipped) == 0 {
		t.Fatalf("expected alpha linked and missing skill skipped, got %+v", msRes)
	}

	// 5. syncCopies reports a directory at copy target as Occupied instead of EISDIR.
	dirRoot := filepath.Join(home, ".r2-dir")
	os.MkdirAll(filepath.Join(dirRoot, "agents", "reviewer.md"), 0755)
	dirRes, err := applyHarnessPlan(paths, "dirtest", p, HarnessPlan{
		APIVersion: 1,
		Root:       dirRoot,
		Copies:     map[string]string{"agents/reviewer.md": "body\n"},
	})
	if err != nil {
		t.Fatalf("syncCopies on directory failed: %v", err)
	}
	if !slices.Contains(dirRes.Occupied, "agents/reviewer.md") {
		t.Fatalf("expected directory reported as Occupied, got %+v", dirRes)
	}
}
