package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var captureDry bool

var profileCaptureCmd = &cobra.Command{
	Use:   "capture [name]",
	Short: "Capture settings changes as a fragment",
	Long: `Capture the diff between current settings.json and the base template,
saving the result as settings-fragment.json.

This preserves manual edits (permissions, env, model overrides) so they
survive the next 'ccp use' or 'ccp profile sync'.

Hooks are excluded — they are managed separately by the hub.

Examples:
  ccp profile capture             # Capture from active profile
  ccp profile capture dev         # Capture from named profile
  ccp profile capture --dry-run   # Show diff without saving`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runProfileCapture,
}

func init() {
	profileCaptureCmd.Flags().BoolVar(&captureDry, "dry-run", false, "Show fragment without saving")
	profileCmd.AddCommand(profileCaptureCmd)
}

func runProfileCapture(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	var profileName string
	if len(args) > 0 {
		profileName = args[0]
	} else {
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return fmt.Errorf("no active profile and no profile name specified")
		}
		profileName = active.Name
	}

	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	if captureDry {
		fragment, err := profile.PreviewFragment(paths, p.Path, p.Manifest)
		if err != nil {
			return err
		}
		if len(fragment) == 0 {
			fmt.Printf("No differences between settings.json and template '%s'\n", p.Manifest.SettingsTemplate)
			return nil
		}
		data, _ := json.MarshalIndent(fragment, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fragment, err := profile.UpdateFragment(paths, p.Path, p.Manifest)
	if err != nil {
		return err
	}

	if len(fragment) == 0 {
		tmplName := p.Manifest.SettingsTemplate
		if tmplName == "" {
			tmplName = "(none)"
		}
		fmt.Printf("No differences found (base template: %s)\n", tmplName)
		return nil
	}

	fmt.Printf("Captured %d setting keys into fragment for profile '%s'\n", countKeys(fragment), profileName)
	return nil
}

func countKeys(m map[string]interface{}) int {
	n := 0
	for _, v := range m {
		if sub, ok := v.(map[string]interface{}); ok {
			n += countKeys(sub)
		} else {
			n++
		}
	}
	return n
}
