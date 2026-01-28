package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var autoCmd = &cobra.Command{
	Use:   "auto",
	Short: "Auto-select profile based on project config",
	Long: `Detect and display the profile for the current project.

Looks for .ccp.yaml in the current directory or parent directories.

Example .ccp.yaml:
  profile: dev

Use with shell integration:
  export CLAUDE_CONFIG_DIR=$(ccp auto --path)`,
	RunE: runAuto,
}

var (
	autoPath bool
)

func init() {
	autoCmd.Flags().BoolVar(&autoPath, "path", false, "Output profile path instead of name")
	rootCmd.AddCommand(autoCmd)
}

type projectConfig struct {
	Profile string `yaml:"profile"`
}

func runAuto(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Search for .ccp.yaml in current dir and parents
	configPath, err := findProjectConfig()
	if err != nil {
		return fmt.Errorf("no .ccp.yaml found in current directory or parents")
	}

	// Parse config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	var cfg projectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse %s: %w", configPath, err)
	}

	if cfg.Profile == "" {
		return fmt.Errorf("no profile specified in %s", configPath)
	}

	// Verify profile exists
	mgr := profile.NewManager(paths)
	if !mgr.Exists(cfg.Profile) {
		return fmt.Errorf("profile not found: %s (from %s)", cfg.Profile, configPath)
	}

	if autoPath {
		fmt.Println(paths.ProfileDir(cfg.Profile))
	} else {
		fmt.Println(cfg.Profile)
	}

	return nil
}

func findProjectConfig() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		configPath := filepath.Join(dir, ".ccp.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		// Also check .ccp.yml
		configPath = filepath.Join(dir, ".ccp.yml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		// Move to parent
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("not found")
}
