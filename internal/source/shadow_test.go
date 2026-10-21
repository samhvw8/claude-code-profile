package source

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/testutil"
)

const shadowSource = "owner/repo"

// setupShadow makes a ccp dir with a git source clone holding skills/x,
// agents/a.md, a plugin skill and a deep URL-style skill, registered in a
// registry, and returns an installer over it.
func setupShadow(t *testing.T) (*Installer, *config.Paths, string) {
	t.Helper()
	dir := t.TempDir()
	paths := &config.Paths{CcpDir: dir, HubDir: filepath.Join(dir, "hub")}
	sourceDir := paths.SourceDir(shadowSource)

	createTestFile(t, sourceDir, "skills/x/SKILL.md", "---\nname: x\n---\n# X")
	createTestFile(t, sourceDir, "skills/x/ref/notes.md", "notes")
	createTestFile(t, sourceDir, "agents/a.md", "# Agent")
	createTestFile(t, sourceDir, "plugins/p/skills/n/SKILL.md", "# N")
	createTestFile(t, sourceDir, "deep/dir/foo/SKILL.md", "---\nname: foo\n---\n# Foo")
	testutil.Git(t, sourceDir, "init", "-q")
	testutil.CommitAll(t, sourceDir, "initial")

	registry := NewRegistry(dir)
	if err := registry.AddSource(shadowSource, Source{Provider: "git", Path: sourceDir}); err != nil {
		t.Fatal(err)
	}
	return NewInstaller(paths, registry), paths, sourceDir
}

func installed(i *Installer) []string {
	src, _ := i.registry.GetSource(shadowSource)
	return src.Installed
}

func TestInstall_CreatesShadowLink(t *testing.T) {
	i, paths, _ := setupShadow(t)

	got, err := i.Install(shadowSource, []string{"skills/x", "agents/a"}, Shadow)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"skills/x", "agents/a.md"}) {
		t.Errorf("installed = %v", got)
	}
	for _, item := range got {
		path := filepath.Join(paths.HubDir, item)
		if !testutil.IsSymlink(path) || !i.IsShadow(path) {
			t.Errorf("%s is not a shadow link", item)
		}
		if i.State(shadowSource, item) != ItemShadow {
			t.Errorf("%s state = %v, want shadow", item, i.State(shadowSource, item))
		}
	}
	if data, err := os.ReadFile(filepath.Join(paths.HubDir, "skills/x/ref/notes.md")); err != nil || string(data) != "notes" {
		t.Errorf("content through the shadow = %q, %v", data, err)
	}
	if target, _ := os.Readlink(filepath.Join(paths.HubDir, "skills/x")); filepath.IsAbs(target) {
		t.Errorf("shadow link is absolute: %s", target)
	}
	if !slices.Equal(installed(i), got) {
		t.Errorf("registry installed = %v", installed(i))
	}
}

// A source update moves the clone; a shadow shows the new content with no re-copy.
func TestShadow_FollowsSourceUpdate(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	if _, err := i.Install(shadowSource, []string{"skills/x"}, Shadow); err != nil {
		t.Fatal(err)
	}
	createTestFile(t, sourceDir, "skills/x/SKILL.md", "v2")
	testutil.CommitAll(t, sourceDir, "v2")

	data, _ := os.ReadFile(filepath.Join(paths.HubDir, "skills/x/SKILL.md"))
	if string(data) != "v2" {
		t.Errorf("hub item = %q, want v2", data)
	}
}

// The hub reached through a symlink (a dotfiles-managed hub) still gets a
// shadow that resolves.
func TestInstall_ShadowResolvesThroughLinkedHub(t *testing.T) {
	i, paths, _ := setupShadow(t)
	realHub := filepath.Join(t.TempDir(), "elsewhere", "hub")
	os.MkdirAll(realHub, 0755)
	if err := os.Symlink(realHub, paths.HubDir); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(shadowSource, []string{"skills/x"}, Shadow); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.HubDir, "skills/x/SKILL.md")); err != nil {
		t.Errorf("shadow does not resolve through a linked hub: %v", err)
	}
	if !i.IsShadow(filepath.Join(paths.HubDir, "skills/x")) {
		t.Error("IsShadow = false through a linked hub")
	}
}

