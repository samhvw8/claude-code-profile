package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var templateEditCmd = &cobra.Command{
	Use:               "edit <name>",
	Short:             "Edit a settings template in $EDITOR",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTemplateNames,
	RunE:              runTemplateEdit,
}

func init() {
	templateCmd.AddCommand(templateEditCmd)
}

func runTemplateEdit(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	mgr := hub.NewTemplateManager(paths.HubDir)
	t, err := mgr.Load(name)
	if err != nil {
		return fmt.Errorf("template not found: %s", name)
	}

	settings, err := editSettingsInEditor(t.Settings)
	if err != nil {
		return err
	}

	// Remove hooks if present
	delete(settings, "hooks")

	t.Settings = settings
	if err := mgr.Save(t); err != nil {
		return fmt.Errorf("failed to save template: %w", err)
	}

	fmt.Printf("Updated template: %s (%d settings keys)\n", name, len(settings))
	return nil
}
