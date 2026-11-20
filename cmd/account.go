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

var (
	accountListJSON   bool
	accountAddHarness string
)

var accountCmd = &cobra.Command{
	Use:     "account",
	Aliases: []string{"accounts", "acc"},
	Short:   "List and add accounts (logins a profile runs under)",
	Long: `An account is a folder, ~/.ccp/accounts/<name>/, that runs one profile under
its own login. For Claude Code accounts, skills, settings and history link to
the profile; only the login differs. For agy accounts (--harness agy), agy uses
file-based token storage in ~/.gemini/antigravity-cli-ccp-<name>/.

  ccp account                                    # list accounts with plan usage
  ccp account add gt1 companyA                  # create a Claude account
  ccp account add agy1 companyA --harness agy   # create an agy account
  ccp run --account gt1                          # start Claude on it; type /login once
  ccp run --account agy1                         # start agy on it; sign in once
  ccp run companyA --account auto               # the account with the most limit left
  eval "$(ccp env --account gt2)"                # this terminal only (Claude only)

With no subcommand, ccp account lists the accounts.`,
	Args: cobra.NoArgs,
	RunE: runAccountList,
}

var accountListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List accounts with their harness, profile, login and plan usage",
	Long: `List every account with its harness (claude/agy), profile, email, 5-hour and
weekly plan usage, model-scoped weeks, the age of the usage read and when ccp
last started it. Usage is read at most every 5 minutes per account.`,
	Args: cobra.NoArgs,
	RunE: runAccountList,
}

var accountAddCmd = &cobra.Command{
	Use:   "add <name> [<profile>]",
	Short: "Create an account for a profile (default: the current profile)",
	Long: `Create the account folder ~/.ccp/accounts/<name>/ for a profile.

With --harness claude (default), it links every profile entry except the Claude
login, so it starts logged out: run 'ccp run --account <name>' and type /login
in Claude Code once.

With --harness agy, it creates ~/.gemini/antigravity-cli-ccp-<name>/ with merged
settings and no symlinks: run 'ccp run --account <name>' to sign in once.

The profile defaults to the current one (CLAUDE_CONFIG_DIR, else ~/.claude).
An account that already exists is refused; move it to another profile with
--relink.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runAccountAdd,
}

func init() {
	accountCmd.PersistentFlags().BoolVarP(&accountListJSON, "json", "j", false, "Output as JSON")
	accountAddCmd.Flags().StringVar(&accountAddHarness, "harness", profile.HarnessClaude, "Account harness: claude or agy")
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
	otherLogins := readOtherLogins(paths)
	if len(accounts) == 0 {
		fmt.Println("No accounts yet. Add one, then log in once:")
		fmt.Println("  ccp account add <name> [<profile>]")
		fmt.Println("  ccp run --account <name>        # type /login in Claude Code")
		if len(otherLogins) > 0 {
			fmt.Println()
			printOtherLogins(otherLogins)
		}
		return nil
	}
	printAccounts(statuses)
	if len(otherLogins) > 0 {
		printOtherLogins(otherLogins)
	}
	return nil
}

func runAccountAdd(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	name := args[0]
	if err := profile.ValidateAccountName(name); err != nil {
		return err
	}
	harness, err := profile.NormalizeHarness(accountAddHarness)
	if err != nil {
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

	if harness == profile.HarnessAgy {
		dir, err := profile.CreateAgyAccount(paths, name, profileName)
		if err != nil {
			return err
		}
		fmt.Printf("Account %s (agy) created for profile %s: %s\n", name, profileName, dir)
		fmt.Println("Sign in once with:")
		fmt.Printf("  ccp run --account %s\n", name)
		return nil
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
