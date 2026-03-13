package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
)

var templateExtractFrom string

var templateExtractCmd = &cobra.Command{
	Use:   "extract <name>",
	Short: "Extract a settings template from a profile",
	Long: `Extract a settings.json template from an existing profile's settings.

Hooks are excluded from the template (they are managed separately).

Examples:
  ccp template extract my-template                    # From active profile
  ccp template extract my-template --from=default    # From specific profile`,
	Args: cobra.ExactArgs(1),
	RunE: runTemplateExtract,
}

func init() {
	templateExtractCmd.Flags().StringVar(&templateExtractFrom, "from", "", "Profile to extract from (default: active profile)")
	templateCmd.AddCommand(templateExtractCmd)
}

func runTemplateExtract(cmd *cobra.Command, args []string) error {
	name := args[0]

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

	// Determine source profile
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
	settings, err := hub.ExtractFromSettings(settingsPath)
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
