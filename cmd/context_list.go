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

var contextListJSON bool

var contextListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all contexts",
	RunE:    runContextList,
}

func init() {
	contextListCmd.Flags().BoolVarP(&contextListJSON, "json", "j", false, "Output as JSON")
	contextCmd.AddCommand(contextListCmd)
}

func runContextList(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewContextManager(paths)
	contexts, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list contexts: %w", err)
	}

	if len(contexts) == 0 {
		fmt.Println("No contexts found. Create one with 'ccp context create <name>'")
		return nil
	}

	if contextListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(contexts)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tDESCRIPTION\tSKILLS\tAGENTS\n")

	for _, c := range contexts {
		desc := c.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		skills := strings.Join(c.Hub.Skills, ", ")
		agents := strings.Join(c.Hub.Agents, ", ")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Name, desc, skills, agents)
	}

	w.Flush()
	return nil
}
