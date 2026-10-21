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

An installed item is a shadow: a symlink into the package's clone, so
'ccp source update' updates it in place. To edit an item, install it with
--clone: ccp copies it into the hub as a local item and stops tracking it.

When called without arguments, syncs all sources from ccp.toml:
- Clones missing sources at the commit ccp.toml records
- Recreates missing shadow links

Examples:
  ccp install                           # Sync all from ccp.toml
  ccp install remorses/playwriter       # Auto-add, interactive selection
  ccp install owner/repo skills/my-skill
  ccp install owner/repo --all
  ccp install owner/repo skills/my-skill --link .        # ...and link to the active profile
  ccp install owner/repo skills/my-skill --link work     # ...and link to 'work'
  ccp install owner/repo skills/my-skill --clone         # local copy to edit`,
	Args: cobra.MinimumNArgs(0),
	RunE: runSourceInstall,
}

func init() {
	installCmd.Flags().BoolVarP(&sourceInstallAll, "all", "a", false, "Install all available items")
	installCmd.Flags().BoolVarP(&sourceInstallInteractive, "interactive", "i", false, "Interactive item selection")
	addInstallLinkFlag(installCmd)
	rootCmd.AddCommand(installCmd)
}
