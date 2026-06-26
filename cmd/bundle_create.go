package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/source"
)

var (
	bundleCreateDesc     string
	bundleCreateSkills   []string
	bundleCreateAgents   []string
	bundleCreateHooks    []string
	bundleCreateRules    []string
	bundleCreateCommands []string
)

// bundleMemberTypes are the leaf types a bundle may contain.
// settings-templates are referenced by name, not bundled.
var bundleMemberTypes = []config.HubItemType{
	config.HubSkills, config.HubAgents, config.HubHooks, config.HubRules, config.HubCommands,
}

var bundleCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a bundle from hub items",
	Long: `Create a bundle by choosing which hub items to package together.

With no member flags an interactive multi-select picker lets you choose skills,
agents, hooks, rules and commands across tabs. Selected items are COPIED into the
bundle (the original standalone hub items are left untouched). Link it as a unit:

  ccp bundle create design --desc "Design review bundle"
  ccp link <profile> bundles/design

Members live inside the bundle and cannot be linked individually.`,
	Args: cobra.ExactArgs(1),
	RunE: runBundleCreate,
}

func init() {
	f := bundleCreateCmd.Flags()
	f.StringVar(&bundleCreateDesc, "desc", "", "Bundle description")
	f.StringArrayVar(&bundleCreateSkills, "skill", nil, "Skill to include (repeatable)")
	f.StringArrayVar(&bundleCreateAgents, "agent", nil, "Agent to include (repeatable)")
	f.StringArrayVar(&bundleCreateHooks, "hook", nil, "Hook to include (repeatable)")
	f.StringArrayVar(&bundleCreateRules, "rule", nil, "Rule to include (repeatable)")
	f.StringArrayVar(&bundleCreateCommands, "command", nil, "Command to include (repeatable)")
	bundleCmd.AddCommand(bundleCreateCmd)
}

func runBundleCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

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

	// Explicit member flags take precedence over the interactive picker.
	members := hub.ComponentList{
		Skills:   bundleCreateSkills,
		Agents:   bundleCreateAgents,
		Hooks:    bundleCreateHooks,
		Rules:    bundleCreateRules,
		Commands: bundleCreateCommands,
	}
	if members.Count() == 0 {
		members, err = pickBundleMembers(h, name)
		if err != nil {
			return err
		}
		if members.Count() == 0 {
			fmt.Println("No items selected")
			return nil
		}
	}

	if err := createBundleFromHub(paths, h, name, bundleCreateDesc, members); err != nil {
		return err
	}

	printBundleSummary(name, members)
	return nil
}

// pickBundleMembers runs the tabbed multi-select picker over available hub items.
func pickBundleMembers(h *hub.Hub, name string) (hub.ComponentList, error) {
	var tabs []picker.Tab
	for _, itemType := range bundleMemberTypes {
		items := h.GetItems(itemType)
		if len(items) == 0 {
			continue
		}
		var pickerItems []picker.Item
		for _, item := range items {
			pickerItems = append(pickerItems, picker.Item{ID: item.Name, Label: item.Name})
		}
		tabs = append(tabs, picker.Tab{Name: string(itemType), Items: pickerItems})
	}
	if len(tabs) == 0 {
		return hub.ComponentList{}, fmt.Errorf("no hub items available to bundle")
	}

	fmt.Printf("Choose items to bundle into '%s' (space to select, tab to switch tabs, enter to confirm)\n\n", name)
	selections, err := picker.RunTabbed(tabs)
	if err != nil {
		return hub.ComponentList{}, fmt.Errorf("picker error: %w", err)
	}
	if selections == nil {
		return hub.ComponentList{}, nil // cancelled
	}
	return selectionsToComponentList(selections), nil
}

func selectionsToComponentList(sel map[string][]string) hub.ComponentList {
	return hub.ComponentList{
		Skills:   sel[string(config.HubSkills)],
		Agents:   sel[string(config.HubAgents)],
		Hooks:    sel[string(config.HubHooks)],
		Rules:    sel[string(config.HubRules)],
		Commands: sel[string(config.HubCommands)],
	}
}

// createBundleFromHub copies each selected hub item into a new bundle directory
// and writes its manifest. Copying (not moving) keeps the bundle self-contained
// while leaving the original standalone hub items untouched.
func createBundleFromHub(paths *config.Paths, h *hub.Hub, name, desc string, members hub.ComponentList) error {
	if name == "" {
		return fmt.Errorf("bundle name is required")
	}
	if members.Count() == 0 {
		return fmt.Errorf("a bundle needs at least one member")
	}
	if h.GetBundle(name) != nil {
		return fmt.Errorf("bundle already exists: %s", name)
	}
	bundleDir := paths.BundleDir(name)
	if _, err := os.Stat(bundleDir); err == nil {
		return fmt.Errorf("bundle directory already exists: %s", bundleDir)
	}

	for _, m := range members.AllComponents() {
		itemType := config.HubItemType(m.Type)
		if !h.HasItem(itemType, m.Name) {
			os.RemoveAll(bundleDir) // roll back partial bundle
			return fmt.Errorf("hub item not found: %s/%s", m.Type, m.Name)
		}
		src := paths.HubItemPath(itemType, m.Name)
		dst := filepath.Join(bundleDir, m.Type, m.Name)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			os.RemoveAll(bundleDir)
			return err
		}
		if err := source.CopyTree(src, dst); err != nil {
			os.RemoveAll(bundleDir)
			return fmt.Errorf("failed to copy %s/%s: %w", m.Type, m.Name, err)
		}
	}

	bundle := &hub.Bundle{Name: name, Description: desc, Version: "1.0.0", Members: members}
	if err := bundle.Save(paths.BundlesDir()); err != nil {
		os.RemoveAll(bundleDir)
		return fmt.Errorf("failed to write bundle manifest: %w", err)
	}
	return nil
}

func printBundleSummary(name string, members hub.ComponentList) {
	fmt.Printf("Created bundle '%s' with %d member(s):\n\n", name, members.Count())
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, m := range members.AllComponents() {
		fmt.Fprintf(w, "  %s\t%s\n", m.Type, m.Name)
	}
	w.Flush()
	fmt.Printf("\nLink it to a profile with:\n  ccp link <profile> bundles/%s\n", name)
}
