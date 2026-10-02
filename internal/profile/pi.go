package profile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// pi (the pi coding agent) appends <agent-dir>/APPEND_SYSTEM.md to its system
// prompt, which is the one profile part ccp carries there: the output style.
// pi has no output styles of its own.

// PiAgentDir returns pi's user-level agent directory: PI_CODING_AGENT_DIR when
// set, else ~/.pi/agent — the same rule pi uses.
func PiAgentDir() string {
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent")
}

// PiOptedIn reports whether ccp has synced to pi before: the test for following
// automatically. It is recorded in the carrier ledger, not read off
// APPEND_SYSTEM.md, because a profile without an output style removes that file
// and must still be followed.
func PiOptedIn(paths *config.Paths) bool {
	ledger, err := loadCarrierLedger(paths)
	return err == nil && ledger.follows(carrierGroupPi)
}

// PiSync writes the profile's output style into pi's APPEND_SYSTEM.md, or
// removes ccp's file when the profile selects none, and opts in to following.
// It returns lines for the report, none when nothing changed.
func PiSync(paths *config.Paths, p *Profile) ([]string, error) {
	dir := PiAgentDir()
	if dir == "" {
		return nil, fmt.Errorf("cannot locate pi's agent directory")
	}
	lines, _, err := syncStyleCarriers(paths, p, carrierGroupPi, filepath.Join(dir, "APPEND_SYSTEM.md"))
	if err != nil {
		return lines, err
	}
	_, err = updateCarrierLedger(paths, func(ledger *carrierLedger) error {
		if !ledger.follows(carrierGroupPi) {
			ledger.Follow = append(ledger.Follow, carrierGroupPi)
		}
		return nil
	})
	return lines, err
}

// PiFollowProfile re-syncs after the active profile changed, once the user has
// opted in by running 'ccp pi sync'.
func PiFollowProfile(paths *config.Paths, p *Profile) ([]string, error) {
	if !PiOptedIn(paths) {
		return nil, nil
	}
	return PiSync(paths, p)
}
