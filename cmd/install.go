package cmd

import (
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:     "install [package] [items...]",
	Aliases: []string{"i"},
	Short:   "Install skills, agents, and more from packages",
	Long: `Install items from a package (GitHub repo or skills.sh).

If the package is not already added, it will be added automatically.
If no items are specified, interactive selection is shown.

When called without arguments, syncs all sources from ccp.toml:
- Clones missing sources
- Reinstalls items listed in registry

Examples:
  ccp install                           # Sync all from ccp.toml
  ccp install remorses/playwriter       # Auto-add, interactive selection
  ccp install owner/repo skills/my-skill
  ccp install owner/repo --all`,
	Args: cobra.MinimumNArgs(0),
	RunE: runSourceInstall,
}

func init() {
	installCmd.Flags().BoolVarP(&sourceInstallAll, "all", "a", false, "Install all available items")
	installCmd.Flags().BoolVarP(&sourceInstallInteractive, "interactive", "i", false, "Interactive item selection")
	rootCmd.AddCommand(installCmd)
}
