package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var engineListJSON bool

var engineListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all engines",
	RunE:    runEngineList,
}

func init() {
	engineListCmd.Flags().BoolVarP(&engineListJSON, "json", "j", false, "Output as JSON")
	engineCmd.AddCommand(engineListCmd)
}

func runEngineList(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewEngineManager(paths)
	engines, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list engines: %w", err)
	}

	if len(engines) == 0 {
		fmt.Println("No engines found. Create one with 'ccp engine create <name>'")
		return nil
	}

	if engineListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(engines)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tDESCRIPTION\tFRAGMENTS\tHOOKS\n")

	for _, e := range engines {
		desc := e.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		fragments := strings.Join(e.Hub.SettingFragments, ", ")
		hooks := strings.Join(e.Hub.Hooks, ", ")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, desc, fragments, hooks)
	}

	w.Flush()
	return nil
}
