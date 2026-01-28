package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
)

var hubEditCmd = &cobra.Command{
	Use:   "edit <type>/<name>",
	Short: "Edit a hub item in $EDITOR",
	Long: `Open a hub item in your default editor.

Uses $EDITOR environment variable (falls back to vim).

Examples:
  ccp hub edit skills/my-skill.md
  ccp hub edit hooks/pre-commit.sh`,
	Args: cobra.ExactArgs(1),
	RunE: runHubEdit,
}

func init() {
	hubCmd.AddCommand(hubEditCmd)
}

func runHubEdit(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Parse type/name
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format: use <type>/<name>")
	}

	itemType := config.HubItemType(parts[0])
	itemName := parts[1]

	if !isValidHubType(itemType) {
		return fmt.Errorf("invalid type: %s", parts[0])
	}

	itemPath := paths.HubItemPath(itemType, itemName)
	info, err := os.Stat(itemPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("item not found: %s/%s", itemType, itemName)
	}
	if err != nil {
		return err
	}

	if info.IsDir() {
		return fmt.Errorf("%s/%s is a directory - specify a file inside it", itemType, itemName)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	editorCmd := exec.Command(editor, itemPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	return editorCmd.Run()
}
