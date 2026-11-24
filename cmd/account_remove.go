package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/accountusage"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var (
	accountRemoveForce bool
	accountRemoveAll   bool

	// Test seams so tests never touch the real Keychain, process table, or TTY.
	accountRemoveGOOS        = func() string { return runtime.GOOS }
	newRemoveCredentialStore = migration.NewCredentialStore
	newRemoveAgyStore        = func() migration.CredentialStore { return migration.FileStore{} }
	stdinIsTerminal          = func() bool {
		info, err := os.Stdin.Stat()
		return err == nil && info.Mode()&os.ModeCharDevice != 0
	}
)

var accountRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Delete an account's login, or with --all the whole account",
	Long: `Delete the stored login of an account (Keychain item on macOS,
.credentials.json on Linux, or antigravity-oauth-token for agy). The account
folder, its binding and pool, and the agy data directory stay, so the account
can log in again with 'ccp run --account <name>'.

With --all, also delete the account folder (~/.ccp/accounts/<name>/) and, for
an agy account, its data directory (~/.gemini/antigravity-cli-ccp-<name>/,
with its conversations).

Refuses while a session is running on the account (even with --force).
Prompts for confirmation unless --force is passed. --json requires --force.

  ccp account remove ca1                 # log ca1 out, keep folder and pool
  ccp account remove ca1 --all           # delete login, folder and agy data
  ccp account rm agy1 --force --json     # non-interactive JSON output`,
	Args: cobra.ExactArgs(1),
	RunE: runAccountRemove,
}

func init() {
	accountRemoveCmd.Flags().BoolVarP(&accountRemoveForce, "force", "f", false, "Skip confirmation prompt")
	accountRemoveCmd.Flags().BoolVar(&accountRemoveAll, "all", false, "Also delete the account folder and, for agy, its data directory")
}

type accountRemoveJSON struct {
	Name    string   `json:"name"`
	Harness string   `json:"harness"`
	Mode    string   `json:"mode"`
	Removed []string `json:"removed"`
}

