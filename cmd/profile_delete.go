package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var deleteForce bool

var profileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Long: `Delete a profile and all its contents.

This action is irreversible. Use --force to skip confirmation.
Note: The 'default' profile cannot be deleted if it's the only profile.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runProfileDelete,
}

func init() {
	profileDeleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation")
	profileCmd.AddCommand(profileDeleteCmd)
}

func runProfileDelete(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Check profile exists
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	// Check if this is the active profile: the ~/.claude symlink, or this
	// shell's CLAUDE_CONFIG_DIR (the profile folder or one of its accounts).
	// Both count: ~/.claude may point at the profile while CLAUDE_CONFIG_DIR
	// names another, and deleting it would leave ~/.claude dangling.
	globalPaths := *paths
	globalPaths.ClaudeDir = paths.GlobalClaudeDir
	globalActive, _ := profile.NewManager(&globalPaths).GetActive()
	current, _ := mgr.GetCurrent()
	for _, active := range []*profile.Profile{globalActive, current} {
		if active != nil && active.Name == profileName {
			return fmt.Errorf("cannot delete active profile '%s'\n\nSwitch to another profile first: ccp use <other-profile>", profileName)
		}
	}
	if err := refuseBoundAccounts(paths, profileName); err != nil {
		return err
	}

	// Check if this is the only profile
	profiles, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}
	if len(profiles) == 1 && profiles[0].Name == profileName {
		return fmt.Errorf("cannot delete the only profile\n\nCreate another profile first: ccp profile create <name>")
	}

	// Confirm deletion
	if !deleteForce {
		fmt.Printf("Delete profile '%s'? This action is irreversible.\n", profileName)
		fmt.Printf("Location: %s\n", p.Path)
		fmt.Print("\nType 'yes' to confirm: ")

		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		if strings.TrimSpace(strings.ToLower(input)) != "yes" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	// Delete the profile
	if err := mgr.Delete(profileName); err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	fmt.Printf("Deleted profile: %s\n", profileName)
	return nil
}

// refuseBoundAccounts stops a delete that would leave accounts linked to a
// missing profile: each one must be relinked to another profile first.
func refuseBoundAccounts(paths *config.Paths, profileName string) error {
	// An account whose binding cannot be read may be linked to this profile
	// (and may be this shell's CLAUDE_CONFIG_DIR): unknown counts as bound.
	all, err := profile.ListAccounts(paths)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	var unreadable []string
	for _, a := range all {
		if a.Err != nil {
			unreadable = append(unreadable, fmt.Sprintf("  %s: %v", a.Name, a.Err))
		}
	}
	if len(unreadable) > 0 {
		return fmt.Errorf("cannot delete profile '%s': the binding of these accounts cannot be read, so they may be linked to it\n%s\n\nFix each %s, or relink it: ccp env <profile> --account <name> --relink",
			profileName, strings.Join(unreadable, "\n"), profile.AccountFile)
	}
	accounts, err := profile.AccountsBoundTo(paths, profileName)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	if len(accounts) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "cannot delete profile '%s': %d account(s) are linked to it\n\nRelink each one to another profile first (close its Claude sessions):\n", profileName, len(accounts))
	for _, a := range accounts {
		fmt.Fprintf(&b, "  ccp env <other-profile> --account %s --relink\n", a.Name)
	}
	return errors.New(strings.TrimRight(b.String(), "\n"))
}
