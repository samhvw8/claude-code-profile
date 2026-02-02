package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var profileListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all profiles",
	Long:    `List all profiles and indicate which one is currently active.`,
	RunE:    runProfileList,
}

func init() {
	profileCmd.AddCommand(profileListCmd)
}

func runProfileList(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	profiles, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles found")
		return nil
	}

	// Get active profile
	active, _ := mgr.GetActive()
	activeName := ""
	if active != nil {
		activeName = active.Name
	}

	// Also check CLAUDE_CONFIG_DIR
	envProfile := os.Getenv("CLAUDE_CONFIG_DIR")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tDESCRIPTION\tSTATUS\n")

	for _, p := range profiles {
		status := ""
		if p.Name == activeName {
			status = "active (symlink)"
		}
		if envProfile != "" && envProfile == p.Path {
			if status != "" {
				status += ", "
			}
			status += "active (env)"
		}

		desc := p.Manifest.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, desc, status)
	}

	w.Flush()
	return nil
}
