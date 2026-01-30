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
	skillsUpdateAll    bool
	skillsUpdateForce  bool
	skillsUpdateDryRun bool
)

var skillsUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update installed skills from GitHub",
	Long: `Update skills that were installed from GitHub.

Without arguments, shows all updateable skills.
With a name, updates just that skill.

Examples:
  ccp skills update                   # Show all updateable skills
  ccp skills update --all             # Update all skills
  ccp skills update my-skill          # Update specific skill
  ccp skills update --dry-run         # Show what would be updated`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSkillsUpdate,
}

func init() {
	skillsUpdateCmd.Flags().BoolVarP(&skillsUpdateAll, "all", "a", false, "Update all skills without prompting")
	skillsUpdateCmd.Flags().BoolVarP(&skillsUpdateForce, "force", "f", false, "Force update even if local changes detected")
	skillsUpdateCmd.Flags().BoolVarP(&skillsUpdateDryRun, "dry-run", "n", false, "Show what would be updated")
	skillsCmd.AddCommand(skillsUpdateCmd)
}

func runSkillsUpdate(cmd *cobra.Command, args []string) error {
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

	skills := h.GetItems(config.HubSkills)

	// If specific skill requested
	if len(args) > 0 {
		skillName := args[0]
		for _, skill := range skills {
			if skill.Name == skillName {
				return updateSkill(paths, skill)
			}
		}
		return fmt.Errorf("skill not found: %s", skillName)
	}

	// Find all updateable skills
	var updateable []hub.Item
	var manual []hub.Item

	for _, skill := range skills {
		if skill.Source != nil && skill.Source.Type == hub.SourceTypeGitHub {
			updateable = append(updateable, skill)
		} else if skill.IsDir {
			manual = append(manual, skill)
		}
	}

	if len(updateable) == 0 {
		fmt.Println("No updateable skills found")
		if len(manual) > 0 {
			fmt.Printf("  %d skills without source tracking (manually added)\n", len(manual))
		}
		return nil
	}

	// Show updateable skills
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tSOURCE\n")
	fmt.Fprintf(w, "----\t------\n")
	for _, skill := range updateable {
		fmt.Fprintf(w, "%s\t%s\n", skill.Name, skill.Source.SourceInfo())
	}
	w.Flush()
	fmt.Println()

	if skillsUpdateDryRun {
		fmt.Printf("Would update %d skills\n", len(updateable))
		return nil
	}

	if !skillsUpdateAll {
		fmt.Printf("Found %d updateable skills. Use --all to update all.\n", len(updateable))
		return nil
	}

	// Update all skills
	updated := 0
	failed := 0
	for _, skill := range updateable {
		fmt.Printf("Updating %s...\n", skill.Name)
		if err := updateSkillFromGitHub(paths, skill); err != nil {
			fmt.Printf("  Error: %v\n", err)
			failed++
		} else {
			fmt.Printf("  Updated\n")
			updated++
		}
	}

	fmt.Println()
	fmt.Printf("Updated %d skills", updated)
	if failed > 0 {
		fmt.Printf(", %d failed", failed)
	}
	fmt.Println()

	return nil
}

func updateSkill(paths *config.Paths, skill hub.Item) error {
	if skill.Source == nil {
		return fmt.Errorf("skill has no source tracking (manually added)")
	}

	if skill.Source.Type == hub.SourceTypePlugin {
		return fmt.Errorf("skill installed via plugin '%s', use 'ccp plugin update %s' instead",
			skill.Source.Plugin.Name, skill.Source.Plugin.Name)
	}

	if skill.Source.Type != hub.SourceTypeGitHub {
		return fmt.Errorf("skill has no GitHub source")
	}

	if skillsUpdateDryRun {
		fmt.Printf("Would update %s from %s\n", skill.Name, skill.Source.SourceInfo())
		return nil
	}

	fmt.Printf("Updating %s from %s...\n", skill.Name, skill.Source.SourceInfo())
	if err := updateSkillFromGitHub(paths, skill); err != nil {
		return err
	}
	fmt.Println("Updated successfully")
	return nil
}

func updateSkillFromGitHub(paths *config.Paths, skill hub.Item) error {
	src := skill.Source.GitHub
	if src == nil {
		return fmt.Errorf("missing GitHub source information")
	}

	// Clone the repo to temp dir
	tempDir, err := os.MkdirTemp("", "ccp-skill-update-*")
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
	newCommit := getSkillUpdateCommit(tempDir)

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

	// Remove existing skill
	if err := os.RemoveAll(skill.Path); err != nil {
		return fmt.Errorf("failed to remove old skill: %w", err)
	}

	// Copy new content
	if err := copySkillUpdateDir(sourceDir, skill.Path); err != nil {
		return fmt.Errorf("failed to copy updated content: %w", err)
	}

	// Update source manifest
	newSource := hub.NewGitHubSource(src.Owner, src.Repo, src.Ref, newCommit, src.Path)
	newSource.InstalledAt = skill.Source.InstalledAt
	if err := newSource.Save(skill.Path); err != nil {
		return fmt.Errorf("failed to update source tracking: %w", err)
	}

	return nil
}

func getSkillUpdateCommit(repoDir string) string {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func copySkillUpdateDir(src, dst string) error {
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

		if entry.Name() == ".git" {
			continue
		}

		if entry.IsDir() {
			if err := copySkillUpdateDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copySkillUpdateFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copySkillUpdateFile(src, dst string) error {
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
