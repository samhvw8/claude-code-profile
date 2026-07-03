package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/source"
)

var bootstrapPush bool

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Set up ccp on a new machine, or push local items to chezmoi",
	Long: `Without flags: pulls — sync sources + fix all profiles.
With --push: adds local-only hub items + ccp.toml + profiles to chezmoi.

New machine:  chezmoi apply && ccp bootstrap
After changes: ccp bootstrap --push`,
	RunE: runBootstrap,
}

func init() {
	rootCmd.AddCommand(bootstrapCmd)
	bootstrapCmd.Flags().BoolVar(&bootstrapPush, "push", false, "add local-only hub items to chezmoi (skip source-installed)")
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	if bootstrapPush {
		return runBootstrapPush()
	}
	return runBootstrapPull()
}

func runBootstrapPull() error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Step 1: source sync
	fmt.Println("=== Syncing sources ===")
	if err := runSourceSync(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: source sync had errors: %v\n", err)
	}

	// Step 2: profile fix --all --force
	fmt.Println("=== Fixing profiles ===")
	mgr := profile.NewManager(paths)
	profiles, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	fixForce = true
	for _, p := range profiles {
		fmt.Printf("Fixing profile: %s\n", p.Name)
		if err := fixProfile(paths, p); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		}
	}

	fmt.Println("\nBootstrap complete.")
	return nil
}

func runBootstrapPush() error {
	if _, err := exec.LookPath("chezmoi"); err != nil {
		return fmt.Errorf("chezmoi not found in PATH")
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Build set of source-installed items ("skills/name", "agents/name.md")
	sourceItems := make(map[string]bool)
	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err == nil {
		for _, entry := range registry.ListSources() {
			for _, item := range entry.Source.Installed {
				sourceItems[item] = true
			}
		}
	}

	// Scan hub for all items
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// Collect local-only item paths
	var addPaths []string
	for _, item := range h.AllItems() {
		key := string(item.Type) + "/" + item.Name
		if sourceItems[key] {
			continue
		}
		addPaths = append(addPaths, item.Path)
	}

	// Also add bundles (check if source-installed)
	for _, b := range h.Bundles {
		key := "bundles/" + b.Name
		if sourceItems[key] {
			continue
		}
		addPaths = append(addPaths, paths.BundleDir(b.Name))
	}

	// Always add ccp.toml and profiles
	addPaths = append(addPaths, filepath.Join(paths.CcpDir, "ccp.toml"))
	profileDirs, _ := os.ReadDir(paths.ProfilesDir)
	for _, d := range profileDirs {
		if !d.IsDir() || d.Name() == "shared" {
			continue
		}
		pDir := filepath.Join(paths.ProfilesDir, d.Name())
		// Add profile.toml and settings files, not runtime data
		for _, name := range []string{"profile.toml", "settings.json", "settings-fragment.json", "CLAUDE.md"} {
			p := filepath.Join(pDir, name)
			if _, err := os.Stat(p); err == nil {
				addPaths = append(addPaths, p)
			}
		}
	}

	if len(addPaths) == 0 {
		fmt.Println("Nothing to add to chezmoi.")
		return nil
	}

	fmt.Printf("Adding %d local items to chezmoi (%d source items skipped)...\n", len(addPaths), len(sourceItems))

	// chezmoi add in batches to avoid arg limits
	batchSize := 50
	for i := 0; i < len(addPaths); i += batchSize {
		end := i + batchSize
		if end > len(addPaths) {
			end = len(addPaths)
		}
		batch := addPaths[i:end]
		args := append([]string{"add"}, batch...)
		cmd := exec.Command("chezmoi", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("chezmoi add failed: %w", err)
		}
	}

	// Show summary
	var types []string
	typeCounts := make(map[string]int)
	for _, p := range addPaths {
		rel, _ := filepath.Rel(paths.HubDir, p)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		if len(parts) >= 1 && parts[0] != ".." {
			typeCounts[parts[0]]++
		}
	}
	for t, c := range typeCounts {
		types = append(types, fmt.Sprintf("%s: %d", t, c))
	}
	fmt.Printf("Done. %s\n", strings.Join(types, ", "))

	return nil
}
