package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
)

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show hub item usage across profiles",
	Long: `Display which hub items are used by which profiles.

Helps identify:
- Orphaned items (not used by any profile)
- Shared items (used by multiple profiles)
- Missing items (referenced but not in hub)`,
	RunE: runUsage,
}

func init() {
	rootCmd.AddCommand(usageCmd)
}

func runUsage(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Scan hub
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// Build usage map: item -> profiles
	usage := make(map[string][]string)
	missing := make(map[string][]string)

	// Initialize with all hub items
	for _, item := range h.AllItems() {
		key := fmt.Sprintf("%s/%s", item.Type, item.Name)
		usage[key] = []string{}
	}

	// Scan profiles
	entries, err := os.ReadDir(paths.ProfilesDir)
	if err != nil {
		return fmt.Errorf("failed to read profiles: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		profileName := entry.Name()
		profileDir := filepath.Join(paths.ProfilesDir, profileName)
		manifestPath := profile.ManifestPath(profileDir)

		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}

		for _, itemType := range config.AllHubItemTypes() {
			items := manifest.GetHubItems(itemType)
			for _, itemName := range items {
				key := fmt.Sprintf("%s/%s", itemType, itemName)
				if _, exists := usage[key]; exists {
					usage[key] = append(usage[key], profileName)
				} else {
					// Item in manifest but not in hub
					missing[key] = append(missing[key], profileName)
				}
			}
		}
	}

	// Sort keys for consistent output
	var keys []string
	for k := range usage {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Find orphans and shared
	var orphans []string
	var shared []string

	for _, key := range keys {
		profiles := usage[key]
		if len(profiles) == 0 {
			orphans = append(orphans, key)
		} else if len(profiles) > 1 {
			shared = append(shared, key)
		}
	}

	// Print results
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if len(orphans) > 0 {
		fmt.Println("=== Orphaned Items (not used by any profile) ===")
		for _, item := range orphans {
			fmt.Printf("  %s\n", item)
		}
		fmt.Println()
	}

	if len(missing) > 0 {
		fmt.Println("=== Missing Items (referenced but not in hub) ===")
		for item, profiles := range missing {
			fmt.Printf("  %s (in: %s)\n", item, strings.Join(profiles, ", "))
		}
		fmt.Println()
	}

	if len(shared) > 0 {
		fmt.Println("=== Shared Items (used by multiple profiles) ===")
		for _, item := range shared {
			fmt.Fprintf(w, "  %s\t(%s)\n", item, strings.Join(usage[item], ", "))
		}
		w.Flush()
		fmt.Println()
	}

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Printf("  Hub items: %d\n", h.ItemCount())
	fmt.Printf("  Orphaned:  %d\n", len(orphans))
	fmt.Printf("  Missing:   %d\n", len(missing))
	fmt.Printf("  Shared:    %d\n", len(shared))

	return nil
}
