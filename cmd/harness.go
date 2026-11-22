package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var harnessListJSON bool

var harnessCmd = &cobra.Command{
	Use:    "harness",
	Short:  "Manage custom harness plugins (TOML manifests and executable plugins)",
	Hidden: true,
	Long: `Mirror a ccp profile into custom harnesses using declarative TOML manifests
(~/.ccp/harnesses/<id>.toml) or executable stdin/stdout JSON plugins
(~/.ccp/harnesses/<id> or ccp-harness-<id> on PATH).

Both plugin kinds produce a declarative sync plan; ccp validates every path and
owns all file writes, symlinks, and hash-tracked ownership ledgers.

After the first 'ccp harness sync <id>', ccp keeps that harness current on
'ccp use -g', linking or unlinking in the active profile, hub updates, and
'ccp bootstrap'.`,
}

var harnessListCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered custom harness plugins",
	Args:  cobra.NoArgs,
	RunE:  runHarnessList,
}

var harnessSyncCmd = &cobra.Command{
	Use:   "sync <id> [profile]",
	Short: "Mirror a profile into a custom harness (default: active profile)",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runHarnessSync,
}

var harnessCheckCmd = &cobra.Command{
	Use:   "check [id] [profile]",
	Short: "Validate custom harness plugin(s) and preview their sync plan without writing",
	Args:  cobra.MaximumNArgs(2),
	RunE:  runHarnessCheck,
}

var harnessRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove ccp-managed files and opt out of automatic syncing for a custom harness",
	Args:  cobra.ExactArgs(1),
	RunE:  runHarnessRemove,
}

func init() {
	harnessListCmd.Flags().BoolVarP(&harnessListJSON, "json", "j", false, "Output as JSON")
	harnessCmd.AddCommand(harnessListCmd, harnessSyncCmd, harnessCheckCmd, harnessRemoveCmd)
	rootCmd.AddCommand(harnessCmd)
}

func runHarnessList(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}
	plugins, err := profile.DiscoverHarnesses(paths)
	if err != nil {
		return err
	}
	if harnessListJSON {
		type itemJSON struct {
			ID      string `json:"id"`
			Name    string `json:"name,omitempty"`
			Kind    string `json:"kind"`
			Path    string `json:"path"`
			Root    string `json:"root,omitempty"`
			OptedIn bool   `json:"opted_in"`
			Error   string `json:"error,omitempty"`
		}
		out := make([]itemJSON, len(plugins))
		for i, p := range plugins {
			j := itemJSON{ID: p.ID, Name: p.Name, Kind: p.Kind, Path: p.Path, Root: p.Root, OptedIn: p.OptedIn}
			if p.Err != nil {
				j.Error = p.Err.Error()
			}
			out[i] = j
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	if len(plugins) == 0 {
		fmt.Printf("No custom harnesses found in %s or on PATH (ccp-harness-<id>).\n", paths.HarnessesDir())
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  ID\tKIND\tOPTED IN\tROOT\tPATH")
	for _, p := range plugins {
		opted := "no"
		if p.OptedIn {
			opted = "yes"
		}
		root := p.Root
		if root == "" {
			root = "-"
		}
		if p.Err != nil {
			root = "ERROR: " + p.Err.Error()
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", p.ID, p.Kind, opted, root, p.Path)
	}
	w.Flush()
	return nil
}

func resolveHarnessTargetProfile(paths *config.Paths, profileName string) (*profile.Profile, error) {
	mgr := profile.NewManager(paths)
	if profileName != "" {
		p, err := mgr.Get(profileName)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, fmt.Errorf("profile not found: %s", profileName)
		}
		return p, nil
	}
	p, err := mgr.GetActive()
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("no active profile; name one explicitly")
	}
	return p, nil
}

func runHarnessSync(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}
	profileName := ""
	if len(args) == 2 {
		profileName = args[1]
	}
	p, err := resolveHarnessTargetProfile(paths, profileName)
	if err != nil {
		return err
	}
	result, err := profile.HarnessSync(paths, args[0], p)
	if err != nil {
		return err
	}
	fmt.Printf("Mirrored profile '%s' into %s (%s)\n", p.Name, result.Root, args[0])
	warnInactiveCarrier(paths, p)
	reportHarnessResult(result, true)
	return nil
}

func runHarnessCheck(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}
	var ids []string
	profileName := ""
	if len(args) >= 1 {
		ids = []string{args[0]}
		if len(args) == 2 {
			profileName = args[1]
		}
	} else {
		plugins, err := profile.DiscoverHarnesses(paths)
		if err != nil {
			return err
		}
		if len(plugins) == 0 {
			fmt.Printf("No custom harnesses found in %s or on PATH (ccp-harness-<id>).\n", paths.HarnessesDir())
			return nil
		}
		for _, pl := range plugins {
			ids = append(ids, pl.ID)
		}
	}
	p, err := resolveHarnessTargetProfile(paths, profileName)
	if err != nil {
		return err
	}

	failed := 0
	for _, id := range ids {
		plan, err := profile.HarnessCheck(paths, id, p)
		if err != nil {
			fmt.Printf("%s: FAIL — %v\n", id, err)
			failed++
			continue
		}
		styleNote := ""
		if plan.StyleFile != "" {
			styleNote = ", style=" + plan.StyleFile
		}
		fmt.Printf("%s: OK — root=%s (%d links, %d copies%s)\n",
			id, plan.Root, len(plan.Links), len(plan.Copies), styleNote)
		for _, s := range plan.Skipped {
			fmt.Printf("  skipped: %s\n", s)
		}
		for _, n := range plan.Notes {
			fmt.Printf("  note: %s\n", n)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d harness plugin(s) failed check", failed)
	}
	return nil
}

func runHarnessRemove(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}
	result, err := profile.HarnessRemove(paths, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Opted out of custom harness '%s'\n", args[0])
	reportHarnessResult(result, false)
	return nil
}

func reportHarnessResult(r profile.HarnessResult, verbose bool) {
	line := func(label string, items []string) {
		if len(items) > 0 {
			fmt.Printf("  %s: %s\n", label, strings.Join(items, ", "))
		}
	}
	line("written", r.Written)
	line("linked", r.Linked)
	line("removed", r.Removed)
	line("left alone (edited by hand since ccp wrote it)", r.Edited)
	line("left alone (not made by ccp — remove it to let ccp manage it)", r.Occupied)
	line("skipped", r.Skipped)
	for _, s := range append(r.Notes, r.Style...) {
		fmt.Printf("  %s\n", s)
	}
	if verbose && !r.HasChanges() {
		fmt.Println("  already up to date")
	}
}

// followCustomHarnesses re-syncs every opted-in custom harness after the active
// profile changed. Failures are warnings: the profile change already happened.
func followCustomHarnesses(paths *config.Paths, profileName string) {
	active := harnessProfile(paths, profileName)
	if active == nil {
		return
	}
	for _, res := range profile.HarnessFollowAll(paths, active) {
		if res.Err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not sync harness %s: %v\n", res.ID, res.Err)
			continue
		}
		if res.Result.HasChanges() {
			fmt.Printf("%s:\n", res.ID)
			reportHarnessResult(res.Result, false)
		}
	}
}

func sortedHarnessRoots(m map[string][]profile.HarnessLink) []string {
	return slices.Sorted(maps.Keys(m))
}
