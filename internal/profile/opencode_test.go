package profile

import (
	"os"
	"path/filepath"
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
