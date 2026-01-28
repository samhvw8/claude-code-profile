package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var hubCmd = &cobra.Command{
	Use:   "hub",
	Short: "Manage the hub of reusable components",
	Long:  `The hub contains all reusable skills, hooks, rules, commands, and md-fragments.`,
}

var hubListCmd = &cobra.Command{
	Use:   "list [type]",
	Short: "List hub contents",
	Long: `List all items in the hub, optionally filtered by type.

Types: skills, hooks, rules, commands, md-fragments`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHubList,
}

func init() {
	rootCmd.AddCommand(hubCmd)
	hubCmd.AddCommand(hubListCmd)
}

func runHubList(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// Filter by type if specified
	var typesToShow []config.HubItemType
	if len(args) > 0 {
		itemType := config.HubItemType(args[0])
		// Validate type
		valid := false
		for _, t := range config.AllHubItemTypes() {
			if t == itemType {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid type: %s (valid: skills, hooks, rules, commands, md-fragments)", args[0])
		}
		typesToShow = []config.HubItemType{itemType}
	} else {
		typesToShow = config.AllHubItemTypes()
	}

	// Print results
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	for _, itemType := range typesToShow {
		items := h.GetItems(itemType)
		if len(items) == 0 {
			continue
		}

		fmt.Fprintf(w, "\n%s:\n", itemType)
		for _, item := range items {
			typeIndicator := "file"
			if item.IsDir {
				typeIndicator = "dir"
			}
			fmt.Fprintf(w, "  %s\t(%s)\n", item.Name, typeIndicator)
		}
	}

	w.Flush()

	if h.ItemCount() == 0 {
		fmt.Println("Hub is empty")
	}

	return nil
}
