package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var (
	pluginUpdateAll    bool
	pluginUpdateForce  bool
	pluginUpdateDryRun bool
)

var pluginUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update installed plugins and their components",
	Long: `Update plugins that were installed from a marketplace.

This will update all components (skills, agents, commands, rules) that
were installed as part of the plugin.

Without arguments, shows all updateable plugins.
With a name, updates just that plugin and its components.

Examples:
  ccp plugin update                       # Show all updateable plugins
  ccp plugin update --all                 # Update all plugins
  ccp plugin update compound-engineering  # Update specific plugin
  ccp plugin update --dry-run             # Show what would be updated`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPluginUpdate,
}

func init() {
	pluginUpdateCmd.Flags().BoolVarP(&pluginUpdateAll, "all", "a", false, "Update all plugins without prompting")
	pluginUpdateCmd.Flags().BoolVarP(&pluginUpdateForce, "force", "f", false, "Force update even if local changes detected")
	pluginUpdateCmd.Flags().BoolVarP(&pluginUpdateDryRun, "dry-run", "n", false, "Show what would be updated")
	pluginCmd.AddCommand(pluginUpdateCmd)
}

func runPluginUpdate(cmd *cobra.Command, args []string) error {
	// Show migration hint
	if len(args) > 0 {
		fmt.Printf("Hint: ccp source update %s\n\n", args[0])
	} else {
		fmt.Println("Hint: ccp source update")
		fmt.Println()
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// List installed plugins
	plugins, err := hub.ListPlugins(paths.PluginsDir())
	if err != nil {
		return fmt.Errorf("failed to list plugins: %w", err)
	}

	if len(plugins) == 0 {
		fmt.Println("No plugins installed")
		return nil
	}

	// If specific plugin requested
	if len(args) > 0 {
		pluginName := args[0]
		for _, p := range plugins {
			if p.Name == pluginName {
				return updatePlugin(paths, p)
			}
		}
		return fmt.Errorf("plugin not found: %s", pluginName)
	}

	// Show all plugins
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tVERSION\tSOURCE\tCOMPONENTS\n")
	fmt.Fprintf(w, "----\t-------\t------\t----------\n")
	for _, p := range plugins {
		source := p.GitHub.Owner + "/" + p.GitHub.Repo
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", p.Name, p.Version, source, p.Components.Count())
	}
	w.Flush()
	fmt.Println()

	if pluginUpdateDryRun {
		fmt.Printf("Would update %d plugins\n", len(plugins))
		return nil
	}

	if !pluginUpdateAll {
		fmt.Printf("Found %d plugins. Use --all to update all.\n", len(plugins))
		return nil
	}

	// Update all plugins
	updated := 0
	failed := 0
	for _, p := range plugins {
		fmt.Printf("Updating plugin %s...\n", p.Name)
		if err := updatePluginFromGitHub(paths, p); err != nil {
			fmt.Printf("  Error: %v\n", err)
			failed++
		} else {
			fmt.Printf("  Updated\n")
			updated++
		}
	}

	fmt.Println()
	fmt.Printf("Updated %d plugins", updated)
	if failed > 0 {
		fmt.Printf(", %d failed", failed)
	}
	fmt.Println()

	return nil
}

func updatePlugin(paths *config.Paths, p *hub.PluginManifest) error {
	if pluginUpdateDryRun {
		fmt.Printf("Would update plugin %s (%d components)\n", p.Name, p.Components.Count())
		return nil
	}

	fmt.Printf("Updating plugin %s from %s/%s...\n", p.Name, p.GitHub.Owner, p.GitHub.Repo)
	if err := updatePluginFromGitHub(paths, p); err != nil {
		return err
	}
	fmt.Println("Updated successfully")
	return nil
}

func updatePluginFromGitHub(paths *config.Paths, pm *hub.PluginManifest) error {
	// Clone the repo to temp dir
	tempDir, err := os.MkdirTemp("", "ccp-plugin-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", pm.GitHub.Owner, pm.GitHub.Repo)
	gitCmd := exec.Command("git", "clone", "--depth", "1", repoURL, tempDir)
	gitCmd.Stdout = io.Discard
	gitCmd.Stderr = io.Discard

	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repo: %w", err)
	}

	// Get new commit SHA
	newCommit := getPluginUpdateCommit(tempDir)

	// Check if already up to date
	if pm.GitHub.Commit != "" && pm.GitHub.Commit == newCommit {
		return fmt.Errorf("already up to date")
	}

	// Find plugin directory in repo
	pluginDir := tempDir
	if pm.GitHub.Path != "" && pm.GitHub.Path != "." {
		pluginDir = filepath.Join(tempDir, strings.TrimPrefix(pm.GitHub.Path, "./"))
	}

	if _, err := os.Stat(pluginDir); err != nil {
		return fmt.Errorf("plugin path not found in repo: %s", pm.GitHub.Path)
	}

	// Update each component type
	componentDirs := map[string][]string{
		"skills":   pm.Components.Skills,
		"agents":   pm.Components.Agents,
		"commands": pm.Components.Commands,
		"rules":    pm.Components.Rules,
	}

	for componentType, names := range componentDirs {
		srcDir := filepath.Join(pluginDir, componentType)
		for _, name := range names {
			componentSrc := filepath.Join(srcDir, name)
			componentDst := filepath.Join(paths.HubDir, componentType, name)

			if _, err := os.Stat(componentSrc); err != nil {
				fmt.Printf("  Warning: component %s/%s not found in updated repo\n", componentType, name)
				continue
			}

			// Remove existing component
			if err := os.RemoveAll(componentDst); err != nil {
				fmt.Printf("  Warning: failed to remove %s/%s: %v\n", componentType, name, err)
				continue
			}

			// Copy new content
			if err := copyPluginUpdateDir(componentSrc, componentDst); err != nil {
				fmt.Printf("  Warning: failed to copy %s/%s: %v\n", componentType, name, err)
				continue
			}

			// Update source manifest for component
			sourceManifest := hub.NewPluginSource(pm.Name, pm.GitHub.Owner, pm.GitHub.Repo, pm.Version)
			if err := sourceManifest.Save(componentDst); err != nil {
				fmt.Printf("  Warning: failed to update source for %s/%s: %v\n", componentType, name, err)
			}
		}
	}

	// Update hooks (entire folder, not subdirectories)
	for _, hookName := range pm.Components.Hooks {
		hookSrc := filepath.Join(pluginDir, "hooks")
		hookDst := filepath.Join(paths.HubDir, "hooks", hookName)

		if _, err := os.Stat(hookSrc); err != nil {
			fmt.Printf("  Warning: hooks folder not found in updated repo\n")
			continue
		}

		// Remove existing hooks
		if err := os.RemoveAll(hookDst); err != nil {
			fmt.Printf("  Warning: failed to remove hooks/%s: %v\n", hookName, err)
			continue
		}

		// Copy new content
		if err := copyPluginUpdateDir(hookSrc, hookDst); err != nil {
			fmt.Printf("  Warning: failed to copy hooks/%s: %v\n", hookName, err)
			continue
		}

		// Update source manifest
		sourceManifest := hub.NewPluginSource(pm.Name, pm.GitHub.Owner, pm.GitHub.Repo, pm.Version)
		if err := sourceManifest.Save(hookDst); err != nil {
			fmt.Printf("  Warning: failed to update source for hooks/%s: %v\n", hookName, err)
		}
	}

	// Update plugin manifest
	pm.GitHub.Commit = newCommit
	if err := pm.Save(paths.PluginsDir()); err != nil {
		return fmt.Errorf("failed to update plugin manifest: %w", err)
	}

	return nil
}

func getPluginUpdateCommit(repoDir string) string {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func copyPluginUpdateDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.Name() == ".git" {
			continue
		}

		if entry.IsDir() {
			if err := copyPluginUpdateDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyPluginUpdateFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyPluginUpdateFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
