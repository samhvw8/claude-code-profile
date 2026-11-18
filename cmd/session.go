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
	Use:    "session [<profile>]",
	Hidden: true,
	Short:  "Start a shell session with a profile active",
	Long: `Start a new shell with CLAUDE_CONFIG_DIR set to the specified profile.

Exit the shell to return to your previous environment.
With --account, CLAUDE_CONFIG_DIR is the account folder ~/.ccp/accounts/<name>,
which keeps its own Claude login and links everything else to the profile.
With a named account the profile may be left out; --account auto picks the
account with the most plan limit left (see 'ccp run --help').

Examples:
  ccp session dev
  ccp session --account work1
  ccp session growtrics --account auto`,
	Args: cobra.RangeArgs(0, 1),
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
	cmd.SilenceUsage = true
	profileName := ""
	if len(args) > 0 {
		profileName = args[0]
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	accountSet := cmd.Flags().Changed("account")
	if accountSet && sessionAccount == "" {
		return profile.ValidateAccountName("")
	}
	profileName, err = activationProfile(paths, profileName, sessionAccount)
	if err != nil {
		return err
	}
	account, err := chooseAccount(paths, profileName, sessionAccount, sessionRelink)
	if err != nil {
		return err
	}
	configDir, err := activationDir(paths, profileName, account, accountSet, sessionRelink)
	if err != nil {
		return err
	}
	if account != "" {
		markAccountUsed(configDir)
	}

	// Get user's shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	if account != "" {
		fmt.Printf("Starting session with profile: %s (account %s)\n", profileName, account)
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
