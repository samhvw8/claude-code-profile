package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var profileSyncCmd = &cobra.Command{
	Use:   "sync [name]",
	Short: "Regenerate profile symlinks and settings.json",
	Long: `Sync a profile by regenerating hub item symlinks and settings.json based on profile.toml.

This command is useful when:
- Hub items have been added/removed and you want to update the profile
- Settings.json hooks are out of sync with the hub
- Symlinks are broken or missing

If no profile name is given, syncs the active profile.

Examples:
  ccp profile sync           # Sync active profile
  ccp profile sync default   # Sync the 'default' profile
  ccp profile sync --all     # Sync all profiles

settings.json is regenerated from the settings template, settings-fragment.json
and linked hub hooks. If it holds edits that regeneration would drop (made by
hand or by Claude Code since the last 'ccp profile capture'), only its hooks are
updated and the affected keys are listed. Capture them to keep them, or pass
--force to discard them.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runProfileSync,
}

var (
	syncAll   bool
	syncForce bool
)

func init() {
	profileSyncCmd.Flags().BoolVar(&syncAll, "all", false, "Sync all profiles")
	profileSyncCmd.Flags().BoolVarP(&syncForce, "force", "f", false, "Regenerate settings.json even if it discards edits not captured in settings-fragment.json")
	profileCmd.AddCommand(profileSyncCmd)
}

func runProfileSync(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	if syncAll {
		// Sync all profiles
		profiles, err := mgr.List()
		if err != nil {
			return fmt.Errorf("failed to list profiles: %w", err)
		}

		for _, p := range profiles {
			fmt.Printf("Syncing profile: %s\n", p.Name)
			if err := syncProfile(paths, p, syncForce); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
			} else {
				fmt.Println("  Done")
			}
		}
		return nil
	}

	// Get target profile
	var profileName string
	if len(args) > 0 {
		profileName = args[0]
	} else {
		// Use active profile
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return fmt.Errorf("no active profile and no profile name specified")
		}
		profileName = active.Name
	}

	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	fmt.Printf("Syncing profile: %s\n", p.Name)
	if err := syncProfile(paths, p, syncForce); err != nil {
		return err
	}
	fmt.Println("Done")

	return nil
}

func syncProfile(paths *config.Paths, p *profile.Profile, force bool) error {
	if err := profile.SyncLinks(paths, p, func(format string, args ...any) { fmt.Printf(format, args...) }); err != nil {
		return err
	}

	// Regenerate settings.json
	hasFragment := profile.FragmentExists(p.Path)
	hasSources := len(p.Manifest.Hub.Hooks) > 0 || len(p.Manifest.Hub.Bundles) > 0 || len(p.Manifest.Hooks) > 0 ||
		p.Manifest.SettingsTemplate != "" || hasFragment || profile.SettingsHaveHooks(p.Path)

	if hasSources {
		changed, err := profile.SettingsChanged(paths, p.Path, p.Manifest)
		if err != nil {
			return fmt.Errorf("failed to check settings: %w", err)
		}

		if changed {
			uncaptured, err := profile.ApplySettings(paths, p.Path, p.Manifest, force)
			if err != nil {
				return fmt.Errorf("failed to update settings.json: %w", err)
			}
			if len(uncaptured) > 0 {
				fmt.Println("  Synced settings.json hooks; kept the other keys")
				reportUncaptured(p.Name, uncaptured)
				return nil
			}
			fmt.Println("  Regenerated settings.json")
			if p.Manifest.SettingsTemplate != "" {
				fmt.Printf("  Applied settings template: %s\n", p.Manifest.SettingsTemplate)
			}
			if hasFragment {
				fmt.Println("  Applied settings fragment")
			}
			if n := profile.CountHookEntries(paths, p.Path, p.Manifest); n > 0 {
				fmt.Printf("  Configured %d hook entries from hub hooks and bundles\n", n)
			}
		} else {
			fmt.Println("  Settings up to date")
		}
	}
	return nil
}

// reportUncaptured explains why settings.json was not fully regenerated.
func reportUncaptured(profileName string, keys []string) {
	fmt.Printf("  settings.json has edits not captured in settings-fragment.json: %s\n", strings.Join(keys, ", "))
	fmt.Println("  Template and fragment changes are not applied until these are captured or discarded.")
	fmt.Printf("  → keep them: ccp profile capture %s && ccp profile sync %s\n", profileName, profileName)
	fmt.Printf("  → discard them: ccp profile sync %s --force\n", profileName)
}

// noteUncaptured is reportUncaptured in one line, for commands where settings
// are a side effect.
func noteUncaptured(profileName string, keys []string) {
	fmt.Printf("Note: kept settings.json edits not in settings-fragment.json (%s); updated hooks only, template and fragment changes wait — 'ccp profile capture %s' saves them\n",
		strings.Join(keys, ", "), profileName)
}
