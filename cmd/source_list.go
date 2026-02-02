package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/source"
)

var sourceListJSON bool

var sourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sources",
	RunE:  runSourceList,
}

func init() {
	sourceListCmd.Flags().BoolVarP(&sourceListJSON, "json", "j", false, "Output as JSON")
	sourceCmd.AddCommand(sourceListCmd)
}

func runSourceList(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	entries := registry.ListSources()
	if len(entries) == 0 {
		if sourceListJSON {
			fmt.Println("[]")
			return nil
		}
		fmt.Println("No sources installed")
		fmt.Println()
		fmt.Println("Add a source with:")
		fmt.Println("  ccp source add <package>")
		fmt.Println("  ccp source find <query>")
		return nil
	}

	// JSON output
	if sourceListJSON {
		type sourceJSON struct {
			ID           string   `json:"id"`
			Provider     string   `json:"provider"`
			URL          string   `json:"url"`
			Ref          string   `json:"ref,omitempty"`
			Commit       string   `json:"commit,omitempty"`
			Installed    []string `json:"installed"`
			InstalledCnt int      `json:"installed_count"`
			Updated      string   `json:"updated"`
		}

		var output []sourceJSON
		for _, entry := range entries {
			sj := sourceJSON{
				ID:           entry.ID,
				Provider:     entry.Source.Provider,
				URL:          entry.Source.URL,
				Ref:          entry.Source.Ref,
				Commit:       entry.Source.Commit,
				Installed:    entry.Source.Installed,
				InstalledCnt: len(entry.Source.Installed),
				Updated:      entry.Source.Updated.Format("2006-01-02"),
			}
			output = append(output, sj)
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "SOURCE\tPROVIDER\tINSTALLED\tUPDATED\n")

	for _, entry := range entries {
		updated := entry.Source.Updated.Format("2006-01-02")
		fmt.Fprintf(w, "%s\t%s\t%d items\t%s\n",
			entry.ID,
			entry.Source.Provider,
			len(entry.Source.Installed),
			updated,
		)
	}
	w.Flush()

	return nil
}