func TestInstall_ReplacesBrokenShadow(t *testing.T) {
	i, paths, _ := setupShadow(t)
	dst := filepath.Join(paths.HubDir, "skills/x")
	os.MkdirAll(filepath.Dir(dst), 0755)
	os.Symlink("../../sources/gone/skills/x", dst)

	if _, err := i.Install(shadowSource, []string{"skills/x"}, Shadow); err != nil {
		t.Fatalf("Install over a broken link: %v", err)
	}
	if i.State(shadowSource, "skills/x") != ItemShadow {
		t.Error("broken link not replaced by a shadow")
	}
}

func TestInstall_RefusesExistingLocalItem(t *testing.T) {
	i, paths, _ := setupShadow(t)
	createTestFile(t, paths.HubDir, "skills/x/SKILL.md", "mine")

	if _, err := i.Install(shadowSource, []string{"skills/x"}, Shadow); err == nil {
		t.Fatal("Install over a local item succeeded")
	}
	if data, _ := os.ReadFile(filepath.Join(paths.HubDir, "skills/x/SKILL.md")); string(data) != "mine" {
		t.Errorf("local item changed: %q", data)
	}
}

// An upstream repo must not point a hub item outside sources/, at the item
// itself or through a symlink inside it.
func TestInstall_RefusesLinksOutOfSources(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	secret := filepath.Join(t.TempDir(), "ssh")
	createTestFile(t, secret, "id_rsa", "KEY")

	os.Symlink(secret, filepath.Join(sourceDir, "skills", "evil"))
	createTestFile(t, sourceDir, "skills/sneaky/SKILL.md", "# s")
	os.Symlink(filepath.Join(secret, "id_rsa"), filepath.Join(sourceDir, "skills", "sneaky", "key"))
	// A link that stays inside the clone is fine.
	os.Symlink("../x/ref", filepath.Join(sourceDir, "skills", "sneaky", "ref"))

	for _, item := range []string{"skills/evil", "skills/sneaky"} {
		for _, mode := range []InstallMode{Shadow, Clone} {
			if _, err := i.Install(shadowSource, []string{item}, mode); err == nil {
				t.Errorf("%s (mode %d) installed despite a link out of sources", item, mode)
			}
			if _, err := os.Lstat(filepath.Join(paths.HubDir, item)); err == nil {
				t.Errorf("%s (mode %d) left a hub entry", item, mode)
			}
		}
	}

	os.Remove(filepath.Join(sourceDir, "skills", "sneaky", "key"))
	if _, err := i.Install(shadowSource, []string{"skills/sneaky"}, Shadow); err != nil {
		t.Errorf("a link inside the clone was refused: %v", err)
	}
}

func TestInstallClone_MakesUntrackedLocalCopy(t *testing.T) {
	i, paths, _ := setupShadow(t)

	if _, err := i.Install(shadowSource, []string{"skills/x"}, Clone); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(paths.HubDir, "skills/x")
	if testutil.IsSymlink(dst) {
		t.Error("--clone made a symlink")
	}
	if _, err := os.Stat(filepath.Join(dst, "ref/notes.md")); err != nil {
		t.Errorf("copy incomplete: %v", err)
	}
	if len(installed(i)) != 0 {
		t.Errorf("a clone is a local item, but installed = %v", installed(i))
	}
}

// Editing a shadow edits the clone. --clone moves the edit into a local copy
// and resets the clone, so source update is no longer blocked.
func TestInstallClone_ConvertsEditedShadow(t *testing.T) {
	i, paths, _ := setupShadow(t)
	if _, err := i.Install(shadowSource, []string{"skills/x"}, Shadow); err != nil {
		t.Fatal(err)
	}
	createTestFile(t, paths.HubDir, "skills/x/SKILL.md", "edited")
	createTestFile(t, paths.HubDir, "skills/x/new.md", "added")

	edits, err := i.LocalEdits(shadowSource)
	if err != nil || len(edits) != 2 {
		t.Fatalf("LocalEdits = %v, %v; want the edit and the new file", edits, err)
	}

	if _, err := i.Install(shadowSource, []string{"skills/x"}, Clone); err != nil {
		t.Fatalf("clone of a shadow: %v", err)
	}
	dst := filepath.Join(paths.HubDir, "skills/x")
	if testutil.IsSymlink(dst) {
		t.Fatal("still a symlink after --clone")
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "SKILL.md")); string(data) != "edited" {
		t.Errorf("local copy lost the edit: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dst, "new.md")); err != nil {
		t.Errorf("local copy lost the added file: %v", err)
	}
	if edits, _ := i.LocalEdits(shadowSource); len(edits) != 0 {
		t.Errorf("clone still has edits: %v", edits)
	}
	if len(installed(i)) != 0 {
		t.Errorf("installed = %v after clone", installed(i))
	}
	if _, err := os.Lstat(leftoverPath(dst, cloneSuffix)); !os.IsNotExist(err) {
		t.Error("clone left its temporary copy behind")
	}
}

