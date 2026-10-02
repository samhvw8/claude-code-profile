package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// Commands that change hub items in place must follow the active profile, or
// the other harnesses keep a renamed or rewritten style's old text.
func TestHubChangingCommandsFollow(t *testing.T) {
	for _, cmd := range []*cobra.Command{hubRenameCmd, hubRemoveCmd, hubUpdateCmd, installCmd, sourceInstallCmd, sourceUpdateCmd} {
		if cmd.PostRun == nil {
			t.Errorf("%s does not follow the active profile", cmd.CommandPath())
		}
	}
}
