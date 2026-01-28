package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/migration"
)

var (
	resetForce bool
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove ccp and restore original ~/.claude directory",
	Long: `Undo ccp initialization and restore ~/.claude as a regular directory.

This command:
1. Copies the active profile contents to ~/.claude (replacing the symlink)
2. Removes ~/.ccp directory entirely

WARNING: This will remove all profiles except the currently active one.
Hub items are preserved in the restored ~/.claude directory.`,
	RunE: runReset,
}

func init() {
	resetCmd.Flags().BoolVarP(&resetForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(resetCmd)
}

func runReset(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	// Check if ccp is initialized
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp is not initialized (no ~/.ccp directory)")
	}

	// Check if ~/.claude is a symlink
	if !paths.ClaudeDirIsSymlink() {
		return fmt.Errorf("~/.claude is not a symlink - ccp may not be properly initialized")
	}

	// Get the active profile (symlink target)
	target, err := os.Readlink(paths.ClaudeDir)
	if err != nil {
		return fmt.Errorf("failed to read symlink: %w", err)
	}

	fmt.Println("Reset Plan:")
	fmt.Println()
	fmt.Printf("  Active profile: %s\n", target)
	fmt.Printf("  Will restore to: %s\n", paths.ClaudeDir)
	fmt.Printf("  Will remove: %s\n", paths.CcpDir)
	fmt.Println()

	if !resetForce {
		fmt.Print("This will remove all profiles except the active one. Continue? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted")
			return nil
		}
	}

	fmt.Println("Executing reset...")

	resetter := migration.NewResetter(paths)
	if err := resetter.Execute(); err != nil {
		return fmt.Errorf("reset failed: %w", err)
	}

	fmt.Println()
	fmt.Println("Reset complete!")
	fmt.Println()
	fmt.Printf("Your Claude Code configuration has been restored to %s\n", paths.ClaudeDir)

	return nil
}
