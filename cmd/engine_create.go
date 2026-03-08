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
	engineCreateInteractive      bool
	engineCreateEmpty            bool
	engineCreateDescription      string
	engineCreateFrom             string
	engineCreateSettingFragments []string
	engineCreateHooks            []string
)

var engineCreateCmd = &cobra.Command{
	Use:     "create <name>",
	Aliases: []string{"c", "new"},
	Short:   "Create a new engine",
	Long: `Create a new engine by selecting setting fragments, hooks, and data sharing config.

Examples:
  ccp engine create opus-full -i
  ccp engine create haiku-fast --setting-fragments=model-haiku
  ccp engine create minimal --empty
  ccp engine create copy --from=opus-full`,
	Args: cobra.ExactArgs(1),
	RunE: runEngineCreate,
}

func init() {
	engineCreateCmd.Flags().BoolVarP(&engineCreateInteractive, "interactive", "i", false, "Interactive picker mode")
	engineCreateCmd.Flags().BoolVarP(&engineCreateEmpty, "empty", "e", false, "Create empty engine")
	engineCreateCmd.Flags().StringVarP(&engineCreateDescription, "description", "d", "", "Engine description")
	engineCreateCmd.Flags().StringVar(&engineCreateFrom, "from", "", "Copy from existing engine")
	engineCreateCmd.Flags().StringSliceVar(&engineCreateSettingFragments, "setting-fragments", nil, "Setting fragments to include")
	engineCreateCmd.Flags().StringSliceVar(&engineCreateHooks, "hooks", nil, "Hooks to include")
	engineCmd.AddCommand(engineCreateCmd)
}

func runEngineCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewEngineManager(paths)

	if mgr.Exists(name) {
		return fmt.Errorf("engine already exists: %s", name)
	}

	engine := profile.NewEngine(name, engineCreateDescription)

	// Copy from existing
	if engineCreateFrom != "" {
		source, err := mgr.Get(engineCreateFrom)
		if err != nil {
			return fmt.Errorf("source engine not found: %s", engineCreateFrom)
		}
		engine.Hub = source.Hub
		engine.Data = source.Data
		if engineCreateDescription == "" {
			engine.Description = fmt.Sprintf("Created from %s", engineCreateFrom)
		}
	}

	// Apply CLI flags
	if len(engineCreateSettingFragments) > 0 {
		engine.Hub.SettingFragments = engineCreateSettingFragments
	}
	if len(engineCreateHooks) > 0 {
		engine.Hub.Hooks = engineCreateHooks
	}

	// Interactive mode
	hasFlags := len(engineCreateSettingFragments) > 0 || len(engineCreateHooks) > 0 ||
		engineCreateFrom != "" || engineCreateEmpty

	if engineCreateInteractive || !hasFlags {
		scanner := hub.NewScanner()
		h, err := scanner.Scan(paths.HubDir)
		if err != nil {
			return fmt.Errorf("failed to scan hub: %w", err)
		}

		var tabs []picker.Tab

		// Setting fragments tab
		engineHubTypes := []config.HubItemType{config.HubSettingFragments, config.HubHooks}
		for _, itemType := range engineHubTypes {
			items := h.GetItems(itemType)
			if len(items) == 0 {
				continue
			}

			currentSelected := make(map[string]bool)
			if itemType == config.HubSettingFragments {
				for _, n := range engine.Hub.SettingFragments {
					currentSelected[n] = true
				}
			} else {
				for _, n := range engine.Hub.Hooks {
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

		// Data sharing tab
		var dataItems []picker.Item
		for _, dataType := range config.AllDataItemTypes() {
			isShared := engine.Data.Tasks == config.ShareModeShared // approximate
			switch dataType {
			case config.DataTasks:
				isShared = engine.Data.Tasks == config.ShareModeShared
			case config.DataTodos:
				isShared = engine.Data.Todos == config.ShareModeShared
			case config.DataPasteCache:
				isShared = engine.Data.PasteCache == config.ShareModeShared
			case config.DataHistory:
				isShared = engine.Data.History == config.ShareModeShared
			case config.DataFileHistory:
				isShared = engine.Data.FileHistory == config.ShareModeShared
			case config.DataSessionEnv:
				isShared = engine.Data.SessionEnv == config.ShareModeShared
			case config.DataProjects:
				isShared = engine.Data.Projects == config.ShareModeShared
			case config.DataPlans:
				isShared = engine.Data.Plans == config.ShareModeShared
			}

			label := fmt.Sprintf("%s (default: %s)", dataType, map[bool]string{true: "shared", false: "isolated"}[isShared])
			dataItems = append(dataItems, picker.Item{
				ID:       string(dataType),
				Label:    label,
				Selected: isShared,
			})
		}
		tabs = append(tabs, picker.Tab{
			Name:  "data-sharing",
			Items: dataItems,
		})

		if len(tabs) > 0 {
			selections, err := picker.RunTabbed(tabs)
			if err != nil {
				return fmt.Errorf("picker error: %w", err)
			}
			if selections == nil {
				fmt.Println("Cancelled")
				return nil
			}

			if items, ok := selections[string(config.HubSettingFragments)]; ok {
				engine.Hub.SettingFragments = items
			}
			if items, ok := selections[string(config.HubHooks)]; ok {
				engine.Hub.Hooks = items
			}

			// Apply data sharing
			sharedSet := make(map[string]bool)
			if dataItems, ok := selections["data-sharing"]; ok {
				for _, dt := range dataItems {
					sharedSet[dt] = true
				}
			}
			applyDataSharing(&engine.Data, sharedSet)
		}
	}

	if err := mgr.Create(name, engine); err != nil {
		return fmt.Errorf("failed to create engine: %w", err)
	}

	fmt.Printf("Created engine: %s\n", name)
	fmt.Printf("Location: %s\n", paths.EngineDir(name))

	var parts []string
	if len(engine.Hub.SettingFragments) > 0 {
		parts = append(parts, fmt.Sprintf("%d setting-fragments", len(engine.Hub.SettingFragments)))
	}
	if len(engine.Hub.Hooks) > 0 {
		parts = append(parts, fmt.Sprintf("%d hooks", len(engine.Hub.Hooks)))
	}
	if len(parts) > 0 {
		fmt.Printf("Linked: %s\n", strings.Join(parts, ", "))
	}

	return nil
}

func applyDataSharing(data *profile.DataConfig, sharedSet map[string]bool) {
	setMode := func(dt config.DataItemType) config.ShareMode {
		if sharedSet[string(dt)] {
			return config.ShareModeShared
		}
		return config.ShareModeIsolated
	}
	data.Tasks = setMode(config.DataTasks)
	data.Todos = setMode(config.DataTodos)
	data.PasteCache = setMode(config.DataPasteCache)
	data.History = setMode(config.DataHistory)
	data.FileHistory = setMode(config.DataFileHistory)
	data.SessionEnv = setMode(config.DataSessionEnv)
	data.Projects = setMode(config.DataProjects)
	data.Plans = setMode(config.DataPlans)
}
