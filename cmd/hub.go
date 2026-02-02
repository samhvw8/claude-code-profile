package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var hubListJSON bool

var hubCmd = &cobra.Command{
	Use:     "hub",
	Aliases: []string{"h"},
	Short:   "Manage the hub of reusable components",
	Long:    `The hub contains all reusable skills, agents, hooks, rules, commands, and setting-fragments.`,
}

var hubListCmd = &cobra.Command{
	Use:     "list [type]",
	Aliases: []string{"ls", "l"},
	Short:   "List hub contents",
	Long: `List all items in the hub, optionally filtered by type.

Types: skills, agents, hooks, rules, commands, setting-fragments`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHubList,
}

func init() {
	rootCmd.AddCommand(hubCmd)
	hubListCmd.Flags().BoolVarP(&hubListJSON, "json", "j", false, "Output as JSON")
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
			return fmt.Errorf("invalid type: %s (valid: skills, agents, hooks, rules, commands, setting-fragments)", args[0])
		}
		typesToShow = []config.HubItemType{itemType}
	} else {
		typesToShow = config.AllHubItemTypes()
	}

	// Print results
	// JSON output
	if hubListJSON {
		type hubItemJSON struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			IsDir bool   `json:"is_dir"`
		}

		var output []hubItemJSON
		for _, itemType := range typesToShow {
			items := h.GetItems(itemType)
			for _, item := range items {
				output = append(output, hubItemJSON{
					Type:  string(itemType),
					Name:  item.Name,
					IsDir: item.IsDir,
				})
			}
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

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
