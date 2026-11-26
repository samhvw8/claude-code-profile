package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// profileRuleFlag makes 'ccp run' take the account from the loaded profile's
// rule. Only the global shim passes it, so it is not in the run help.
const profileRuleFlag = "--profile-rule"

var (
	accountUseProfile string
	accountUseClear   bool
)

var accountUseCmd = &cobra.Command{
	Use:   "use [<name>|auto|auto:<pool>]",
	Short: "Set the account plain claude runs on wherever a profile loads",
	Long: `Bind an account rule to a profile. Wherever the profile loads (globally
through ~/.claude, or in a project through CLAUDE_CONFIG_DIR), plain claude
then runs on that rule's account, with no --account:

  <name>        that account; it must be a Claude account linked to the profile
  auto          the profile's account with the most plan limit left
  auto:<pool>   the pool's account with the most limit left, relinked to the
                profile when no session runs on it

Without --profile the rule goes to the current profile: the project's
(CLAUDE_CONFIG_DIR), else the global one (~/.claude). With no value it prints
the rule; --clear removes it, and plain claude runs on the profile as before.

The rule is the 'account' field of the profile's profile.toml. Plain claude
follows it through the shim ~/.ccp/bin/claude, which this command writes; put
~/.ccp/bin first on PATH once. A session already on an account keeps it, and
with no usable account claude runs as it would without the shim.

  ccp account use default                    # current profile -> account default
  ccp account use auto:company --profile ca  # profile ca -> freest of pool company
  ccp account use                            # show the current profile's rule
  ccp account use --clear                    # remove it`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAccountUse,
}

func init() {
	accountUseCmd.Flags().StringVar(&accountUseProfile, "profile", "", "Profile to bind (default: the current one)")
	accountUseCmd.Flags().BoolVar(&accountUseClear, "clear", false, "Remove the profile's account rule")
	accountCmd.AddCommand(accountUseCmd)
}

func runAccountUse(cmd *cobra.Command, args []string) error {
	if accountUseClear && len(args) == 1 {
		return fmt.Errorf("give a rule or --clear, not both")
	}
	cmd.SilenceUsage = true
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}
	mgr := profile.NewManager(paths)
	name := accountUseProfile
	if name == "" {
		p, err := mgr.GetCurrent()
		if err != nil || p == nil {
			return fmt.Errorf("no current profile; name one with --profile <name>")
		}
		name = p.Name
	}
	if err := profile.ValidateProfileName(name); err != nil {
		return err
	}
	p, err := mgr.Get(name)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", name)
	}

	switch {
	case len(args) == 0 && !accountUseClear:
		if p.Manifest.Account == "" {
			fmt.Printf("Profile %s has no account rule; plain claude runs on the profile\n", name)
		} else {
			fmt.Printf("Profile %s: account rule %s\n", name, p.Manifest.Account)
		}
		return nil
	case accountUseClear:
		p.Manifest.Account = ""
	default:
		if err := validateAccountRule(paths, name, args[0]); err != nil {
			return err
		}
		p.Manifest.Account = args[0]
	}
	if err := p.Manifest.Save(profile.ManifestPath(p.Path)); err != nil {
		return err
	}
	if accountUseClear {
		fmt.Printf("Profile %s has no account rule; plain claude runs on the profile\n", name)
		return nil
	}
	fmt.Printf("Profile %s: account rule %s\n", name, p.Manifest.Account)
	return ensureGlobalShim(paths)
}

// validateAccountRule checks a rule for profileName: auto and auto:<pool>
// always pass (the pick happens at each start); a named account must exist,
// be a Claude account and be linked to the profile.
func validateAccountRule(paths *config.Paths, profileName, rule string) error {
	isAuto, _, err := profile.ParseAutoAccount(rule)
	if err != nil {
		return err
	}
	if isAuto {
		return nil
	}
	if err := profile.ValidateAccountName(rule); err != nil {
		return err
	}
	bound, harness, err := profile.AccountInfo(paths.AccountDir(rule))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("account %q does not exist; add it with 'ccp account add %s %s'", rule, rule, profileName)
	}
	if err != nil {
		return err
	}
	if harness != profile.HarnessClaude {
		return fmt.Errorf("account %q is an %s account; the rule names the Claude login plain claude runs on", rule, harness)
	}
	if bound != profileName {
		return fmt.Errorf("account %q is linked to profile %q, not %q; put it in a pool and use auto:<pool>, or relink it with 'ccp run %s --account %s --relink'", rule, bound, profileName, profileName, rule)
	}
	return nil
}

