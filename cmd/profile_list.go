package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var profileListJSON bool

var profileListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all profiles",
	Long:    `List all profiles and indicate which one is currently active.`,
	RunE:    runProfileList,
}

func init() {
	profileListCmd.Flags().BoolVarP(&profileListJSON, "json", "j", false, "Output as JSON")
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

	// JSON output
	if profileListJSON {
		type profileJSON struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Path        string `json:"path"`
			Active      bool   `json:"active"`
			ActiveEnv   bool   `json:"active_env,omitempty"`
		}

		var output []profileJSON
		for _, p := range profiles {
			pj := profileJSON{
				Name:        p.Name,
				Description: p.Manifest.Description,
				Path:        p.Path,
				Active:      p.Name == activeName,
			}
			if envProfile != "" && envProfile == p.Path {
				pj.ActiveEnv = true
			}
			output = append(output, pj)
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

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
