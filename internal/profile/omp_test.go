package profile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/symlink"
)

// ompTestEnv builds a ccp layout plus a stand-in omp agent directory, and
// points omp's own environment resolution at it.
func ompTestEnv(t *testing.T) (*config.Paths, string) {
	t.Helper()

	root := t.TempDir()
	ccpDir := filepath.Join(root, "ccp")
	agentDir := filepath.Join(root, "omp", "agent")

	t.Setenv("PI_CODING_AGENT_DIR", agentDir)
	t.Setenv("PI_CONFIG_DIR", "")
	t.Setenv("HOME", root)
	os.Unsetenv("OMP_PROFILE")
	os.Unsetenv("PI_PROFILE")

	return &config.Paths{
		CcpDir:      ccpDir,
		HubDir:      filepath.Join(ccpDir, "hub"),
		ProfilesDir: filepath.Join(ccpDir, "profiles"),
		SharedDir:   filepath.Join(ccpDir, "profiles", "shared"),
	}, agentDir
}

func ompTestWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// ompTestSkill adds a hub skill and returns its directory.
func ompTestSkill(t *testing.T, hubDir, name string) string {
	t.Helper()
	dir := filepath.Join(hubDir, "skills", name)
	ompTestWrite(t, filepath.Join(dir, "SKILL.md"),
		"---\nname: "+name+"\ndescription: "+name+" skill\n---\n\n# "+name+"\n")
	return dir
}

// ompTestCommand adds a hub command and returns its path.
func ompTestCommand(t *testing.T, hubDir, name string) string {
	t.Helper()
	path := filepath.Join(hubDir, "commands", name+".md")
	ompTestWrite(t, path, "# "+name+"\n\n$ARGUMENTS\n")
	return path
}

// ompTestProfile gives a manifest a profile directory, so sync can read the
// profile-level files (CLAUDE.md, rules/) it mirrors into omp.
func ompTestProfile(t *testing.T, paths *config.Paths, manifest *Manifest) *Profile {
	t.Helper()
	dir := filepath.Join(paths.ProfilesDir, manifest.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return &Profile{Name: manifest.Name, Path: dir, Manifest: manifest}
}

func ompTestExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	return err == nil
}

// ompTestTarget resolves a link's target, requiring it to be relative so the
// link stays portable.
func ompTestTarget(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink(%s) error: %v", path, err)
	}
	if filepath.IsAbs(target) {
		t.Fatalf("link %s is absolute (%s), want relative", path, target)
	}
	return filepath.Join(filepath.Dir(path), target)
}

func ompTestForeignLink(t *testing.T, linkPath string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestOmpLinkLinksSkillsAndCommands(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	skillDir := ompTestSkill(t, paths.HubDir, "review")
	cmdPath := ompTestCommand(t, paths.HubDir, "ship")

	result, err := OmpLink(paths, []string{"skills/review", "commands/ship.md"})
	if err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}
	if want := []string{"skills/review", "commands/ship.md"}; !reflect.DeepEqual(result.Linked, want) {
		t.Errorf("Linked = %v, want %v", result.Linked, want)
	}
	if len(result.Skipped) != 0 || len(result.Notes) != 0 {
		t.Errorf("Skipped = %v, Ignored = %v, want both empty", result.Skipped, result.Notes)
	}

	// omp discovers exactly these two paths.
	if _, err := os.Stat(filepath.Join(agentDir, "skills", "review", "SKILL.md")); err != nil {
		t.Errorf("omp cannot read the linked skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentDir, "commands", "ship.md")); err != nil {
		t.Errorf("omp cannot read the linked command: %v", err)
	}

	if got := ompTestTarget(t, filepath.Join(agentDir, "skills", "review")); got != skillDir {
		t.Errorf("skill link target = %s, want %s", got, skillDir)
	}
	if got := ompTestTarget(t, filepath.Join(agentDir, "commands", "ship.md")); got != cmdPath {
		t.Errorf("command link target = %s, want %s", got, cmdPath)
	}

	again, err := OmpLink(paths, []string{"skills/review", "commands/ship.md"})
	if err != nil {
		t.Fatalf("second OmpLink() error: %v", err)
	}
	if len(again.Linked) != 0 {
		t.Errorf("Linked = %v on repeat, want nothing relinked", again.Linked)
	}
	if want := []string{"skills/review", "commands/ship.md"}; !reflect.DeepEqual(again.Skipped, want) {
		t.Errorf("Skipped = %v, want %v", again.Skipped, want)
	}
}

func TestOmpLinkRejectsUnsupportedRefs(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestWrite(t, filepath.Join(paths.HubDir, "settings-templates", "team", "settings.json"), "{}\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "skills", "broken", "notes.md"), "# no SKILL.md here\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "commands", "grouped", "inner.md"), "# nested\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "rules", "tone.md"), "# tone\n")

	tests := []struct {
		name    string
		ref     string
		wantErr string
	}{
		{"settings templates are not linkable", "settings-templates/team", "omp cannot link"},
		{"unknown type", "widgets/w", "omp cannot link"},
		{"malformed ref", "skills", "expected type/name"},
		{"missing hub item", "skills/nope", "hub item not found"},
		{"rules are carried by RULES.md", "rules/tone.md", "RULES.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := OmpLink(paths, []string{tt.ref})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("OmpLink(%q) error = %v, want containing %q", tt.ref, err, tt.wantErr)
			}
		})
	}

	// An item omp ignores is linked anyway, and reported: a wrong guess about
	// omp's loader must never decide whether the user's item exists in omp.
	t.Run("skill without SKILL.md is linked and reported", func(t *testing.T) {
		result, err := OmpLink(paths, []string{"skills/broken"})
		if err != nil {
			t.Fatalf("OmpLink() error: %v", err)
		}
		if want := []string{"skills/broken"}; !reflect.DeepEqual(result.Linked, want) {
			t.Errorf("Linked = %v, want %v", result.Linked, want)
		}
		if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], "SKILL.md") {
			t.Errorf("Notes = %v, want one SKILL.md reason", result.Notes)
		}
		if !ompTestExists(t, filepath.Join(agentDir, "skills", "broken")) {
			t.Error("a skill omp does not load was not linked")
		}
	})

	t.Run("command that is not a .md file is reported", func(t *testing.T) {
		result, err := OmpLink(paths, []string{"commands/grouped"})
		if err != nil {
			t.Fatalf("OmpLink() error: %v", err)
		}
		if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], ".md") {
			t.Errorf("Notes = %v, want one .md reason", result.Notes)
		}
	})
}

func TestOmpLinkLeavesForeignContentAlone(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "review")

	t.Run("real directory", func(t *testing.T) {
		ompTestWrite(t, filepath.Join(agentDir, "skills", "review", "SKILL.md"), "# mine\n")

		_, err := OmpLink(paths, []string{"skills/review"})
		if err == nil || !strings.Contains(err.Error(), "occupied by a file ccp did not create") {
			t.Fatalf("OmpLink() error = %v, want refusing to overwrite", err)
		}
		data, err := os.ReadFile(filepath.Join(agentDir, "skills", "review", "SKILL.md"))
		if err != nil || string(data) != "# mine\n" {
			t.Errorf("existing content = %q (err %v), want untouched", data, err)
		}
	})

	t.Run("foreign symlink", func(t *testing.T) {
		ompTestSkill(t, paths.HubDir, "other")
		ompTestForeignLink(t, filepath.Join(agentDir, "skills", "other"))

		_, err := OmpLink(paths, []string{"skills/other"})
		if err == nil || !strings.Contains(err.Error(), "occupied by a file ccp did not create") {
			t.Fatalf("OmpLink() error = %v, want refusing to overwrite", err)
		}
	})
}

