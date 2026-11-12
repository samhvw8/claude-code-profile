package profile

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

func agyAgent(t *testing.T, in string) (string, map[string]any, string) {
	t.Helper()
	name, out, err := convertAgyAgent("stem", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	fm, body, err := splitFrontmatter(out)
	if err != nil {
		t.Fatalf("output is not valid frontmatter: %v\n%s", err, out)
	}
	return name, fm, body
}

func toolList(fm map[string]any) []string {
	raw, ok := fm["tools"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

func TestConvertAgyAgent_ToolsString(t *testing.T) {
	name, fm, body := agyAgent(t, "---\nname: applier\ndescription: \"Applies edits: carefully\"\ntools: Read, Grep, Bash(git:*), Task, mcp__x__y, Write\nmodel: inherit\ncolor: purple\neffort: high\nmaxTurns: 4\n---\n\nYou apply edits.\n")
	if name != "applier" || fm["name"] != "applier" || fm["description"] != "Applies edits: carefully" {
		t.Errorf("name/description = %q %v", name, fm)
	}
	if fm["subagent"] != true || fm["mainAgent"] != false {
		t.Errorf("modes = %v", fm)
	}
	if got, want := toolList(fm), []string{"read_file", "write_file", "run_command", "manage_task"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
	for _, gone := range []string{"model", "color", "effort", "maxTurns"} {
		if _, ok := fm[gone]; ok {
			t.Errorf("%s should not reach agy: %v", gone, fm)
		}
	}
	if body != "\nYou apply edits.\n" {
		t.Errorf("body changed: %q", body)
	}
}

func TestConvertAgyAgent_ToolsList(t *testing.T) {
	_, fm, _ := agyAgent(t, "---\ndescription: d\ntools:\n  - WebFetch\n  - Edit\n  - MultiEdit\n  - Glob\nmodel: haiku\n---\nx\n")
	if got, want := toolList(fm), []string{"read_file", "edit_file", "search_web"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
	if fm["model"] != "flash" {
		t.Errorf("haiku not mapped to flash: %v", fm["model"])
	}
	if fm["name"] != "stem" {
		t.Errorf("name should default to the file stem: %v", fm["name"])
	}
}

func TestConvertAgyAgent_NoToolsGrantsAll(t *testing.T) {
	_, fm, _ := agyAgent(t, "---\ndescription: d\nmodel: opus\n---\nx\n")
	if got := toolList(fm); !reflect.DeepEqual(got, agyTools) {
		t.Errorf("tools = %v, want all", got)
	}
	if fm["model"] != "pro" {
		t.Errorf("opus not mapped to pro: %v", fm["model"])
	}
}

func TestConvertAgyAgent_OnlyUnmappedToolsWritesEmptyList(t *testing.T) {
	_, out, err := convertAgyAgent("x", []byte("---\ndescription: d\ntools: Task, TodoWrite\n---\nx\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "tools: []") {
		t.Errorf("tools must be written even when empty:\n%s", out)
	}
}

func TestConvertAgyAgent_DisallowedTools(t *testing.T) {
	_, fm, _ := agyAgent(t, "---\ndescription: d\ndisallowedTools: Write, Bash, WebSearch, Edit(src/**)\n---\nx\n")
	if got, want := toolList(fm), []string{"read_file", "edit_file", "search_web"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
	_, fm, _ = agyAgent(t, "---\ndescription: d\ndisallowedTools:\n  - Read\n  - WebSearch\n  - WebFetch\n---\nx\n")
	if got, want := toolList(fm), []string{"edit_file", "write_file", "run_command", "manage_task"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
}

func TestConvertAgyAgent_ModelsOtherThanAliasesDropped(t *testing.T) {
	for _, model := range []string{"sonnet", "inherit", "claude-opus-4-5"} {
		_, fm, _ := agyAgent(t, "---\ndescription: d\nmodel: "+model+"\n---\nx\n")
		if _, ok := fm["model"]; ok {
			t.Errorf("model %s should be dropped: %v", model, fm["model"])
		}
	}
}

func TestConvertAgyAgent_MissingDescriptionAndNameClash(t *testing.T) {
	if _, _, err := convertAgyAgent("x", []byte("---\nname: x\n---\nbody\n")); err == nil {
		t.Error("agent without description accepted")
	}
	name, fm, _ := agyAgent(t, "---\nname: research\ndescription: d\n---\nx\n")
	if name != "claude-research" || fm["name"] != "claude-research" {
		t.Errorf("built-in name not prefixed: %q %v", name, fm["name"])
	}
}

func TestConvertAgyAgent_ProseDescriptionThatBreaksYAML(t *testing.T) {
	name, fm, _ := agyAgent(t, "---\nname: rev\ndescription: Reviews code. Examples: a, b\ntools: Read\n---\nx\n")
	if name != "rev" || fm["description"] != "Reviews code. Examples: a, b" {
		t.Errorf("lenient parse failed: %v", fm)
	}
}

func agyRule(t *testing.T, in string) (map[string]any, string) {
	t.Helper()
	out, err := convertAgyRule([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	fm, body, err := splitFrontmatter(out)
	if err != nil {
		t.Fatalf("output is not valid frontmatter: %v\n%s", err, out)
	}
	return fm, body
}

func TestConvertAgyRule(t *testing.T) {
	fm, body := agyRule(t, "# Rule\n\nDo it.\n")
	if fm["trigger"] != "always_on" || len(fm) != 1 || body != "# Rule\n\nDo it.\n" {
		t.Errorf("no frontmatter: %v %q", fm, body)
	}

	fm, _ = agyRule(t, "---\ndescription: Route code questions\nalwaysApply: true\n---\nbody\n")
	if fm["trigger"] != "always_on" || fm["description"] != "Route code questions" {
		t.Errorf("alwaysApply: %v", fm)
	}
	if _, ok := fm["alwaysApply"]; ok {
		t.Errorf("alwaysApply should be dropped: %v", fm)
	}

	fm, _ = agyRule(t, "---\npaths: \"src/**/*.{ts,tsx}\"\n---\nbody\n")
	if fm["trigger"] != "glob" || fm["globs"] != "**/src/**/*.ts, **/src/**/*.tsx" {
		t.Errorf("paths string: %v", fm)
	}

	fm, _ = agyRule(t, "---\npaths:\n  - \"**/*.go\"\n  - ./docs/*.md\n  - /abs/x.txt\n  - \"*.{a,b{c,d}}\"\n---\nbody\n")
	if fm["globs"] != "**/*.go, **/docs/*.md, /abs/x.txt, **/*.a, **/*.bc, **/*.bd" {
		t.Errorf("paths list: %v", fm["globs"])
	}
}

func TestExpandBraces(t *testing.T) {
	got := expandBraces("{src,lib}/*.{js,ts}")
	want := []string{"src/*.js", "src/*.ts", "lib/*.js", "lib/*.ts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expandBraces = %v", got)
	}
	if got := expandBraces("a{b"); !reflect.DeepEqual(got, []string{"a{b"}) {
		t.Errorf("unclosed brace = %v", got)
	}
}

// setupAgyTest gives the test its own HOME, so nothing touches the real
// ~/.gemini, and a profile with a CLAUDE.md, a rule, an agent and two skills.
func setupAgyTest(t *testing.T) (*config.Paths, *Manager, string) {
	t.Helper()
	paths, mgr := setupBundleTest(t)
	t.Setenv("HOME", t.TempDir())
	dir := AgyConfigDir()

	mustWrite(t, filepath.Join(paths.HubItemPath(config.HubRules, "scoped.md")), "---\npaths: \"**/*.go\"\n---\nGo rule\n")
	mustWrite(t, filepath.Join(paths.HubItemPath(config.HubAgents, "helper.md")), "---\ndescription: helps\ntools: Read\n---\nbody\n")
	mustWrite(t, filepath.Join(paths.HubItemPath(config.HubSkills, "good"), "SKILL.md"), "---\ndescription: a good skill\n---\n")
	mustWrite(t, filepath.Join(paths.HubItemPath(config.HubSkills, "bare"), "SKILL.md"), "# no frontmatter\n")
	mustWrite(t, filepath.Join(paths.HubItemPath(config.HubCommands, "ship.md")), "---\ndescription: ship\n---\nx\n")
	for _, item := range []struct {
		t    config.HubItemType
		name string
	}{
		{config.HubRules, "scoped.md"}, {config.HubAgents, "helper.md"},
		{config.HubSkills, "good"}, {config.HubSkills, "bare"}, {config.HubCommands, "ship.md"},
	} {
		if err := mgr.LinkHubItem("p", item.t, item.name); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(paths.ProfileDir("p"), "CLAUDE.md"), "# context\n")
	return paths, mgr, dir
}

func TestAgySync_WritesLinksAndPrunes(t *testing.T) {
	paths, mgr, dir := setupAgyTest(t)
	mustWrite(t, filepath.Join(dir, "skills", "mine", "SKILL.md"), "---\ndescription: hand-made\n---\n")
	p, _ := mgr.Get("p")

	r, err := AgySync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Written, []string{"agents/helper.md", "rules/scoped.md"}) {
		t.Errorf("written = %v", r.Written)
	}
	if !slices.Contains(r.Linked, "skills/good") || !slices.Contains(r.Linked, "AGENTS.md → CLAUDE.md") {
		t.Errorf("linked = %v", r.Linked)
	}
	if len(r.Skipped) != 1 || !strings.HasPrefix(r.Skipped[0], "skills/bare:") {
		t.Errorf("skipped = %v", r.Skipped)
	}
	if !reflect.DeepEqual(r.Commands, []string{"ship"}) {
		t.Errorf("commands = %v", r.Commands)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "rules", "scoped.md")); !strings.Contains(string(data), "trigger: glob") {
		t.Errorf("rule not converted: %s", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(data) != "# context\n" {
		t.Errorf("AGENTS.md does not reach CLAUDE.md: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "skills", "good", "SKILL.md")); !strings.Contains(string(data), "a good skill") {
		t.Errorf("skill link does not resolve: %q", data)
	}
	if !AgyOptedIn(paths) {
		t.Error("not opted in after a sync")
	}

	// Second sync changes nothing.
	r, err = AgySync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Written)+len(r.Linked)+len(r.Removed) != 0 {
		t.Errorf("second sync not idempotent: %+v", r)
	}

	// Unlinking prunes ccp's copies and links, never the hand-made skill.
	for _, item := range []struct {
		t    config.HubItemType
		name string
	}{{config.HubSkills, "good"}, {config.HubAgents, "helper.md"}} {
		if err := mgr.UnlinkHubItem("p", item.t, item.name); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(filepath.Join(paths.ProfileDir("p"), "CLAUDE.md"))
	p, _ = mgr.Get("p")
	r, err = AgySync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"skills/good", "agents/helper.md", "AGENTS.md"} {
		if _, err := os.Lstat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s not removed", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "mine", "SKILL.md")); err != nil {
		t.Error("hand-made skill removed")
	}
	if len(r.Removed) != 3 {
		t.Errorf("removed = %v", r.Removed)
	}
}

func TestAgySync_EditedAndOccupied(t *testing.T) {
	paths, mgr, dir := setupAgyTest(t)
	mustWrite(t, filepath.Join(dir, "agents", "helper.md"), "the user's own\n")
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "the user's own context\n")
	mustWrite(t, filepath.Join(dir, "skills", "good", "SKILL.md"), "---\ndescription: theirs\n---\n")
	p, _ := mgr.Get("p")

	r, err := AgySync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"agents/helper.md", "AGENTS.md", "skills/good"} {
		if !slices.Contains(r.Occupied, want) {
			t.Errorf("%s not reported occupied: %v", want, r.Occupied)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "agents", "helper.md")); string(data) != "the user's own\n" {
		t.Error("overwrote a file ccp did not write")
	}

	mustWrite(t, filepath.Join(dir, "rules", "scoped.md"), "edited by hand\n")
	if err := mgr.UnlinkHubItem("p", config.HubRules, "scoped.md"); err != nil {
		t.Fatal(err)
	}
	p, _ = mgr.Get("p")
	r, err = AgySync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.Edited, "rules/scoped.md") {
		t.Errorf("edited = %v", r.Edited)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "rules", "scoped.md")); string(data) != "edited by hand\n" {
		t.Error("removed a copy edited by hand")
	}
}

func TestAgySync_RefusesSymlinkedDirectories(t *testing.T) {
	paths, mgr, dir := setupAgyTest(t)
	elsewhere := t.TempDir()
	for _, sub := range []string{"agents", "skills"} {
		mustMkdir(t, filepath.Join(elsewhere, sub))
		mustMkdir(t, dir)
		if err := os.Symlink(filepath.Join(elsewhere, sub), filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := mgr.Get("p")
	r, err := AgySync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"agents", "skills"} {
		entries, _ := os.ReadDir(filepath.Join(elsewhere, sub))
		if len(entries) != 0 {
			t.Errorf("wrote through the %s link: %v", sub, entries)
		}
		if !slices.ContainsFunc(r.Notes, func(n string) bool { return strings.Contains(n, "remove the "+sub+" link first") }) {
			t.Errorf("%s refusal not reported: %v", sub, r.Notes)
		}
	}
	if !slices.Contains(r.Written, "rules/scoped.md") {
		t.Errorf("rules should still sync: %v", r.Written)
	}
}

func TestAgySync_ReportsOversizedRule(t *testing.T) {
	paths, mgr, dir := setupAgyTest(t)
	mustWrite(t, paths.HubItemPath(config.HubRules, "big.md"), strings.Repeat("x", agyRuleLimit+1))
	if err := mgr.LinkHubItem("p", config.HubRules, "big.md"); err != nil {
		t.Fatal(err)
	}
	p, _ := mgr.Get("p")
	r, err := AgySync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(r.Notes, func(n string) bool { return strings.HasPrefix(n, "rules/big.md:") }) {
		t.Errorf("oversized rule not reported: %v", r.Notes)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "big.md")); err != nil {
		t.Error("oversized rule should still be written")
	}
}

func TestAgyFollowProfile_OnlyAfterOptIn(t *testing.T) {
	paths, mgr, dir := setupAgyTest(t)
	p, _ := mgr.Get("p")
	if _, followed, err := AgyFollowProfile(paths, p); err != nil || followed {
		t.Fatalf("followed before opt-in: %v %v", followed, err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Error("follow wrote into agy before opt-in")
	}
	if _, err := AgySync(paths, p); err != nil {
		t.Fatal(err)
	}
	if _, followed, err := AgyFollowProfile(paths, p); err != nil || !followed {
		t.Fatalf("did not follow after opt-in: %v %v", followed, err)
	}
}

func TestAgySync_StyleGoesToGeminiMd(t *testing.T) {
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	t.Setenv("HOME", t.TempDir())
	r, err := AgySync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(AgyConfigDir(), "GEMINI.md"))
	if err != nil {
		t.Fatalf("GEMINI.md not written: %v (style lines %v)", err, r.Style)
	}
	if strings.HasPrefix(string(data), "---") || !strings.HasPrefix(string(data), generatedMarker) {
		t.Errorf("GEMINI.md must be the style body without frontmatter:\n%s", data)
	}
	owned, _ := OwnedStyleCarriers(paths, carrierGroupAgy)
	if len(owned) != 1 {
		t.Errorf("GEMINI.md not recorded in the carrier ledger: %v", owned)
	}
}
