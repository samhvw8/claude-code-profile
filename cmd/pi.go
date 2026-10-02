package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var piCmd = &cobra.Command{
	Use:    "pi",
	Short:  "Carry a profile's output style into pi",
	Hidden: true,
	Long: `Carry a ccp profile's output style into pi (~/.pi/agent, or
PI_CODING_AGENT_DIR).

pi has no output styles, but it appends APPEND_SYSTEM.md in its agent directory
to every system prompt. ccp writes the body of the style the profile's
outputStyle setting selects into that file, and removes it when the profile
selects none or a built-in style. A file ccp did not write is never touched.

After the first sync, ccp keeps pi current whenever the active profile changes:
'ccp use -g', link and unlink, 'profile edit', 'profile sync' and 'ccp bootstrap'.
A carrier ccp wrote earlier under another PI_CODING_AGENT_DIR is kept current
too, and one you edited by hand is left alone.`,
}

var piSyncCmd = &cobra.Command{
	Use:   "sync [profile]",
	Short: "Write a profile's output style into pi (default: active profile)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPiSync,
}

func init() {
	piCmd.AddCommand(piSyncCmd)
	rootCmd.AddCommand(piCmd)
}

func runPiSync(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}
	mgr := profile.NewManager(paths)
	var p *profile.Profile
	if len(args) == 1 {
		p, err = mgr.Get(args[0])
	} else {
		p, err = mgr.GetActive()
	}
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("profile not found (no active profile, or no such name)")
	}

	lines, err := profile.PiSync(paths, p)
	if err != nil {
		return err
	}
	fmt.Printf("Synced profile '%s' into %s\n", p.Name, profile.PiAgentDir())
	warnInactiveCarrier(paths, p)
	if len(lines) == 0 {
		lines = []string{"already up to date"}
	}
	for _, line := range lines {
		fmt.Printf("  %s\n", line)
	}
	return nil
}
