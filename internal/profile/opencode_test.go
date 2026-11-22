package profile

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

func TestConvertOpencodeAgent(t *testing.T) {
	in := "---\nname: applier\ndescription: \"Applies edits: carefully\"\ntools: Read, Write, Edit, Bash\nmodel: inherit\ncolor: purple\n---\n\nYou apply edits.\n"
	out, err := convertOpencodeAgent([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	fm, body, err := splitFrontmatter(out)
	if err != nil {
		t.Fatal(err)
	}
	if fm["description"] != "Applies edits: carefully" || fm["mode"] != "subagent" {
		t.Errorf("frontmatter = %v", fm)
	}
	for _, gone := range []string{"name", "model", "color"} {
		if _, ok := fm[gone]; ok {
			t.Errorf("%s should not reach opencode: %v", gone, fm)
		}
	}
	tools, _ := fm["tools"].(map[string]any)
	if tools["read"] != true || tools["bash"] != true || tools["webfetch"] != false || tools["task"] != false {
		t.Errorf("tools allowlist not converted: %v", tools)
	}
	if body != "\nYou apply edits.\n" {
		t.Errorf("body changed: %q", body)
	}
}

func TestConvertOpencodeAgent_KeepsProviderModelAndRequiresDescription(t *testing.T) {
	out, err := convertOpencodeAgent([]byte("---\ndescription: d\nmodel: openrouter/z-ai/glm-5.3\n---\nx\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "model: openrouter/z-ai/glm-5.3") {
		t.Errorf("provider model dropped: %s", out)
	}
	if _, err := convertOpencodeAgent([]byte("---\nname: x\n---\nbody\n")); err == nil {
		t.Error("agent without description accepted")
	}
}

func TestConvertOpencodeCommand(t *testing.T) {
	out, err := convertOpencodeCommand([]byte("---\ndescription: Ship it\nallowed-tools: Bash(git:*)\nargument-hint: [msg]\n---\nCommit $ARGUMENTS\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "---\ndescription: Ship it\n---\nCommit $ARGUMENTS\n" {
		t.Errorf("converted command = %q", out)
	}
}

// Sync writes copies, leaves files it did not write or that were edited by
// hand, and removes only its own unedited copies.
func TestOpencodeSync_Ownership(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	oc := OpencodeConfigDir()
	agentFile := func(name string) string { return filepath.Join(oc, "agent", name) }
	for _, name := range []string{"one.md", "two.md", "mine.md"} {
		mustWrite(t, filepath.Join(paths.HubItemDir(config.HubAgents), name), "---\ndescription: "+name+"\n---\nbody\n")
		if err := mgr.LinkHubItem("p", config.HubAgents, name); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, agentFile("mine.md"), "hand-written\n") // the user's own file
	p, _ := mgr.Get("p")

	r, err := OpencodeSync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Written) != 2 || len(r.Occupied) != 1 || r.Occupied[0] != "agent/mine.md" {
		t.Errorf("first sync = %+v", r)
	}
	if data, _ := os.ReadFile(agentFile("mine.md")); string(data) != "hand-written\n" {
		t.Error("overwrote a file ccp did not write")
	}
	if !OpencodeOptedIn(paths) {
		t.Error("not opted in after a sync")
	}

	mustWrite(t, agentFile("two.md"), "edited by hand\n")
	for _, name := range []string{"one.md", "two.md"} {
		if err := mgr.UnlinkHubItem("p", config.HubAgents, name); err != nil {
			t.Fatal(err)
		}
	}
	p, _ = mgr.Get("p")
	r, err = OpencodeSync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(agentFile("one.md")); !os.IsNotExist(err) {
		t.Error("ccp's unedited copy not removed after unlink")
	}
	if data, _ := os.ReadFile(agentFile("two.md")); string(data) != "edited by hand\n" {
		t.Error("removed or overwrote a copy edited by hand")
	}
	if len(r.Removed) != 1 || len(r.Edited) != 1 {
		t.Errorf("second sync = %+v", r)
	}
}

func TestSyncCopies_LeafSymlinkSafety(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	content := []byte("matching content\n")
	outsideFile := filepath.Join(outside, "user.md")
	mustWrite(t, outsideFile, string(content))

	// Write loop: user symlink whose target content matches desired (and a broken
	// symlink) must both be reported Occupied, never adopted or overwritten.
	matchLink := filepath.Join(dir, "match.md")
	if err := os.Symlink(outsideFile, matchLink); err != nil {
		t.Fatal(err)
	}
	brokenLink := filepath.Join(dir, "broken.md")
	if err := os.Symlink(filepath.Join(outside, "missing.md"), brokenLink); err != nil {
		t.Fatal(err)
	}

	state := &copyState{Files: map[string]string{}}
	r, err := syncCopies(dir, state, map[string][]byte{
		"match.md":  content,
		"broken.md": []byte("new\n"),
	}, nil)
	if err != nil {
		t.Fatalf("write syncCopies: %v", err)
	}
	if !slices.Contains(r.Occupied, "match.md") || !slices.Contains(r.Occupied, "broken.md") || len(r.Written) != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
	if _, adopted := state.Files["match.md"]; adopted {
		t.Fatal("user symlink was adopted into state.Files")
	}
	for _, p := range []string{matchLink, brokenLink} {
		if info, err := os.Lstat(p); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is no longer a symlink: info=%v err=%v", p, info, err)
		}
	}

	// Remove loop: a tracked path replaced by a symlink must never be deleted;
	// it is reported as Edited and dropped from state.Files.
	if r, err := syncCopies(dir, state, map[string][]byte{"tracked.md": content}, nil); err != nil || !slices.Contains(r.Written, "tracked.md") {
		t.Fatalf("seed tracked.md: r=%+v err=%v", r, err)
	}
	trackedPath := filepath.Join(dir, "tracked.md")
	if err := os.Remove(trackedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, trackedPath); err != nil {
		t.Fatal(err)
	}

	r, err = syncCopies(dir, state, nil, nil)
	if err != nil {
		t.Fatalf("remove syncCopies: %v", err)
	}
	if !slices.Contains(r.Edited, "tracked.md") || len(r.Removed) != 0 {
		t.Fatalf("expected symlink reported Edited and not Removed: %+v", r)
	}
	if _, ok := state.Files["tracked.md"]; ok {
		t.Fatal("symlink entry was not deleted from state.Files")
	}
	if info, err := os.Lstat(trackedPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("tracked symlink was deleted or replaced: info=%v err=%v", info, err)
	}
	if got, _ := os.ReadFile(outsideFile); string(got) != string(content) {
		t.Fatalf("outside target modified: %q", got)
	}
}
