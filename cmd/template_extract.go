package cmd

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	templateExtractFrom string
	templateExtractKeys []string
	templateExtractPick bool
)

var templateExtractCmd = &cobra.Command{
	Use:   "extract <name>",
	Short: "Extract a settings template from a profile",
	Long: `Extract a settings.json template from an existing profile's settings.

Hooks are excluded from the template (they are managed separately).

By default all top-level keys are included. Use --keys to cherry-pick
specific keys, or --pick to select interactively.

Examples:
  ccp template extract my-template                               # All keys from active profile
  ccp template extract my-template --from=default               # From specific profile
  ccp template extract base --keys=model,env,permissions         # Only listed keys
  ccp template extract base --pick                               # Interactive picker`,
	Args: cobra.ExactArgs(1),
	RunE: runTemplateExtract,
}

func init() {
	templateExtractCmd.Flags().StringVar(&templateExtractFrom, "from", "", "Profile to extract from (default: active profile)")
	templateExtractCmd.Flags().StringSliceVar(&templateExtractKeys, "keys", nil, "Comma-separated top-level keys to include (default: all except hooks)")
	templateExtractCmd.Flags().BoolVar(&templateExtractPick, "pick", false, "Interactively pick which keys to include")
	templateCmd.AddCommand(templateExtractCmd)
}

func runTemplateExtract(cmd *cobra.Command, args []string) error {
	name := args[0]

	if templateExtractPick && len(templateExtractKeys) > 0 {
		return fmt.Errorf("--pick and --keys are mutually exclusive")
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	tmplMgr := hub.NewTemplateManager(paths.HubDir)
	if tmplMgr.Exists(name) {
		return fmt.Errorf("template already exists: %s", name)
	}

	mgr := profile.NewManager(paths)

	var profileName string
	if templateExtractFrom != "" {
		profileName = templateExtractFrom
	} else {
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return fmt.Errorf("no active profile and no --from specified")
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

	settingsPath := filepath.Join(p.Path, "settings.json")

	keys := templateExtractKeys
	if templateExtractPick {
		all, err := hub.ExtractFromSettings(settingsPath)
		if err != nil {
			return fmt.Errorf("failed to read settings: %w", err)
		}
		if len(all) == 0 {
			fmt.Println("No settings found (or only hooks, which are managed separately)")
			return nil
		}
		items := make([]picker.Item, 0, len(all))
		for k := range all {
			items = append(items, picker.Item{ID: k, Label: k, Selected: true})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

		picked, err := picker.Run(fmt.Sprintf("Select keys for template '%s'", name), items)
		if err != nil {
			return fmt.Errorf("picker failed: %w", err)
		}
		if picked == nil {
			return fmt.Errorf("cancelled")
		}
		if len(picked) == 0 {
			return fmt.Errorf("no keys selected")
		}
		keys = picked
	}

	settings, err := hub.ExtractFromSettings(settingsPath, keys...)
	if err != nil {
		return fmt.Errorf("failed to extract settings: %w", err)
	}

	if len(settings) == 0 {
		fmt.Println("No settings found (or only hooks, which are managed separately)")
		return nil
	}

	t := &hub.Template{Name: name, Settings: settings}
	if err := tmplMgr.Save(t); err != nil {
		return fmt.Errorf("failed to save template: %w", err)
	}

	fmt.Printf("Extracted template '%s' from profile '%s' (%d settings keys)\n", name, profileName, len(settings))
	return nil
}
