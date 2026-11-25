package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/source"
	"github.com/samhvw8/claude-code-profile/internal/testutil"
)

// setupProjectLink makes a ccp dir with a git clone of owner/repo holding
// skills/a and skills/b, and a project that is its own git repo. When global
// is true, ~/.ccp/ccp.toml lists the source.
func setupProjectLink(t *testing.T, global bool) (paths *config.Paths, root, sourceDir string) {
	t.Helper()
	ccpDir := t.TempDir()
	t.Setenv("CCP_DIR", ccpDir)
	paths, err := config.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	sourceDir = paths.SourceDir("owner/repo")
	writeFile(t, filepath.Join(sourceDir, "skills", "a", "SKILL.md"), "# a")
	writeFile(t, filepath.Join(sourceDir, "skills", "b", "SKILL.md"), "# b")
	testutil.Git(t, sourceDir, "init", "-q")
	testutil.Git(t, sourceDir, "remote", "add", "origin", "https://github.com/owner/repo.git")
	testutil.CommitAll(t, sourceDir, "initial")

	if global {
		registry := loadRegistry(t, paths)
		registry.AddSource("owner/repo", source.Source{Provider: "git", URL: "https://github.com/owner/repo.git",
			Path: sourceDir, Commit: (&source.GitProvider{}).GetCommit(sourceDir)})
		if err := registry.Save(); err != nil {
			t.Fatal(err)
		}
	}

	root, _ = filepath.EvalSymlinks(t.TempDir())
	testutil.Git(t, root, "init", "-q")
	return paths, root, sourceDir
}

func loadProject(t *testing.T, root string, paths *config.Paths) *source.Registry {
	t.Helper()
	project, err := source.LoadProjectRegistry(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func TestProjectLink_RecordsInProjectAndExcludes(t *testing.T) {
	paths, root, sourceDir := setupProjectLink(t, true)
	globalBefore, _ := os.ReadFile(filepath.Join(paths.CcpDir, "ccp.toml"))

	project := loadProject(t, root, paths)
	id, err := addProjectSource("owner/repo", paths, project, loadRegistry(t, paths))
	if err != nil {
		t.Fatal(err)
	}
	if err := linkProjectItems(paths, root, project, id, []string{"skills/a"}); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, ".claude", "skills", "a")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("no link: %v", err)
	}
	if want, _ := filepath.EvalSymlinks(filepath.Join(sourceDir, "skills", "a")); target != want {
		t.Errorf("link target = %s, want absolute %s", target, want)
	}

	reloaded := loadProject(t, root, paths)
	src, err := reloaded.GetSource("owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(src.Installed, []string{"skills/a"}) || !source.ValidCommit(src.Commit) {
		t.Errorf("project source = %+v, want installed [skills/a] and a locked commit", src)
	}
	if _, err := os.Stat(filepath.Join(root, "ccp.lock")); err != nil {
		t.Errorf("project ccp.lock missing: %v", err)
	}

	globalAfter, _ := os.ReadFile(filepath.Join(paths.CcpDir, "ccp.toml"))
	if string(globalBefore) != string(globalAfter) {
		t.Errorf("~/.ccp/ccp.toml changed:\n%s", globalAfter)
	}

	exclude, _ := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	for _, want := range []string{"/ccp.toml", "/ccp.lock", "/.claude/skills/a"} {
		if !slices.Contains(strings.Split(string(exclude), "\n"), want) {
			t.Errorf(".git/info/exclude lacks %s:\n%s", want, exclude)
		}
	}
	// Running it again adds nothing twice.
	if err := excludeProjectLinks(root, reloaded); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if string(again) != string(exclude) {
		t.Errorf("second run changed exclude:\n%s", again)
	}
	if out := testutil.Git(t, root, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("git status shows ccp files: %q", out)
	}
}

