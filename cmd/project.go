package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/source"
)

// projectHubItemTypes are the hub types valid for project commands (excludes settings-templates)
var projectHubItemTypes = []config.HubItemType{
	config.HubSkills,
	config.HubAgents,
	config.HubHooks,
	config.HubRules,
	config.HubCommands,
}

var projectDirFlag string
var projectAddInteractive bool

var projectCmd = &cobra.Command{
	Use:     "project",
	Aliases: []string{"proj"},
	Short:   "Manage hub items in the current project's .claude/ directory",
	Long:    `Copy hub items into or manage them within the current project's .claude/ directory.`,
}

var projectAddCmd = &cobra.Command{
	Use:   "add [type/name...]",
	Short: "Copy hub items into the project's .claude/ directory",
	Long: `Copy hub items from the ccp hub into the current project's .claude/ directory.

Items are copied (not symlinked), so they become local to the project.

Examples:
  ccp project add skills/coding agents/reviewer   # Copy specific items
  ccp project add -i                               # Interactive picker
  ccp project add --dir /path/to/project skills/x  # Specify project root`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeProjectAddArgs,
	RunE:              runProjectAdd,
}

var projectListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List hub items in the project's .claude/ directory",
	Long: `Scan the current project's .claude/ directory and list items by type.

Examples:
  ccp project list
  ccp project list --dir /path/to/project`,
	RunE: runProjectList,
}

var projectRemoveCmd = &cobra.Command{
	Use:     "remove [type/name...]",
	Aliases: []string{"rm"},
	Short:   "Remove hub items from the project's .claude/ directory",
	Long: `Remove items from the current project's .claude/ directory.

Examples:
  ccp project remove skills/coding
  ccp project remove skills/coding agents/reviewer`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeProjectRemoveArgs,
	RunE:              runProjectRemove,
}

func init() {
	rootCmd.AddCommand(projectCmd)

	projectCmd.PersistentFlags().StringVar(&projectDirFlag, "dir", "", "Project root directory (default: git root or cwd)")

	projectAddCmd.Flags().BoolVarP(&projectAddInteractive, "interactive", "i", false, "Interactive picker for hub items")
	projectCmd.AddCommand(projectAddCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectRemoveCmd)
}

// findProjectClaudeDir locates the .claude/ directory for the current project.
// Walks up from cwd to find .git/, uses <git-root>/.claude/. Falls back to cwd/.claude/.
func findProjectClaudeDir(dirFlag string) (string, error) {
	if dirFlag != "" {
		return filepath.Join(dirFlag, ".claude"), nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return filepath.Join(dir, ".claude"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// No git root found, use cwd
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("failed to get working directory: %w", err)
			}
			return filepath.Join(cwd, ".claude"), nil
		}
		dir = parent
	}
}

// isValidProjectHubType checks if a type is valid for project commands
func isValidProjectHubType(t config.HubItemType) bool {
	for _, valid := range projectHubItemTypes {
		if t == valid {
			return true
		}
	}
	return false
}

// parseItemRef parses "type/name" into HubItemType and name, validating the type
func parseItemRef(ref string) (config.HubItemType, string, error) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid item format: %s (expected type/name)", ref)
	}

	itemType := config.HubItemType(parts[0])
	if !isValidProjectHubType(itemType) {
		return "", "", fmt.Errorf("invalid item type: %s (valid: skills, agents, hooks, rules, commands)", parts[0])
	}

	return itemType, parts[1], nil
}