// --clone on a tracked copy from before shadows keeps it and untracks it.
func TestInstallClone_UntracksExistingCopy(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	dst := filepath.Join(paths.HubDir, "skills/x")
	CopyTree(filepath.Join(sourceDir, "skills/x"), dst)
	createTestFile(t, dst, "SKILL.md", "my old version")
	i.registry.AddInstalled(shadowSource, "skills/x")

	if _, err := i.Install(shadowSource, []string{"skills/x"}, Clone); err != nil {
		t.Fatalf("--clone of a tracked copy: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "SKILL.md")); string(data) != "my old version" {
		t.Errorf("copy changed: %q", data)
	}
	if len(installed(i)) != 0 {
		t.Errorf("installed = %v, want untracked", installed(i))
	}
}

// A shadow that points into another source's clone is not converted.
func TestInstallClone_RefusesShadowOfAnotherSource(t *testing.T) {
	i, paths, _ := setupShadow(t)
	other := paths.SourceDir("other/repo")
	createTestFile(t, other, "skills/x/SKILL.md", "# other")
	dst := filepath.Join(paths.HubDir, "skills/x")
	os.MkdirAll(filepath.Dir(dst), 0755)
	os.Symlink(filepath.Join(other, "skills/x"), dst)

	if _, err := i.Install(shadowSource, []string{"skills/x"}, Clone); err == nil {
		t.Fatal("converted a shadow of another source")
	}
	if !testutil.IsSymlink(dst) {
		t.Error("the other source's shadow was changed")
	}
}

// Plugin items and URL installs are listed under hub names that differ from
// their path in the source; sync must still find them.
func TestInstall_ByHubNameForPluginAndURLItems(t *testing.T) {
	i, paths, _ := setupShadow(t)
	if _, err := i.Install(shadowSource, []string{"plugins/p/skills/n"}, Shadow); err != nil {
		t.Fatal(err)
	}
	if _, err := i.InstallPath(shadowSource, "deep/dir/foo", Shadow); err != nil {
		t.Fatal(err)
	}
	want := []string{"skills/p-n", "skills/foo"}
	if !slices.Equal(installed(i), want) {
		t.Fatalf("installed = %v, want %v", installed(i), want)
	}

	// A new machine: the links are gone, only ccp.toml's names remain.
	for _, item := range want {
		os.Remove(filepath.Join(paths.HubDir, item))
		if _, err := i.Install(shadowSource, []string{item}, Shadow); err != nil {
			t.Errorf("reinstall %s by hub name: %v", item, err)
		}
		if i.State(shadowSource, item) != ItemShadow {
			t.Errorf("%s not restored as a shadow", item)
		}
	}

	// A copy of a URL-installed item is recognized as unchanged.
	dst := filepath.Join(paths.HubDir, "skills/foo")
	os.Remove(dst)
	CopyTree(filepath.Join(paths.SourceDir(shadowSource), "deep/dir/foo"), dst)
	if s := i.State(shadowSource, "skills/foo"); s != ItemCopySame {
		t.Errorf("state = %v, want copy-same", s)
	}
}

func TestReshadow_UnchangedCopy(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	dst := filepath.Join(paths.HubDir, "skills/x")
	if err := CopyTree(filepath.Join(sourceDir, "skills/x"), dst); err != nil {
		t.Fatal(err)
	}
	i.registry.AddInstalled(shadowSource, "skills/x")

	if s := i.State(shadowSource, "skills/x"); s != ItemCopySame {
		t.Fatalf("state = %v, want copy-same", s)
	}
	if err := i.Reshadow(shadowSource, "skills/x"); err != nil {
		t.Fatal(err)
	}
	if i.State(shadowSource, "skills/x") != ItemShadow {
		t.Error("copy not replaced by a shadow")
	}
	if _, err := os.Lstat(leftoverPath(dst, reshadowSuffix)); !os.IsNotExist(err) {
		t.Error("aside copy left behind")
	}
}

// A copy with a local edit must never be replaced.
func TestReshadow_RefusesEditedCopy(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	dst := filepath.Join(paths.HubDir, "skills/x")
	CopyTree(filepath.Join(sourceDir, "skills/x"), dst)
	i.registry.AddInstalled(shadowSource, "skills/x")
	createTestFile(t, dst, "SKILL.md", "local edit")

	if s := i.State(shadowSource, "skills/x"); s != ItemCopyDiffers {
		t.Fatalf("state = %v, want copy-differs", s)
	}
	if err := i.Reshadow(shadowSource, "skills/x"); err == nil {
		t.Fatal("Reshadow replaced an edited copy")
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "SKILL.md")); string(data) != "local edit" {
		t.Errorf("edit lost: %q", data)
	}
	if testutil.IsSymlink(dst) {
		t.Error("edited copy became a symlink")
	}
}

