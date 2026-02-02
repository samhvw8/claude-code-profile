package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
)

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st"},
	Short:   "Show ccp status and health",
	Long: `Display comprehensive status of ccp configuration.

Shows:
- Active profile
- Hub item counts
- Profile health (drift, broken symlinks)
- Overall system health`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		fmt.Println("Status: NOT INITIALIZED")
		fmt.Println()
		fmt.Println("Run 'ccp init' to initialize ccp")
		return nil
	}

	fmt.Println("=== CCP Status ===")
	fmt.Println()

	// Active profile
	activeProfile := "none"
	if paths.ClaudeDirIsSymlink() {
		target, err := os.Readlink(paths.ClaudeDir)
		if err == nil {
			activeProfile = filepath.Base(target)
		}
	}
	fmt.Printf("Active Profile: %s\n", activeProfile)
	fmt.Printf("CCP Directory:  %s\n", paths.CcpDir)
	fmt.Println()

	// Hub summary
	fmt.Println("--- Hub ---")
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		fmt.Printf("Error scanning hub: %v\n", err)
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, itemType := range config.AllHubItemTypes() {
			count := len(h.GetItems(itemType))
			if count > 0 {
				fmt.Fprintf(w, "  %s:\t%d\n", itemType, count)
			}
		}
		w.Flush()
		fmt.Printf("  Total: %d items\n", h.ItemCount())
	}
	fmt.Println()

	// Profiles summary
	fmt.Println("--- Profiles ---")
	profiles, err := listAllProfiles(paths)
	if err != nil {
		fmt.Printf("Error listing profiles: %v\n", err)
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, p := range profiles {
			status := "ok"
			if p.name == activeProfile {
				status = "active"
			}
			if p.hasDrift {
				status = "drift"
			}
			if p.brokenLinks > 0 {
				status = fmt.Sprintf("broken:%d", p.brokenLinks)
			}
			fmt.Fprintf(w, "  %s\t[%s]\n", p.name, status)
		}
		w.Flush()
		fmt.Printf("  Total: %d profiles\n", len(profiles))
	}
	fmt.Println()

	// Health check
	fmt.Println("--- Health ---")
	issues := checkHealth(paths)
	if len(issues) == 0 {
		fmt.Println("  All systems healthy")
	} else {
		for _, issue := range issues {
			fmt.Printf("  ⚠ %s\n", issue)
		}
	}

	return nil
}

type profileInfo struct {
	name        string
	hasDrift    bool
	brokenLinks int
}

func listAllProfiles(paths *config.Paths) ([]profileInfo, error) {
	var profiles []profileInfo

	entries, err := os.ReadDir(paths.ProfilesDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		p := profileInfo{name: entry.Name()}

		// Check for drift/broken links
		profileDir := filepath.Join(paths.ProfilesDir, entry.Name())
		p.brokenLinks = countBrokenLinks(profileDir)

		// Check manifest drift
		manifestPath := profile.ManifestPath(profileDir)
		if _, err := profile.LoadManifest(manifestPath); err != nil {
			p.hasDrift = true
		}

		profiles = append(profiles, p)
	}

	return profiles, nil
}

func countBrokenLinks(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				count++
			}
		}
		return nil
	})
	return count
}

func checkHealth(paths *config.Paths) []string {
	var issues []string

	// Check ~/.claude is symlink
	if !paths.ClaudeDirIsSymlink() {
		if paths.ClaudeDirExistsAsDir() {
			issues = append(issues, "~/.claude is a directory, not a symlink")
		}
	}

	// Check hub directory exists
	if _, err := os.Stat(paths.HubDir); os.IsNotExist(err) {
		issues = append(issues, "Hub directory missing")
	}

	// Check profiles directory exists
	if _, err := os.Stat(paths.ProfilesDir); os.IsNotExist(err) {
		issues = append(issues, "Profiles directory missing")
	}

	// Check active profile symlink target exists
	if paths.ClaudeDirIsSymlink() {
		target, err := os.Readlink(paths.ClaudeDir)
		if err == nil {
			if _, err := os.Stat(target); os.IsNotExist(err) {
				issues = append(issues, fmt.Sprintf("Active profile target missing: %s", target))
			}
		}
	}

	return issues
}
