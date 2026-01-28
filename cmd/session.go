package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var sessionCmd = &cobra.Command{
	Use:   "session <profile>",
	Short: "Start a shell session with a profile active",
	Long: `Start a new shell with CLAUDE_CONFIG_DIR set to the specified profile.

Exit the shell to return to your previous environment.

Examples:
  ccp session dev
  ccp session minimal`,
	Args: cobra.ExactArgs(1),
	RunE: runSession,
}

func init() {
	rootCmd.AddCommand(sessionCmd)
}

func runSession(cmd *cobra.Command, args []string) error {
	profileName := args[0]

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

	// Get user's shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	fmt.Printf("Starting session with profile: %s\n", profileName)
	fmt.Printf("Exit the shell to return to your previous environment.\n\n")

	// Execute shell with CLAUDE_CONFIG_DIR set
	execCmd := exec.Command(shell)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = append(os.Environ(),
		fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", profilePath),
		fmt.Sprintf("CCP_SESSION=%s", profileName),
	)

	return execCmd.Run()
}
