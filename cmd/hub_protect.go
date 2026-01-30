package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
)

var hubProtectCmd = &cobra.Command{
	Use:   "protect [type/name...]",
	Short: "Protect hub items from pruning",
	Long: `Mark hub items as protected so they won't be removed by 'hub prune'.

Protected items are kept even if unused by any profile.
Use 'hub unprotect' to remove protection.

Examples:
  ccp hub protect skills/debugging        # Protect a specific item
  ccp hub protect hooks/pre-commit        # Protect a hook
  ccp hub protect -i                      # Interactive selection`,
	RunE: runHubProtect,
}

var hubUnprotectCmd = &cobra.Command{
	Use:   "unprotect [type/name...]",
	Short: "Remove protection from hub items",
	Long: `Remove protection from hub items, allowing them to be pruned.

Examples:
  ccp hub unprotect skills/debugging      # Unprotect a specific item
  ccp hub unprotect -i                    # Interactive selection`,
	RunE: runHubUnprotect,
}

var (
	protectInteractive bool
	protectList        bool
)

func init() {
	hubProtectCmd.Flags().BoolVarP(&protectInteractive, "interactive", "i", false, "Interactive selection")
	hubProtectCmd.Flags().BoolVarP(&protectList, "list", "l", false, "List protected items")
	hubUnprotectCmd.Flags().BoolVarP(&protectInteractive, "interactive", "i", false, "Interactive selection")
	hubCmd.AddCommand(hubProtectCmd)
	hubCmd.AddCommand(hubUnprotectCmd)
}

const protectedFileName = ".protected"

func getProtectedFile(paths *config.Paths) string {
	return filepath.Join(paths.HubDir, protectedFileName)
}

func loadProtectedItems(paths *config.Paths) (map[string]bool, error) {
	protected := make(map[string]bool)
	filePath := getProtectedFile(paths)

	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return protected, nil
	}
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			protected[line] = true
		}
	}

	return protected, nil
}

func saveProtectedItems(paths *config.Paths, protected map[string]bool) error {
	filePath := getProtectedFile(paths)

	var items []string
	for item := range protected {
		items = append(items, item)
	}
	sort.Strings(items)

	content := "# Protected hub items (won't be removed by 'hub prune')\n"
	for _, item := range items {
		content += item + "\n"
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func runHubProtect(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	protected, err := loadProtectedItems(paths)
	if err != nil {
		return fmt.Errorf("failed to load protected items: %w", err)
	}

	// List mode
	if protectList {
		if len(protected) == 0 {
			fmt.Println("No protected items")
			return nil
		}

		var items []string
		for item := range protected {
			items = append(items, item)
		}
		sort.Strings(items)

		fmt.Printf("Protected items (%d):\n", len(items))
		for _, item := range items {
			fmt.Printf("  %s\n", item)
		}
		return nil
	}

	// Get items to protect
	var toProtect []string

	if protectInteractive || len(args) == 0 {
		// Scan hub for all items
		scanner := hub.NewScanner()
		h, err := scanner.Scan(paths.HubDir)
		if err != nil {
			return fmt.Errorf("failed to scan hub: %w", err)
		}

		allItems := h.AllItems()
		if len(allItems) == 0 {
			fmt.Println("Hub is empty")
			return nil
		}

		var pickerItems []picker.Item
		for _, item := range allItems {
			key := fmt.Sprintf("%s/%s", item.Type, item.Name)
			label := key
			if protected[key] {
				label += " (protected)"
			}
			pickerItems = append(pickerItems, picker.Item{
				ID:       key,
				Label:    label,
				Selected: false,
			})
		}

		selected, err := picker.Run("Select items to protect:", pickerItems)
		if err != nil {
			return fmt.Errorf("picker error: %w", err)
		}

		if len(selected) == 0 {
			fmt.Println("No items selected")
			return nil
		}

		toProtect = selected
	} else {
		toProtect = args
	}

	// Validate and protect items
	added := 0
	for _, item := range toProtect {
		// Validate format
		if !strings.Contains(item, "/") {
			fmt.Printf("  Skipped: %s (invalid format, use type/name)\n", item)
			continue
		}

		if protected[item] {
			fmt.Printf("  Already protected: %s\n", item)
			continue
		}

		protected[item] = true
		fmt.Printf("  Protected: %s\n", item)
		added++
	}

	if added > 0 {
		if err := saveProtectedItems(paths, protected); err != nil {
			return fmt.Errorf("failed to save protected items: %w", err)
		}
		fmt.Printf("\nAdded %d protected items\n", added)
	}

	return nil
}

func runHubUnprotect(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	protected, err := loadProtectedItems(paths)
	if err != nil {
		return fmt.Errorf("failed to load protected items: %w", err)
	}

	if len(protected) == 0 {
		fmt.Println("No protected items")
		return nil
	}

	var toUnprotect []string

	if protectInteractive || len(args) == 0 {
		var items []string
		for item := range protected {
			items = append(items, item)
		}
		sort.Strings(items)

		var pickerItems []picker.Item
		for _, item := range items {
			pickerItems = append(pickerItems, picker.Item{
				ID:       item,
				Label:    item,
				Selected: false,
			})
		}

		selected, err := picker.Run("Select items to unprotect:", pickerItems)
		if err != nil {
			return fmt.Errorf("picker error: %w", err)
		}

		if len(selected) == 0 {
			fmt.Println("No items selected")
			return nil
		}

		toUnprotect = selected
	} else {
		toUnprotect = args
	}

	// Unprotect items
	removed := 0
	for _, item := range toUnprotect {
		if !protected[item] {
			fmt.Printf("  Not protected: %s\n", item)
			continue
		}

		delete(protected, item)
		fmt.Printf("  Unprotected: %s\n", item)
		removed++
	}

	if removed > 0 {
		if err := saveProtectedItems(paths, protected); err != nil {
			return fmt.Errorf("failed to save protected items: %w", err)
		}
		fmt.Printf("\nRemoved protection from %d items\n", removed)
	}

	return nil
}

// IsProtected checks if a hub item is protected (exported for use by prune)
func IsHubItemProtected(paths *config.Paths, itemKey string) bool {
	protected, err := loadProtectedItems(paths)
	if err != nil {
		return false
	}
	return protected[itemKey]
}