func runProjectAdd(cmd *cobra.Command, args []string) error {
	if !projectAddInteractive && len(args) == 0 {
		return fmt.Errorf("specify items to add (type/name) or use -i for interactive mode")
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	claudeDir, err := findProjectClaudeDir(projectDirFlag)
	if err != nil {
		return err
	}

	if projectAddInteractive {
		return runProjectAddInteractive(paths, claudeDir)
	}

	return runProjectAddDirect(paths, claudeDir, args)
}

func runProjectAddDirect(paths *config.Paths, claudeDir string, items []string) error {
	for _, ref := range items {
		itemType, itemName, err := parseItemRef(ref)
		if err != nil {
			return err
		}

		srcPath := filepath.Join(paths.HubDir, string(itemType), itemName)
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf("hub item not found: %s/%s", itemType, itemName)
		}

		dstPath := filepath.Join(claudeDir, string(itemType), itemName)

		// Warn if overwriting
		if _, err := os.Stat(dstPath); err == nil {
			fmt.Printf("Warning: overwriting existing %s/%s\n", itemType, itemName)
			if err := os.RemoveAll(dstPath); err != nil {
				return fmt.Errorf("failed to remove existing item: %w", err)
			}
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if err := source.CopyTree(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy %s/%s: %w", itemType, itemName, err)
		}

		fmt.Printf("Added %s/%s to %s\n", itemType, itemName, claudeDir)
	}

	return nil
}

func runProjectAddInteractive(paths *config.Paths, claudeDir string) error {
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// Build tabs for the picker, excluding settings-templates
	var tabs []picker.Tab
	for _, itemType := range projectHubItemTypes {
		items := h.GetItems(itemType)
		if len(items) == 0 {
			continue
		}

		var pickerItems []picker.Item
		for _, item := range items {
			pickerItems = append(pickerItems, picker.Item{
				ID:    item.Name,
				Label: item.Name,
			})
		}

		tabs = append(tabs, picker.Tab{
			Name:  string(itemType),
			Items: pickerItems,
		})
	}

	if len(tabs) == 0 {
		fmt.Println("No hub items available to add")
		return nil
	}

	fmt.Printf("Select hub items to copy into %s\n\n", claudeDir)

	selections, err := picker.RunTabbed(tabs)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}
	if selections == nil {
		fmt.Println("Cancelled")
		return nil
	}

	// Copy selected items
	copied := 0
	for _, itemType := range projectHubItemTypes {
		names, ok := selections[string(itemType)]
		if !ok {
			continue
		}
		for _, name := range names {
			srcPath := filepath.Join(paths.HubDir, string(itemType), name)
			dstPath := filepath.Join(claudeDir, string(itemType), name)

			// Warn if overwriting
			if _, err := os.Stat(dstPath); err == nil {
				fmt.Printf("Warning: overwriting existing %s/%s\n", itemType, name)
				if err := os.RemoveAll(dstPath); err != nil {
					return fmt.Errorf("failed to remove existing item: %w", err)
				}
			}

			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			if err := source.CopyTree(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy %s/%s: %w", itemType, name, err)
			}

			fmt.Printf("Added %s/%s\n", itemType, name)
			copied++
		}
	}

	if copied == 0 {
		fmt.Println("No items selected")
	} else {
		fmt.Printf("\nCopied %d item(s) to %s\n", copied, claudeDir)
	}

	return nil
}

func runProjectList(cmd *cobra.Command, args []string) error {
	claudeDir, err := findProjectClaudeDir(projectDirFlag)
	if err != nil {
		return err
	}

	if _, err := os.Stat(claudeDir); err != nil {
		fmt.Printf("No .claude/ directory found at %s\n", claudeDir)
		return nil
	}

	scanner := hub.NewScanner()
	h, err := scanner.ScanSource(claudeDir)
	if err != nil {
		return fmt.Errorf("failed to scan %s: %w", claudeDir, err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	for _, itemType := range projectHubItemTypes {
		items := h.GetItems(itemType)
		if len(items) == 0 {
			continue
		}

		fmt.Fprintf(w, "\n%s:\n", itemType)
		for _, item := range items {
			typeIndicator := "file"
			if item.IsDir {
				typeIndicator = "dir"
			}
			fmt.Fprintf(w, "  %s\t(%s)\n", item.Name, typeIndicator)
		}
	}

	w.Flush()

	if h.ItemCount() == 0 {
		fmt.Printf("No hub items in %s\n", claudeDir)
	}

	return nil
}

func runProjectRemove(cmd *cobra.Command, args []string) error {
	claudeDir, err := findProjectClaudeDir(projectDirFlag)
	if err != nil {
		return err
	}

	for _, ref := range args {
		itemType, itemName, err := parseItemRef(ref)
		if err != nil {
			return err
		}

		itemPath := filepath.Join(claudeDir, string(itemType), itemName)
		if _, err := os.Stat(itemPath); err != nil {
			return fmt.Errorf("item not found: %s/%s in %s", itemType, itemName, claudeDir)
		}

		if err := os.RemoveAll(itemPath); err != nil {
			return fmt.Errorf("failed to remove %s/%s: %w", itemType, itemName, err)
		}

		fmt.Printf("Removed %s/%s from %s\n", itemType, itemName, claudeDir)
	}

	return nil
}
