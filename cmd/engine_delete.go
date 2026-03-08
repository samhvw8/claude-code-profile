package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var engineDeleteForce bool

var engineDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an engine",
	Args:  cobra.ExactArgs(1),
	RunE:  runEngineDelete,
}

func init() {
	engineDeleteCmd.Flags().BoolVarP(&engineDeleteForce, "force", "f", false, "Skip confirmation")
	engineCmd.AddCommand(engineDeleteCmd)
}

func runEngineDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewEngineManager(paths)

	if !mgr.Exists(name) {
		return fmt.Errorf("engine not found: %s", name)
	}

	// Check if any profiles reference this engine
	users, err := mgr.ProfilesUsing(name)
	if err != nil {
		return fmt.Errorf("failed to check engine usage: %w", err)
	}
	if len(users) > 0 {
		fmt.Printf("Warning: engine '%s' is used by profiles: %s\n", name, strings.Join(users, ", "))
		if !engineDeleteForce {
			fmt.Print("Delete anyway? Type 'yes' to confirm: ")
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			if strings.TrimSpace(strings.ToLower(input)) != "yes" {
				fmt.Println("Cancelled")
				return nil
			}
		}
	} else if !engineDeleteForce {
		fmt.Printf("Delete engine '%s'? Type 'yes' to confirm: ", name)
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.TrimSpace(strings.ToLower(input)) != "yes" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	if err := mgr.Delete(name); err != nil {
		return fmt.Errorf("failed to delete engine: %w", err)
	}

	fmt.Printf("Deleted engine: %s\n", name)
	return nil
}
