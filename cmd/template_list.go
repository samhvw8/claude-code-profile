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

var templateListJSON bool

var templateListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List available settings templates",
	RunE:    runTemplateList,
}

func init() {
	templateListCmd.Flags().BoolVar(&templateListJSON, "json", false, "Output as JSON")
	templateCmd.AddCommand(templateListCmd)
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := hub.NewTemplateManager(paths.HubDir)
	templates, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}

	if len(templates) == 0 {
		fmt.Println("No settings templates found")
		fmt.Println("\nCreate one with:")
		fmt.Println("  ccp template create <name>")
		fmt.Println("  ccp template extract <name> --from <profile>")
		return nil
	}

	if templateListJSON {
		type jsonTemplate struct {
			Name string   `json:"name"`
			Keys []string `json:"keys"`
		}
		var output []jsonTemplate
		for _, t := range templates {
			var keys []string
			for k := range t.Settings {
				keys = append(keys, k)
			}
			output = append(output, jsonTemplate{Name: t.Name, Keys: keys})
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tKEYS")
	for _, t := range templates {
		fmt.Fprintf(w, "%s\t%d\n", t.Name, len(t.Settings))
	}
	w.Flush()
	return nil
}
