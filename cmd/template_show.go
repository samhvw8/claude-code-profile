package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var templateShowCmd = &cobra.Command{
	Use:               "show <name>",
	Short:             "Show a settings template",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeTemplateNames,
	RunE:              runTemplateShow,
}

func init() {
	templateCmd.AddCommand(templateShowCmd)
}

func runTemplateShow(cmd *cobra.Command, args []string) error {
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

	data, err := json.MarshalIndent(t.Settings, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func completeTemplateNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	paths, err := config.ResolvePaths()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	mgr := hub.NewTemplateManager(paths.HubDir)
	templates, err := mgr.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, t := range templates {
		names = append(names, t.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
