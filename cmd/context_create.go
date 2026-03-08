package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	contextCreateInteractive bool
	contextCreateEmpty       bool
	contextCreateDescription string
	contextCreateFrom        string
	contextCreateSkills      []string
	contextCreateAgents      []string
	contextCreateRules       []string
	contextCreateCommands    []string
	contextCreateHooks       []string
)

var contextCreateCmd = &cobra.Command{
	Use:     "create <name>",
	Aliases: []string{"c", "new"},
	Short:   "Create a new context",
	Long: `Create a new context by selecting skills, agents, rules, commands, and hooks.

Examples:
  ccp context create coding -i
  ccp context create writing --skills=writing,grammar
  ccp context create minimal --empty
  ccp context create copy --from=coding`,
	Args: cobra.ExactArgs(1),
	RunE: runContextCreate,
}

func init() {
	contextCreateCmd.Flags().BoolVarP(&contextCreateInteractive, "interactive", "i", false, "Interactive picker mode")
	contextCreateCmd.Flags().BoolVarP(&contextCreateEmpty, "empty", "e", false, "Create empty context")
	contextCreateCmd.Flags().StringVarP(&contextCreateDescription, "description", "d", "", "Context description")
	contextCreateCmd.Flags().StringVar(&contextCreateFrom, "from", "", "Copy from existing context")
	contextCreateCmd.Flags().StringSliceVar(&contextCreateSkills, "skills", nil, "Skills to include")
	contextCreateCmd.Flags().StringSliceVar(&contextCreateAgents, "agents", nil, "Agents to include")
	contextCreateCmd.Flags().StringSliceVar(&contextCreateRules, "rules", nil, "Rules to include")
	contextCreateCmd.Flags().StringSliceVar(&contextCreateCommands, "commands", nil, "Commands to include")
	contextCreateCmd.Flags().StringSliceVar(&contextCreateHooks, "hooks", nil, "Hooks to include")
	contextCmd.AddCommand(contextCreateCmd)
}

func runContextCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewContextManager(paths)

	if mgr.Exists(name) {
		return fmt.Errorf("context already exists: %s", name)
	}

	ctx := profile.NewContext(name, contextCreateDescription)

	// Copy from existing
	if contextCreateFrom != "" {
		source, err := mgr.Get(contextCreateFrom)
		if err != nil {
			return fmt.Errorf("source context not found: %s", contextCreateFrom)
		}
		ctx.Hub = source.Hub
		if contextCreateDescription == "" {
			ctx.Description = fmt.Sprintf("Created from %s", contextCreateFrom)
		}
	}

	// Apply CLI flags
	if len(contextCreateSkills) > 0 {
		ctx.Hub.Skills = contextCreateSkills
	}
	if len(contextCreateAgents) > 0 {
		ctx.Hub.Agents = contextCreateAgents
	}
	if len(contextCreateRules) > 0 {
		ctx.Hub.Rules = contextCreateRules
	}
	if len(contextCreateCommands) > 0 {
		ctx.Hub.Commands = contextCreateCommands
	}
	if len(contextCreateHooks) > 0 {
		ctx.Hub.Hooks = contextCreateHooks
	}

	// Interactive mode
	hasFlags := len(contextCreateSkills) > 0 || len(contextCreateAgents) > 0 ||
		len(contextCreateRules) > 0 || len(contextCreateCommands) > 0 ||
		len(contextCreateHooks) > 0 || contextCreateFrom != "" || contextCreateEmpty

	if contextCreateInteractive || !hasFlags {
		scanner := hub.NewScanner()
		h, err := scanner.Scan(paths.HubDir)
		if err != nil {
			return fmt.Errorf("failed to scan hub: %w", err)
		}

		contextHubTypes := []config.HubItemType{
			config.HubSkills, config.HubAgents, config.HubRules, config.HubCommands, config.HubHooks,
		}

		var tabs []picker.Tab
		for _, itemType := range contextHubTypes {
			items := h.GetItems(itemType)
			if len(items) == 0 {
				continue
			}

			currentSelected := make(map[string]bool)
			switch itemType {
			case config.HubSkills:
				for _, n := range ctx.Hub.Skills {
					currentSelected[n] = true
				}
			case config.HubAgents:
				for _, n := range ctx.Hub.Agents {
					currentSelected[n] = true
				}
			case config.HubRules:
				for _, n := range ctx.Hub.Rules {
					currentSelected[n] = true
				}
			case config.HubCommands:
				for _, n := range ctx.Hub.Commands {
					currentSelected[n] = true
				}
			case config.HubHooks:
				for _, n := range ctx.Hub.Hooks {
					currentSelected[n] = true
				}
			}

			var pickerItems []picker.Item
			for _, item := range items {
				pickerItems = append(pickerItems, picker.Item{
					ID:       item.Name,
					Label:    item.Name,
					Selected: currentSelected[item.Name],
				})
			}

			tabs = append(tabs, picker.Tab{
				Name:  string(itemType),
				Items: pickerItems,
			})
		}

		if len(tabs) > 0 {
			selections, err := picker.RunTabbed(tabs)
			if err != nil {
				return fmt.Errorf("picker error: %w", err)
			}
			if selections == nil {
				fmt.Println("Cancelled")
				return nil
			}

			if items, ok := selections[string(config.HubSkills)]; ok {
				ctx.Hub.Skills = items
			}
			if items, ok := selections[string(config.HubAgents)]; ok {
				ctx.Hub.Agents = items
			}
			if items, ok := selections[string(config.HubRules)]; ok {
				ctx.Hub.Rules = items
			}
			if items, ok := selections[string(config.HubCommands)]; ok {
				ctx.Hub.Commands = items
			}
			if items, ok := selections[string(config.HubHooks)]; ok {
				ctx.Hub.Hooks = items
			}
		}
	}

	if err := mgr.Create(name, ctx); err != nil {
		return fmt.Errorf("failed to create context: %w", err)
	}

	fmt.Printf("Created context: %s\n", name)
	fmt.Printf("Location: %s\n", paths.ContextDir(name))

	var parts []string
	if len(ctx.Hub.Skills) > 0 {
		parts = append(parts, fmt.Sprintf("%d skills", len(ctx.Hub.Skills)))
	}
	if len(ctx.Hub.Agents) > 0 {
		parts = append(parts, fmt.Sprintf("%d agents", len(ctx.Hub.Agents)))
	}
	if len(ctx.Hub.Rules) > 0 {
		parts = append(parts, fmt.Sprintf("%d rules", len(ctx.Hub.Rules)))
	}
	if len(ctx.Hub.Commands) > 0 {
		parts = append(parts, fmt.Sprintf("%d commands", len(ctx.Hub.Commands)))
	}
	if len(ctx.Hub.Hooks) > 0 {
		parts = append(parts, fmt.Sprintf("%d hooks", len(ctx.Hub.Hooks)))
	}
	if len(parts) > 0 {
		fmt.Printf("Linked: %s\n", strings.Join(parts, ", "))
	}

	return nil
}
