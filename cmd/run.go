package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var runCmd = &cobra.Command{
	Use:    "run [<profile>] [--account <name>|auto [--relink]] [-- <command> [args...]]",
	Hidden: true,
	Short:  "Run a command with a specific profile active",
	Long: `Execute a command with CLAUDE_CONFIG_DIR set to the specified profile.

With --account, CLAUDE_CONFIG_DIR is the account folder ~/.ccp/accounts/<name>,
which keeps its own Claude login and links everything else to the profile.
The profile may then be left out: a named account runs its own profile.
--relink moves an account that is linked to another profile.

--account auto picks, among the accounts linked to the profile (the current
one when none is named), the one with the most plan limit left: Claude's usage
endpoint, read at most every 5 minutes per account, else the account started
least recently. The command defaults to claude.

Examples:
  ccp run --account work1                  # claude, account work1's profile
  ccp run --account auto -- claude -c      # continue on the freest account
  ccp run growtrics --account auto
  ccp run minimal -- claude "fix this bug"
  ccp run dev -- npm test`,
	DisableFlagParsing: true,
	RunE:               runRunCmd,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRunCmd(cmd *cobra.Command, args []string) error {
	ra, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	profileName, cmdArgs := ra.profile, ra.command
	// Past argument parsing an error is about profiles or accounts, not usage.
	cmd.SilenceUsage = true

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	if ra.accountSet && ra.account == "" {
		return profile.ValidateAccountName("")
	}
	profileName, err = activationProfile(paths, profileName, ra.account)
	if err != nil {
		return err
	}
	account, err := chooseAccount(paths, profileName, ra.account, ra.relink)
	if err != nil {
		return err
	}
	configDir, err := activationDir(paths, profileName, account, ra.accountSet, ra.relink)
	if err != nil {
		return err
	}
	if account != "" {
		markAccountUsed(configDir)
	}

	// Execute command with CLAUDE_CONFIG_DIR set
	execCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = append(os.Environ(), fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", configDir))

	return execCmd.Run()
}

type runArgs struct {
	profile, account   string
	accountSet, relink bool
	command            []string
}

// parseRunArgs reads "[<profile>] [--account <name>] [--relink] [-- <command>]".
// Flag parsing is off for run so the command's own flags pass through; only
// the arguments before "--" belong to ccp. Without "--" the command is claude.
func parseRunArgs(args []string) (runArgs, error) {
	var ra runArgs
	own, command := args, []string{"claude"}
	if sep := slices.Index(args, "--"); sep >= 0 {
		own, command = args[:sep], args[sep+1:]
	}
	for i := 0; i < len(own); i++ {
		arg := own[i]
		switch {
		case arg == "--relink":
			ra.relink = true
		case arg == "--account":
			if i+1 >= len(own) {
				return ra, fmt.Errorf("--account needs a name")
			}
			i++
			ra.account = own[i]
			ra.accountSet = true
		case strings.HasPrefix(arg, "--account="):
			ra.account = strings.TrimPrefix(arg, "--account=")
			ra.accountSet = true
		case strings.HasPrefix(arg, "-"):
			return ra, fmt.Errorf("unknown flag before --: %s", arg)
		case ra.profile == "":
			ra.profile = arg
		default:
			return ra, fmt.Errorf("unexpected argument before --: %s", arg)
		}
	}
	ra.command = command
	if ra.profile == "" && !ra.accountSet {
		return ra, fmt.Errorf("profile name required (or --account)")
	}
	if len(ra.command) == 0 {
		return ra, fmt.Errorf("command required after --")
	}
	if ra.accountSet && ra.account == "" {
		return ra, profile.ValidateAccountName("")
	}
	if ra.relink && ra.account == "" {
		return ra, fmt.Errorf("--relink needs --account")
	}
	return ra, nil
}

// activationDir returns the CLAUDE_CONFIG_DIR for a profile, or for the
// profile run under an account when one is given (see profile.LinkAccount).
// Warnings go to stderr so shell output stays evaluable. accountSet means the
// flag was given: an empty name then is an error, never the profile folder.
func activationDir(paths *config.Paths, profileName, account string, accountSet, relink bool) (string, error) {
	if accountSet && account == "" {
		return "", profile.ValidateAccountName(account)
	}
	if account == "" {
		if relink {
			return "", fmt.Errorf("--relink needs --account")
		}
		return paths.ProfileDir(profileName), nil
	}
	dir, warnings, err := profile.LinkAccount(paths, account, profileName, relink)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}
	return dir, err
}

// addAccountFlags registers --account and --relink on a command that sets
// CLAUDE_CONFIG_DIR.
func addAccountFlags(c *cobra.Command, account *string, relink *bool) {
	c.Flags().StringVar(account, "account", "", "Run the profile under an account folder (~/.ccp/accounts/<name>) with its own Claude login")
	c.Flags().BoolVar(relink, "relink", false, "With --account: link an account that is linked to another profile")
}
