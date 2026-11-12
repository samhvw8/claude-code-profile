package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var agyCmd = &cobra.Command{
	Use:    "agy",
	Short:  "Mirror a profile into Antigravity CLI (agy)",
	Hidden: true,
	Long: `Mirror a ccp profile into Antigravity CLI (agy), whose global config is
~/.gemini/config.

agy does not read ~/.claude, so ccp sets it up from the profile:
  AGENTS.md   a link to the profile's CLAUDE.md (agy loads it always-on)
  GEMINI.md   the body of the output style the profile's outputStyle selects
  rules/      converted copies: paths becomes a glob trigger, the rest always_on
  agents/     converted copies: agy's tool names, subagent mode, flash/pro models
  skills/     links to the hub items behind the profile's skills

ccp records the files it writes and only ever replaces or removes those: a file
you wrote, or one of ccp's you edited by hand, is left alone and reported. A
rules/, agents/ or skills/ directory that is itself a symlink is not written
through. Hooks are not mirrored (agy has its own hook schema and tool names),
and agy has no commands.

After the first sync, ccp keeps agy current on 'ccp use -g', on linking or
unlinking in the active profile, and on 'ccp bootstrap'.`,
}

var agySyncCmd = &cobra.Command{
	Use:   "sync [profile]",
	Short: "Mirror a profile's context, rules, agents, skills and output style into agy (default: active profile)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAgySync,
}

func init() {
	agyCmd.AddCommand(agySyncCmd)
	rootCmd.AddCommand(agyCmd)
}

func runAgySync(cmd *cobra.Command, args []string) error {
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

	result, err := profile.AgySync(paths, p)
	if err != nil {
		return err
	}
	fmt.Printf("Mirrored profile '%s' into %s\n", p.Name, profile.AgyConfigDir())
	warnInactiveCarrier(paths, p)
	reportAgyResult(result, true)
	return nil
}

// reportAgyResult prints a sync's outcome. verbose adds the standing notes
// (hooks, commands) that a follow-up after another command should not repeat.
func reportAgyResult(r profile.AgyResult, verbose bool) {
	line := func(label string, items []string) {
		if len(items) > 0 {
			fmt.Printf("  %s: %s\n", label, strings.Join(items, ", "))
		}
	}
	line("written", r.Written)
	line("linked", r.Linked)
	line("removed", r.Removed)
	line("left alone (edited by hand since ccp wrote it)", r.Edited)
	line("left alone (not made by ccp — remove it to let ccp manage it)", r.Occupied)
	line("skipped", r.Skipped)
	for _, s := range append(r.Notes, r.Style...) {
		fmt.Printf("  %s\n", s)
	}
	if verbose {
		if len(r.Written)+len(r.Linked)+len(r.Removed)+len(r.Style) == 0 {
			fmt.Println("  already up to date")
		}
		if len(r.Hooks) > 0 {
			fmt.Printf("  not mirrored: hooks (%s) — agy has its own hook schema and tool names\n", strings.Join(r.Hooks, ", "))
		}
		if len(r.Commands) > 0 {
			fmt.Printf("  not mirrored: commands (%s) — agy has no commands\n", strings.Join(r.Commands, ", "))
		}
	}
}

// followAgy re-syncs agy after the given profile changed, when the user has
// opted in and that profile is the active one. Failures are warnings: the
// change itself already happened.
func followAgy(paths *config.Paths, profileName string) {
	if !profile.AgyOptedIn(paths) {
		return
	}
	active, err := profile.NewManager(paths).GetActive()
	if err != nil || active == nil || active.Name != profileName {
		return
	}
	result, _, err := profile.AgyFollowProfile(paths, active)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not sync agy: %v\n", err)
		return
	}
	if len(result.Written)+len(result.Linked)+len(result.Removed)+len(result.Edited)+len(result.Occupied)+
		len(result.Skipped)+len(result.Notes)+len(result.Style) > 0 {
		fmt.Println("agy:")
		reportAgyResult(result, false)
	}
}