func TestOmpUnlinkLeavesForeignContentAlone(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "review")
	if _, err := OmpLink(paths, []string{"skills/review"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}

	ompTestForeignLink(t, filepath.Join(agentDir, "skills", "other"))
	ompTestWrite(t, filepath.Join(agentDir, "commands", "handwritten.md"), "# mine\n")

	unlinked, err := OmpUnlink(paths, []string{
		"skills/review", "skills/other", "commands/handwritten.md", "commands/missing.md",
	})
	if err != nil {
		t.Fatalf("OmpUnlink() error: %v", err)
	}
	if want := []string{"skills/review"}; !reflect.DeepEqual(unlinked, want) {
		t.Errorf("Unlinked = %v, want %v", unlinked, want)
	}
	if ompTestExists(t, filepath.Join(agentDir, "skills", "review")) {
		t.Error("ccp link survived unlink")
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "other")) {
		t.Error("foreign symlink was removed")
	}
	data, err := os.ReadFile(filepath.Join(agentDir, "commands", "handwritten.md"))
	if err != nil || string(data) != "# mine\n" {
		t.Errorf("handwritten command = %q (err %v), want untouched", data, err)
	}
}

func TestOmpSyncMirrorsProfile(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "keep")
	ompTestSkill(t, paths.HubDir, "bundled")
	ompTestSkill(t, paths.HubDir, "stale")
	ompTestCommand(t, paths.HubDir, "ship")

	// Bundle "pack" holds skills/bundled inside the bundle directory.
	bundleDir := paths.BundleDir("pack")
	ompTestWrite(t, filepath.Join(bundleDir, hub.BundleManifestFile), "name: pack\nmembers:\n  skills:\n    - bundled\n")
	ompTestWrite(t, filepath.Join(bundleDir, "skills", "bundled", "SKILL.md"), "---\nname: bundled\ndescription: bundled skill\n---\n")

	// Pre-existing omp state: one ccp link the profile does not use, one foreign link.
	if _, err := OmpLink(paths, []string{"skills/stale"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}
	ompTestForeignLink(t, filepath.Join(agentDir, "skills", "other"))

	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"keep", "gone"}
	manifest.Hub.Commands = []string{"ship.md"}
	manifest.Hub.Bundles = []string{"pack"}

	result, err := OmpSync(paths, ompTestProfile(t, paths, manifest))
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if want := []string{"skills/bundled", "skills/keep", "commands/ship.md"}; !reflect.DeepEqual(result.Linked, want) {
		t.Errorf("Linked = %v, want %v", result.Linked, want)
	}
	if want := []string{"skills/stale"}; !reflect.DeepEqual(result.Removed, want) {
		t.Errorf("Removed = %v, want %v", result.Removed, want)
	}
	if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], "skills/gone") {
		t.Errorf("Notes = %v, want skills/gone reported", result.Notes)
	}

	// A bundle member is linked from inside the bundle, not the hub leaf dir.
	if got := ompTestTarget(t, filepath.Join(agentDir, "skills", "bundled")); got != filepath.Join(bundleDir, "skills", "bundled") {
		t.Errorf("bundle member link target = %s, want the bundle's copy", got)
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "other")) {
		t.Error("foreign symlink was removed")
	}

	again, err := OmpSync(paths, ompTestProfile(t, paths, manifest))
	if err != nil {
		t.Fatalf("second OmpSync() error: %v", err)
	}
	if len(again.Linked) != 0 || len(again.Removed) != 0 {
		t.Errorf("Linked = %v, Removed = %v on repeat, want no changes", again.Linked, again.Removed)
	}
	if want := []string{"skills/bundled", "skills/keep", "commands/ship.md"}; !reflect.DeepEqual(again.Skipped, want) {
		t.Errorf("Skipped = %v, want %v", again.Skipped, want)
	}
}

func TestOmpListReportsHubLinksOnly(t *testing.T) {
	paths, agentDir := ompTestEnv(t)

	items, err := OmpList(paths)
	if err != nil {
		t.Fatalf("OmpList() error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("OmpList() = %v, want empty before any link", items)
	}

	ompTestSkill(t, paths.HubDir, "review")
	ompTestCommand(t, paths.HubDir, "ship")
	if _, err := OmpLink(paths, []string{"skills/review", "commands/ship.md"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}
	ompTestForeignLink(t, filepath.Join(agentDir, "skills", "other"))
	ompTestWrite(t, filepath.Join(agentDir, "commands", "handwritten.md"), "# mine\n")

	items, err = OmpList(paths)
	if err != nil {
		t.Fatalf("OmpList() error: %v", err)
	}
	if want := []string{"commands/ship.md", "skills/review"}; !reflect.DeepEqual(items, want) {
		t.Errorf("OmpList() = %v, want %v", items, want)
	}
}

func TestOmpAgentDirResolution(t *testing.T) {
	strptr := func(s string) *string { return &s }

	tests := []struct {
		name       string
		agentDir   string
		configDir  string
		home       string
		ompProfile *string // nil means unset
		piProfile  *string // nil means unset
		want       string
	}{
		{"agent dir override wins", "/custom/agent", "", "/home/u", nil, nil, "/custom/agent"},
		{"config dir renames the root", "", ".omp-dev", "/home/u", nil, nil, "/home/u/.omp-dev/agent"},
		{"default root", "", "", "/home/u", nil, nil, "/home/u/.omp/agent"},
		{"named profile relocates the base", "", "", "/home/u", strptr("work"), nil, "/home/u/.omp/profiles/work/agent"},
		{"named profile ignores agent dir override", "/custom/agent", "", "/home/u", strptr("work"), nil, "/home/u/.omp/profiles/work/agent"},
		{"pi profile is the fallback", "", "", "/home/u", nil, strptr("legacy"), "/home/u/.omp/profiles/legacy/agent"},
		{"defined-but-empty omp profile selects default", "/custom/agent", "", "/home/u", strptr(""), strptr("legacy"), "/custom/agent"},
		{"default profile name", "", "", "/home/u", strptr("default"), nil, "/home/u/.omp/agent"},
		{"whitespace profile selects default", "", "", "/home/u", strptr("  "), nil, "/home/u/.omp/agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PI_CODING_AGENT_DIR", tt.agentDir)
			t.Setenv("PI_CONFIG_DIR", tt.configDir)
			t.Setenv("HOME", tt.home)
			setOmpTestEnv(t, "OMP_PROFILE", tt.ompProfile)
			setOmpTestEnv(t, "PI_PROFILE", tt.piProfile)

			if got := OmpAgentDir(); got != tt.want {
				t.Errorf("OmpAgentDir() = %s, want %s", got, tt.want)
			}
		})
	}
}

// setOmpTestEnv sets or clears an environment variable for the test.
func setOmpTestEnv(t *testing.T, key string, value *string) {
	t.Helper()
	if value == nil {
		os.Unsetenv(key)
		return
	}
	t.Setenv(key, *value)
}