// applyProfileRule turns 'ccp run --profile-rule' into the run the loaded
// profile's rule asks for. plain is true when claude should run with the
// environment unchanged: no ccp profile, no rule, or a rule that no longer
// holds (said on stderr, never silently).
func applyProfileRule(paths *config.Paths, ra runArgs) (_ runArgs, plain bool) {
	// A session already on an account (a child of it): keep that account.
	if currentAccountDir(paths) != "" {
		ra.account, ra.accountSet, ra.orProfile, ra.plainFallback = profile.AutoAccount, true, true, true
		return ra, false
	}
	p, err := profile.NewManager(paths).GetCurrent()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ccp: cannot read the current profile (%v); running without an account\n", err)
		return ra, true
	}
	if p == nil || p.Manifest.Account == "" {
		return ra, true
	}
	rule := p.Manifest.Account
	if err := validateAccountRule(paths, p.Name, rule); err != nil {
		fmt.Fprintf(os.Stderr, "ccp: account rule of profile %s: %v; running without an account\n", p.Name, err)
		return ra, true
	}
	ra.profile, ra.account, ra.accountSet = p.Name, rule, true
	if isAuto, _, _ := profile.ParseAutoAccount(rule); isAuto {
		ra.orProfile, ra.plainFallback = true, true
	}
	return ra, false
}

// plainCmd runs command with this process's environment unchanged.
func plainCmd(command []string) *exec.Cmd {
	c := exec.Command(command[0], command[1:]...)
	c.Env = os.Environ()
	return c
}

// globalShimPath is ~/.ccp/bin/claude.
func globalShimPath(paths *config.Paths) string {
	return filepath.Join(paths.CcpDir, "bin", "claude")
}

// ensureGlobalShim writes ~/.ccp/bin/claude when it is missing or stale, and
// says how to put its folder on PATH when claude does not resolve to it.
func ensureGlobalShim(paths *config.Paths) error {
	shim := globalShimPath(paths)
	want := globalShimScript()
	if cur, err := os.ReadFile(shim); err != nil || string(cur) != want {
		if err := os.MkdirAll(filepath.Dir(shim), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(shim, []byte(want), 0o755); err != nil {
			return err
		}
		if err := os.Chmod(shim, 0o755); err != nil {
			return err
		}
		fmt.Printf("Wrote %s: plain claude follows the loaded profile's account rule\n", shim)
	}
	if found, err := exec.LookPath("claude"); err == nil && filepath.Clean(found) == filepath.Clean(shim) {
		return nil
	}
	dir := filepath.Dir(shim)
	fmt.Printf("Put %s first on PATH so plain claude uses the shim, for example in your global mise config:\n", dir)
	fmt.Println("  [env]")
	fmt.Printf("  _.path = [%q]\n", dir)
	return nil
}

// printAccountRules lists the profiles that have an account rule.
func printAccountRules(paths *config.Paths) {
	profiles, err := profile.NewManager(paths).List()
	if err != nil {
		return
	}
	var rows [][2]string
	for _, p := range profiles {
		if p.Manifest.Account != "" {
			rows = append(rows, [2]string{p.Name, p.Manifest.Account})
		}
	}
	if len(rows) == 0 {
		return
	}
	fmt.Println("--- Account rules (ccp account use) ---")
	w := len("PROFILE")
	for _, r := range rows {
		w = max(w, len(r[0]))
	}
	fmt.Printf("  %-*s  %s\n", w, "PROFILE", "RULE")
	for _, r := range rows {
		fmt.Printf("  %-*s  %s\n", w, r[0], r[1])
	}
	fmt.Println()
}
