package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
)

type pluginJSON struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

var (
	pluginAddSelect bool
)

var pluginAddCmd = &cobra.Command{
	Use:   "add <owner/repo@plugin>",
	Short: "Install a plugin from a marketplace",
	Long: `Install a plugin from a Claude Code marketplace repository.

The plugin's agents, commands, skills, and rules are copied to your hub.

Use --select to interactively choose which components to install.

Examples:
  ccp plugin add EveryInc/compound-engineering-plugin@compound-engineering
  ccp plugin add EveryInc/compound-engineering-plugin@coding-tutor --select

After installing, link the components to your profile:
  ccp link <profile> skills/<skill-name>
  ccp link <profile> agents/<agent-name>`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginAdd,
}

func init() {
	pluginAddCmd.Flags().BoolVarP(&pluginAddSelect, "select", "s", false, "Interactively select which components to install")
	pluginCmd.AddCommand(pluginAddCmd)
}

func runPluginAdd(cmd *cobra.Command, args []string) error {
	source := args[0]

	// Parse source: owner/repo@plugin
	owner, repo, pluginName, err := parsePluginSource(source)
	if err != nil {
		return err
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	fmt.Printf("Installing plugin '%s' from %s/%s...\n", pluginName, owner, repo)

	// Fetch marketplace to find plugin source path
	mp, err := fetchMarketplace(owner, repo)
	if err != nil {
		return err
	}

	// Find the plugin in marketplace
	var pluginInfo *marketplacePlugin
	for _, p := range mp.Plugins {
		if p.Name == pluginName {
			pluginInfo = &p
			break
		}
	}

	if pluginInfo == nil {
		available := make([]string, len(mp.Plugins))
		for i, p := range mp.Plugins {
			available[i] = p.Name
		}
		return fmt.Errorf("plugin '%s' not found in marketplace\n  Available: %s", pluginName, strings.Join(available, ", "))
	}

	// Clone the repo to temp dir
	tempDir, err := os.MkdirTemp("", "ccp-plugin-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	gitCmd := exec.Command("git", "clone", "--depth", "1", repoURL, tempDir)
	gitCmd.Stdout = io.Discard
	gitCmd.Stderr = os.Stderr

	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repo: %w", err)
	}

	// Find plugin directory
	pluginDir := filepath.Join(tempDir, strings.TrimPrefix(pluginInfo.Source, "./"))
	if _, err := os.Stat(pluginDir); err != nil {
		return fmt.Errorf("plugin directory not found: %s", pluginInfo.Source)
	}

	// Read plugin.json for metadata
	pluginJSONPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	var pj pluginJSON
	if data, err := os.ReadFile(pluginJSONPath); err == nil {
		json.Unmarshal(data, &pj)
	}

	// Discover available components
	available := discoverPluginComponents(pluginDir)

	if len(available) == 0 {
		return fmt.Errorf("no components found in plugin")
	}

	// Determine which components to install
	toInstall := available
	if pluginAddSelect {
		// Build picker items
		var items []picker.Item
		for _, comp := range available {
			items = append(items, picker.Item{
				ID:       comp.Type + "/" + comp.Name,
				Label:    fmt.Sprintf("[%s] %s", comp.Type, comp.Name),
				Selected: true, // default all selected
			})
		}

		fmt.Printf("\nFound %d components in plugin '%s':\n", len(items), pluginName)
		selected, err := picker.Run("Select components to install", items)
		if err != nil {
			return fmt.Errorf("picker error: %w", err)
		}
		if selected == nil {
			fmt.Println("Installation cancelled")
			return nil
		}

		// Filter to selected only
		selectedSet := make(map[string]bool)
		for _, s := range selected {
			selectedSet[s] = true
		}
		toInstall = nil
		for _, comp := range available {
			if selectedSet[comp.Type+"/"+comp.Name] {
				toInstall = append(toInstall, comp)
			}
		}

		if len(toInstall) == 0 {
			fmt.Println("No components selected")
			return nil
		}
	}

	// Copy selected components to hub
	installed := make(map[string][]string)
	for _, comp := range toInstall {
		var src, dst string
		var installName string

		if comp.IsHook {
			// Hooks: copy entire hooks folder with plugin-prefixed name
			src = filepath.Join(pluginDir, "hooks")
			installName = pluginName + "-hooks"
			dst = filepath.Join(paths.HubDir, "hooks", installName)
		} else {
			// Standard components: copy subdirectory
			src = filepath.Join(pluginDir, comp.Type, comp.Name)
			installName = comp.Name
			dst = filepath.Join(paths.HubDir, comp.Type, installName)
		}

		if err := copyPluginDir(src, dst); err != nil {
			fmt.Printf("  Warning: failed to copy %s %s: %v\n", comp.Type, comp.Name, err)
		} else {
			installed[comp.Type] = append(installed[comp.Type], installName)
		}
	}

	// Get commit SHA for source tracking
	commit := getPluginGitCommit(tempDir)

	// Create plugin manifest for tracking
	pluginManifest := hub.NewPluginManifest(
		pluginName,
		pj.Description,
		pluginInfo.Version,
		hub.GitHubSource{
			Owner:  owner,
			Repo:   repo,
			Commit: commit,
			Path:   pluginInfo.Source,
		},
		hub.ComponentList{
			Skills:   installed["skills"],
			Agents:   installed["agents"],
			Commands: installed["commands"],
			Rules:    installed["rules"],
			Hooks:    installed["hooks"],
		},
	)
	if err := pluginManifest.Save(paths.PluginsDir()); err != nil {
		fmt.Printf("Warning: failed to save plugin manifest: %v\n", err)
	}

	// Create source.yaml for each installed component
	for componentType, names := range installed {
		for _, name := range names {
			componentPath := filepath.Join(paths.HubDir, componentType, name)
			sourceManifest := hub.NewPluginSource(pluginName, owner, repo, pluginInfo.Version)
			if err := sourceManifest.Save(componentPath); err != nil {
				fmt.Printf("Warning: failed to save source for %s/%s: %v\n", componentType, name, err)
			}
		}
	}

	// Print summary
	fmt.Println()
	fmt.Printf("Installed plugin: %s (v%s)\n", pluginName, pluginInfo.Version)

	total := 0
	for itemType, items := range installed {
		if len(items) > 0 {
			fmt.Printf("  %s: %d items\n", itemType, len(items))
			total += len(items)
		}
	}

	if total == 0 {
		fmt.Println("  No components found in plugin")
	}

	fmt.Println()
	fmt.Println("To use these components, link them to your profile:")
	for itemType, items := range installed {
		if len(items) > 0 {
			fmt.Printf("  ccp link <profile> %s/%s\n", itemType, items[0])
		}
	}

	return nil
}

func parsePluginSource(source string) (owner, repo, plugin string, err error) {
	// Remove URL prefix
	source = strings.TrimPrefix(source, "https://github.com/")
	source = strings.TrimPrefix(source, "github.com/")
	source = strings.TrimSuffix(source, ".git")

	// Find @ separator for plugin name
	atIdx := strings.LastIndex(source, "@")
	if atIdx <= 0 {
		return "", "", "", fmt.Errorf("missing plugin name: %s\n  Expected format: owner/repo@plugin-name", source)
	}

	plugin = source[atIdx+1:]
	repoPath := source[:atIdx]

	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid source format: %s\n  Expected: owner/repo@plugin-name", source)
	}

	return parts[0], parts[1], plugin, nil
}

