package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var profileDiffCmd = &cobra.Command{
	Use:   "diff <profile-a> [profile-b]",
	Short: "Compare two profiles",
	Long: `Show differences between two profiles.

If only one profile is specified, compares against the active profile.

Examples:
  ccp profile diff dev prod
  ccp profile diff minimal  # compares to active profile`,
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: completeProfileNames,
	RunE:              runProfileDiff,
}

func init() {
	profileCmd.AddCommand(profileDiffCmd)
}

func runProfileDiff(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Get profile names
	profileA := args[0]
	profileB := ""

	if len(args) == 2 {
		profileB = args[1]
	} else {
		// Use active profile
		if paths.ClaudeDirIsSymlink() {
			target, err := os.Readlink(paths.ClaudeDir)
			if err == nil {
				profileB = filepath.Base(target)
			}
		}
		if profileB == "" {
			return fmt.Errorf("no second profile specified and no active profile found")
		}
	}

	// Load profiles
	a, err := mgr.Get(profileA)
	if err != nil || a == nil {
		return fmt.Errorf("profile not found: %s", profileA)
	}

	b, err := mgr.Get(profileB)
	if err != nil || b == nil {
		return fmt.Errorf("profile not found: %s", profileB)
	}

	fmt.Printf("Comparing: %s vs %s\n\n", profileA, profileB)

	hasDiff := false

	// Compare hub items
	for _, itemType := range config.AllHubItemTypes() {
		itemsA := a.Manifest.GetHubItems(itemType)
		itemsB := b.Manifest.GetHubItems(itemType)

		onlyInA, onlyInB, _ := diffSlices(itemsA, itemsB)

		if len(onlyInA) > 0 || len(onlyInB) > 0 {
			hasDiff = true
			fmt.Printf("=== %s ===\n", itemType)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, item := range onlyInA {
				fmt.Fprintf(w, "  - %s\t(only in %s)\n", item, profileA)
			}
			for _, item := range onlyInB {
				fmt.Fprintf(w, "  + %s\t(only in %s)\n", item, profileB)
			}
			w.Flush()
			fmt.Println()
		}
	}

	// Compare data config
	dataA := a.Manifest.Data
	dataB := b.Manifest.Data
	dataDiff := false

	for _, dataType := range config.AllDataItemTypes() {
		modeA := a.Manifest.GetDataShareMode(dataType)
		modeB := b.Manifest.GetDataShareMode(dataType)
		if modeA != modeB {
			if !dataDiff {
				fmt.Println("=== Data Sharing ===")
				dataDiff = true
				hasDiff = true
			}
			fmt.Printf("  %s: %s (%s) vs %s (%s)\n", dataType, modeA, profileA, modeB, profileB)
		}
	}
	if dataDiff {
		fmt.Println()
	}
	_ = dataA
	_ = dataB

	if !hasDiff {
		fmt.Println("Profiles are identical")
	}

	return nil
}

func diffSlices(a, b []string) (onlyInA, onlyInB, inBoth []string) {
	setA := make(map[string]bool)
	setB := make(map[string]bool)

	for _, v := range a {
		setA[v] = true
	}
	for _, v := range b {
		setB[v] = true
	}

	for _, v := range a {
		if setB[v] {
			inBoth = append(inBoth, v)
		} else {
			onlyInA = append(onlyInA, v)
		}
	}
	for _, v := range b {
		if !setA[v] {
			onlyInB = append(onlyInB, v)
		}
	}

	sort.Strings(onlyInA)
	sort.Strings(onlyInB)
	sort.Strings(inBoth)

	return
}
