package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var templateCreateFromFile string

var templateCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new settings template",
	Long: `Create a new settings.json template.

If --from-file is specified, reads the JSON from that file.
Otherwise, opens $EDITOR to write the template.

Examples:
  ccp template create opus-full --from-file=my-settings.json
  ccp template create minimal                                  # Opens $EDITOR`,
	Args: cobra.ExactArgs(1),
	RunE: runTemplateCreate,
}

func init() {
	templateCreateCmd.Flags().StringVar(&templateCreateFromFile, "from-file", "", "Read settings from JSON file")
	templateCmd.AddCommand(templateCreateCmd)
}

func runTemplateCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := hub.NewTemplateManager(paths.HubDir)
	if mgr.Exists(name) {
		return fmt.Errorf("template already exists: %s (use 'ccp template edit %s' to modify)", name, name)
	}

	var settings map[string]interface{}

	if templateCreateFromFile != "" {
		data, err := os.ReadFile(templateCreateFromFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	} else {
		settings, err = editSettingsInEditor(nil)
		if err != nil {
			return err
		}
	}

	// Remove hooks if present — they're managed separately
	delete(settings, "hooks")

	t := &hub.Template{Name: name, Settings: settings}
	if err := mgr.Save(t); err != nil {
		return fmt.Errorf("failed to save template: %w", err)
	}

	fmt.Printf("Created template: %s (%d settings keys)\n", name, len(settings))
	return nil
}

func editSettingsInEditor(initial map[string]interface{}) (map[string]interface{}, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	tmpFile, err := os.CreateTemp("", "ccp-template-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write initial content
	var data []byte
	if initial != nil {
		data, _ = json.MarshalIndent(initial, "", "  ")
	} else {
		data = []byte("{\n  \n}\n")
	}
	if err := os.WriteFile(tmpFile.Name(), data, 0644); err != nil {
		return nil, err
	}

	// Open editor
	editorCmd := exec.Command(editor, tmpFile.Name())
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return nil, fmt.Errorf("editor failed: %w", err)
	}

	// Read back
	edited, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(edited, &settings); err != nil {
		return nil, fmt.Errorf("invalid JSON after editing: %w", err)
	}

	return settings, nil
}
