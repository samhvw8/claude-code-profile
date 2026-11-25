package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/source"
)

var projectUpdateCmd = &cobra.Command{
	Use:   "update [source]",
	Short: "Update the sources in the project's ccp.toml",
	Long: `Update the sources that the project's ccp.toml lists (see 'ccp project install --link')
and move the project's ccp.lock forward. The links in .claude/ follow the clone.

A source that ~/.ccp/ccp.toml also lists is updated with 'ccp source update';
here its project lock only takes the clone's current commit. A clone with local
edits is not updated, as with 'ccp source update'.

Examples:
  ccp project update
  ccp project update owner/repo`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProjectUpdate,
}

func init() {
	projectCmd.AddCommand(projectUpdateCmd)
}

// findProjectRoot returns the project root that holds .claude/ and ccp.toml.
func findProjectRoot(dirFlag string) (string, error) {
	claudeDir, err := findProjectClaudeDir(dirFlag)
	if err != nil {
		return "", err
	}
	return filepath.Dir(claudeDir), nil
}

// sortedSources lists a registry's sources by id, so output is stable.
func sortedSources(r *source.Registry) []source.SourceEntry {
	entries := r.ListSources()
	slices.SortFunc(entries, func(a, b source.SourceEntry) int { return strings.Compare(a.ID, b.ID) })
	return entries
}

// addProjectSource puts sourceID into the project registry. A source that
// ~/.ccp/ccp.toml already has is taken over with its clone and commit; any
// other source is looked up and cloned like 'ccp install' does.
func addProjectSource(sourceID string, paths *config.Paths, project, global *source.Registry) (string, error) {
	if g, err := global.GetSource(sourceID); err == nil {
		src := *g
		src.Path = paths.SourceDir(sourceID)
		src.Installed = nil
		if err := project.AddSource(sourceID, src); err != nil {
			return "", err
		}
		return sourceID, nil
	}
	fmt.Printf("Source not in the project, adding: %s\n", sourceID)
	if err := addSourceForInstall(sourceID, "", paths, project); err != nil {
		return "", fmt.Errorf("failed to add source: %w", err)
	}
	resolvedID := generateSourceID(sourceID, sourceID)
	if !project.HasSource(resolvedID) {
		return "", fmt.Errorf("source not found after add: %s", sourceID)
	}
	return resolvedID, nil
}

// reusableClone reports whether sourceDir already holds the source, cloned for
// another project or the hub. Fetching again would delete it, and with it any
// local edits made through links, so an existing directory is either reused
// (a git clone of the same origin, or any download of a non-git source) or
// refused, never fetched over.
func reusableClone(sourceDir, url, providerType string) (bool, error) {
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if providerType != "git" {
		return true, nil
	}
	if _, err := os.Stat(filepath.Join(sourceDir, ".git")); err != nil {
		return false, fmt.Errorf("%s exists but is not a git clone of %s: move it away first", sourceDir, url)
	}
	out, err := exec.Command("git", "-C", sourceDir, "remote", "get-url", "origin").Output()
	if err != nil {
		return false, fmt.Errorf("%s is a git clone without an origin: move it away first", sourceDir)
	}
	if normalizeOrigin(string(out)) != normalizeOrigin(url) {
		return false, fmt.Errorf("%s is a clone of %s, not %s: move it away first", sourceDir, strings.TrimSpace(string(out)), url)
	}
	return true, nil
}

// normalizeOrigin reduces a git URL to host/path, so the https, ssh and
// scp-like (git@host:owner/repo) forms of one repository compare equal.
func normalizeOrigin(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	for _, scheme := range []string{"https://", "http://", "ssh://", "git://"} {
		u = strings.TrimPrefix(u, scheme)
	}
	if at := strings.Index(u, "@"); at >= 0 && at < strings.IndexAny(u+"/", "/:") {
		u = u[at+1:]
	}
	if host, path, ok := strings.Cut(u, ":"); ok && !strings.Contains(host, "/") {
		u = host + "/" + path
	}
	return strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
}

