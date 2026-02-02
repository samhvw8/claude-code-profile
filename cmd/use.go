package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	useShowFlag   bool
	useGlobalFlag bool
)

var useCmd = &cobra.Command{
	Use:     "use [profile]",
	Aliases: []string{"u"},
	Short:   "Set or show the active profile",
	Long: `Set which profile to use for the current project or globally.

Without -g flag: Updates project environment (auto-detects mise.toml or .envrc).
With -g flag: Updates global ~/.claude symlink.

Auto-detection order:
  1. mise.toml exists → update it
  2. .envrc exists → update it
  3. mise command available → offer to create mise.toml
  4. Otherwise → print shell export command

Examples:
  ccp use dev              # Auto-detect and update project env
  ccp use dev -g           # Update global ~/.claude symlink
  ccp use                  # Interactive picker (project env)
  ccp use -g               # Interactive picker (global symlink)
  ccp use --show           # Show current active profile`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runUse,
}

func init() {
	useCmd.Flags().BoolVar(&useShowFlag, "show", false, "Show current active profile")
	useCmd.Flags().BoolVarP(&useGlobalFlag, "global", "g", false, "Update global ~/.claude symlink")
	rootCmd.AddCommand(useCmd)
}

func runUse(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Show mode (explicit --show flag)
	if useShowFlag {
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}

		if active == nil {
			fmt.Println("No active profile (symlink not set)")
			return nil
		}

		fmt.Printf("Active profile: %s\n", active.Name)
		if active.Manifest.Description != "" {
			fmt.Printf("Description: %s\n", active.Manifest.Description)
		}
		return nil
	}

	// Interactive mode (no args)
	if len(args) == 0 {
		return runUseInteractive(mgr, paths)
	}

	// Direct mode (profile name provided)
	return switchToProfile(mgr, paths, args[0], useGlobalFlag)
}

func runUseInteractive(mgr *profile.Manager, paths *config.Paths) error {
	profiles, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles found. Create one with 'ccp profile create <name>'")
		return nil
	}

	// Get active profile to mark it
	active, _ := mgr.GetActive()
	activeName := ""
	if active != nil {
		activeName = active.Name
	}

	// Build picker items
	items := make([]picker.Item, len(profiles))
	for i, p := range profiles {
		label := p.Name
		if p.Manifest.Description != "" {
			desc := p.Manifest.Description
			if len(desc) > 30 {
				desc = desc[:27] + "..."
			}
			label = fmt.Sprintf("%s - %s", p.Name, desc)
		}
		if p.Name == activeName {
			label = label + " (active)"
		}
		items[i] = picker.Item{
			ID:       p.Name,
			Label:    label,
			Selected: p.Name == activeName,
		}
	}

	selected, err := picker.RunSingle("Select profile", items)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	if selected == "" {
		// User cancelled
		return nil
	}

	return switchToProfile(mgr, paths, selected, useGlobalFlag)
}

func switchToProfile(mgr *profile.Manager, paths *config.Paths, profileName string, global bool) error {
	// Check profile exists
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	profilePath := paths.ProfileDir(profileName)

	// Global mode: update ~/.claude symlink
	if global {
		// Check for drift before switching
		detector := profile.NewDetector(paths)
		report, err := detector.Detect(p)
		if err != nil {
			fmt.Printf("Warning: could not check profile health: %v\n", err)
		} else if report.HasDrift() {
			fmt.Printf("Warning: profile '%s' has configuration drift (%d issues)\n", profileName, len(report.Issues))
			fmt.Printf("  Run 'ccp profile fix %s' to reconcile\n\n", profileName)
		}

		if err := mgr.SetActive(profileName); err != nil {
			return fmt.Errorf("failed to set active profile: %w", err)
		}

		if err := profile.RegenerateSettings(paths, p.Path, p.Manifest); err != nil {
			fmt.Printf("Warning: failed to regenerate settings.json: %v\n", err)
		}

		fmt.Printf("Switched global profile to: %s\n", profileName)
		return nil
	}

	// Project mode: auto-detect environment file
	return updateProjectEnv(profilePath, profileName)
}

func updateProjectEnv(profilePath, profileName string) error {
	// Environment variables to set
	envVars := map[string]string{
		"CLAUDE_CONFIG_DIR": profilePath,
		// Enable loading CLAUDE.md from additional directories (for --add-dir usage)
		"CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD": "1",
	}

	// 1. Check for mise.toml
	if _, err := os.Stat("mise.toml"); err == nil {
		if err := updateMiseTomlMulti(envVars); err != nil {
			return err
		}
		fmt.Printf("Profile '%s' configured for this project\n", profileName)
		return nil
	}

	// 2. Check for .envrc
	if _, err := os.Stat(".envrc"); err == nil {
		if err := updateEnvrcMulti(envVars); err != nil {
			return err
		}
		fmt.Printf("Profile '%s' configured for this project\n", profileName)
		return nil
	}

	// 3. Check if mise command exists
	if commandExists("mise") {
		fmt.Printf("No mise.toml or .envrc found in current directory.\n")
		fmt.Printf("Create mise.toml with Claude profile env vars? [Y/n] ")

		var response string
		fmt.Scanln(&response)
		if response == "" || response == "y" || response == "Y" {
			// Create minimal mise.toml with env vars
			content := "[env]\n"
			for k, v := range envVars {
				content += fmt.Sprintf("%s = \"%s\"\n", k, v)
			}
			if err := os.WriteFile("mise.toml", []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to create mise.toml: %w", err)
			}
			fmt.Printf("Created mise.toml with Claude profile env vars\n")
			fmt.Printf("Profile '%s' configured for this project\n", profileName)
			return nil
		}
	}

	// 4. Fallback: print shell exports
	fmt.Printf("No mise.toml or .envrc found.\n\n")
	fmt.Printf("Add to your shell or run:\n")
	for k, v := range envVars {
		fmt.Printf("  export %s=\"%s\"\n", k, v)
	}
	return nil
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
