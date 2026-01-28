package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var runCmd = &cobra.Command{
	Use:   "run <profile> -- <command> [args...]",
	Short: "Run a command with a specific profile active",
	Long: `Execute a command with CLAUDE_CONFIG_DIR set to the specified profile.

Examples:
  ccp run minimal -- claude "fix this bug"
  ccp run dev -- npm test
  ccp run quickfix -- bash -c "claude && git commit"`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE:               runRunCmd,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRunCmd(cmd *cobra.Command, args []string) error {
	// Parse args manually to handle -- separator
	profileName := ""
	var cmdArgs []string
	foundSeparator := false

	for _, arg := range args {
		if arg == "--" {
			foundSeparator = true
			continue
		}
		if !foundSeparator {
			if profileName == "" {
				profileName = arg
			}
		} else {
			cmdArgs = append(cmdArgs, arg)
		}
	}

	if profileName == "" {
		return fmt.Errorf("profile name required")
	}

	if len(cmdArgs) == 0 {
		return fmt.Errorf("command required after --")
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Verify profile exists
	mgr := profile.NewManager(paths)
	if !mgr.Exists(profileName) {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	profilePath := paths.ProfileDir(profileName)

	// Execute command with CLAUDE_CONFIG_DIR set
	execCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = append(os.Environ(), fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", profilePath))

	return execCmd.Run()
}
