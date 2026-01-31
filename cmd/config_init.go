package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
)

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate default ccp.toml configuration",
	Long: `Generate a default ccp.toml configuration file.

The config file controls:
  - GitHub search topics for finding skills
  - skills.sh API settings
  - Default registry for searches

Example ccp.toml:

  default_registry = "skills.sh"

  [github]
  topics = ["agent-skills", "claude-code", "claude-skills"]
  per_page = 10

  [skillssh]
  base_url = "https://skills.sh"
  limit = 10`,
	RunE: runConfigInit,
}

func init() {
	configCmd.AddCommand(configInitCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	configPath := filepath.Join(paths.CcpDir, "ccp.toml")

	// Check if already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists: %s\n", configPath)
		fmt.Println("Edit it directly or delete to regenerate.")
		return nil
	}

	// Create default config
	cfg := config.DefaultCcpConfig()
	if err := cfg.Save(paths.CcpDir); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Created: %s\n", configPath)
	return nil
}
