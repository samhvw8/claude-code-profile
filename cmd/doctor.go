package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	doctorFix       bool
	doctorVerifyOmp bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose and fix common issues",
	Long: `Check for common ccp issues and optionally fix them.

Checks:
- Is ~/.claude a symlink?
- Are there broken symlinks?
- Are profile manifests valid?
- Is the hub structure correct?

Use --fix to automatically repair issues that can be fixed.

--verify-omp additionally asks a live omp what its system prompt actually
carries, which is the only ground truth about what omp loads: ccp's own checks
predict omp's loader, and a wrong prediction makes an item silently absent. The
probe starts omp in RPC mode, so it needs a configured model, touches omp's own
state, and spends no model tokens.`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Automatically fix issues where possible")
	doctorCmd.Flags().BoolVar(&doctorVerifyOmp, "verify-omp", false, "Ask a live omp what its system prompt carries (starts omp in RPC mode)")
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
	fixed := 0

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
		} else {
			// Resolve relative symlink targets against the symlink's parent dir
			resolvedTarget := target
			if !filepath.IsAbs(resolvedTarget) {
				resolvedTarget = filepath.Join(filepath.Dir(paths.ClaudeDir), resolvedTarget)
			}
			if _, err := os.Stat(resolvedTarget); os.IsNotExist(err) {
				fmt.Println("FAIL")
				fmt.Printf("  → Symlink target does not exist: %s\n", target)
				issues++
			} else {
				fmt.Printf("OK → %s\n", filepath.Base(target))
			}
		}
	}

	// Check 3: Hub structure
	fmt.Print("Checking hub structure... ")
	missingHubDirs := []config.HubItemType{}
	for _, itemType := range config.AllHubItemTypes() {
		dir := paths.HubItemDir(itemType)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			missingHubDirs = append(missingHubDirs, itemType)
		}
	}
	if len(missingHubDirs) > 0 {
		if doctorFix {
			// Fix: create missing hub directories
			for _, itemType := range missingHubDirs {
				dir := paths.HubItemDir(itemType)
				if err := os.MkdirAll(dir, 0755); err != nil {
					fmt.Printf("FAIL\n  → Could not create %s: %v\n", dir, err)
					issues++
				} else {
					fixed++
				}
			}
			fmt.Printf("FIXED (%d directories created)\n", len(missingHubDirs))
		} else {
			fmt.Printf("WARN (%d missing directories)\n", len(missingHubDirs))
			fmt.Println("  → Run 'ccp doctor --fix' or 'ccp init --force' to fix")
		}
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
		profileDir := filepath.Join(paths.ProfilesDir, entry.Name())
		manifestPath := profile.ManifestPath(profileDir)
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

	// Check 5: Broken symlinks and profile drift
	fmt.Print("Checking for broken symlinks... ")
	brokenLinks := findBrokenSymlinks(paths.ProfilesDir)
	if len(brokenLinks) > 0 {
		if doctorFix {
			// Fix: run drift detection and fix for all profiles
			fixedProfiles := 0
			mgr := profile.NewManager(paths)
			detector := profile.NewDetector(paths)

			for _, entry := range entries {
				if !entry.IsDir() || entry.Name() == "shared" {
					continue
				}
				p, err := mgr.Get(entry.Name())
				if err != nil || p == nil {
					continue
				}

				report, err := detector.Detect(p)
				if err != nil {
					continue
				}

				if report.HasDrift() {
					opts := profile.FixOptions{
						DryRun: false,
						Force:  true, // Auto-fix without prompts in doctor --fix
					}
					result, err := detector.Fix(p, report, opts)
					if err == nil && len(result.Actions) > 0 {
						fixedProfiles++
						fixed += len(result.Actions)
					}
				}
			}
			fmt.Printf("FIXED (%d profiles repaired)\n", fixedProfiles)
		} else {
			fmt.Printf("WARN (%d broken)\n", len(brokenLinks))
			for _, link := range brokenLinks[:min(5, len(brokenLinks))] {
				fmt.Printf("  → %s\n", link)
			}
			if len(brokenLinks) > 5 {
				fmt.Printf("  → ... and %d more\n", len(brokenLinks)-5)
			}
			fmt.Println("  → Run 'ccp doctor --fix' to repair")
			issues += len(brokenLinks)
		}
	} else {
		fmt.Println("OK")
	}

	// Check 6: omp links. Informational — only inspected once ccp has linked
	// items there, so users who never ran 'ccp omp link' see nothing.
	fmt.Print("Checking omp links... ")
	ompManaged, managedErr := profile.OmpManaged(paths)
	if managedErr != nil {
		fmt.Println("SKIP")
		fmt.Printf("  → %v\n", managedErr)
	} else if len(ompManaged) == 0 {
		// Nothing of ccp's there, so nothing to check — including a context-only
		// setup, which OmpManaged covers.
		fmt.Println("OK (nothing linked)")
	} else {
		broken, err := profile.OmpBroken(paths)
		switch {
		case err != nil:
			fmt.Println("FAIL")
			fmt.Printf("  → %v\n", err)
			issues++
		case len(broken) > 0 && doctorFix:
			pruned, err := profile.OmpPruneBroken(paths)
			if err != nil {
				fmt.Println("FAIL")
				fmt.Printf("  → %v\n", err)
				issues++
			} else {
				fmt.Printf("FIXED (%d broken link(s) removed)\n", len(pruned))
				fixed += len(pruned)
			}
		case len(broken) > 0:
			fmt.Printf("WARN (%d broken)\n", len(broken))
			fmt.Printf("  → hub items gone: %s\n", summarizeRefs(broken))
			fmt.Println("  → Run 'ccp doctor --fix' to remove them, or 'ccp omp sync' to relink")
			issues++
		default:
			reportOmpDrift(paths, len(ompManaged))
		}
	}

	// Opt-in check: what omp actually loads. Predictions about another program's
	// loader fail silently — an item in no bucket is simply absent — so the only
	// honest check is to read the prompt omp built for a session.
	if doctorVerifyOmp {
		fmt.Print("Checking what omp loads... ")
		p, err := profile.NewManager(paths).GetActive()
		switch {
		case err != nil || p == nil:
			fmt.Println("SKIP (no active profile)")
		default:
			report, err := profile.OmpVerifyProfile(paths, p)
			switch {
			case err != nil:
				fmt.Println("SKIP")
				fmt.Printf("  → %v\n", err)
				fmt.Println("  → the probe needs omp with a configured model; it starts no session and spends no tokens")
			default:
				if !report.Verified() {
					fmt.Printf("WARN (%d of %d carriers missing)\n", len(report.Missing), report.Carriers)
					fmt.Printf("  → omp's system prompt does not carry: %s\n", summarizeRefs(report.Missing))
					fmt.Println("  → those are the files ccp generates for omp: run 'ccp omp sync', then check for a competing RULES.md")
					issues++
				} else {
					fmt.Printf("OK (%d of %d linked items named, %d chars of prompt)\n", report.Named, report.Items, report.Chars)
				}
				if len(report.Unnamed) > 0 {
					fmt.Printf("  → not named in omp's answer: %s\n", summarizeRefs(report.Unnamed))
					fmt.Println("  → links are in place: omp lists skills under its own ignoredSkills/includeSkills filters, and names agents through tool schemas")
				}
			}
		}
	}

	// Summary
	fmt.Println()
	if issues == 0 && fixed == 0 {
		fmt.Println("All checks passed!")
	} else if doctorFix && fixed > 0 {
		fmt.Printf("Fixed %d issue(s)\n", fixed)
		if issues > 0 {
			fmt.Printf("%d issue(s) require manual intervention\n", issues)
		}
	} else {
		fmt.Printf("Found %d issue(s)\n", issues)
		if !doctorFix && (len(missingHubDirs) > 0 || len(brokenLinks) > 0) {
			fmt.Println("Run 'ccp doctor --fix' to attempt automatic repair")
		}
	}

	return nil
}