// linkProjectItems links items of sourceID into the project and records them.
func linkProjectItems(paths *config.Paths, root string, project *source.Registry, sourceID string, items []string) error {
	installer := source.NewInstaller(paths, project)
	claudeDir := filepath.Join(root, ".claude")
	linked, linkErr := installer.LinkToDir(sourceID, items, claudeDir, projectAllowedTypes())
	for _, item := range linked {
		project.AddInstalled(sourceID, item)
	}
	if len(linked) > 0 || project.HasSource(sourceID) {
		if err := project.Save(); err != nil {
			return err
		}
		if err := excludeProjectLinks(root, project); err != nil {
			fmt.Printf("WARN: could not update .git/info/exclude: %v\n", err)
		}
		if err := source.RecordProject(paths, root); err != nil {
			fmt.Printf("WARN: could not record the project in ~/.ccp/state: %v\n", err)
		}
	}
	if len(linked) > 0 {
		fmt.Printf("Linked %d item(s) into %s:\n", len(linked), claudeDir)
		for _, item := range linked {
			fmt.Printf("  - %s → %s\n", item, sourceID)
		}
		fmt.Printf("Recorded in %s\n", filepath.Join(root, source.ProjectFile))
	}
	return linkErr
}

// runProjectSync replays the project's ccp.toml: it clones missing sources,
// moves each clone to the project's locked commit, and recreates missing links.
func runProjectSync(paths *config.Paths, root string) error {
	project, err := source.LoadProjectRegistry(root, paths)
	if err != nil {
		return err
	}
	if project.SourceCount() == 0 {
		fmt.Printf("No sources in %s.\n", filepath.Join(root, source.ProjectFile))
		fmt.Println("Link one with: ccp project install <owner/repo> <type/name> --link")
		return nil
	}
	global, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	installer := source.NewInstaller(paths, project)
	claudeDir := filepath.Join(root, ".claude")
	ctx := context.Background()
	failed := 0

	for _, entry := range sortedSources(project) {
		fmt.Printf("Source: %s\n", entry.ID)
		src := entry.Source
		provider := source.GetProvider(src.Provider)
		if provider == nil {
			fmt.Printf("  ⚠ Unknown provider: %s, skipping\n", src.Provider)
			failed++
			continue
		}

		if _, err := os.Stat(src.Path); os.IsNotExist(err) {
			fmt.Printf("  Cloning from %s...\n", src.URL)
			if err := provider.Fetch(ctx, src.URL, src.Path, source.FetchOptions{Ref: src.Ref}); err != nil {
				fmt.Printf("  ⚠ Clone failed: %v\n", err)
				failed++
				continue
			}
		}

		git, isGit := provider.(*source.GitProvider)
		if g, err := global.GetSource(entry.ID); err == nil {
			// The hub shadows of this clone follow ~/.ccp/ccp.lock; moving the
			// clone to the project's commit would move them too.
			if isGit && src.Commit != "" && g.Commit != "" && g.Commit != src.Commit {
				fmt.Printf("  ⚠ ~/.ccp/ccp.toml also lists this source; its lock (%s) wins over this project's (%s)\n",
					shortenSHA(g.Commit), shortenSHA(src.Commit))
			}
		} else if isGit && src.Commit != "" {
			head := git.GetCommit(src.Path)
			if err := checkoutLockedCommit(ctx, installer, entry, src.Path); err != nil {
				fmt.Printf("  ⚠ %v\n", err)
			} else if head != "" && head != src.Commit {
				fmt.Printf("  Moved the shared clone from %s: other projects linked to it see %s too\n",
					shortenSHA(head), shortenSHA(src.Commit))
			}
		}
		if isGit && src.Commit == "" {
			src.Commit = git.GetCommit(src.Path)
			project.UpdateSource(entry.ID, src)
		}

		ok := 0
		for _, item := range src.Installed {
			dst := filepath.Join(claudeDir, filepath.FromSlash(item))
			if _, err := os.Stat(dst); err == nil {
				if installer.IsShadow(dst) {
					ok++
					continue
				}
				fmt.Printf("  ⚠ %s: %s is not a ccp link, left as it is\n", item, dst)
				failed++
				continue
			}
			if err := installer.LinkInstalled(entry.ID, item, claudeDir); err != nil {
				fmt.Printf("  ⚠ %s: %v\n", item, err)
				failed++
				continue
			}
			fmt.Printf("  + %s\n", item)
			ok++
		}
		fmt.Printf("  ✓ %d of %d item(s) linked\n", ok, len(src.Installed))
	}

	if err := project.Save(); err != nil {
		return err
	}
	if err := excludeProjectLinks(root, project); err != nil {
		fmt.Printf("WARN: could not update .git/info/exclude: %v\n", err)
	}
	if err := source.RecordProject(paths, root); err != nil {
		fmt.Printf("WARN: could not record the project in ~/.ccp/state: %v\n", err)
	}
	if failed > 0 {
		return fmt.Errorf("%d project source(s) or link(s) failed (listed above)", failed)
	}
	return nil
}

