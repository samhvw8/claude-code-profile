package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose and fix common issues",
	Long: `Check for common ccp issues and optionally fix them.

Checks:
- Is ~/.claude a symlink?
- Are there broken symlinks?
- Are profile manifests valid?
- Is the hub structure correct?`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	fmt.Println("=== CCP Doctor ===")
	fmt.Println()

	issues := 0

	// Check 1: Is ccp initialized?
	fmt.Print("Checking initialization... ")
	if !paths.IsInitialized() {
		fmt.Println("FAIL")
		fmt.Println("  → ccp is not initialized. Run 'ccp init' first.")
		return nil
	}
	fmt.Println("OK")

	// Check 2: Is ~/.claude a symlink?
	fmt.Print("Checking ~/.claude symlink... ")
	if !paths.ClaudeDirIsSymlink() {
		fmt.Println("FAIL")
		fmt.Println("  → ~/.claude is not a symlink")
		fmt.Println("  → Run 'ccp init --force' to reinitialize")
		issues++
	} else {
		target, err := os.Readlink(paths.ClaudeDir)
		if err != nil {
			fmt.Println("FAIL")
			fmt.Printf("  → Cannot read symlink: %v\n", err)
			issues++
		} else if _, err := os.Stat(target); os.IsNotExist(err) {
			fmt.Println("FAIL")
			fmt.Printf("  → Symlink target does not exist: %s\n", target)
			issues++
		} else {
			fmt.Printf("OK → %s\n", filepath.Base(target))
		}
	}

	// Check 3: Hub structure
	fmt.Print("Checking hub structure... ")
	hubIssues := 0
	for _, itemType := range config.AllHubItemTypes() {
		dir := paths.HubItemDir(itemType)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			hubIssues++
		}
	}
	if hubIssues > 0 {
		fmt.Printf("WARN (%d missing directories)\n", hubIssues)
		fmt.Println("  → Run 'ccp init --force' to fix")
	} else {
		fmt.Println("OK")
	}

	// Check 4: Profile manifests
	fmt.Print("Checking profile manifests... ")
	profileIssues := 0
	entries, _ := os.ReadDir(paths.ProfilesDir)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		manifestPath := filepath.Join(paths.ProfilesDir, entry.Name(), "profile.yaml")
		if _, err := profile.LoadManifest(manifestPath); err != nil {
			profileIssues++
			fmt.Printf("\n  → Invalid manifest in profile '%s': %v", entry.Name(), err)
		}
	}
	if profileIssues > 0 {
		fmt.Println()
		issues += profileIssues
	} else {
		fmt.Println("OK")
	}

	// Check 5: Broken symlinks
	fmt.Print("Checking for broken symlinks... ")
	brokenLinks := findBrokenSymlinks(paths.ProfilesDir)
	if len(brokenLinks) > 0 {
		fmt.Printf("WARN (%d broken)\n", len(brokenLinks))
		for _, link := range brokenLinks[:min(5, len(brokenLinks))] {
			fmt.Printf("  → %s\n", link)
		}
		if len(brokenLinks) > 5 {
			fmt.Printf("  → ... and %d more\n", len(brokenLinks)-5)
		}
		fmt.Println("  → Run 'ccp profile fix <name>' to repair")
	} else {
		fmt.Println("OK")
	}

	// Summary
	fmt.Println()
	if issues == 0 {
		fmt.Println("All checks passed!")
	} else {
		fmt.Printf("Found %d issue(s)\n", issues)
	}

	return nil
}

func findBrokenSymlinks(dir string) []string {
	var broken []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Check if it's a symlink by using Lstat
		linfo, err := os.Lstat(path)
		if err != nil {
			return nil
		}
		if linfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink, check if target exists
			if _, err := os.Stat(path); os.IsNotExist(err) {
				broken = append(broken, path)
			}
		}
		return nil
	})
	return broken
}