// reportOmpDrift prints omp's link count, noting when omp no longer matches the
// active profile. Drift is informational: omp links are optional.
func reportOmpDrift(paths *config.Paths, linked int) {
	p, _ := profile.NewManager(paths).GetActive()
	if p == nil {
		fmt.Printf("OK (%d linked)\n", linked)
		return
	}

	diff, err := profile.OmpStatus(paths, p.Manifest)
	if err != nil {
		fmt.Printf("OK (%d linked, matches '%s')\n", linked, p.Name)
		return
	}
	if !diff.Empty() {
		fmt.Printf("OK (%d linked)\n", linked)
		fmt.Printf("  → out of sync with profile '%s' (%d missing, %d stale)\n", p.Name, len(diff.Missing), len(diff.Stale))
		fmt.Println("  → Run 'ccp omp sync' to reconcile")
		return
	}

	context, err := profile.OmpContextStatus(paths, p)
	if err != nil {
		fmt.Printf("OK (%d linked, matches '%s')\n", linked, p.Name)
		return
	}

	fmt.Printf("OK (%d linked, matches '%s')\n", linked, p.Name)
	for _, state := range context.Stale {
		fmt.Printf("  → context %s\n", state)
	}
	if context.OutOfDate() {
		fmt.Println("  → Run 'ccp omp sync' to refresh omp's context files")
	}
	for _, state := range context.Occupied {
		fmt.Printf("  → %s is occupied by a file ccp did not create\n", state)
	}
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