// The user's own symlink is theirs, even when its content matches the source.
func TestReshadow_LeavesForeignLink(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	mine := filepath.Join(t.TempDir(), "x")
	CopyTree(filepath.Join(sourceDir, "skills/x"), mine)
	dst := filepath.Join(paths.HubDir, "skills/x")
	os.MkdirAll(filepath.Dir(dst), 0755)
	os.Symlink(mine, dst)
	i.registry.AddInstalled(shadowSource, "skills/x")

	if s := i.State(shadowSource, "skills/x"); s != ItemForeignLink {
		t.Fatalf("state = %v, want foreign-link", s)
	}
	if err := i.Reshadow(shadowSource, "skills/x"); err == nil {
		t.Error("Reshadow replaced the user's own link")
	}
	if target, _ := os.Readlink(dst); target != mine {
		t.Errorf("link now points at %s", target)
	}
}

// An extra file, an empty directory or a changed mode is a difference.
func TestSameTree_Differences(t *testing.T) {
	cases := map[string]func(dir string){
		"extra file": func(dir string) { createTestFile(t, dir, "sub/extra.md", "y") },
		"empty dir":  func(dir string) { os.MkdirAll(filepath.Join(dir, "empty"), 0755) },
		"exec bit":   func(dir string) { os.Chmod(filepath.Join(dir, "f.md"), 0755) },
	}
	for name, change := range cases {
		a, b := t.TempDir(), t.TempDir()
		createTestFile(t, a, "f.md", "x")
		createTestFile(t, b, "f.md", "x")
		if same, _ := sameTree(a, b); !same {
			t.Fatalf("%s: identical trees reported different", name)
		}
		change(b)
		if same, _ := sameTree(a, b); same {
			t.Errorf("%s not detected", name)
		}
	}
}

func TestSameTree_SymlinkLoopIsAnError(t *testing.T) {
	a := t.TempDir()
	createTestFile(t, a, "f.md", "x")
	os.Symlink("..", filepath.Join(a, "loop"))
	if _, err := sameTree(a, a); err == nil {
		t.Error("symlink loop not reported")
	}
}

func TestState_MissingAndInvalid(t *testing.T) {
	i, _, _ := setupShadow(t)
	if s := i.State(shadowSource, "skills/x"); s != ItemMissing {
		t.Errorf("state = %v, want missing", s)
	}
	for _, item := range []string{"skills/..", "../x", "skills", "/abs/x", "skills/./x"} {
		if s := i.State(shadowSource, item); s != ItemUnresolved {
			t.Errorf("State(%q) = %v, want unresolved", item, s)
		}
	}
}

// Untracked files outside installed items survive a reset and do not block;
// tracked changes anywhere and new files inside an item do.
func TestLocalEdits_Scope(t *testing.T) {
	i, paths, sourceDir := setupShadow(t)
	if _, err := i.Install(shadowSource, []string{"skills/x"}, Shadow); err != nil {
		t.Fatal(err)
	}
	createTestFile(t, sourceDir, ".DS_Store", "junk")
	createTestFile(t, sourceDir, "agents/untracked.md", "junk")
	if edits, err := i.LocalEdits(shadowSource); err != nil || len(edits) != 0 {
		t.Errorf("untracked files outside items block: %v, %v", edits, err)
	}

	createTestFile(t, sourceDir, "agents/a.md", "changed")
	if edits, _ := i.LocalEdits(shadowSource); !slices.Equal(edits, []string{"agents/a.md"}) {
		t.Errorf("tracked change: edits = %v", edits)
	}

	createTestFile(t, paths.HubDir, "skills/x/new.md", "new")
	if edits, _ := i.LocalEdits(shadowSource); !slices.Contains(edits, "skills/x/new.md") {
		t.Errorf("new file inside an item: edits = %v", edits)
	}
}

