package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
)

var (
	hubAddFromProfile string
	hubAddReplace     bool
)

var hubAddCmd = &cobra.Command{
	Use:   "add <type> <name-or-path>",
	Short: "Add an item to the hub",
	Long: `Add a file/directory to the hub, or promote an item from a profile.

Types: skills, agents, hooks, rules, commands, md-fragments

Examples:
  # Add from filesystem
  ccp hub add skills ./my-skill.md
  ccp hub add agents ./my-agent/
  ccp hub add hooks ~/projects/shared-hooks/pre-commit.sh

  # Promote from profile to hub
  ccp hub add skills my-skill --from-profile=default
  ccp hub add hooks session-start --from-profile=dev

  # Replace existing hub item
  ccp hub add skills my-skill --from-profile=default --replace`,
	Args: cobra.ExactArgs(2),
	RunE: runHubAdd,
}

func init() {
	hubAddCmd.Flags().StringVar(&hubAddFromProfile, "from-profile", "", "Promote item from specified profile to hub")
	hubAddCmd.Flags().BoolVar(&hubAddReplace, "replace", false, "Replace existing hub item if it exists")
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

	// Validate type
	if !isValidHubType(itemType) {
		return fmt.Errorf("invalid type: %s (valid: skills, agents, hooks, rules, commands, md-fragments)", args[0])
	}

	if hubAddFromProfile != "" {
		return runHubAddFromProfile(paths, itemType, args[1])
	}

	return runHubAddFromPath(paths, itemType, args[1])
}

func runHubAddFromProfile(paths *config.Paths, itemType config.HubItemType, itemName string) error {
	mgr := profile.NewManager(paths)

	// Get the profile
	p, err := mgr.Get(hubAddFromProfile)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", hubAddFromProfile)
	}

	// Find the item in the profile
	profileItemPath := filepath.Join(p.Path, string(itemType), itemName)

	// Check if it's a symlink (already linked from hub)
	symMgr := symlink.New()
	isLink, _ := symMgr.IsSymlink(profileItemPath)
	if isLink {
		target, err := symMgr.ReadLink(profileItemPath)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}
		// Check if it points to hub
		if strings.HasPrefix(target, paths.HubDir) {
			return fmt.Errorf("item '%s' is already linked from hub (target: %s)", itemName, target)
		}
		// Resolve to actual content
		profileItemPath = target
	}

	// Check source exists
	srcInfo, err := os.Stat(profileItemPath)
	if err != nil {
		return fmt.Errorf("item not found in profile: %s/%s", itemType, itemName)
	}

	// Check if already exists in hub
	dstPath := paths.HubItemPath(itemType, itemName)
	if _, err := os.Stat(dstPath); err == nil {
		if !hubAddReplace {
			return fmt.Errorf("item already exists in hub: %s/%s (use --replace to overwrite)", itemType, itemName)
		}
		// Remove existing
		if err := os.RemoveAll(dstPath); err != nil {
			return fmt.Errorf("failed to remove existing hub item: %w", err)
		}
		fmt.Printf("Replacing existing hub item: %s/%s\n", itemType, itemName)
	}

	// Copy to hub
	if srcInfo.IsDir() {
		if err := copyDirRecursive(profileItemPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy directory: %w", err)
		}
	} else {
		if err := copyFileSimple(profileItemPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}
	}

	fmt.Printf("Added %s/%s to hub from profile '%s'\n", itemType, itemName, hubAddFromProfile)

	// Offer to replace profile item with symlink
	fmt.Printf("\nTo link this item back to the profile, run:\n")
	fmt.Printf("  ccp link %s %s --profile=%s\n", itemType, itemName, hubAddFromProfile)

	return nil
}

func runHubAddFromPath(paths *config.Paths, itemType config.HubItemType, srcPath string) error {
	// Resolve source path
	srcPath, err := filepath.Abs(srcPath)
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
		if !hubAddReplace {
			return fmt.Errorf("item already exists: %s/%s (use --replace to overwrite)", itemType, itemName)
		}
		// Remove existing
		if err := os.RemoveAll(dstPath); err != nil {
			return fmt.Errorf("failed to remove existing hub item: %w", err)
		}
		fmt.Printf("Replacing existing hub item: %s/%s\n", itemType, itemName)
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
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dest, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}

func copyDirRecursive(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
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
