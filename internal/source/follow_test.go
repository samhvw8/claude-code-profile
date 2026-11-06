package source

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/testutil"
)

// A repo that keeps its items only under .claude/ installs them by the path
// shown in the clone; the hub name drops the harness directory.
func TestInstall_HarnessPrefixedPath(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	createTestFile(t, sourceDir, ".claude/skills/h/SKILL.md", "# H")
	createTestFile(t, sourceDir, ".claude/agents/ag.md", "# Ag")

	got, err := i.Install(shadowSource, []string{".claude/skills/h", ".claude/agents/ag.md"}, Shadow)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"skills/h", "agents/ag.md"}) {
		t.Errorf("installed = %v", got)
	}
	for _, item := range got {
		if i.State(shadowSource, item) != ItemShadow {
			t.Errorf("%s is not a shadow", item)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.HubDir, "agents/ag.md")); err != nil {
		t.Errorf("agent shadow does not resolve: %v", err)
	}
}

// installed holds hub names, and a hub name finds the root item first. A
// harness path that its hub name would not find again is refused, not
// installed as one path and later read as another.
func TestInstall_HarnessPathHiddenByRootItem(t *testing.T) {
	i, _, sourceDir := setupShadow(t)
	createTestFile(t, sourceDir, ".claude/skills/x/SKILL.md", "# other x")

	_, err := i.Install(shadowSource, []string{".claude/skills/x"}, Shadow)
	if err == nil || !strings.Contains(err.Error(), "skills/x resolves to skills/x") {
		t.Fatalf("err = %v, want a refusal naming where skills/x resolves", err)
	}
	if slices.Contains(installed(i), "skills/x") {
		t.Error("refused item was tracked")
	}
}

func TestInstall_HarnessPrefixedPathRejectsEscape(t *testing.T) {
	i, _, _ := setupShadow(t)
	for _, item := range []string{".claude/../skills/x", ".claude/skills/..", ".git/skills/x"} {
		if _, err := i.Install(shadowSource, []string{item}, Shadow); err == nil {
			t.Errorf("Install(%q) succeeded", item)
		}
	}
}

// Upstream moves an installed item (skills/x -> .claude/skills/x): the shadow
// breaks, and FollowUpdate points it at the new path.
func TestFollowUpdate_RelinksMovedItem(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	if _, err := i.Install(shadowSource, []string{"skills/x", "agents/a"}, Shadow); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(sourceDir, ".claude"), 0755)
	testutil.Git(t, sourceDir, "mv", "skills", ".claude/skills")
	testutil.Git(t, sourceDir, "mv", "agents", ".claude/agents")
	testutil.CommitAll(t, sourceDir, "move under .claude")

	relinked, behind, err := i.FollowUpdate(shadowSource)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(relinked, []string{"skills/x", "agents/a.md"}) {
		t.Errorf("relinked = %v", relinked)
	}
	if len(behind) != 0 {
		t.Errorf("behind = %v", behind)
	}
	if data, _ := os.ReadFile(filepath.Join(paths.HubDir, "skills/x/SKILL.md")); !strings.Contains(string(data), "# X") {
		t.Errorf("relinked skill reads %q", data)
	}
	if i.State(shadowSource, "agents/a.md") != ItemShadow {
		t.Error("agent not relinked")
	}
}

// Every installed item an update cannot reach is reported, and none is
// touched: a copy (unchanged or edited), an item gone upstream, an item not in
// the hub, and the user's own link.
func TestFollowUpdate_ReportsItemsTheUpdateDoesNotReach(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	createTestFile(t, sourceDir, "skills/y/SKILL.md", "# Y")
	createTestFile(t, sourceDir, "skills/gone/SKILL.md", "# Gone")
	createTestFile(t, sourceDir, "skills/absent/SKILL.md", "# Absent")
	createTestFile(t, sourceDir, "skills/mine/SKILL.md", "# Mine")
	testutil.CommitAll(t, sourceDir, "more")
	if _, err := i.Install(shadowSource, []string{"skills/gone", "skills/absent", "agents/a"}, Shadow); err != nil {
		t.Fatal(err)
	}

	// Copies from before shadows.
	for _, name := range []string{"x", "y"} {
		CopyTree(filepath.Join(sourceDir, "skills", name), filepath.Join(paths.HubDir, "skills", name))
		i.registry.AddInstalled(shadowSource, "skills/"+name)
	}
	createTestFile(t, paths.HubDir, "skills/y/SKILL.md", "local edit")
	// Removed upstream.
	testutil.Git(t, sourceDir, "rm", "-rq", "skills/gone")
	testutil.CommitAll(t, sourceDir, "drop gone")
	// Removed from the hub.
	os.Remove(filepath.Join(paths.HubDir, "skills/absent"))
	// The user's own link.
	own := filepath.Join(t.TempDir(), "mine")
	os.MkdirAll(own, 0755)
	os.Symlink(own, filepath.Join(paths.HubDir, "skills/mine"))
	i.registry.AddInstalled(shadowSource, "skills/mine")

	relinked, behind, err := i.FollowUpdate(shadowSource)
	if err != nil {
		t.Fatal(err)
	}
	if len(relinked) != 0 {
		t.Errorf("relinked = %v", relinked)
	}
	want := map[string]Behind{
		"skills/x":      {Item: "skills/x", State: ItemCopySame},
		"skills/y":      {Item: "skills/y", State: ItemCopyDiffers},
		"skills/gone":   {Item: "skills/gone", State: ItemMissing, Gone: true},
		"skills/absent": {Item: "skills/absent", State: ItemMissing},
		"skills/mine":   {Item: "skills/mine", State: ItemForeignLink},
	}
	if len(behind) != len(want) {
		t.Errorf("behind = %v", behind)
	}
	for _, b := range behind {
		if b != want[b.Item] {
			t.Errorf("%s: got %+v, want %+v", b.Item, b, want[b.Item])
		}
	}
	if data, _ := os.ReadFile(filepath.Join(paths.HubDir, "skills/y/SKILL.md")); string(data) != "local edit" {
		t.Errorf("edited copy touched: %q", data)
	}
}
