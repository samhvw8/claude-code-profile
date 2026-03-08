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

var contextDeleteForce bool

var contextDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a context",
	Args:  cobra.ExactArgs(1),
	RunE:  runContextDelete,
}

func init() {
	contextDeleteCmd.Flags().BoolVarP(&contextDeleteForce, "force", "f", false, "Skip confirmation")
	contextCmd.AddCommand(contextDeleteCmd)
}

func runContextDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewContextManager(paths)

	if !mgr.Exists(name) {
		return fmt.Errorf("context not found: %s", name)
	}

	// Check if any profiles reference this context
	users, err := mgr.ProfilesUsing(name)
	if err != nil {
		return fmt.Errorf("failed to check context usage: %w", err)
	}
	if len(users) > 0 {
		fmt.Printf("Warning: context '%s' is used by profiles: %s\n", name, strings.Join(users, ", "))
		if !contextDeleteForce {
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
	} else if !contextDeleteForce {
		fmt.Printf("Delete context '%s'? Type 'yes' to confirm: ", name)
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
		return fmt.Errorf("failed to delete context: %w", err)
	}

	fmt.Printf("Deleted context: %s\n", name)
	return nil
}
