package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
)

var whichPathFlag bool

var whichCmd = &cobra.Command{
	Use:   "which",
	Short: "Show which profile is currently active",
	Long: `Display the currently active Claude Code profile.

Checks CLAUDE_CONFIG_DIR environment variable first,
then falls back to the ~/.claude symlink target.

Use --path to output just the directory path (for scripts/aliases).`,
	RunE: runWhich,
}

func init() {
	whichCmd.Flags().BoolVar(&whichPathFlag, "path", false, "Output only the profile directory path")
	rootCmd.AddCommand(whichCmd)
}

func runWhich(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	// Check CLAUDE_CONFIG_DIR first
	envDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if envDir != "" {
		if whichPathFlag {
			fmt.Println(envDir)
		} else {
			profileName := filepath.Base(envDir)
			fmt.Printf("%s (from CLAUDE_CONFIG_DIR)\n", profileName)
		}
		return nil
	}

	// Check if ccp is initialized
	if !paths.IsInitialized() {
		if whichPathFlag {
			// Return empty for scripts
			return nil
		}
		fmt.Println("ccp not initialized")
		return nil
	}

	// Check symlink
	if !paths.ClaudeDirIsSymlink() {
		if whichPathFlag {
			// Return ~/.claude as fallback
			fmt.Println(paths.ClaudeDir)
		} else {
			fmt.Println("none (not using ccp profiles)")
		}
		return nil
	}

	target, err := os.Readlink(paths.ClaudeDir)
	if err != nil {
		return fmt.Errorf("failed to read symlink: %w", err)
	}

	if whichPathFlag {
		fmt.Println(target)
	} else {
		profileName := filepath.Base(target)
		fmt.Println(profileName)
	}

	return nil
}