func TestLocalEdits_EmptyPathIsAnError(t *testing.T) {
	registry := NewRegistry(t.TempDir())
	registry.AddSource("x/y", Source{Provider: "git"})
	i := NewInstaller(&config.Paths{CcpDir: t.TempDir()}, registry)
	if _, err := i.LocalEdits("x/y"); err == nil {
		t.Error("an empty path was accepted (it would check the current directory)")
	}
}

func TestSweepLeftovers(t *testing.T) {
	hub := t.TempDir()
	// Interrupted after the link was made: the aside copy is redundant.
	createTestFile(t, hub, "skills/a/SKILL.md", "a")
	createTestFile(t, hub, "skills/.a"+reshadowSuffix+"/SKILL.md", "a")
	// Interrupted before the copy was moved in: it is the only copy.
	createTestFile(t, hub, "skills/.b"+cloneSuffix+"/SKILL.md", "b")

	found, err := SweepLeftovers(hub, false)
	if err != nil || len(found) != 2 {
		t.Fatalf("found = %v, %v", found, err)
	}
	if _, err := SweepLeftovers(hub, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(hub, "skills/.a"+reshadowSuffix)); !os.IsNotExist(err) {
		t.Error("redundant leftover kept")
	}
	if data, _ := os.ReadFile(filepath.Join(hub, "skills/b/SKILL.md")); string(data) != "b" {
		t.Errorf("only copy not restored: %q", data)
	}
	if found, _ := SweepLeftovers(hub, false); len(found) != 0 {
		t.Errorf("leftovers remain: %v", found)
	}
}

// ccp.toml is synced and hand-edited: a commit that git would read as an
// option must never reach git.
func TestCheckoutCommit_RejectsNonSHA(t *testing.T) {
	_, _, sourceDir := setupShadow(t)
	marker := filepath.Join(t.TempDir(), "PWNED")
	for _, commit := range []string{"--upload-pack=touch " + marker, "main", "abc123", ""} {
		if err := (&GitProvider{}).CheckoutCommit(context.Background(), sourceDir, commit); err == nil {
			t.Errorf("commit %q accepted", commit)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("an injected git option ran")
	}
}

// ccp.toml's commit is a lock: CheckoutCommit moves a shallow clone back to it.
func TestCheckoutCommit_ShallowClone(t *testing.T) {
	// Fetch normalizes the URL to end in .git
	base := filepath.Join(t.TempDir(), "origin")
	origin := base + ".git"
	createTestFile(t, origin, "skills/x/SKILL.md", "v1")
	testutil.Git(t, origin, "init", "-q", "-b", "main")
	v1 := testutil.CommitAll(t, origin, "v1")
	createTestFile(t, origin, "skills/x/SKILL.md", "v2")
	testutil.CommitAll(t, origin, "v2")

	clone := filepath.Join(t.TempDir(), "clone")
	git := &GitProvider{}
	if err := git.Fetch(context.Background(), "file://"+base, clone, FetchOptions{Ref: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := git.CheckoutCommit(context.Background(), clone, v1); err != nil {
		t.Fatal(err)
	}
	if git.GetCommit(clone) != v1 {
		t.Errorf("HEAD = %s, want %s", git.GetCommit(clone), v1)
	}
	if data, _ := os.ReadFile(filepath.Join(clone, "skills/x/SKILL.md")); string(data) != "v1" {
		t.Errorf("content = %q, want v1", data)
	}
}

func TestUpdate_RejectsOptionRef(t *testing.T) {
	_, _, sourceDir := setupShadow(t)
	if _, err := (&GitProvider{}).Update(context.Background(), sourceDir, UpdateOptions{Ref: "--upload-pack=x"}); err == nil {
		t.Error("a ref starting with - was passed to git")
	}
}