func TestOmpLinkMirrorsLinkableItems(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestWrite(t, filepath.Join(paths.HubDir, "skills", "pack", "SKILL.md"),
		"---\nname: pack\ndescription: d\n---\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: d\n---\n\nReview things.\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "rules", "group", "tone.md"),
		"---\nalwaysApply: true\n---\n\n# tone\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "commands", "ship.md"), "# ship\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "hooks", "guard", "hooks.json"), "{\"hooks\":{}}\n")

	// hooks are not a link type at all: omp imports modules from hooks/pre and
	// hooks/post, so ccp refuses rather than managing that directory.
	if _, err := OmpLink(paths, []string{"hooks/guard"}); err == nil {
		t.Error("linking a hook was accepted, want a refusal")
	}
	// Rules are not a link type either: omp surfaces a rule only when its
	// frontmatter routes it somewhere, which ccp cannot predict, so every
	// profile rule travels in RULES.md instead.
	if _, err := OmpLink(paths, []string{"rules/group/tone.md"}); err == nil {
		t.Error("linking a rule was accepted, want a refusal")
	}

	refs := []string{"skills/pack", "agents/reviewer.md", "commands/ship.md"}
	result, err := OmpLink(paths, refs)
	if err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}
	if len(result.Linked) != len(refs) {
		t.Fatalf("Linked = %v, want all %d refs", result.Linked, len(refs))
	}
	if len(result.Notes) != 0 {
		t.Errorf("Notes = %v, want nothing", result.Notes)
	}

	// Every linkable type lands one level down, in the directory omp reads.
	for _, want := range []struct{ rel, target string }{
		{"skills/pack", filepath.Join(paths.HubDir, "skills", "pack")},
		{"agents/reviewer.md", filepath.Join(paths.HubDir, "agents", "reviewer.md")},
		{"commands/ship.md", filepath.Join(paths.HubDir, "commands", "ship.md")},
	} {
		linkPath := filepath.Join(agentDir, want.rel)
		if !ompTestExists(t, linkPath) {
			t.Errorf("%s not linked", want.rel)
			continue
		}
		if got := ompTestTarget(t, linkPath); got != want.target {
			t.Errorf("%s → %s, want %s", want.rel, got, want.target)
		}
	}
	if ompTestExists(t, filepath.Join(agentDir, "hooks", "guard")) {
		t.Error("linked a hook omp can never import")
	}
	if ompTestExists(t, filepath.Join(agentDir, "rules")) {
		t.Error("linked a rule, which would inject it a second time when omp already surfaces it")
	}

	items, err := OmpList(paths)
	if err != nil {
		t.Fatalf("OmpList() error: %v", err)
	}
	want := []string{"agents/reviewer.md", "commands/ship.md", "skills/pack"}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("OmpList() = %v, want %v", items, want)
	}
}

func TestOmpStatusReportsDiff(t *testing.T) {
	paths, _ := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "keep")
	ompTestSkill(t, paths.HubDir, "extra")
	ompTestWrite(t, filepath.Join(paths.HubDir, "skills", "unloadable", "notes.md"), "# no SKILL.md\n")
	ompTestCommand(t, paths.HubDir, "ship")

	if _, err := OmpLink(paths, []string{"skills/extra", "commands/ship.md"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}

	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"keep", "unloadable", "ghost"}
	manifest.Hub.Commands = []string{"ship.md"}

	diff, err := OmpStatus(paths, manifest)
	if err != nil {
		t.Fatalf("OmpStatus() error: %v", err)
	}
	// A skill omp ignores is still linked by sync, so it is missing until then;
	// an item that is not in the hub at all cannot be linked, so it is not.
	if want := []string{"skills/keep", "skills/unloadable"}; !reflect.DeepEqual(diff.Missing, want) {
		t.Errorf("Missing = %v, want %v (a hub gap is not 'missing')", diff.Missing, want)
	}
	if want := []string{"skills/extra"}; !reflect.DeepEqual(diff.Stale, want) {
		t.Errorf("Stale = %v, want %v", diff.Stale, want)
	}
	if len(diff.Ignored) != 1 || !strings.Contains(diff.Ignored[0], "skills/unloadable") {
		t.Errorf("Ignored = %v, want skills/unloadable reported with its reason", diff.Ignored)
	}
	if diff.Empty() {
		t.Error("Empty() = true, want false")
	}

	if _, err := OmpSync(paths, ompTestProfile(t, paths, manifest)); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}

	diff, err = OmpStatus(paths, manifest)
	if err != nil {
		t.Fatalf("OmpStatus() after sync error: %v", err)
	}
	if !diff.Empty() {
		t.Errorf("after sync Missing = %v, Stale = %v, want in sync", diff.Missing, diff.Stale)
	}
}

func TestOmpBrokenReportsAndPrunes(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "gone")
	if _, err := OmpLink(paths, []string{"skills/gone"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}

	// The hub item disappears (deleted, or never pulled on this machine).
	if err := os.RemoveAll(filepath.Join(paths.HubDir, "skills", "gone")); err != nil {
		t.Fatal(err)
	}
	// A broken symlink ccp did not create must be left alone.
	foreign := filepath.Join(agentDir, "skills", "foreign")
	if err := os.Symlink(filepath.Join(t.TempDir(), "nope"), foreign); err != nil {
		t.Fatal(err)
	}

	broken, err := OmpBroken(paths)
	if err != nil {
		t.Fatalf("OmpBroken() error: %v", err)
	}
	if want := []string{"skills/gone"}; !reflect.DeepEqual(broken, want) {
		t.Errorf("OmpBroken() = %v, want %v", broken, want)
	}

	pruned, err := OmpPruneBroken(paths)
	if err != nil {
		t.Fatalf("OmpPruneBroken() error: %v", err)
	}
	if want := []string{"skills/gone"}; !reflect.DeepEqual(pruned, want) {
		t.Errorf("OmpPruneBroken() = %v, want %v", pruned, want)
	}
	if ompTestExists(t, filepath.Join(agentDir, "skills", "gone")) {
		t.Error("broken ccp link survived pruning")
	}
	if !ompTestExists(t, foreign) {
		t.Error("broken foreign symlink was removed")
	}
}

func TestOmpFollowProfileRetiresPreviousProfile(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "shared")
	ompTestSkill(t, paths.HubDir, "work-only")
	ompTestSkill(t, paths.HubDir, "by-hand")

	work := NewManifest("work", "")
	work.Hub.Skills = []string{"shared", "work-only"}
	personal := NewManifest("personal", "")
	personal.Hub.Skills = []string{"shared"}

	// Nothing linked yet: a switch must not create links on its own.
	result, err := OmpFollowProfile(paths, nil, ompTestProfile(t, paths, personal))
	if err != nil {
		t.Fatalf("OmpFollowProfile() error: %v", err)
	}
	if len(result.Linked) != 0 || len(result.Skipped) != 0 || len(result.Removed) != 0 {
		t.Errorf("OmpFollowProfile() = %+v, want no changes without an opt-in", result)
	}
	if ompTestExists(t, filepath.Join(agentDir, "skills", "shared")) {
		t.Error("following created a link before the user opted in")
	}

	// Opt in, then add a link no profile asked for.
	if _, err := OmpSync(paths, ompTestProfile(t, paths, work)); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if _, err := OmpLink(paths, []string{"skills/by-hand"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}

	// Switching to personal retires work's items — and only those: a link no
	// profile asked for survives, reported as left behind.
	result, err = OmpFollowProfile(paths, ompTestProfile(t, paths, work), ompTestProfile(t, paths, personal))
	if err != nil {
		t.Fatalf("OmpFollowProfile() error: %v", err)
	}
	if want := []string{"skills/work-only"}; !reflect.DeepEqual(result.Removed, want) {
		t.Errorf("Removed = %v, want %v (the previous profile's item)", result.Removed, want)
	}
	if want := []string{"skills/by-hand"}; !reflect.DeepEqual(result.LeftBehind, want) {
		t.Errorf("LeftBehind = %v, want %v (a link no profile asked for survives)", result.LeftBehind, want)
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "by-hand")) {
		t.Error("a link no profile asked for was pruned by a switch")
	}
	if ompTestExists(t, filepath.Join(agentDir, "skills", "work-only")) {
		t.Error("the previous profile's item survived the switch")
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "shared")) {
		t.Error("an item both profiles want was pruned")
	}

	// A link outside both profiles is reported, not deleted.
	if _, err := OmpLink(paths, []string{"skills/by-hand"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}
	result, err = OmpFollowProfile(paths, ompTestProfile(t, paths, personal), ompTestProfile(t, paths, work))
	if err != nil {
		t.Fatalf("OmpFollowProfile() error: %v", err)
	}
	if want := []string{"skills/by-hand"}; !reflect.DeepEqual(result.LeftBehind, want) {
		t.Errorf("LeftBehind = %v, want %v (reported, not deleted)", result.LeftBehind, want)
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "by-hand")) {
		t.Error("a by-hand link was pruned by a profile switch")
	}

	// The explicit sync is the reconciling command.
	if _, err := OmpSync(paths, ompTestProfile(t, paths, work)); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if ompTestExists(t, filepath.Join(agentDir, "skills", "by-hand")) {
		t.Error("explicit sync did not prune the stale link")
	}
}

