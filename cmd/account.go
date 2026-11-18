package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var accountListJSON bool

var accountCmd = &cobra.Command{
	Use:     "account",
	Aliases: []string{"accounts", "acc"},
	Short:   "List and add accounts (Claude logins a profile runs under)",
	Long: `An account is a folder, ~/.ccp/accounts/<name>/, that runs one profile under
its own Claude login. Skills, settings and history link to the profile; only
the login differs. Log in once per account, then reuse it.

  ccp account                          # list accounts with plan usage
  ccp account add gt1 growtrics        # create an account for a profile
  ccp run --account gt1                # start Claude on it; type /login once
  ccp run growtrics --account auto     # the account with the most limit left
  eval "$(ccp env --account gt2)"      # this terminal only

With no subcommand, ccp account lists the accounts.`,
	Args: cobra.NoArgs,
	RunE: runAccountList,
}

var accountListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List accounts with their profile, login and plan usage",
	Long: `List every account with its profile, email, 5-hour and weekly plan usage,
model-scoped weeks, the age of the usage read and when ccp last started it.
Usage is read from Claude's usage endpoint at most every 5 minutes per account.`,
	Args: cobra.NoArgs,
	RunE: runAccountList,
}

var accountAddCmd = &cobra.Command{
	Use:   "add <name> [<profile>]",
	Short: "Create an account for a profile (default: the current profile)",
	Long: `Create the account folder ~/.ccp/accounts/<name>/ for a profile. It links
every profile entry except the Claude login, so it starts logged out: run
'ccp run --account <name>' and type /login in Claude Code once.

The profile defaults to the current one (CLAUDE_CONFIG_DIR, else ~/.claude).
An account that already exists is refused; move it to another profile with
'ccp env <profile> --account <name> --relink'.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runAccountAdd,
}

func init() {
	accountCmd.PersistentFlags().BoolVarP(&accountListJSON, "json", "j", false, "Output as JSON")
	accountCmd.AddCommand(accountListCmd, accountAddCmd)
	rootCmd.AddCommand(accountCmd)
}

func runAccountList(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}
	accounts, err := profile.ListAccounts(paths)
	if err != nil {
		return err
	}
	statuses := readAccountStatuses(paths)
	if accountListJSON {
		out := accountJSON(statuses)
		if out == nil {
			out = []accountStatusJSON{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	if len(accounts) == 0 {
		fmt.Println("No accounts yet. Add one, then log in once:")
		fmt.Println("  ccp account add <name> [<profile>]")
		fmt.Println("  ccp run --account <name>        # type /login in Claude Code")
		return nil
	}
	printAccounts(statuses)
	return nil
}

func runAccountAdd(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	name := args[0]
	if err := profile.ValidateAccountName(name); err != nil {
		return err
	}
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}
	if _, err := os.Stat(paths.AccountDir(name)); err == nil {
		bound, _ := profile.AccountProfile(paths.AccountDir(name))
		return fmt.Errorf("account %q already exists (profile %q); start it with: ccp run --account %s", name, bound, name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	mgr := profile.NewManager(paths)
	profileName := ""
	if len(args) == 2 {
		profileName = args[1]
		if !mgr.Exists(profileName) {
			return fmt.Errorf("profile not found: %s", profileName)
		}
	} else {
		// No ~/.claude and no CLAUDE_CONFIG_DIR is an error from GetCurrent;
		// either way the fix is to name the profile.
		p, err := mgr.GetCurrent()
		if err != nil || p == nil {
			return fmt.Errorf("no current profile; name one: ccp account add %s <profile>", name)
		}
		profileName = p.Name
	}

	dir, warnings, err := profile.LinkAccount(paths, name, profileName, false)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}
	if err != nil {
		return err
	}
	fmt.Printf("Account %s created for profile %s: %s\n", name, profileName, dir)
	fmt.Println("Log in once:")
	fmt.Printf("  ccp run --account %s        # type /login in Claude Code\n", name)
	return nil
}