func runProjectUpdate(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	root, err := findProjectRoot(projectDirFlag)
	if err != nil {
		return err
	}
	project, err := source.LoadProjectRegistry(root, paths)
	if err != nil {
		return err
	}
	global, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	toUpdate := sortedSources(project)
	if len(args) > 0 {
		src, err := project.GetSource(args[0])
		if err != nil {
			return fmt.Errorf("%s is not in %s", args[0], filepath.Join(root, source.ProjectFile))
		}
		toUpdate = []source.SourceEntry{{ID: args[0], Source: *src}}
	}
	if len(toUpdate) == 0 {
		fmt.Printf("No sources in %s\n", filepath.Join(root, source.ProjectFile))
		return nil
	}

	installer := source.NewInstaller(paths, project)
	ctx := context.Background()
	blocked := 0
	for _, entry := range toUpdate {
		src := entry.Source
		provider := source.GetProvider(src.Provider)
		if provider == nil {
			fmt.Printf("  %s: unknown provider %s\n", entry.ID, src.Provider)
			blocked++
			continue
		}
		git, isGit := provider.(*source.GitProvider)

		if global.HasSource(entry.ID) {
			fmt.Printf("  %s: ~/.ccp/ccp.toml also lists it; update it with: ccp source update %s\n", entry.ID, entry.ID)
			if isGit {
				if head := git.GetCommit(src.Path); head != "" && head != src.Commit {
					src.Commit = head
					project.UpdateSource(entry.ID, src)
					fmt.Printf("  %s: project lock set to the clone's commit %s\n", entry.ID, shortenSHA(head))
				}
			}
			continue
		}

		fmt.Printf("Updating %s...\n", entry.ID)
		dirty, err := installer.LocalEdits(entry.ID)
		if err != nil {
			fmt.Printf("  %s: %v\n", entry.ID, err)
			blocked++
			continue
		}
		if len(dirty) > 0 {
			fmt.Printf("  %s: not updated, local edits would be lost:\n", entry.ID)
			for _, f := range dirty {
				fmt.Printf("    %s\n", f)
			}
			fmt.Printf("  → discard them: git -C %s checkout . && git -C %s clean -fd\n", src.Path, src.Path)
			blocked++
			continue
		}

		result, err := provider.Update(ctx, src.Path, source.UpdateOptions{Ref: src.Ref, URL: src.URL})
		if err != nil {
			fmt.Printf("  %s: %v\n", entry.ID, err)
			blocked++
			continue
		}
		if result.Updated {
			src.Commit = result.NewCommit
			project.UpdateSource(entry.ID, src)
			fmt.Printf("  %s: updated %s -> %s\n", entry.ID, shortenSHA(result.OldCommit), shortenSHA(result.NewCommit))
		} else {
			fmt.Printf("  %s: already up to date\n", entry.ID)
		}
	}

	if err := project.Save(); err != nil {
		return err
	}
	if blocked > 0 {
		return fmt.Errorf("%d source(s) not updated", blocked)
	}
	return nil
}

// excludeProjectHeader marks the lines ccp adds to .git/info/exclude.
const excludeProjectHeader = "# ccp: machine-local project links (ccp project install --link)"

// excludeProjectLinks keeps the project's ccp.toml, ccp.lock and links out of
// git through .git/info/exclude, git's own per-clone ignore file: the links
// point into this machine's ~/.ccp, so they must never be committed. A
// directory outside git needs nothing.
func excludeProjectLinks(root string, project *source.Registry) error {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		return nil
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}

	// Patterns are anchored at the top of the work tree; the project may be a
	// subdirectory of it (--dir).
	prefixOut, err := exec.Command("git", "-C", root, "rev-parse", "--show-prefix").Output()
	if err != nil {
		return err
	}
	prefix := "/" + strings.TrimSpace(string(prefixOut))
	want := []string{prefix + source.ProjectFile, prefix + "ccp.lock"}
	for _, entry := range sortedSources(project) {
		for _, item := range entry.Source.Installed {
			want = append(want, prefix+".claude/"+item)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	have := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var add []string
	if !have[excludeProjectHeader] {
		add = append(add, excludeProjectHeader)
	}
	for _, w := range want {
		if !have[w] {
			add = append(add, w)
		}
	}
	if len(add) == 1 && add[0] == excludeProjectHeader || len(add) == 0 {
		return nil
	}
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += strings.Join(add, "\n") + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return err
	}

	if exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", source.ProjectFile).Run() == nil {
		fmt.Printf("WARN: %s is tracked by git; .git/info/exclude does not hide a tracked file\n", source.ProjectFile)
	}
	return nil
}
