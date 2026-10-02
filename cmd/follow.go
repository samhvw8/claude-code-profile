package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// followHarnesses brings every harness the user opted into in step after the
// given profile changed, when that profile is the active one. It is the one
// place a change reaches the other harnesses, so they all follow the same
// events. omp gets only its output style here: its links and context follow
// profile switches ('ccp use -g', bootstrap) on their own. Failures are
// warnings: the change itself already happened.
func followHarnesses(paths *config.Paths, profileName string) {
	// A corrupt output-style ledger is repaired before anything follows, so the
	// loss is reported once, here, rather than as a warning per harness.
	notes, err := profile.RepairStyleLedger(paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	for _, note := range notes {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", note)
	}

	followOpencode(paths, profileName)

	active, err := profile.NewManager(paths).GetActive()
	if err != nil || active == nil || active.Name != profileName {
		return
	}
	for _, h := range []struct {
		name   string
		follow func(*config.Paths, *profile.Profile) ([]string, error)
	}{
		{"pi", profile.PiFollowProfile},
		{"omp", profile.OmpRefreshStyle},
	} {
		lines, err := h.follow(paths, active)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not sync %s: %v\n", h.name, err)
			continue
		}
		for _, line := range lines {
			fmt.Printf("%s: %s\n", h.name, line)
		}
	}
}

// Commands that change hub items in place — renaming, removing or updating
// them, installing from a source — follow the active profile when they
// succeed: a renamed or rewritten style would otherwise keep loading its old
// text in the other harnesses until some unrelated command removed it.
func init() {
	for _, cmd := range []*cobra.Command{hubRenameCmd, hubRemoveCmd, hubUpdateCmd, installCmd, sourceInstallCmd, sourceUpdateCmd} {
		cmd.PostRun = followActive
	}
}

// followActive runs followHarnesses for the active profile, if there is one.
func followActive(*cobra.Command, []string) {
	paths, err := config.ResolvePaths()
	if err != nil || !paths.IsInitialized() {
		return
	}
	if active, err := profile.NewManager(paths).GetActive(); err == nil && active != nil {
		followHarnesses(paths, active.Name)
	}
}

// warnInactiveCarrier says, for a manual sync of a profile that is not the
// active one, that the global style carrier will not keep its style.
func warnInactiveCarrier(paths *config.Paths, p *profile.Profile) {
	active, err := profile.NewManager(paths).GetActive()
	if err == nil && active != nil && active.Name != p.Name {
		fmt.Printf("  note: output-style carriers follow the active profile ('%s'); its next change replaces '%s''s style\n",
			active.Name, p.Name)
	}
}
