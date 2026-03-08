package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var engineShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show engine details",
	Args:  cobra.ExactArgs(1),
	RunE:  runEngineShow,
}

func init() {
	engineCmd.AddCommand(engineShowCmd)
}

func runEngineShow(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewEngineManager(paths)
	engine, err := mgr.Get(args[0])
	if err != nil {
		return fmt.Errorf("engine not found: %s", args[0])
	}

	fmt.Printf("Engine: %s\n", engine.Name)
	if engine.Description != "" {
		fmt.Printf("Description: %s\n", engine.Description)
	}
	fmt.Printf("Path: %s\n", paths.EngineDir(args[0]))

	if len(engine.Hub.SettingFragments) > 0 {
		fmt.Printf("\nSetting Fragments: %s\n", strings.Join(engine.Hub.SettingFragments, ", "))
	}
	if len(engine.Hub.Hooks) > 0 {
		fmt.Printf("Hooks: %s\n", strings.Join(engine.Hub.Hooks, ", "))
	}

	// Data sharing
	fmt.Println("\nData Sharing:")
	for _, dt := range config.AllDataItemTypes() {
		var mode config.ShareMode
		switch dt {
		case config.DataTasks:
			mode = engine.Data.Tasks
		case config.DataTodos:
			mode = engine.Data.Todos
		case config.DataPasteCache:
			mode = engine.Data.PasteCache
		case config.DataHistory:
			mode = engine.Data.History
		case config.DataFileHistory:
			mode = engine.Data.FileHistory
		case config.DataSessionEnv:
			mode = engine.Data.SessionEnv
		case config.DataProjects:
			mode = engine.Data.Projects
		case config.DataPlans:
			mode = engine.Data.Plans
		}
		fmt.Printf("  %s: %s\n", dt, mode)
	}

	// Show which profiles use this engine
	users, err := mgr.ProfilesUsing(args[0])
	if err == nil && len(users) > 0 {
		fmt.Printf("\nUsed by profiles: %s\n", strings.Join(users, ", "))
	}

	return nil
}
