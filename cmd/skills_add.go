package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
)

var (
	skillsAddGlobal bool
	skillsAddYes    bool
)

var skillsAddCmd = &cobra.Command{
	Use:   "add <source>",
	Short: "Install a skill from GitHub",
	Long: `Install a skill from GitHub into your hub.

Source formats:
  owner/repo@skill     - Install specific skill from repo
  owner/repo           - Install all skills from repo
  https://github.com/owner/repo  - Full URL

Examples:
  ccp skills add vercel-labs/agent-skills@vercel-react-best-practices
  ccp skills add ComposioHQ/awesome-claude-skills@debugging
  ccp skills add owner/repo --skill=my-skill

The skill will be cloned and copied to ~/.ccp/hub/skills/`,
	Args: cobra.ExactArgs(1),
	RunE: runSkillsAdd,
}

func init() {
	skillsAddCmd.Flags().BoolVarP(&skillsAddGlobal, "global", "g", false, "Install globally (user-level)")
	skillsAddCmd.Flags().BoolVarP(&skillsAddYes, "yes", "y", false, "Skip confirmation prompts")
	skillsCmd.AddCommand(skillsAddCmd)
}

func runSkillsAdd(cmd *cobra.Command, args []string) error {
	source := args[0]

	// Parse source
	owner, repo, skillName, err := parseSkillSource(source)
	if err != nil {
		return err
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Check if skill already exists
	targetDir := filepath.Join(paths.HubDir, "skills", skillName)
	if _, err := os.Stat(targetDir); err == nil {
		return fmt.Errorf("skill already exists: %s\n  Use 'ccp hub remove skills/%s' first to replace it", skillName, skillName)
	}

	fmt.Printf("Installing skill '%s' from %s/%s...\n", skillName, owner, repo)

	// Clone the repo to temp dir
	tempDir, err := os.MkdirTemp("", "ccp-skill-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	gitCmd := exec.Command("git", "clone", "--depth", "1", repoURL, tempDir)
	gitCmd.Stdout = io.Discard
	gitCmd.Stderr = os.Stderr

	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repo: %w\n  Make sure the repo exists and is accessible", err)
	}

	// Find the skill in the repo
	skillSourceDir, err := findSkillInRepo(tempDir, skillName)
	if err != nil {
		return err
	}

	// Copy skill to hub
	if err := copyDir(skillSourceDir, targetDir); err != nil {
		return fmt.Errorf("failed to copy skill: %w", err)
	}

	fmt.Printf("Installed skill: %s\n", skillName)
	fmt.Printf("Location: %s\n", targetDir)
	fmt.Println()
	fmt.Println("To use this skill, add it to a profile:")
	fmt.Printf("  ccp link <profile> skills/%s\n", skillName)

	return nil
}

// parseSkillSource parses source formats like "owner/repo@skill" or "owner/repo"
func parseSkillSource(source string) (owner, repo, skill string, err error) {
	// Remove https://github.com/ prefix if present
	source = strings.TrimPrefix(source, "https://github.com/")
	source = strings.TrimPrefix(source, "github.com/")
	source = strings.TrimSuffix(source, ".git")

	// Check for @ separator
	if atIdx := strings.LastIndex(source, "@"); atIdx > 0 {
		skill = source[atIdx+1:]
		source = source[:atIdx]
	}

	// Parse owner/repo
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid source format: %s\n  Expected: owner/repo or owner/repo@skill", source)
	}

	owner = parts[0]
	repo = parts[1]

	if skill == "" {
		// Use repo name as skill name if not specified
		skill = repo
	}

	// Validate
	validName := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validName.MatchString(owner) || !validName.MatchString(repo) {
		return "", "", "", fmt.Errorf("invalid owner/repo: %s/%s", owner, repo)
	}

	return owner, repo, skill, nil
}

// findSkillInRepo looks for a skill in common locations
func findSkillInRepo(repoDir, skillName string) (string, error) {
	// Common skill locations to check
	locations := []string{
		filepath.Join(repoDir, "skills", skillName),           // skills/<name>/
		filepath.Join(repoDir, skillName),                     // <name>/
		filepath.Join(repoDir, "src", skillName),              // src/<name>/
		filepath.Join(repoDir, ".claude", "skills", skillName), // .claude/skills/<name>/
	}

	// Check each location
	for _, loc := range locations {
		if isValidSkillDir(loc) {
			return loc, nil
		}
	}

	// If skill name matches repo, check if repo root is a skill
	if isValidSkillDir(repoDir) {
		return repoDir, nil
	}

	// List available skills
	available := findAvailableSkills(repoDir)
	if len(available) > 0 {
		return "", fmt.Errorf("skill '%s' not found in repo\n  Available skills: %s", skillName, strings.Join(available, ", "))
	}

	return "", fmt.Errorf("skill '%s' not found in repo\n  No SKILL.md files found", skillName)
}

// isValidSkillDir checks if directory contains a valid skill (has SKILL.md)
func isValidSkillDir(dir string) bool {
	skillFile := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(skillFile); err == nil {
		return true
	}
	return false
}

// findAvailableSkills scans repo for directories with SKILL.md
func findAvailableSkills(repoDir string) []string {
	var skills []string

	// Check skills/ directory
	skillsDir := filepath.Join(repoDir, "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && isValidSkillDir(filepath.Join(skillsDir, e.Name())) {
				skills = append(skills, e.Name())
			}
		}
	}

	// Check root for SKILL.md
	if isValidSkillDir(repoDir) {
		// Get repo name from path
		skills = append(skills, filepath.Base(repoDir))
	}

	return skills
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	// Create destination
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
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func copyFile(src, dst string) error {
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

// fetchSkillInfo fetches skill info from skills.sh API (optional, for metadata)
func fetchSkillInfo(owner, repo, skill string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/skills/%s/%s/%s", skillsAPIBase, owner, repo, skill)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skill not found on skills.sh")
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