func TestProjectLink_RefusesRealDirectory(t *testing.T) {
	paths, root, _ := setupProjectLink(t, true)
	own := filepath.Join(root, ".claude", "skills", "a", "SKILL.md")
	writeFile(t, own, "team copy")

	project := loadProject(t, root, paths)
	id, err := addProjectSource("owner/repo", paths, project, loadRegistry(t, paths))
	if err != nil {
		t.Fatal(err)
	}
	if err := linkProjectItems(paths, root, project, id, []string{"skills/a"}); err == nil {
		t.Fatal("linked over a real directory")
	}
	if data, _ := os.ReadFile(own); string(data) != "team copy" {
		t.Errorf("real directory changed: %q", data)
	}
}

// Sync recreates a missing link and moves a project-only clone to the
// project's locked commit.
func TestProjectSync_RecreatesLinksAtLockedCommit(t *testing.T) {
	paths, root, sourceDir := setupProjectLink(t, false)
	git := &source.GitProvider{}
	first := git.GetCommit(sourceDir)
	writeFile(t, filepath.Join(sourceDir, "skills", "a", "SKILL.md"), "# a v2")
	testutil.CommitAll(t, sourceDir, "second")
	// CheckoutCommit fetches the locked commit from origin: point it at the clone itself.
	testutil.Git(t, sourceDir, "remote", "set-url", "origin", sourceDir)

	project := loadProject(t, root, paths)
	project.AddSource("owner/repo", source.Source{Provider: "git", URL: "https://github.com/owner/repo.git",
		Path: sourceDir, Commit: first, Installed: []string{"skills/a"}})
	if err := project.Save(); err != nil {
		t.Fatal(err)
	}

	if err := runProjectSync(paths, root); err != nil {
		t.Fatal(err)
	}
	if head := git.GetCommit(sourceDir); head != first {
		t.Errorf("clone at %s, want locked %s", head, first)
	}
	data, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "a", "SKILL.md"))
	if err != nil || string(data) != "# a" {
		t.Errorf("linked SKILL.md = %q, %v; want the locked content", data, err)
	}
}

