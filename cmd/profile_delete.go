package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var deleteForce bool

var profileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Long: `Delete a profile and all its contents.

This action is irreversible. Use --force to skip confirmation.
Note: The 'default' profile cannot be deleted if it's the only profile.`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileDelete,
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

	// Check if this is the active profile
	active, _ := mgr.GetActive()
	if active != nil && active.Name == profileName {
		return fmt.Errorf("cannot delete active profile '%s'\n\nSwitch to another profile first: ccp use <other-profile>", profileName)
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
