package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
)

var hubAddCmd = &cobra.Command{
	Use:   "add <type> <path>",
	Short: "Add an item to the hub",
	Long: `Add a file or directory to the hub.

Types: skills, agents, hooks, rules, commands, md-fragments

Examples:
  ccp hub add skills ./my-skill.md
  ccp hub add agents ./my-agent/
  ccp hub add hooks ~/projects/shared-hooks/pre-commit.sh`,
	Args: cobra.ExactArgs(2),
	RunE: runHubAdd,
}

func init() {
	hubCmd.AddCommand(hubAddCmd)
}

func runHubAdd(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	itemType := config.HubItemType(args[0])
	srcPath := args[1]

	// Validate type
	if !isValidHubType(itemType) {
		return fmt.Errorf("invalid type: %s (valid: skills, agents, hooks, rules, commands, md-fragments)", args[0])
	}

	// Resolve source path
	srcPath, err = filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check source exists
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}

	// Get item name from source path
	itemName := filepath.Base(srcPath)

	// Check if already exists in hub
	dstPath := paths.HubItemPath(itemType, itemName)
	if _, err := os.Stat(dstPath); err == nil {
		return fmt.Errorf("item already exists: %s/%s", itemType, itemName)
	}

	// Copy to hub
	if srcInfo.IsDir() {
		if err := copyDirRecursive(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy directory: %w", err)
		}
	} else {
		if err := copyFileSimple(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}
	}

	fmt.Printf("Added %s/%s\n", itemType, itemName)
	return nil
}

func isValidHubType(t config.HubItemType) bool {
	for _, valid := range config.AllHubItemTypes() {
		if valid == t {
			return true
		}
	}
	return false
}

func copyFileSimple(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}

func copyDirRecursive(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFileSimple(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
