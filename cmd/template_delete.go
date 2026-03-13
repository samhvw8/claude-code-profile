package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var templateDeleteCmd = &cobra.Command{
	Use:               "delete <name>",
	Aliases:           []string{"rm", "remove"},
	Short:             "Delete a settings template",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTemplateNames,
	RunE:              runTemplateDelete,
}

func init() {
	templateCmd.AddCommand(templateDeleteCmd)
}

func runTemplateDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	mgr := hub.NewTemplateManager(paths.HubDir)
	if !mgr.Exists(name) {
		return fmt.Errorf("template not found: %s", name)
	}

	if err := mgr.Delete(name); err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	fmt.Printf("Deleted template: %s\n", name)
	return nil
}
