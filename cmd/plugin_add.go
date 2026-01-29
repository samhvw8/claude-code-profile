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
)

type pluginJSON struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

var pluginAddCmd = &cobra.Command{
	Use:   "add <owner/repo@plugin>",
	Short: "Install a plugin from a marketplace",
	Long: `Install a plugin from a Claude Code marketplace repository.

The plugin's agents, commands, skills, and rules are copied to your hub.

Examples:
  ccp plugin add EveryInc/compound-engineering-plugin@compound-engineering
  ccp plugin add EveryInc/compound-engineering-plugin@coding-tutor

After installing, link the components to your profile:
  ccp link <profile> skills/<skill-name>
  ccp link <profile> agents/<agent-name>`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginAdd,
}

func init() {
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

	// Copy components to hub
	installed := make(map[string][]string)

	// Copy skills
	skillsDir := filepath.Join(pluginDir, "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				src := filepath.Join(skillsDir, e.Name())
				dst := filepath.Join(paths.HubDir, "skills", e.Name())
				if err := copyPluginDir(src, dst); err != nil {
					fmt.Printf("  Warning: failed to copy skill %s: %v\n", e.Name(), err)
				} else {
					installed["skills"] = append(installed["skills"], e.Name())
				}
			}
		}
	}

	// Copy agents
	agentsDir := filepath.Join(pluginDir, "agents")
	if entries, err := os.ReadDir(agentsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				src := filepath.Join(agentsDir, e.Name())
				dst := filepath.Join(paths.HubDir, "agents", e.Name())
				if err := copyPluginDir(src, dst); err != nil {
					fmt.Printf("  Warning: failed to copy agent %s: %v\n", e.Name(), err)
				} else {
					installed["agents"] = append(installed["agents"], e.Name())
				}
			}
		}
	}

	// Copy commands
	commandsDir := filepath.Join(pluginDir, "commands")
	if entries, err := os.ReadDir(commandsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				src := filepath.Join(commandsDir, e.Name())
				dst := filepath.Join(paths.HubDir, "commands", e.Name())
				if err := copyPluginDir(src, dst); err != nil {
					fmt.Printf("  Warning: failed to copy command %s: %v\n", e.Name(), err)
				} else {
					installed["commands"] = append(installed["commands"], e.Name())
				}
			}
		}
	}

	// Copy rules if present
	rulesDir := filepath.Join(pluginDir, "rules")
	if entries, err := os.ReadDir(rulesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				src := filepath.Join(rulesDir, e.Name())
				dst := filepath.Join(paths.HubDir, "rules", e.Name())
				if err := copyPluginDir(src, dst); err != nil {
					fmt.Printf("  Warning: failed to copy rule %s: %v\n", e.Name(), err)
				} else {
					installed["rules"] = append(installed["rules"], e.Name())
				}
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