func TestOmpSyncCarriesRulesDespiteOccupiedPath(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestWrite(t, filepath.Join(paths.HubDir, "rules", "codegraph.md"), "# hub codegraph\n\nHUB-RULE-BODY\n")
	ompTestSkill(t, paths.HubDir, "keep")

	// The user's own file occupies the name under omp's rules directory. ccp does
	// not link rules at all, so the profile's rule must still reach the model
	// through RULES.md, and the user's file must be left exactly as it was.
	occupied := filepath.Join(agentDir, "rules", "codegraph.md")
	ompTestWrite(t, occupied, "# mine\n")

	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"keep"}
	manifest.Hub.Rules = []string{"codegraph.md"}
	p := ompTestProfile(t, paths, manifest)
	ompTestWrite(t, filepath.Join(p.Path, "rules", "codegraph.md"), "# hub codegraph\n\nHUB-RULE-BODY\n")

	result, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if want := []string{"skills/keep"}; !reflect.DeepEqual(result.Linked, want) {
		t.Errorf("Linked = %v, want %v", result.Linked, want)
	}
	if len(result.Notes) != 0 {
		t.Errorf("Notes = %v, want nothing: an occupied rules/ path is not ccp's business", result.Notes)
	}
	data, err := os.ReadFile(occupied)
	if err != nil || string(data) != "# mine\n" {
		t.Errorf("occupied file = %q (err %v), want untouched", data, err)
	}
	rules, err := os.ReadFile(filepath.Join(agentDir, "RULES.md"))
	if err != nil || !strings.Contains(string(rules), "HUB-RULE-BODY") {
		t.Errorf("RULES.md = %q (err %v), want the profile's rule text", rules, err)
	}

	diff, err := OmpStatus(paths, manifest)
	if err != nil {
		t.Fatalf("OmpStatus() error: %v", err)
	}
	if !diff.Empty() {
		t.Errorf("OmpStatus() = %+v, want empty", diff)
	}
}

func TestOmpSyncSetsUpContextFiles(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	manifest := NewManifest("dev", "")
	p := ompTestProfile(t, paths, manifest)

	ompTestWrite(t, filepath.Join(p.Path, "CLAUDE.md"), "# global instructions\n")
	ompTestWrite(t, filepath.Join(p.Path, "rules", "tone.md"), "# Tone\nBe terse.\n")
	ompTestWrite(t, filepath.Join(p.Path, "rules", "flow.md"), "# Flow\nPlan first.\n")

	result, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if len(result.Context) != 3 {
		t.Errorf("Context = %v, want the created agent dir, AGENTS.md and RULES.md reported", result.Context)
	}

	// AGENTS.md is omp's user context file, pointing at the profile's CLAUDE.md.
	agentsPath := filepath.Join(agentDir, "AGENTS.md")
	if got := ompTestTarget(t, agentsPath); got != filepath.Join(p.Path, "CLAUDE.md") {
		t.Errorf("AGENTS.md → %s, want the profile's CLAUDE.md", got)
	}

	// RULES.md is generated: omp injects it into every session.
	rulesPath := filepath.Join(agentDir, "RULES.md")
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("RULES.md not written: %v", err)
	}
	content := string(data)
	for _, want := range []string{"# Tone\nBe terse.", "# Flow\nPlan first.", ompRulesMarker} {
		if !strings.Contains(content, want) {
			t.Errorf("RULES.md missing %q; got:\n%s", want, content)
		}
	}

	// A second run changes nothing.
	again, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("second OmpSync() error: %v", err)
	}
	if len(again.Context) != 0 {
		t.Errorf("Context = %v on repeat, want no changes", again.Context)
	}

	// Editing a rule regenerates; dropping every rule removes ccp's file.
	ompTestWrite(t, filepath.Join(p.Path, "rules", "tone.md"), "# Tone\nBe very terse.\n")
	edited, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() after edit error: %v", err)
	}
	if len(edited.Context) != 1 || !strings.Contains(edited.Context[0], "RULES.md") {
		t.Errorf("Context = %v, want RULES.md regenerated", edited.Context)
	}
	if data, _ := os.ReadFile(rulesPath); !strings.Contains(string(data), "very terse") {
		t.Error("RULES.md was not regenerated after the rule changed")
	}

	if err := os.Remove(filepath.Join(p.Path, "rules", "tone.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(p.Path, "rules", "flow.md")); err != nil {
		t.Fatal(err)
	}
	emptied, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() after emptying rules error: %v", err)
	}
	if len(emptied.Context) != 1 || !strings.Contains(emptied.Context[0], "no rules") {
		t.Errorf("Context = %v, want RULES.md removed", emptied.Context)
	}
	if ompTestExists(t, rulesPath) {
		t.Error("RULES.md survived an empty rules directory")
	}

	// Losing CLAUDE.md drops the AGENTS.md link ccp made.
	if err := os.Remove(filepath.Join(p.Path, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() after removing CLAUDE.md error: %v", err)
	}
	if ompTestExists(t, agentsPath) {
		t.Error("AGENTS.md link survived the profile losing its CLAUDE.md")
	}
}

func TestOmpSyncKeepsForeignContextFiles(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	manifest := NewManifest("dev", "")
	p := ompTestProfile(t, paths, manifest)
	ompTestWrite(t, filepath.Join(p.Path, "CLAUDE.md"), "# global instructions\n")
	ompTestWrite(t, filepath.Join(p.Path, "rules", "tone.md"), "# Tone\n")

	agentsPath := filepath.Join(agentDir, "AGENTS.md")
	rulesPath := filepath.Join(agentDir, "RULES.md")
	ompTestWrite(t, agentsPath, "# my own agents file\n")
	ompTestWrite(t, rulesPath, "# my own rules\n")

	result, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if len(result.Context) != 2 {
		t.Errorf("Context = %v, want both collisions reported", result.Context)
	}
	for path, want := range map[string]string{
		agentsPath: "# my own agents file\n",
		rulesPath:  "# my own rules\n",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Errorf("%s = %q (err %v), want %q untouched", path, data, err, want)
		}
	}
}

