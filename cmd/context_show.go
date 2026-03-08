package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var contextShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show context details",
	Args:  cobra.ExactArgs(1),
	RunE:  runContextShow,
}

func init() {
	contextCmd.AddCommand(contextShowCmd)
}

func runContextShow(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewContextManager(paths)
	ctx, err := mgr.Get(args[0])
	if err != nil {
		return fmt.Errorf("context not found: %s", args[0])
	}

	fmt.Printf("Context: %s\n", ctx.Name)
	if ctx.Description != "" {
		fmt.Printf("Description: %s\n", ctx.Description)
	}
	fmt.Printf("Path: %s\n", paths.ContextDir(args[0]))

	if len(ctx.Hub.Skills) > 0 {
		fmt.Printf("\nSkills: %s\n", strings.Join(ctx.Hub.Skills, ", "))
	}
	if len(ctx.Hub.Agents) > 0 {
		fmt.Printf("Agents: %s\n", strings.Join(ctx.Hub.Agents, ", "))
	}
	if len(ctx.Hub.Rules) > 0 {
		fmt.Printf("Rules: %s\n", strings.Join(ctx.Hub.Rules, ", "))
	}
	if len(ctx.Hub.Commands) > 0 {
		fmt.Printf("Commands: %s\n", strings.Join(ctx.Hub.Commands, ", "))
	}
	if len(ctx.Hub.Hooks) > 0 {
		fmt.Printf("Hooks: %s\n", strings.Join(ctx.Hub.Hooks, ", "))
	}

	// Show which profiles use this context
	users, err := mgr.ProfilesUsing(args[0])
	if err == nil && len(users) > 0 {
		fmt.Printf("\nUsed by profiles: %s\n", strings.Join(users, ", "))
	}

	return nil
}
