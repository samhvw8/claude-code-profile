package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var runCmd = &cobra.Command{
	Use:    "run [<profile>] [--account <name>|auto [--relink|--or-profile]] [-- <command> [args...]]",
	Hidden: true,
	Short:  "Run a command with a specific profile active",
	Long: `Execute a command with CLAUDE_CONFIG_DIR set to the specified profile.

With --account, a Claude account uses ~/.ccp/accounts/<name> as CLAUDE_CONFIG_DIR
and links everything else to the profile. An agy account (--harness agy) runs
agy with --app_data_dir=antigravity-cli-ccp-<name> and SSH_CLIENT set while
keeping CLAUDE_CONFIG_DIR set to the profile folder.
The profile may be left out: a named account runs its own profile, and its
default command is claude (or agy for agy accounts).
--relink moves an account that is linked to another profile.

--account auto picks, among the accounts of the matching harness linked to the
profile (the current one when none is named), the one with the most plan limit
left: cached at most every 5 minutes per account, else the account started
least recently.

--or-profile (with --account auto) never fails for lack of an account: a
session already on an account keeps it, and a pool with no account logged in
runs on the profile itself. The shim from 'ccp use --accounts' passes it.

Examples:
  ccp run --account work1                  # claude, account work1's profile
  ccp run --account agy1                   # agy, account agy1's profile
  ccp run --account auto -- claude -c      # continue on the freest Claude account
  ccp run --account auto -- agy            # start on the freest agy account
  ccp run companyA --account auto
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
	if ra.help {
		return cmd.Help()
	}
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
	execCmd, err := prepareRunCmd(paths, ra)
	if err != nil {
		return err
	}
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	return execCmd.Run()
}

func commandHarness(cmd0 string) string {
	if filepath.Base(cmd0) == profile.HarnessAgy {
		return profile.HarnessAgy
	}
	return profile.HarnessClaude
}

func hasAppDataDirFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--app_data_dir" || a == "-app_data_dir" ||
			strings.HasPrefix(a, "--app_data_dir=") || strings.HasPrefix(a, "-app_data_dir=") {
			return true
		}
	}
	return false
}

// prepareRunCmd resolves the profile, account, and harness, returning the ready
// exec.Cmd with environment and flags applied.
func prepareRunCmd(paths *config.Paths, ra runArgs) (*exec.Cmd, error) {
	cmdArgs := slices.Clone(ra.command)
	harness := ""
	if ra.account != "" && ra.account != profile.AutoAccount {
		if err := profile.ValidateAccountName(ra.account); err != nil {
			return nil, err
		}
		_, acctHarness, bindErr := profile.AccountInfo(paths.AccountDir(ra.account))
		switch {
		case bindErr == nil:
			if !ra.commandExplicit && acctHarness == profile.HarnessAgy {
				cmdArgs = []string{profile.HarnessAgy}
			}
			if cmdH := commandHarness(cmdArgs[0]); cmdH != acctHarness {
				return nil, fmt.Errorf("account %q uses harness %s; cannot run %q", ra.account, acctHarness, filepath.Base(cmdArgs[0]))
			}
			harness = acctHarness
		case errors.Is(bindErr, os.ErrNotExist):
			if commandHarness(cmdArgs[0]) == profile.HarnessAgy {
				profHint := "<profile>"
				if ra.profile != "" {
					profHint = ra.profile
				}
				return nil, fmt.Errorf("agy account %q does not exist yet; create it with 'ccp account add %s %s --harness agy'", ra.account, ra.account, profHint)
			}
			harness = profile.HarnessClaude
		default:
			if !ra.relink {
				return nil, fmt.Errorf("%w; fix the file or rerun with --relink", bindErr)
			}
			if profile.IsAgyAccountDir(ra.account, paths.AccountDir(ra.account)) || commandHarness(cmdArgs[0]) == profile.HarnessAgy {
				if !ra.commandExplicit {
					cmdArgs = []string{profile.HarnessAgy}
				}
				if commandHarness(cmdArgs[0]) != profile.HarnessAgy {
					return nil, fmt.Errorf("account %q uses harness %s; cannot run %q", ra.account, profile.HarnessAgy, filepath.Base(cmdArgs[0]))
				}
				harness = profile.HarnessAgy
			} else {
				harness = profile.HarnessClaude
			}
		}
	} else if ra.account == profile.AutoAccount {
		harness = commandHarness(cmdArgs[0])
	}

	if ra.orProfile {
		if dir := currentAccountDir(paths); dir != "" {
			if h, err := profile.AccountHarness(dir); err == nil && h == harness && h == profile.HarnessClaude {
				execCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
				execCmd.Env = profile.WithEnv(os.Environ(), "CLAUDE_CONFIG_DIR="+dir)
				return execCmd, nil
			}
		}
	}

	profileName, err := activationProfile(paths, ra.profile, ra.account)
	if err != nil {
		return nil, err
	}
	account, err := chooseAccount(paths, profileName, ra.account, ra.relink, harness)
	if ra.orProfile && errors.Is(err, errNoUsableAccount) {
		fmt.Fprintf(os.Stderr, "ccp: no account of profile %s is logged in; running on the profile\n", profileName)
		execCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		execCmd.Env = profile.WithEnv(os.Environ(), "CLAUDE_CONFIG_DIR="+paths.ProfileDir(profileName))
		return execCmd, nil
	}
	if err != nil {
		return nil, err
	}

	if account != "" && harness == profile.HarnessAgy {
		if hasAppDataDirFlag(cmdArgs[1:]) {
			return nil, fmt.Errorf("do not pass --app_data_dir with --account; ccp sets it for account %q", account)
		}
		exe, err := exec.LookPath(cmdArgs[0])
		if err != nil {
			return nil, fmt.Errorf("agy not found on PATH; install Antigravity CLI (agy) first: %w", err)
		}
		if _, err := profile.RelinkAgyAccount(paths, account, profileName, ra.relink); err != nil {
			return nil, err
		}
		if err := profile.SyncAgyAccountSettings(account); err != nil {
			return nil, err
		}
		markAccountUsed(paths.AccountDir(account))
		profileDir := paths.ProfileDir(profileName)
		if abs, err := filepath.Abs(profileDir); err == nil {
			profileDir = abs
		}
		flagArg, envEntry := profile.AgyLaunch(account)
		launchArgs := append([]string{flagArg}, cmdArgs[1:]...)
		execCmd := exec.Command(exe, launchArgs...)
		execCmd.Env = profile.WithEnv(os.Environ(), envEntry, "CLAUDE_CONFIG_DIR="+profileDir)
		return execCmd, nil
	}

	configDir, err := activationDir(paths, profileName, account, ra.accountSet, ra.relink)
	if err != nil {
		return nil, err
	}
	if account != "" {
		markAccountUsed(configDir)
	}
	execCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	execCmd.Env = profile.WithEnv(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir)
	return execCmd, nil
}

type runArgs struct {
	profile, account   string
	accountSet, relink bool
	orProfile          bool
	help               bool
	commandExplicit    bool
	command            []string
}

// parseRunArgs reads "[<profile>] [--account <name>] [--relink] [-- <command>]".
// Flag parsing is off for run so the command's own flags pass through; only
// the arguments before "--" belong to ccp. Without "--" the command is claude
// (or agy when the named account uses harness agy).
func parseRunArgs(args []string) (runArgs, error) {
	var ra runArgs
	own, command := args, []string{"claude"}
	if sep := slices.Index(args, "--"); sep >= 0 {
		own, command = args[:sep], args[sep+1:]
		ra.commandExplicit = true
	}
	// Flag parsing is off, so cobra never sees a help flag: handle it here.
	if slices.Contains(own, "-h") || slices.Contains(own, "--help") {
		ra.help = true
		return ra, nil
	}
	for i := 0; i < len(own); i++ {
		arg := own[i]
		switch {
		case arg == "--relink":
			ra.relink = true
		case arg == "--or-profile":
			ra.orProfile = true
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
	if ra.orProfile && ra.account != profile.AutoAccount {
		return ra, fmt.Errorf("--or-profile needs --account auto")
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