func TestOmpContextStatusTracksDrift(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	manifest := NewManifest("dev", "")
	p := ompTestProfile(t, paths, manifest)
	ompTestWrite(t, filepath.Join(p.Path, "CLAUDE.md"), "# global instructions\n")
	ompTestWrite(t, filepath.Join(p.Path, "rules", "tone.md"), "# Tone\nBe terse.\n")

	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}

	report, err := OmpContextStatus(paths, p)
	if err != nil {
		t.Fatalf("OmpContextStatus() error: %v", err)
	}
	if report.OutOfDate() {
		t.Errorf("OutOfDate() = true right after sync: %+v", report)
	}
	if len(report.Current) != 2 {
		t.Errorf("Current = %v, want AGENTS.md and RULES.md", report.Current)
	}

	// A rule edit is drift, even without another sync.
	ompTestWrite(t, filepath.Join(p.Path, "rules", "tone.md"), "# Tone\nBe very terse.\n")
	report, err = OmpContextStatus(paths, p)
	if err != nil {
		t.Fatalf("OmpContextStatus() error: %v", err)
	}
	if !report.OutOfDate() || !strings.Contains(strings.Join(report.Stale, " "), "RULES.md") {
		t.Errorf("Stale = %v, want RULES.md outdated", report.Stale)
	}

	// A deleted RULES.md is drift too, and sync repairs it.
	if err := os.Remove(filepath.Join(agentDir, "RULES.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if report, _ := OmpContextStatus(paths, p); report.OutOfDate() {
		t.Errorf("Stale = %v after sync, want up to date", report.Stale)
	}

	// A file ccp did not generate is occupied, not drift: sync cannot fix it.
	ompTestWrite(t, filepath.Join(agentDir, "RULES.md"), "# mine\n")
	report, err = OmpContextStatus(paths, p)
	if err != nil {
		t.Fatalf("OmpContextStatus() error: %v", err)
	}
	if report.OutOfDate() {
		t.Error("OutOfDate() = true for an occupied path, want false")
	}
	if len(report.Occupied) != 1 || report.Occupied[0] != "RULES.md" {
		t.Errorf("Occupied = %v, want [RULES.md]", report.Occupied)
	}
}

func TestOmpTeardownRemovesOnlyCcpPaths(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	manifest := NewManifest("dev", "")
	p := ompTestProfile(t, paths, manifest)
	ompTestWrite(t, filepath.Join(p.Path, "CLAUDE.md"), "# global instructions\n")
	ompTestWrite(t, filepath.Join(p.Path, "rules", "tone.md"), "# Tone\n")
	ompTestSkill(t, paths.HubDir, "pack")
	manifest.Hub.Skills = []string{"pack"}

	// ccp's own content, plus content it must not touch.
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	foreignFile := filepath.Join(agentDir, "skills", "handwritten")
	ompTestWrite(t, foreignFile, "# mine\n")
	foreignLink := filepath.Join(agentDir, "rules", "other.md")
	ompTestForeignLink(t, foreignLink)
	foreignAgents := filepath.Join(agentDir, "AGENTS.md")

	managed, err := OmpManaged(paths)
	if err != nil {
		t.Fatalf("OmpManaged() error: %v", err)
	}
	want := []string{
		filepath.Join(agentDir, "AGENTS.md"),
		filepath.Join(agentDir, "RULES.md"),
		filepath.Join(agentDir, "skills", "pack"),
	}
	if !reflect.DeepEqual(managed, want) {
		t.Errorf("OmpManaged() = %v, want %v", managed, want)
	}

	if _, err := OmpTeardown(paths); err != nil {
		t.Fatalf("OmpTeardown() error: %v", err)
	}
	for _, path := range want {
		if ompTestExists(t, path) {
			t.Errorf("%s survived teardown", path)
		}
	}
	if !ompTestExists(t, foreignFile) {
		t.Error("a file ccp did not create was removed")
	}
	if !ompTestExists(t, foreignLink) {
		t.Error("a foreign symlink was removed")
	}
	if ompTestExists(t, foreignAgents) {
		t.Error("AGENTS.md survived teardown")
	}

	// Teardown is idempotent.
	again, err := OmpTeardown(paths)
	if err != nil {
		t.Fatalf("second OmpTeardown() error: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second OmpTeardown() = %v, want nothing left to remove", again)
	}
}

func TestOmpSyncLinksItemsOmpIgnores(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestWrite(t, filepath.Join(paths.HubDir, "skills", "nodesc", "SKILL.md"), "---\nname: nodesc\n---\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "agents", "inherit.md"), "---\nname: inherit\ndescription: d\nmodel: inherit\n---\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "rules", "plain.md"), "# plain\n")
	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"nodesc"}
	manifest.Hub.Agents = []string{"inherit.md"}
	manifest.Hub.Rules = []string{"plain.md"}
	p := ompTestProfile(t, paths, manifest)

	// A rule link from an older ccp: it would inject the rule twice, once from
	// the link and once from RULES.md, so sync retires it.
	if err := symlink.New().Create(filepath.Join(agentDir, "rules", "plain.md"), filepath.Join(paths.HubDir, "rules", "plain.md")); err != nil {
		t.Fatal(err)
	}
	// RULES.md is generated from the profile's rules directory, where a real
	// profile holds a link per item.
	ompTestWrite(t, filepath.Join(p.Path, "rules", "plain.md"), "# plain\n")

	result, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if want := []string{"skills/nodesc", "agents/inherit.md"}; !reflect.DeepEqual(result.Linked, want) {
		t.Errorf("Linked = %v, want %v", result.Linked, want)
	}
	if len(result.Notes) != 2 {
		t.Errorf("Notes = %v, want the two ignored items reported with a reason", result.Notes)
	}
	for _, want := range []string{"description", "model: inherit"} {
		if !strings.Contains(strings.Join(result.Notes, "\n"), want) {
			t.Errorf("Notes = %v, want a reason mentioning %q", result.Notes, want)
		}
	}
	if want := []string{"rules/plain.md (rules travel in RULES.md now)"}; !reflect.DeepEqual(result.Removed, want) {
		t.Errorf("Removed = %v, want the leftover rule link retired", result.Removed)
	}

	// The items omp ignores are in place; the rule reaches the model through
	// RULES.md instead of a link.
	for _, rel := range []string{"skills/nodesc", "agents/inherit.md"} {
		if !ompTestExists(t, filepath.Join(agentDir, rel)) {
			t.Errorf("%s was not linked", rel)
		}
	}
	if ompTestExists(t, filepath.Join(agentDir, "rules", "plain.md")) {
		t.Error("the leftover rule link survived sync")
	}
	rules, err := os.ReadFile(filepath.Join(agentDir, "RULES.md"))
	if err != nil || !strings.Contains(string(rules), "# plain") {
		t.Errorf("RULES.md = %q (err %v), want the profile's rule text", rules, err)
	}
}