func copyPluginDir(src, dst string) error {
	// Check if destination already exists
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("already exists")
	}

	// Create destination
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

		if entry.IsDir() {
			if err := copyPluginDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyPluginFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyPluginFile(src, dst string) error {
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

// getPluginGitCommit returns the current commit SHA from a git repository
func getPluginGitCommit(repoDir string) string {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// pluginComponent represents a component found in a plugin
type pluginComponent struct {
	Type   string // skills, agents, commands, rules, hooks
	Name   string
	IsHook bool // true if this is the entire hooks folder
}

// discoverPluginComponents scans a plugin directory for available components
func discoverPluginComponents(pluginDir string) []pluginComponent {
	var components []pluginComponent

	// Standard component types with subdirectories
	componentTypes := []string{"skills", "agents", "commands", "rules"}
	for _, compType := range componentTypes {
		dir := filepath.Join(pluginDir, compType)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				components = append(components, pluginComponent{
					Type: compType,
					Name: e.Name(),
				})
			}
		}
	}

	// Hooks are treated as a single unit - check for hooks/hooks.json
	hooksDir := filepath.Join(pluginDir, "hooks")
	hooksJSON := filepath.Join(hooksDir, "hooks.json")
	if _, err := os.Stat(hooksJSON); err == nil {
		// hooks.json exists, treat entire hooks folder as one component
		components = append(components, pluginComponent{
			Type:   "hooks",
			Name:   "hooks", // the folder name
			IsHook: true,
		})
	}

	return components
}