func TestProjectRemove_DropsLinkKeepsClone(t *testing.T) {
	paths, root, sourceDir := setupProjectLink(t, true)
	project := loadProject(t, root, paths)
	id, _ := addProjectSource("owner/repo", paths, project, loadRegistry(t, paths))
	if err := linkProjectItems(paths, root, project, id, []string{"skills/a", "skills/b"}); err != nil {
		t.Fatal(err)
	}

	projectDirFlag = root
	t.Cleanup(func() { projectDirFlag = "" })
	if err := runProjectRemove(nil, []string{"skills/a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "a")); !os.IsNotExist(err) {
		t.Error("link still there")
	}
	if _, err := os.Stat(filepath.Join(sourceDir, "skills", "a", "SKILL.md")); err != nil {
		t.Errorf("clone lost its item: %v", err)
	}
	src, _ := loadProject(t, root, paths).GetSource("owner/repo")
	if !slices.Equal(src.Installed, []string{"skills/b"}) {
		t.Errorf("installed = %v, want [skills/b]", src.Installed)
	}
}

func TestReusableClone(t *testing.T) {
	_, _, sourceDir := setupProjectLink(t, false)
	for _, url := range []string{"https://github.com/owner/repo", "git@github.com:owner/repo.git", "ssh://git@github.com/Owner/repo"} {
		if ok, err := reusableClone(sourceDir, url, "git"); !ok || err != nil {
			t.Errorf("same origin %s: %v, %v; want reuse", url, ok, err)
		}
	}
	if _, err := reusableClone(sourceDir, "https://github.com/other/repo.git", "git"); err == nil {
		t.Error("reused a clone of another repo")
	}
	if ok, err := reusableClone(filepath.Join(t.TempDir(), "none"), "x", "git"); ok || err != nil {
		t.Errorf("missing dir: %v, %v; want a fresh clone", ok, err)
	}
	plain := t.TempDir()
	if _, err := reusableClone(plain, "https://github.com/owner/repo", "git"); err == nil {
		t.Error("a directory without .git would be fetched over")
	}
	if ok, err := reusableClone(plain, "https://example.com/a.tar.gz", "http"); !ok || err != nil {
		t.Errorf("http download: %v, %v; want reuse", ok, err)
	}
}

// A ccp.toml that ccp did not write is never read as links, nor overwritten.
func TestProjectLink_ForeignCcpToml(t *testing.T) {
	paths, root, _ := setupProjectLink(t, true)
	foreign := "default_registry = 'github'\n[sources.'x/y']\nurl = 'u'\n"
	writeFile(t, filepath.Join(root, "ccp.toml"), foreign)

	project := loadProject(t, root, paths)
	if !project.ProjectFileForeign() || project.SourceCount() != 0 {
		t.Fatalf("foreign file read as links: %d sources", project.SourceCount())
	}
	projectDirFlag = root
	t.Cleanup(func() { projectDirFlag = "" })
	if err := runProjectList(nil, nil); err != nil {
		t.Errorf("list failed on a foreign ccp.toml: %v", err)
	}
	project.AddSource("owner/repo", source.Source{Provider: "git"})
	if err := project.Save(); err == nil {
		t.Error("saved over a foreign ccp.toml")
	}
	if data, _ := os.ReadFile(filepath.Join(root, "ccp.toml")); string(data) != foreign {
		t.Errorf("foreign ccp.toml changed:\n%s", data)
	}
}

// With --dir below the git root, the exclude patterns carry the prefix.
func TestProjectLink_ExcludeInSubdirectory(t *testing.T) {
	paths, top, _ := setupProjectLink(t, true)
	root := filepath.Join(top, "mono", "pkg")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	project := loadProject(t, root, paths)
	id, _ := addProjectSource("owner/repo", paths, project, loadRegistry(t, paths))
	if err := linkProjectItems(paths, root, project, id, []string{"skills/a"}); err != nil {
		t.Fatal(err)
	}
	exclude, _ := os.ReadFile(filepath.Join(top, ".git", "info", "exclude"))
	if !strings.Contains(string(exclude), "/mono/pkg/.claude/skills/a\n") || !strings.Contains(string(exclude), "/mono/pkg/ccp.toml\n") {
		t.Errorf("exclude lacks prefixed paths:\n%s", exclude)
	}
	if out := testutil.Git(t, top, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("git status shows ccp files: %q", out)
	}
}

// Remove reads ccp.toml before deleting: a broken file deletes nothing.
func TestProjectRemove_BrokenCcpTomlDeletesNothing(t *testing.T) {
	paths, root, _ := setupProjectLink(t, true)
	project := loadProject(t, root, paths)
	id, _ := addProjectSource("owner/repo", paths, project, loadRegistry(t, paths))
	if err := linkProjectItems(paths, root, project, id, []string{"skills/a"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "ccp.toml"))
	writeFile(t, filepath.Join(root, "ccp.toml"), string(data)+"installed = ['../x']\n[broken\n")

	projectDirFlag = root
	t.Cleanup(func() { projectDirFlag = "" })
	if err := runProjectRemove(nil, []string{"skills/a"}); err == nil {
		t.Error("remove ran with an unreadable ccp.toml")
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "a")); err != nil {
		t.Errorf("link deleted anyway: %v", err)
	}
}

// source remove refuses a clone that a recorded project still links, even
// when ~/.ccp/ccp.toml lists the source too.
func TestSourceRemove_RefusesLinkedClone(t *testing.T) {
	paths, root, sourceDir := setupProjectLink(t, true)
	project := loadProject(t, root, paths)
	id, _ := addProjectSource("owner/repo", paths, project, loadRegistry(t, paths))
	if err := linkProjectItems(paths, root, project, id, []string{"skills/a"}); err != nil {
		t.Fatal(err)
	}
	if err := runSourceRemove(nil, []string{"owner/repo"}); err == nil {
		t.Error("removed a clone a project links")
	}
	if _, err := os.Stat(filepath.Join(sourceDir, "skills", "a")); err != nil {
		t.Errorf("clone deleted: %v", err)
	}

	projectDirFlag = root
	t.Cleanup(func() { projectDirFlag = "" })
	if err := runProjectRemove(nil, []string{"skills/a"}); err != nil {
		t.Fatal(err)
	}
	if using, _ := source.ProjectsUsing(paths, "owner/repo"); len(using) != 1 {
		t.Errorf("project with the source but no items: using = %v, want it listed", using)
	}
}