func TestOmpFollowTriggersOnContextOnlySetup(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestWrite(t, filepath.Join(paths.HubDir, "rules", "plain.md"), "# plain\n")
	ompTestSkill(t, paths.HubDir, "pack")

	// First profile has nothing omp can link as an item: its only trace is the
	// context setup, which the opt-in check has to notice.
	first := NewManifest("first", "")
	first.Hub.Rules = []string{"plain.md"}
	p1 := ompTestProfile(t, paths, first)
	ompTestWrite(t, filepath.Join(p1.Path, "CLAUDE.md"), "# global instructions\n")
	if _, err := OmpSync(paths, p1); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if linked, _ := OmpList(paths); len(linked) != 0 {
		t.Fatalf("OmpList() = %v, want no type-directory links", linked)
	}

	// Switching to a profile with a loadable item must link it. Gating the
	// opt-in on type-directory links alone would silently do nothing here.
	second := NewManifest("second", "")
	second.Hub.Skills = []string{"pack"}
	result, err := OmpFollowProfile(paths, ompTestProfile(t, paths, first), ompTestProfile(t, paths, second))
	if err != nil {
		t.Fatalf("OmpFollowProfile() error: %v", err)
	}
	if want := []string{"skills/pack"}; !reflect.DeepEqual(result.Linked, want) {
		t.Errorf("Linked = %v, want %v", result.Linked, want)
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "pack")) {
		t.Error("follow did not link the new profile's item")
	}
}

func TestOmpSyncLinksProseFrontmatterAgents(t *testing.T) {
	paths, agentDir := ompTestEnv(t)

	// Real hub agents carry long prose descriptions whose colons break YAML
	// (`Examples:\n...`). omp falls back to line parsing for these, so ccp must
	// read them as present instead of reporting "no description".
	ompTestWrite(t, filepath.Join(paths.HubDir, "agents", "prose.md"),
		"---\nname: prose\ndescription: Use this agent when the user asks:\n\n<example>\nContext: x\n</example>\n---\n\nBody.\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "skills", "quoted", "SKILL.md"),
		"---\nname: quoted\ndescription: \"A skill: with a colon\"\n---\n")

	manifest := NewManifest("dev", "")
	manifest.Hub.Agents = []string{"prose.md"}
	manifest.Hub.Skills = []string{"quoted"}

	result, err := OmpSync(paths, ompTestProfile(t, paths, manifest))
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if len(result.Notes) != 0 {
		t.Errorf("Ignored = %v, want nothing: both items do carry a description", result.Notes)
	}
	for _, rel := range []string{"agents/prose.md", "skills/quoted"} {
		if !ompTestExists(t, filepath.Join(agentDir, rel)) {
			t.Errorf("%s was not linked", rel)
		}
	}

	// A frontmatter fence inside an indented block must not truncate the block.
	ompTestWrite(t, filepath.Join(paths.HubDir, "agents", "fenced.md"),
		"---\nname: fenced\ndescription: |\n  Has a fence:\n  ---\n  still the same value\nmodel: haiku\n---\n")
	p := ompTestProfile(t, paths, manifest)
	p.Manifest.Hub.Agents = []string{"fenced.md"}
	if result, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	} else if len(result.Notes) != 0 {
		t.Errorf("Ignored = %v, want nothing for a block scalar containing a fence", result.Notes)
	}

	// Real hub skills arrive with CRLF line endings; "\r" must not hide the fence.
	ompTestWrite(t, filepath.Join(paths.HubDir, "skills", "crlf", "SKILL.md"),
		"---\r\nname: crlf\r\ndescription: A CRLF skill\r\n---\r\n\r\n# crlf\r\n")
	p2 := ompTestProfile(t, paths, manifest)
	p2.Manifest.Hub.Skills = []string{"crlf"}
	if result, err := OmpSync(paths, p2); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	} else if len(result.Notes) != 0 {
		t.Errorf("Ignored = %v, want nothing for a CRLF skill with a description", result.Notes)
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "crlf")) {
		t.Error("a CRLF skill with a description was not linked")
	}
}

func TestOmpSyncIgnoresBundleCopiesOfLinkedItems(t *testing.T) {
	paths, _ := ompTestEnv(t)
	skillDir := ompTestSkill(t, paths.HubDir, "pack")

	// A bundle carries a copy of a hub item the profile also links directly.
	bundleDir := paths.BundleDir("pack-bundle")
	ompTestWrite(t, filepath.Join(bundleDir, hub.BundleManifestFile), "name: pack-bundle\nmembers:\n  skills:\n    - pack\n")
	ompTestWrite(t, filepath.Join(bundleDir, "skills", "pack", "SKILL.md"), "---\nname: pack\ndescription: d\n---\n")

	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"pack"}
	manifest.Hub.Bundles = []string{"pack-bundle"}

	result, err := OmpSync(paths, ompTestProfile(t, paths, manifest))
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if len(result.Notes) != 0 {
		t.Errorf("Ignored = %v, want nothing: a bundle copy of a linked item is not a collision", result.Notes)
	}
	// The direct hub item wins, so the link does not depend on the bundle copy.
	linkPath := filepath.Join(OmpAgentDir(), "skills", "pack")
	if got := ompTestTarget(t, linkPath); got != skillDir {
		t.Errorf("skills/pack → %s, want the hub item %s", got, skillDir)
	}
}

func TestOmpSyncRepairsReroutedLink(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "wanted")
	ompTestWrite(t, filepath.Join(paths.HubDir, "skills", "other", "SKILL.md"),
		"---\nname: other\ndescription: d\n---\n")

	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"wanted"}
	p := ompTestProfile(t, paths, manifest)
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}

	// Somebody re-points the link at a different hub item.
	linkPath := filepath.Join(agentDir, "skills", "wanted")
	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}
	if err := ompCreateLink(linkPath, filepath.Join(paths.HubDir, "skills", "other")); err != nil {
		t.Fatal(err)
	}

	// Status must see it — a link that exists is not a link that is right.
	diff, err := OmpStatus(paths, manifest)
	if err != nil {
		t.Fatalf("OmpStatus() error: %v", err)
	}
	if want := []string{"skills/wanted"}; !reflect.DeepEqual(diff.Stale, want) {
		t.Errorf("Stale = %v, want %v for a re-pointed link", diff.Stale, want)
	}

	// And sync must fix it.
	result, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if want := []string{"skills/wanted"}; !reflect.DeepEqual(result.Linked, want) {
		t.Errorf("Linked = %v, want %v", result.Linked, want)
	}
	if got := ompTestTarget(t, linkPath); !strings.HasSuffix(got, filepath.Join("hub", "skills", "wanted")) {
		t.Errorf("link still points at %s", got)
	}
}

func TestOmpSyncKeepsLinksItCannotRead(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	skillDir := ompTestSkill(t, paths.HubDir, "unreadable")

	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"unreadable"}
	p := ompTestProfile(t, paths, manifest)
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}

	// A transient read failure is not evidence the link is dead.
	if err := os.Chmod(filepath.Join(skillDir, "SKILL.md"), 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(skillDir, "SKILL.md"), 0644)

	result, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "unreadable")) {
		t.Error("sync removed a link because the hub item could not be read")
	}
	if len(result.Notes) == 0 {
		t.Error("Notes is empty, want the unreadable item reported")
	}
}

