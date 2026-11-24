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

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var (
	accountRemoveForce     bool
	accountRemoveCredsOnly bool

	// Test seams so tests never touch the real Keychain, process table, or TTY.
	accountRemoveGOOS        = func() string { return runtime.GOOS }
	newRemoveCredentialStore = migration.NewCredentialStore
	newRemoveAgyStore        = func() migration.CredentialStore { return migration.FileStore{} }
	newAgyProcessLister      = migration.NewAgyProcessLister
	stdinIsTerminal          = func() bool {
		info, err := os.Stdin.Stat()
		return err == nil && info.Mode()&os.ModeCharDevice != 0
	}
)

var accountRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove an account and its login, or only its login",
	Long: `Remove an account's login and folder (~/.ccp/accounts/<name>/). For an agy
account, also remove its data directory (~/.gemini/antigravity-cli-ccp-<name>/).

With --creds-only, delete only the stored login (Keychain item on macOS,
.credentials.json on Linux, or antigravity-oauth-token for agy) and leave the
account folder, binding, pool, and agy data directory intact.

Refuses while a session is running on the account (even with --force).
Prompts for confirmation unless --force is passed. --json requires --force.

  ccp account remove gt1                 # delete login and account folder
  ccp account remove gt1 --creds-only    # log out gt1, keep folder and pool
  ccp account rm agy1 --force --json     # non-interactive JSON output`,
	Args: cobra.ExactArgs(1),
	RunE: runAccountRemove,
}

func init() {
	accountRemoveCmd.Flags().BoolVarP(&accountRemoveForce, "force", "f", false, "Skip confirmation prompt")
	accountRemoveCmd.Flags().BoolVar(&accountRemoveCredsOnly, "creds-only", false, "Delete only the stored login; keep account folder and data")
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

	accountDir := filepath.Clean(paths.AccountDir(name))
	info, err := os.Stat(accountDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("account %q does not exist; list accounts with 'ccp account'", name)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("account path %s is not a directory", accountDir)
	}

	harness := profile.HarnessClaude
	if profile.IsAgyAccountDir(name, accountDir) {
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

	// Interactive confirmation: refuse early if busy, require a TTY, show exact targets.
	if !accountRemoveForce {
		if err := checkAccountNotBusy(harness, name, accountDir, agyDataDir); err != nil {
			return err
		}
		if !stdinIsTerminal() {
			return fmt.Errorf("refusing to remove account %q without a terminal; pass --force to skip confirmation", name)
		}
		if accountRemoveCredsOnly {
			fmt.Printf("Will remove login for account %q (%s):\n", name, harness)
			fmt.Printf("  Login:    %s\n", loginLabel)
		} else {
			fmt.Printf("Will remove account %q (%s):\n", name, harness)
			fmt.Printf("  Login:    %s\n", loginLabel)
			fmt.Printf("  Folder:   %s\n", accountDir)
			if harness == profile.HarnessAgy {
				fmt.Printf("  Data dir: %s\n", agyDataDir)
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

	// Take the Claude account lock and check busy right before deleting.
	release := noRelease
	if harness == profile.HarnessClaude {
		release, err = lockForLink(accountDir)
		if err != nil {
			return err
		}
		defer release()
	}
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
	}

	// 2. Delete account folder (and agy data dir) in default mode.
	mode := "creds-only"
	if !accountRemoveCredsOnly {
		mode = "all"
		if err := os.RemoveAll(accountDir); err != nil {
			return fmt.Errorf("remove account folder %s: %w", accountDir, err)
		}
		removed = append(removed, accountDir)
		if harness == profile.HarnessAgy {
			_, dStatErr := os.Stat(agyDataDir)
			dataExisted := dStatErr == nil
			if err := os.RemoveAll(agyDataDir); err != nil {
				return fmt.Errorf("remove agy data dir %s: %w", agyDataDir, err)
			}
			if dataExisted {
				removed = append(removed, agyDataDir)
			}
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

	if accountRemoveCredsOnly {
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
			return fmt.Errorf("cannot remove account %q: an agy session is running on it (close it first)", name)
		}
		return nil
	}
	procs, scanErr := scanClaudeProcesses()
	if accountBusy(procs, scanErr, accountDir) {
		if scanErr != nil {
			return fmt.Errorf("cannot verify whether Claude Code is running on account %q: %w", name, scanErr)
		}
		return fmt.Errorf("cannot remove account %q: a Claude Code session is running on it (close it first)", name)
	}
	return nil
}

// scanAgyProcesses lists running agy processes, or says why it cannot.
func scanAgyProcesses() ([]migration.AgyProcess, error) {
	lister := newAgyProcessLister()
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
		if p.AppDataDir == appDataDirName {
			return true
		}
		if agyDataDir != "" && filepath.IsAbs(p.AppDataDir) && profile.SameDir(p.AppDataDir, agyDataDir) {
			return true
		}
	}
	return false
}
