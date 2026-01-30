package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var (
	hubUpdateAll    bool
	hubUpdateForce  bool
	hubUpdateDryRun bool
)

var hubUpdateCmd = &cobra.Command{
	Use:   "update [<type>/<name>]",
	Short: "Update hub items from their source repositories",
	Long: `Update hub items that were installed from GitHub.

Without arguments, shows updateable items and prompts for confirmation.
With a specific item, updates just that item.

Examples:
  ccp hub update                      # Show all updateable items
  ccp hub update --all                # Update all items
  ccp hub update skills/my-skill      # Update specific skill
  ccp hub update --dry-run            # Show what would be updated`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHubUpdate,
}

func init() {
	hubUpdateCmd.Flags().BoolVarP(&hubUpdateAll, "all", "a", false, "Update all items without prompting")
	hubUpdateCmd.Flags().BoolVarP(&hubUpdateForce, "force", "f", false, "Force update even if local changes detected")
	hubUpdateCmd.Flags().BoolVarP(&hubUpdateDryRun, "dry-run", "n", false, "Show what would be updated without making changes")
	hubCmd.AddCommand(hubUpdateCmd)
}

func runHubUpdate(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// If specific item requested
	if len(args) > 0 {
		return updateSpecificItem(paths, h, args[0])
	}

	// Find all updateable items
	var updateable []hub.Item
	var manual []hub.Item

	for _, item := range h.AllItems() {
		if item.Source != nil && item.Source.CanUpdate() {
			updateable = append(updateable, item)
		} else if item.IsDir {
			manual = append(manual, item)
		}
	}

	if len(updateable) == 0 {
		fmt.Println("No updateable items found in hub")
		if len(manual) > 0 {
			fmt.Printf("  %d items without source tracking (manually added)\n", len(manual))
		}
		return nil
	}

	// Show updateable items
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "TYPE\tNAME\tSOURCE\n")
	fmt.Fprintf(w, "----\t----\t------\n")
	for _, item := range updateable {
		fmt.Fprintf(w, "%s\t%s\t%s\n", item.Type, item.Name, item.Source.SourceInfo())
	}
	w.Flush()
	fmt.Println()

	if hubUpdateDryRun {
		fmt.Printf("Would update %d items\n", len(updateable))
		return nil
	}

	if !hubUpdateAll {
		fmt.Printf("Found %d updateable items. Use --all to update all.\n", len(updateable))
		return nil
	}

	// Update all items
	updated := 0
	failed := 0
	for _, item := range updateable {
		fmt.Printf("Updating %s/%s...\n", item.Type, item.Name)
		if err := updateItem(paths, item); err != nil {
			fmt.Printf("  Error: %v\n", err)
			failed++
		} else {
			fmt.Printf("  Updated\n")
			updated++
		}
	}

	fmt.Println()
	fmt.Printf("Updated %d items", updated)
	if failed > 0 {
		fmt.Printf(", %d failed", failed)
	}
	fmt.Println()

	return nil
}

func updateSpecificItem(paths *config.Paths, h *hub.Hub, itemPath string) error {
	// Parse type/name
	parts := strings.SplitN(itemPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid item path: %s (expected type/name)", itemPath)
	}

	itemType := config.HubItemType(parts[0])
	itemName := parts[1]

	item := h.GetItem(itemType, itemName)
	if item == nil {
		return fmt.Errorf("item not found: %s", itemPath)
	}

	if item.Source == nil || !item.Source.CanUpdate() {
		return fmt.Errorf("item has no source tracking (manually added)")
	}

	if hubUpdateDryRun {
		fmt.Printf("Would update %s/%s from %s\n", item.Type, item.Name, item.Source.SourceInfo())
		return nil
	}

	fmt.Printf("Updating %s/%s from %s...\n", item.Type, item.Name, item.Source.SourceInfo())
	if err := updateItem(paths, *item); err != nil {
		return err
	}
	fmt.Println("Updated successfully")
	return nil
}

func updateItem(paths *config.Paths, item hub.Item) error {
	if item.Source == nil {
		return fmt.Errorf("no source information")
	}

	switch item.Source.Type {
	case hub.SourceTypeGitHub:
		return updateFromGitHub(paths, item)
	case hub.SourceTypePlugin:
		return fmt.Errorf("use 'ccp plugin update %s' to update plugin components", item.Source.Plugin.Name)
	default:
		return fmt.Errorf("unsupported source type: %s", item.Source.Type)
	}
}

func updateFromGitHub(paths *config.Paths, item hub.Item) error {
	src := item.Source.GitHub
	if src == nil {
		return fmt.Errorf("missing GitHub source information")
	}

	// Clone the repo to temp dir
	tempDir, err := os.MkdirTemp("", "ccp-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", src.Owner, src.Repo)
	gitCmd := exec.Command("git", "clone", "--depth", "1", repoURL, tempDir)
	gitCmd.Stdout = io.Discard
	gitCmd.Stderr = io.Discard

	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repo: %w", err)
	}

	// Get new commit SHA
	newCommit := getUpdateGitCommit(tempDir)

	// Check if already up to date
	if src.Commit != "" && src.Commit == newCommit {
		return fmt.Errorf("already up to date")
	}

	// Find source directory in repo
	sourceDir := tempDir
	if src.Path != "" && src.Path != "." {
		sourceDir = filepath.Join(tempDir, src.Path)
	}

	if _, err := os.Stat(sourceDir); err != nil {
		return fmt.Errorf("source path not found in repo: %s", src.Path)
	}

	// Remove old item (keeping backup if force not set)
	if !hubUpdateForce {
		// TODO: Check for local modifications
	}

	// Remove existing item
	if err := os.RemoveAll(item.Path); err != nil {
		return fmt.Errorf("failed to remove old item: %w", err)
	}

	// Copy new content
	if err := copyUpdateDir(sourceDir, item.Path); err != nil {
		return fmt.Errorf("failed to copy updated content: %w", err)
	}

	// Update source manifest
	newSource := hub.NewGitHubSource(src.Owner, src.Repo, src.Ref, newCommit, src.Path)
	newSource.InstalledAt = item.Source.InstalledAt // Preserve original install time
	if err := newSource.Save(item.Path); err != nil {
		return fmt.Errorf("failed to update source tracking: %w", err)
	}

	return nil
}

func getUpdateGitCommit(repoDir string) string {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func copyUpdateDir(src, dst string) error {
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

		// Skip .git directory
		if entry.Name() == ".git" {
			continue
		}

		if entry.IsDir() {
			if err := copyUpdateDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyUpdateFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyUpdateFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
