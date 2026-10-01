package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var opencodeCmd = &cobra.Command{
	Use:    "opencode",
	Short:  "Mirror a profile into opencode",
	Hidden: true,
	Long: `Mirror a ccp profile into opencode (~/.config/opencode).

opencode already reads ~/.claude/CLAUDE.md and ~/.claude/skills/, which follow
the active profile, and takes rules through the "instructions" list in its
opencode.json — point it at "{env:HOME}/.claude/rules/*.md". What it cannot read
is agents and commands, whose frontmatter differs from Claude Code's, so ccp
writes converted copies into opencode's agent/ and command/ directories.

ccp records the files it writes and only ever replaces or removes those: a file
you wrote, or one of ccp's you edited by hand, is left alone and reported.
Hooks are not mirrored — opencode runs hooks as plugins.

After the first sync, ccp keeps opencode current on 'ccp use -g', on linking or
unlinking in the active profile, and on 'ccp bootstrap'.`,
}

var opencodeSyncCmd = &cobra.Command{
	Use:   "sync [profile]",
	Short: "Mirror a profile's agents and commands into opencode (default: active profile)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runOpencodeSync,
}

func init() {
	opencodeCmd.AddCommand(opencodeSyncCmd)
	rootCmd.AddCommand(opencodeCmd)
}

func runOpencodeSync(cmd *cobra.Command, args []string) error {
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

	result, err := profile.OpencodeSync(paths, p)
	if err != nil {
		return err
	}
	fmt.Printf("Mirrored profile '%s' into %s\n", p.Name, profile.OpencodeConfigDir())
	reportOpencodeResult(result, true)
	return nil
}

// reportOpencodeResult prints a sync's outcome. verbose adds the standing notes
// (hooks) that a follow-up after another command should not repeat.
func reportOpencodeResult(r profile.OpencodeResult, verbose bool) {
	line := func(label string, items []string) {
		if len(items) > 0 {
			fmt.Printf("  %s: %s\n", label, strings.Join(items, ", "))
		}
	}
	line("written", r.Written)
	line("removed", r.Removed)
	line("left alone (edited by hand since ccp wrote it)", r.Edited)
	line("left alone (not written by ccp — remove it to let ccp manage it)", r.Occupied)
	line("skipped", r.Skipped)
	if verbose {
		if len(r.Written)+len(r.Removed) == 0 {
			fmt.Println("  already up to date")
		}
		if len(r.Hooks) > 0 {
			fmt.Printf("  not mirrored: hooks (%s) — opencode runs hooks as plugins\n", strings.Join(r.Hooks, ", "))
		}
	}
}

// followOpencode re-syncs opencode after the given profile changed, when the
// user has opted in and that profile is the active one. Failures are warnings:
// the change itself already happened.
func followOpencode(paths *config.Paths, profileName string) {
	if !profile.OpencodeOptedIn(paths) {
		return
	}
	active, err := profile.NewManager(paths).GetActive()
	if err != nil || active == nil || active.Name != profileName {
		return
	}
	result, _, err := profile.OpencodeFollowProfile(paths, active)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not sync opencode: %v\n", err)
		return
	}
	if len(result.Written)+len(result.Removed)+len(result.Edited)+len(result.Occupied)+len(result.Skipped) > 0 {
		fmt.Println("opencode:")
		reportOpencodeResult(result, false)
	}
}
