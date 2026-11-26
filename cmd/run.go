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
	Use:    "run [<profile>] [--account <name>|auto|auto:<pool> [--relink|--or-profile]] [-- [<command>] [args...]]",
	Hidden: true,
	Short:  "Run a command with a specific profile active",
	Long: `Execute a command with CLAUDE_CONFIG_DIR set to the specified profile.

With --account, a Claude account uses ~/.ccp/accounts/<name> as CLAUDE_CONFIG_DIR
and links everything else to the profile. An agy account (--harness agy) runs
agy with --app_data_dir=antigravity-cli-ccp-<name> and SSH_CLIENT set while
keeping CLAUDE_CONFIG_DIR set to the profile folder; it runs on any profile,
and its binding only names the profile used when none is given.
The profile may be left out: a named account runs its own profile, and its
default command is claude (or agy for agy accounts).
--relink moves an account that is linked to another profile (for agy, it
changes the default profile).

--account auto picks, among the accounts of the matching harness linked to the
profile (the current one when none is named), the one with the most plan limit
left: cached at most every 5 minutes per account, else the account started
least recently.

--account auto:<pool> picks the same way among the accounts of a pool ('ccp
account pool <name> <pool>'), whatever profile they are linked to. An agy
account runs on any profile as is. A Claude account linked to another profile
is relinked to this one, but only while no Claude Code process runs on its
folder (a process ccp cannot read counts as running there); busy accounts are
skipped. The pick line names the pool and any relink.

With --account auto or auto:<pool> and no command, if every candidate account is
agy, ccp starts agy instead of claude (not with --or-profile).

Words after -- that start with "-" are arguments for the default command:
'ccp run --account agy1 -- --model x' runs 'agy --model x'.

--or-profile (with --account auto or auto:<pool>) never fails for lack of an account: a
session already on an account keeps it, and with no account free and logged in it
runs on the profile itself. The shim from 'ccp use --accounts' passes it.

Examples:
  ccp run --account work1                  # claude, account work1's profile
  ccp run --account agy1                   # agy, account agy1's profile
  ccp run --account agy1 -- --model x      # agy --model x (flags only: default command)
  ccp run --account auto -- claude -c      # continue on the freest Claude account
  ccp run --account auto -- agy            # start on the freest agy account
  ccp run companyA --account auto:company  # freest account of pool company
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
	execCmd, release, err := prepareRunCmd(paths, ra)
	if err != nil {
		return err
	}
	defer release()
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	// A pooled pick holds its account's lock until the process exists, never
	// for the session: start, release, then wait.
	err = execCmd.Start()
	release()
	if err != nil {
		return err
	}
	return execCmd.Wait()
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
// exec.Cmd with environment and flags applied, and a release func (never nil,
// safe to call twice) the caller must call right after starting the command:
// a Claude account stays locked (its relink lock) from its binding check
// until then. On an error the lock is already released.
func prepareRunCmd(paths *config.Paths, ra runArgs) (_ *exec.Cmd, release func(), err error) {
	release = noRelease
	if ra.profileRule {
		var plain bool
		if ra, plain = applyProfileRule(paths, ra); plain {
			return plainCmd(ra.command), release, nil
		}
	}
	defer func() {
		if err != nil {
			release()
			release = noRelease
		}
	}()
	cmdArgs := slices.Clone(ra.command)
	harness := ""
	isAuto, pool, err := profile.ParseAutoAccount(ra.account)
	if err != nil {
		return nil, release, err
	}
	if ra.account != "" && !isAuto {
		if err := profile.ValidateAccountName(ra.account); err != nil {
			return nil, release, err
		}
		_, acctHarness, bindErr := profile.AccountInfo(paths.AccountDir(ra.account))
		switch {
		case bindErr == nil:
			if !ra.commandExplicit && acctHarness == profile.HarnessAgy {
				cmdArgs[0] = profile.HarnessAgy
			}
			if cmdH := commandHarness(cmdArgs[0]); cmdH != acctHarness {
				return nil, release, fmt.Errorf("account %q uses harness %s; cannot run %q", ra.account, acctHarness, filepath.Base(cmdArgs[0]))
			}
			harness = acctHarness
		case errors.Is(bindErr, os.ErrNotExist):
			if commandHarness(cmdArgs[0]) == profile.HarnessAgy {
				profHint := "<profile>"
				if ra.profile != "" {
					profHint = ra.profile
				}
				return nil, release, fmt.Errorf("agy account %q does not exist yet; create it with 'ccp account add %s %s --harness agy'", ra.account, ra.account, profHint)
			}
			harness = profile.HarnessClaude
		default:
			if !ra.relink {
				return nil, release, fmt.Errorf("%w; fix the file or rerun with --relink", bindErr)
			}
			if profile.IsAgyAccountDir(ra.account, paths.AccountDir(ra.account)) || commandHarness(cmdArgs[0]) == profile.HarnessAgy {
				if !ra.commandExplicit {
					cmdArgs[0] = profile.HarnessAgy
				}
				if commandHarness(cmdArgs[0]) != profile.HarnessAgy {
					return nil, release, fmt.Errorf("account %q uses harness %s; cannot run %q", ra.account, profile.HarnessAgy, filepath.Base(cmdArgs[0]))
				}
				harness = profile.HarnessAgy
			} else {
				harness = profile.HarnessClaude
			}
		}
	} else if isAuto {
		harness = commandHarness(cmdArgs[0])
	}

	if ra.orProfile {
		if dir := currentAccountDir(paths); dir != "" {
			if h, err := profile.AccountHarness(dir); err == nil && h == harness && h == profile.HarnessClaude {
				if bound, err := profile.AccountProfile(dir); err == nil {
					cmdArgs = withProfileMCP(paths, bound, cmdArgs)
				}
				execCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
				execCmd.Env = profile.WithEnv(os.Environ(), "CLAUDE_CONFIG_DIR="+dir)
				return execCmd, release, nil
			}
		}
	}

	profileName, err := activationProfile(paths, ra.profile, ra.account)
	if err != nil {
		return nil, release, err
	}
	// Not with --or-profile: its fallback runs the command on the profile
	// itself, and a bare agy there would use the shared agy login.
	if isAuto && !ra.commandExplicit && !ra.relink && !ra.orProfile {
		isAgy, err := detectAutoAgy(paths, profileName, pool)
		if err != nil {
			return nil, release, err
		}
		if isAgy {
			cmdArgs[0] = profile.HarnessAgy
			harness = profile.HarnessAgy
			if pool != "" {
				fmt.Fprintf(os.Stderr, "ccp: every account of pool %s is agy; starting agy\n", pool)
			} else {
				fmt.Fprintf(os.Stderr, "ccp: every account of profile %s is agy; starting agy\n", profileName)
			}
		}
	}
	account, release, err := chooseRunAccount(paths, profileName, ra.account, ra.relink, harness)
	pooledLock := pool != "" && harness == profile.HarnessClaude && err == nil
	if ra.orProfile && errors.Is(err, errNoUsableAccount) {
		if ra.plainFallback {
			fmt.Fprintf(os.Stderr, "ccp: account rule %s of profile %s: %v; running without an account\n", ra.account, profileName, err)
			return plainCmd(ra.command), release, nil
		}
		if pool != "" {
			fmt.Fprintf(os.Stderr, "ccp: no account of pool %s is free and logged in; running on profile %s\n", pool, profileName)
		} else {
			fmt.Fprintf(os.Stderr, "ccp: no account of profile %s is logged in; running on the profile\n", profileName)
		}
		cmdArgs = withProfileMCP(paths, profileName, cmdArgs)
		execCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		execCmd.Env = profile.WithEnv(os.Environ(), "CLAUDE_CONFIG_DIR="+paths.ProfileDir(profileName))
		return execCmd, release, nil
	}
	if err != nil {
		return nil, release, err
	}

	if account != "" && harness == profile.HarnessAgy {
		if hasAppDataDirFlag(cmdArgs[1:]) {
			return nil, release, fmt.Errorf("do not pass --app_data_dir with --account; ccp sets it for account %q", account)
		}
		exe, err := exec.LookPath(cmdArgs[0])
		if err != nil {
			return nil, release, fmt.Errorf("agy not found on PATH; install Antigravity CLI (agy) first: %w", err)
		}
		// Held until the process has started, so 'ccp account remove' never
		// deletes the account between this check and the start.
		release, err = lockForLink(paths.AccountDir(account))
		if err != nil {
			return nil, release, err
		}
		// An agy account runs on any profile; its binding only names the
		// default one, and only --relink rewrites it.
		if ra.relink {
			if _, err := profile.RelinkAgyAccount(paths, account, profileName, true); err != nil {
				return nil, release, err
			}
		} else if _, _, err := profile.AccountInfo(paths.AccountDir(account)); err != nil {
			return nil, release, fmt.Errorf("%w; fix the file or rerun with --relink", err)
		} else if !profile.NewManager(paths).Exists(profileName) {
			return nil, release, fmt.Errorf("profile not found: %s", profileName)
		}
		if err := profile.SyncAgyAccountSettings(account); err != nil {
			return nil, release, err
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
		return execCmd, release, nil
	}

	var configDir string
	if pooledLock {
		// The pooled pick holds this account's lock and has linked it.
		configDir, err = filepath.Abs(paths.AccountDir(account))
	} else {
		configDir, release, err = lockedActivationDir(paths, profileName, account, ra.accountSet, ra.relink)
	}
	if err != nil {
		return nil, release, err
	}
	if account != "" {
		markAccountUsed(configDir)
		reportAccountOnlyMCP(paths, profileName, account, configDir)
	}
	cmdArgs = withProfileMCP(paths, profileName, cmdArgs)
	execCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	execCmd.Env = profile.WithEnv(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir)
	return execCmd, release, nil
}

type runArgs struct {
	profile, account   string
	accountSet, relink bool
	orProfile          bool
	// profileRule: the account comes from the loaded profile's rule
	// (--profile-rule, passed by the global shim).
	profileRule bool
	// plainFallback: with no usable account, run the command with the
	// environment unchanged instead of on the profile folder.
	plainFallback   bool
	help            bool
	commandExplicit bool
	command         []string
}

// parseRunArgs reads "[<profile>] [--account <name>] [--relink] [-- <command>]".
// Flag parsing is off for run so the command's own flags pass through; only
// the arguments before "--" belong to ccp. Without "--" the command is claude
// (or agy when the named account uses harness agy). Words after "--" that
// start with "-" are arguments for that default command.
func parseRunArgs(args []string) (runArgs, error) {
	var ra runArgs
	own, command := args, []string{"claude"}
	if sep := slices.Index(args, "--"); sep >= 0 {
		own, command = args[:sep], args[sep+1:]
		ra.commandExplicit = true
		// A program name never starts with "-": such words are arguments
		// for the default command.
		if len(command) > 0 && strings.HasPrefix(command[0], "-") {
			command = append([]string{"claude"}, command...)
			ra.commandExplicit = false
		}
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
		case arg == profileRuleFlag:
			ra.profileRule = true
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
	if ra.profileRule {
		if ra.profile != "" || ra.accountSet || ra.relink || ra.orProfile {
			return ra, fmt.Errorf("%s takes no profile, --account, --relink or --or-profile: the loaded profile's rule decides", profileRuleFlag)
		}
		if len(ra.command) == 0 {
			return ra, fmt.Errorf("command required after --")
		}
		return ra, nil
	}
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
	isAuto, _, err := profile.ParseAutoAccount(ra.account)
	if err != nil {
		return ra, err
	}
	if ra.orProfile && !isAuto {
		return ra, fmt.Errorf("--or-profile needs --account auto or auto:<pool>")
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

// detectAutoAgy reports whether every readable candidate account of pool (or
// profileName when pool is empty) uses harness agy. At least one readable
// candidate must exist.
func detectAutoAgy(paths *config.Paths, profileName, pool string) (bool, error) {
	candidates, err := autoCandidates(paths, profileName, pool)
	if err != nil {
		return false, err
	}
	return allAgyAccounts(candidates), nil
}

func allAgyAccounts(accounts []profile.Account) bool {
	seen := false
	for _, a := range accounts {
		if a.Err != nil {
			continue
		}
		if profile.DefaultHarness(a.Harness) != profile.HarnessAgy {
			return false
		}
		seen = true
	}
	return seen
}

// addAccountFlags registers --account and --relink on a command that sets
// CLAUDE_CONFIG_DIR.
func addAccountFlags(c *cobra.Command, account *string, relink *bool) {
	c.Flags().StringVar(account, "account", "", "Run the profile under an account folder (~/.ccp/accounts/<name>) with its own Claude login; auto picks among the profile's accounts, auto:<pool> among a pool's (in 'ccp run' with no command, starts agy when every candidate is agy)")
	c.Flags().BoolVar(relink, "relink", false, "With --account: link an account that is linked to another profile")
}