func TestOmpSyncWorksUnderSymlinkedConfigRoot(t *testing.T) {
	paths, _ := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "seed")

	// A dotfiles-managed omp root whose symlink points at a *deeper* directory:
	// the path the link is written to goes through a symlink, so a relative
	// target computed from the unresolved path lands at a depth that exists only
	// on paper — and ccp would disown the links it wrote itself.
	real := t.TempDir()
	deep := filepath.Join(real, "dotfiles", "config", "omp")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(real, "omp-link")
	if err := os.Symlink(deep, linked); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(linked, "agent"))

	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"seed"}
	p := ompTestProfile(t, paths, manifest)
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}

	linkPath := filepath.Join(linked, "agent", "skills", "seed")
	if _, err := os.Stat(filepath.Join(linkPath, "SKILL.md")); err != nil {
		t.Errorf("link does not resolve through the symlinked root: %v", err)
	}

	// ccp must recognize the link as its own: list it, claim it, and leave it
	// alone on the next sync instead of calling the path occupied.
	items, err := OmpList(paths)
	if err != nil {
		t.Fatalf("OmpList() error: %v", err)
	}
	if want := []string{"skills/seed"}; !reflect.DeepEqual(items, want) {
		t.Errorf("OmpList() = %v, want %v (ccp disowned its own link)", items, want)
	}
	managed, err := OmpManaged(paths)
	if err != nil {
		t.Fatalf("OmpManaged() error: %v", err)
	}
	if want := []string{linkPath}; !reflect.DeepEqual(managed, want) {
		t.Errorf("OmpManaged() = %v, want %v", managed, want)
	}

	again, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("second OmpSync() error: %v", err)
	}
	if len(again.Linked) != 0 || len(again.Notes) != 0 {
		t.Errorf("Linked = %v, Notes = %v on repeat, want no changes", again.Linked, again.Notes)
	}
	if want := []string{"skills/seed"}; !reflect.DeepEqual(again.Skipped, want) {
		t.Errorf("Skipped = %v, want %v", again.Skipped, want)
	}

	// Status must not call a working link broken, and must not want to touch it.
	diff, err := OmpStatus(paths, manifest)
	if err != nil {
		t.Fatalf("OmpStatus() error: %v", err)
	}
	if !diff.Empty() {
		t.Errorf("OmpStatus() = %+v, want clean", diff)
	}
}

func TestOmpManagedCoversNamedProfiles(t *testing.T) {
	paths, _ := ompTestEnv(t)
	base := ompConfigRootForTest(t)
	ompTestSkill(t, paths.HubDir, "seed")

	// Default profile plus a named one: both are ccp's to clean up.
	defaultAgent := filepath.Join(base, "agent")
	namedAgent := filepath.Join(base, "profiles", "work", "agent")
	t.Setenv("PI_CODING_AGENT_DIR", defaultAgent)
	t.Setenv("OMP_PROFILE", "work")
	if _, err := OmpLink(paths, []string{"skills/seed"}); err != nil {
		t.Fatalf("OmpLink() for the named profile: %v", err)
	}
	t.Setenv("OMP_PROFILE", "")
	if _, err := OmpLink(paths, []string{"skills/seed"}); err != nil {
		t.Fatalf("OmpLink() for the default profile: %v", err)
	}

	managed, err := OmpManaged(paths)
	if err != nil {
		t.Fatalf("OmpManaged() error: %v", err)
	}
	var wantAny []string
	for _, dir := range []string{defaultAgent, namedAgent} {
		wantAny = append(wantAny, filepath.Join(dir, "skills", "seed"))
	}
	for _, want := range wantAny {
		found := false
		for _, got := range managed {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("OmpManaged() = %v, missing %s", managed, want)
		}
	}

	if _, err := OmpTeardown(paths); err != nil {
		t.Fatalf("OmpTeardown() error: %v", err)
	}
	for _, want := range wantAny {
		if ompTestExists(t, want) {
			t.Errorf("%s survived teardown", want)
		}
	}

	// Both links back, then tear down with a named profile selected: the default
	// tree must be covered too — a plain `ccp reset` otherwise leaves it dangling
	// while reporting success — and the active directory must not be removed
	// twice, which would stop the teardown part-way with an error.
	t.Setenv("OMP_PROFILE", "work")
	if _, err := OmpLink(paths, []string{"skills/seed"}); err != nil {
		t.Fatalf("OmpLink() for the named profile: %v", err)
	}
	t.Setenv("OMP_PROFILE", "")
	if _, err := OmpLink(paths, []string{"skills/seed"}); err != nil {
		t.Fatalf("OmpLink() for the default profile: %v", err)
	}
	t.Setenv("OMP_PROFILE", "work")
	if _, err := OmpTeardown(paths); err != nil {
		t.Fatalf("OmpTeardown() with OMP_PROFILE set: %v", err)
	}
	for _, want := range wantAny {
		if ompTestExists(t, want) {
			t.Errorf("%s survived teardown with OMP_PROFILE set", want)
		}
	}
}

// ompConfigRootForTest mirrors ompConfigRoot for assertions without exporting it.
func ompConfigRootForTest(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".omp")
}

