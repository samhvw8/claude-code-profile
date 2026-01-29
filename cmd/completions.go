package cmd

import (
	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
)

// completeProfileNames returns a completion function that lists profile names
func completeProfileNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	paths, err := config.ResolvePaths()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	if !paths.IsInitialized() {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	mgr := profile.NewManager(paths)
	profiles, err := mgr.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, p := range profiles {
		names = append(names, p.Name)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeHubItems returns a completion function that lists hub items in type/name format
func completeHubItems(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	paths, err := config.ResolvePaths()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	if !paths.IsInitialized() {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var items []string
	for itemType, hubItems := range h.Items {
		for _, item := range hubItems {
			items = append(items, string(itemType)+"/"+item.Name)
		}
	}

	return items, cobra.ShellCompDirectiveNoFileComp
}

// completeHubTypes returns completion for hub item types
func completeHubTypes(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	types := []string{"skills", "agents", "hooks", "rules", "commands", "setting-fragments"}
	return types, cobra.ShellCompDirectiveNoFileComp
}

// completeLinkArgs returns completion for link/unlink commands
// First arg: profile names, Second arg: hub items
func completeLinkArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// Complete profile names for first argument
		return completeProfileNames(cmd, args, toComplete)
	}
	if len(args) == 1 {
		// Complete hub items for second argument
		return completeHubItems(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
