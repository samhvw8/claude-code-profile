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
	Use:    "run <profile> [--account <name> [--relink]] -- <command> [args...]",
	Hidden: true,
	Short:  "Run a command with a specific profile active",
	Long: `Execute a command with CLAUDE_CONFIG_DIR set to the specified profile.

With --account, CLAUDE_CONFIG_DIR is the account folder ~/.ccp/accounts/<name>,
which keeps its own Claude login and links everything else to the profile.
--relink moves an account that is linked to another profile.

Examples:
  ccp run minimal -- claude "fix this bug"
  ccp run default --account work1 -- claude
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
	ra, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	profileName, cmdArgs := ra.profile, ra.command

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

	configDir, err := activationDir(paths, profileName, ra.account, ra.relink)
	if err != nil {
		return err
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
	profile, account string
	relink           bool
	command          []string
}

// parseRunArgs reads "<profile> [--account <name>] [--relink] -- <command>".
// Flag parsing is off for run so the command's own flags pass through; only
// the arguments before "--" belong to ccp.
func parseRunArgs(args []string) (runArgs, error) {
	var ra runArgs
	sep := slices.Index(args, "--")
	if sep < 0 {
		return ra, fmt.Errorf("command required after --")
	}
	own := args[:sep]
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
		case strings.HasPrefix(arg, "--account="):
			ra.account = strings.TrimPrefix(arg, "--account=")
		case strings.HasPrefix(arg, "-"):
			return ra, fmt.Errorf("unknown flag before --: %s", arg)
		case ra.profile == "":
			ra.profile = arg
		default:
			return ra, fmt.Errorf("unexpected argument before --: %s", arg)
		}
	}
	ra.command = args[sep+1:]
	if ra.profile == "" {
		return ra, fmt.Errorf("profile name required")
	}
	if len(ra.command) == 0 {
		return ra, fmt.Errorf("command required after --")
	}
	if ra.relink && ra.account == "" {
		return ra, fmt.Errorf("--relink needs --account")
	}
	return ra, nil
}

// activationDir returns the CLAUDE_CONFIG_DIR for a profile, or for the
// profile run under an account when one is given (see profile.LinkAccount).
// Warnings go to stderr so shell output stays evaluable.
func activationDir(paths *config.Paths, profileName, account string, relink bool) (string, error) {
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