func TestValidateOmpProfile(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty selects the default", "", "", false},
		{"default selects the default", "default", "", false},
		{"a plain name", "work", "work", false},
		{"surrounding space is trimmed", " work ", "work", false},
		{"escape attempt", "../evil", "", true},
		{"nested path", "a/b", "", true},
		{"parent directory", "..", "", true},
		{"current directory", ".", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateOmpProfile(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateOmpProfile(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateOmpProfile(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestOmpFollowIgnoresAnotherTreesOptIn(t *testing.T) {
	paths, defaultAgent := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "seed")
	ompTestSkill(t, paths.HubDir, "second")

	// The user opted into omp only in the named 'work' tree.
	t.Setenv("OMP_PROFILE", "work")
	if _, err := OmpLink(paths, []string{"skills/seed"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}
	namedLink := filepath.Join(ompConfigRootForTest(t), "profiles", "work", "agent", "skills", "seed")
	if !ompTestExists(t, namedLink) {
		t.Fatalf("the named tree did not get the link: %s", namedLink)
	}

	// A switch in another tree must not start writing there, even though ccp
	// owns something in omp somewhere.
	t.Setenv("OMP_PROFILE", "")
	next := NewManifest("next", "")
	next.Hub.Skills = []string{"second"}
	result, err := OmpFollowProfile(paths, ompTestProfile(t, paths, NewManifest("previous", "")), ompTestProfile(t, paths, next))
	if err != nil {
		t.Fatalf("OmpFollowProfile() error: %v", err)
	}
	if len(result.Linked) != 0 || len(result.Removed) != 0 {
		t.Errorf("Linked = %v, Removed = %v, want nothing: this tree was never opted into", result.Linked, result.Removed)
	}
	if ompTestExists(t, filepath.Join(defaultAgent, "skills", "second")) {
		t.Error("a profile switch wrote into a tree ccp had never been opted into")
	}
}

func TestOmpSyncSkipsUnreadableRule(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	manifest := NewManifest("dev", "")
	manifest.Hub.Rules = []string{"plain.md", "dangling.md"}
	p := ompTestProfile(t, paths, manifest)
	ompTestWrite(t, filepath.Join(p.Path, "rules", "plain.md"), "# plain\n\nPLAIN\n")
	// A profile rule whose hub item was deleted: the link is dangling.
	if err := os.Symlink(filepath.Join(paths.HubDir, "rules", "gone.md"), filepath.Join(p.Path, "rules", "dangling.md")); err != nil {
		t.Fatal(err)
	}

	result, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("OmpSync() error: %v (one unreadable rule must not abort the sync)", err)
	}
	if !strings.Contains(strings.Join(result.Context, "\n"), "dangling.md") {
		t.Errorf("Context = %v, want the skipped rule reported", result.Context)
	}
	rules, err := os.ReadFile(filepath.Join(agentDir, "RULES.md"))
	if err != nil || !strings.Contains(string(rules), "PLAIN") {
		t.Errorf("RULES.md = %q (err %v), want the readable rule", rules, err)
	}
}

func TestOmpAgentDirRejectsInvalidProfileNames(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("PI_CODING_AGENT_DIR", "")

	for _, bad := range []string{"../evil", "a/b", "..", "."} {
		t.Setenv("OMP_PROFILE", bad)
		got := OmpAgentDir()
		if got != filepath.Join("/home/u", ".omp", "agent") {
			t.Errorf("OMP_PROFILE=%q resolved to %s, want the default agent dir", bad, got)
		}
	}

	t.Setenv("OMP_PROFILE", "work")
	if got, want := OmpAgentDir(), filepath.Join("/home/u", ".omp", "profiles", "work", "agent"); got != want {
		t.Errorf("OMP_PROFILE=work resolved to %s, want %s", got, want)
	}
}

func TestOmpLoadabilityTestsValues(t *testing.T) {
	paths, _ := ompTestEnv(t)

	write := func(rel, content string) string {
		t.Helper()
		ompTestWrite(t, filepath.Join(paths.HubDir, rel), content)
		return filepath.Join(paths.HubDir, rel)
	}

	loads := func(itemType config.HubItemType, path string) bool {
		t.Helper()
		state, _ := ompLoadabilityOf(itemType, path)
		return state == ompLoads
	}

	// agents: omp needs a name as well as a description.
	noName := write("agents/noname.md", "---\ndescription: d\n---\n")
	if loads(config.HubAgents, noName) {
		t.Error("an agent without a name is rejected by omp")
	}

	// skills: an explicit disable wins even with a description.
	disabled := write("skills/off/SKILL.md", "---\ndescription: d\nenabled: false\n---\n")
	if loads(config.HubSkills, filepath.Dir(disabled)) {
		t.Error("enabled: false means omp ignores the skill")
	}
	bom := write("skills/bom/SKILL.md", "\ufeff---\r\ndescription: d\r\n---\r\n")
	if !loads(config.HubSkills, filepath.Dir(bom)) {
		t.Error("a BOM before the frontmatter should not hide it")
	}
}

func TestOmpRulesFileCarriesEveryRule(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestWrite(t, filepath.Join(paths.HubDir, "rules", "codegraph.md"), "---\ndescription: surfaced by omp\n---\n\n# codegraph\n\nHUB-RULE\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "rules", "plain.md"), "# plain\n\nPLAIN-RULE\n")

	manifest := NewManifest("dev", "")
	manifest.Hub.Rules = []string{"codegraph.md", "plain.md"}
	p := ompTestProfile(t, paths, manifest)
	// RULES.md is generated from the profile's rules directory, and a real
	// profile holds a link per item there.
	for _, name := range []string{"codegraph.md", "plain.md"} {
		if err := symlink.New().Create(filepath.Join(p.Path, "rules", name), filepath.Join(paths.HubDir, "rules", name)); err != nil {
			t.Fatal(err)
		}
	}
	// omp already holds a rule of its own under one of those names.
	ompTestWrite(t, filepath.Join(agentDir, "rules", "codegraph.md"), "# mine\n")

	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(agentDir, "RULES.md"))
	if err != nil {
		t.Fatalf("RULES.md not written: %v", err)
	}
	for _, want := range []string{"HUB-RULE", "PLAIN-RULE"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("RULES.md does not carry %s:\n%s", want, data)
		}
	}
	if ompTestExists(t, filepath.Join(agentDir, "rules", "plain.md")) {
		t.Error("ccp linked a rule, which omp injects a second time when the frontmatter routes it")
	}
	if got, err := os.ReadFile(filepath.Join(agentDir, "rules", "codegraph.md")); err != nil || string(got) != "# mine\n" {
		t.Errorf("the user's own rule = %q (err %v), want untouched", got, err)
	}
}

func TestOmpLinkReplacesStaleHubLink(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	alpha := ompTestSkill(t, paths.HubDir, "alpha")
	ompTestSkill(t, paths.HubDir, "beta")

	// A ccp link left over from when this item was a different hub item.
	stale := filepath.Join(agentDir, "skills", "beta")
	if err := symlink.New().Create(stale, alpha); err != nil {
		t.Fatalf("Create(stale link) error: %v", err)
	}

	result, err := OmpLink(paths, []string{"skills/beta"})
	if err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}
	if want := []string{"skills/beta"}; !reflect.DeepEqual(result.Linked, want) {
		t.Errorf("Linked = %v, want %v", result.Linked, want)
	}
	if got := ompTestTarget(t, stale); got != filepath.Join(paths.HubDir, "skills", "beta") {
		t.Errorf("link target = %s, want the hub item it names", got)
	}
}

func TestOmpSyncLinksItemsOmpCannotLoad(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestWrite(t, filepath.Join(paths.HubDir, "skills", "broken", "notes.md"), "# no SKILL.md\n")

	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"broken"}

	result, err := OmpSync(paths, ompTestProfile(t, paths, manifest))
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if want := []string{"skills/broken"}; !reflect.DeepEqual(result.Linked, want) {
		t.Errorf("Linked = %v, want %v", result.Linked, want)
	}
	if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], "skills/broken") {
		t.Errorf("Notes = %v, want skills/broken reported with its reason", result.Notes)
	}
	if !ompTestExists(t, filepath.Join(agentDir, "skills", "broken")) {
		t.Error("an item omp ignores was not linked")
	}
}

func TestOmpSyncRefusesBundleMemberNamesThatEscape(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	// A bundle whose member name climbs out of the bundle directory, with the
	// source it points at present — the case that used to create a link outside
	// the agent directory.
	ompTestWrite(t, filepath.Join(paths.HubDir, "bundles", "evil", hub.BundleManifestFile),
		"name: evil\nmembers:\n  skills:\n    - ../../../../ESCAPED\n")
	ompTestWrite(t, filepath.Join(paths.HubDir, "ESCAPED", "SKILL.md"), "---\nname: ESCAPED\ndescription: d\n---\n")

	manifest := NewManifest("dev", "")
	manifest.Hub.Bundles = []string{"evil"}

	result, err := OmpSync(paths, ompTestProfile(t, paths, manifest))
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if len(result.Linked) != 0 {
		t.Errorf("Linked = %v, want nothing: the bundle is malformed", result.Linked)
	}

	// Nothing may be created outside the agent directory: walk its ancestors.
	dir := agentDir
	for range 6 {
		if ompTestExists(t, filepath.Join(dir, "ESCAPED")) {
			t.Errorf("a bundle member name escaped the agent directory: %s", filepath.Join(dir, "ESCAPED"))
		}
		dir = filepath.Dir(dir)
	}
}

func TestOmpSyncPrunesMembersOfMissingBundle(t *testing.T) {
	paths, agentDir := ompTestEnv(t)
	ompTestSkill(t, paths.HubDir, "member")
	if _, err := OmpLink(paths, []string{"skills/member"}); err != nil {
		t.Fatalf("OmpLink() error: %v", err)
	}

	// The bundle the profile references is not in the hub (not pulled yet, or
	// deleted), so its members are not part of the profile's desired state.
	manifest := NewManifest("dev", "")
	manifest.Hub.Bundles = []string{"gone"}

	result, err := OmpSync(paths, ompTestProfile(t, paths, manifest))
	if err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}
	if want := []string{"skills/member"}; !reflect.DeepEqual(result.Removed, want) {
		t.Errorf("Removed = %v, want %v", result.Removed, want)
	}
	if ompTestExists(t, filepath.Join(agentDir, "skills", "member")) {
		t.Error("link from an unresolvable bundle survived sync")
	}
}
