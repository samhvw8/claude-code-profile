package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var sessionCmd = &cobra.Command{
	Use:    "session <profile>",
	Hidden: true,
	Short:  "Start a shell session with a profile active",
	Long: `Start a new shell with CLAUDE_CONFIG_DIR set to the specified profile.

Exit the shell to return to your previous environment.
With --account, CLAUDE_CONFIG_DIR is the account folder ~/.ccp/accounts/<name>,
which keeps its own Claude login and links everything else to the profile.

Examples:
  ccp session dev
  ccp session minimal
  ccp session default --account work1`,
	Args: cobra.ExactArgs(1),
	RunE: runSession,
}

var (
	sessionAccount string
	sessionRelink  bool
)

func init() {
	addAccountFlags(sessionCmd, &sessionAccount, &sessionRelink)
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

	configDir, err := activationDir(paths, profileName, sessionAccount, sessionRelink)
	if err != nil {
		return err
	}

	// Get user's shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	if sessionAccount != "" {
		fmt.Printf("Starting session with profile: %s (account %s)\n", profileName, sessionAccount)
	} else {
		fmt.Printf("Starting session with profile: %s\n", profileName)
	}
	fmt.Printf("Exit the shell to return to your previous environment.\n\n")

	// Execute shell with CLAUDE_CONFIG_DIR set
	execCmd := exec.Command(shell)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = append(os.Environ(),
		fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", configDir),
		fmt.Sprintf("CCP_SESSION=%s", profileName),
	)

	return execCmd.Run()
}