func runAccountRemove(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	name := args[0]
	if err := profile.ValidateAccountName(name); err != nil {
		return err
	}
	if accountListJSON && !accountRemoveForce {
		return fmt.Errorf("--json requires --force")
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Claude Code hashes the absolute CLAUDE_CONFIG_DIR that ccp run passes
	// (filepath.Abs there too); any other spelling names another Keychain item.
	accountDir, err := filepath.Abs(paths.AccountDir(name))
	if err != nil {
		return err
	}
	info, err := os.Lstat(accountDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("account %q does not exist; list accounts with 'ccp account'", name)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("account folder %s is a symlink; ccp does not remove through a link: delete the link and its target yourself", accountDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("account path %s is not a directory", accountDir)
	}

	harness := profile.HarnessClaude
	if profile.IsAgyAccountDir(name, accountDir) || agyAccountWithoutBinding(name, accountDir) {
		harness = profile.HarnessAgy
	}

	goos := accountRemoveGOOS()
	var (
		claudeLoginKey string
		agyDataDir     string
		agyTokenPath   string
		loginLabel     string
	)
	if harness == profile.HarnessAgy {
		agyDataDir, err = profile.AgyAccountDataDir(name)
		if err != nil {
			return err
		}
		agyTokenPath, err = profile.AgyTokenPath(name)
		if err != nil {
			return err
		}
		loginLabel = agyTokenPath
	} else {
		claudeLoginKey = migration.LoginKey(goos, accountDir)
		loginLabel = migration.LoginLocation(goos, accountDir)
	}
	// Where the Keychain holds the login, Claude Code can still have written
	// it to this file (a Keychain it could not use); it is part of the login.
	claudeCredFile := ""
	if harness == profile.HarnessClaude {
		if f := filepath.Join(accountDir, ".credentials.json"); f != claudeLoginKey {
			if _, err := os.Lstat(f); err == nil {
				claudeCredFile = f
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	// Interactive confirmation: refuse early if busy, require a TTY, show exact targets.
	if !accountRemoveForce {
		if err := checkAccountNotBusy(harness, name, accountDir, agyDataDir); err != nil {
			return err
		}
		if !stdinIsTerminal() {
			return fmt.Errorf("refusing to remove account %q without a terminal; pass --force to skip confirmation", name)
		}
		if !accountRemoveAll {
			fmt.Printf("Will remove login for account %q (%s):\n", name, harness)
			fmt.Printf("  Login:    %s\n", loginLabel)
			if claudeCredFile != "" {
				fmt.Printf("  Login:    %s\n", claudeCredFile)
			}
		} else {
			fmt.Printf("Will remove account %q (%s):\n", name, harness)
			fmt.Printf("  Login:    %s\n", loginLabel)
			if harness == profile.HarnessAgy {
				fmt.Printf("  Folder:   %s\n", accountDir)
				fmt.Printf("  Data dir: %s (agy conversations and settings)\n", agyDataDir)
			} else {
				fmt.Printf("  Folder:   %s (with the state only this account has, such as MCP servers added on it)\n", accountDir)
			}
		}
		fmt.Print("Remove? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		ans := strings.TrimSpace(strings.ToLower(input))
		if ans != "y" && ans != "yes" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	// Take the account lock and check busy right before deleting: every
	// 'ccp run --account' start holds the same lock until its process runs.
	release, err := lockForLink(accountDir)
	if err != nil {
		return err
	}
	defer release()
	if err := checkAccountNotBusy(harness, name, accountDir, agyDataDir); err != nil {
		return err
	}

	removed := []string{}

	// 1. Delete the login first. On failure, stop and delete nothing else.
	if harness == profile.HarnessAgy {
		_, statErr := os.Lstat(agyTokenPath)
		existed := statErr == nil
		warn, delErr := newRemoveAgyStore().Delete(agyTokenPath, "")
		if delErr != nil {
			return fmt.Errorf("remove login %s: %w", agyTokenPath, delErr)
		}
		if warn != "" {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", warn)
		}
		if existed {
			removed = append(removed, agyTokenPath)
		}
	} else {
		store, storeErr := newRemoveCredentialStore(goos)
		if storeErr != nil {
			return storeErr
		}
		_, acct, found, getErr := store.Get(claudeLoginKey)
		if getErr != nil {
			return fmt.Errorf("read login %s: %w", loginLabel, getErr)
		}
		if found {
			warn, delErr := store.Delete(claudeLoginKey, acct)
			if delErr != nil {
				return fmt.Errorf("remove login %s: %w", loginLabel, delErr)
			}
			if warn != "" {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", warn)
			}
			removed = append(removed, claudeLoginKey)
		}
		if claudeCredFile != "" {
			if err := os.Remove(claudeCredFile); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove login %s: %w", claudeCredFile, err)
			}
			removed = append(removed, claudeCredFile)
		}
	}

	// 2. Delete account folder (and agy data dir) in default mode.
	mode := "creds-only"
	if accountRemoveAll {
		mode = "all"
		// The account folder goes last: while it exists, a failed run can be
		// repeated with the same command.
		if harness == profile.HarnessAgy {
			_, dStatErr := os.Stat(agyDataDir)
			dataExisted := dStatErr == nil
			if err := os.RemoveAll(agyDataDir); err != nil {
				return fmt.Errorf("remove agy data dir %s: %w (the account folder is kept; run the command again)", agyDataDir, err)
			}
			if dataExisted {
				removed = append(removed, agyDataDir)
			}
		}
		if err := os.RemoveAll(accountDir); err != nil {
			return fmt.Errorf("remove account folder %s: %w", accountDir, err)
		}
		removed = append(removed, accountDir)
	}

	if !accountRemoveAll {
		// ccp's own saved usage read belongs to the login that is gone;
		// 'ccp account' would show it next to "not logged in".
		if err := os.Remove(filepath.Join(accountDir, accountusage.UsageFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove saved usage of account %s: %w", name, err)
		}
	}

	if accountListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(accountRemoveJSON{
			Name:    name,
			Harness: harness,
			Mode:    mode,
			Removed: removed,
		})
	}

	if !accountRemoveAll {
		if len(removed) == 0 {
			fmt.Printf("No login found for account %s (%s); nothing removed\n", name, harness)
		} else {
			fmt.Printf("Removed login for account %s (%s)\n", name, harness)
		}
		return nil
	}
	fmt.Printf("Removed account %s (%s)\n", name, harness)
	return nil
}

func checkAccountNotBusy(harness, name, accountDir, agyDataDir string) error {
	if harness == profile.HarnessAgy {
		procs, scanErr := scanAgyProcesses()
		if agyAccountBusy(procs, scanErr, profile.AgyAppDataDirName(name), agyDataDir) {
			if scanErr != nil {
				return fmt.Errorf("cannot verify whether agy is running on account %q: %w", name, scanErr)
			}
			return fmt.Errorf("cannot remove account %q: an agy session is running on it, or an agy process ccp cannot read may be (close it first)", name)
		}
		return nil
	}
	procs, scanErr := scanClaudeProcesses()
	if accountBusy(procs, scanErr, accountDir) {
		if scanErr != nil {
			return fmt.Errorf("cannot verify whether Claude Code is running on account %q: %w", name, scanErr)
		}
		return fmt.Errorf("cannot remove account %q: a Claude Code session is running on it, or a process ccp cannot read may be (close it first)", name)
	}
	return nil
}

// agyAccountWithoutBinding reports an agy account whose binding file is gone:
// profile.IsAgyAccountDir only guesses for an unreadable binding, and a
// missing one would be removed as a Claude account, leaving the agy token and
// data dir behind. No Claude state plus an agy data dir means agy.
func agyAccountWithoutBinding(name, accountDir string) bool {
	if _, err := os.Lstat(filepath.Join(accountDir, profile.AccountFile)); !errors.Is(err, os.ErrNotExist) {
		return false
	}
	if _, err := os.Lstat(filepath.Join(accountDir, ".claude.json")); !errors.Is(err, os.ErrNotExist) {
		return false
	}
	dataDir, err := profile.AgyAccountDataDir(name)
	if err != nil {
		return false
	}
	info, err := os.Stat(dataDir)
	return err == nil && info.IsDir()
}

// scanAgyProcesses lists running agy processes, or says why it cannot.
func scanAgyProcesses() ([]migration.AgyProcess, error) {
	lister := newProcessLister()
	if lister == nil {
		return nil, errors.New("no process lister on this system")
	}
	return lister.AgyProcesses()
}

// agyAccountBusy reports whether an agy process may be running on the account.
// It fails closed: a failed scan, or any process whose arguments could not be
// read, counts as running on the account.
func agyAccountBusy(procs []migration.AgyProcess, scanErr error, appDataDirName, agyDataDir string) bool {
	if scanErr != nil {
		return true
	}
	for _, p := range procs {
		if !p.ArgsKnown {
			return true
		}
		// agy resolves a relative value under ~/.gemini, so every spelling
		// that ends in the account's folder name counts (a trailing slash,
		// ./, ~/.gemini/..., another letter case): a wrong "busy" only
		// refuses, a wrong "free" deletes under a live session.
		if strings.EqualFold(filepath.Base(filepath.Clean(p.AppDataDir)), appDataDirName) {
			return true
		}
		if agyDataDir != "" && filepath.IsAbs(p.AppDataDir) && profile.SameDir(p.AppDataDir, agyDataDir) {
			return true
		}
	}
	return false
}
