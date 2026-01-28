package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	envFormat string
)

var envCmd = &cobra.Command{
	Use:   "env <profile>",
	Short: "Configure project environment to use a profile",
	Long: `Update project configuration files to use a specific profile.

Supported formats:
  mise    - Update mise.toml with CLAUDE_CONFIG_DIR
  direnv  - Update .envrc with export CLAUDE_CONFIG_DIR
  shell   - Print shell export command (default)

Examples:
  ccp env dev                    # Print shell export
  ccp env dev --format=mise      # Update mise.toml
  ccp env dev --format=direnv    # Update .envrc`,
	Args: cobra.ExactArgs(1),
	RunE: runEnv,
}

func init() {
	envCmd.Flags().StringVarP(&envFormat, "format", "f", "shell", "Output format: shell, mise, direnv")
	rootCmd.AddCommand(envCmd)
}

func runEnv(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Verify profile exists
	mgr := profile.NewManager(paths)
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	profilePath := paths.ProfileDir(profileName)

	switch envFormat {
	case "shell":
		fmt.Printf("export CLAUDE_CONFIG_DIR=\"%s\"\n", profilePath)
		return nil

	case "mise":
		return updateMiseToml(profilePath)

	case "direnv":
		return updateEnvrc(profilePath)

	default:
		return fmt.Errorf("unknown format: %s (valid: shell, mise, direnv)", envFormat)
	}
}

func updateMiseToml(profilePath string) error {
	miseFile := "mise.toml"

	// Check if mise.toml exists
	content := ""
	if data, err := os.ReadFile(miseFile); err == nil {
		content = string(data)
	}

	envLine := fmt.Sprintf("CLAUDE_CONFIG_DIR = \"%s\"", profilePath)

	// Check if already has CLAUDE_CONFIG_DIR
	if strings.Contains(content, "CLAUDE_CONFIG_DIR") {
		// Replace existing
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.Contains(line, "CLAUDE_CONFIG_DIR") {
				lines[i] = envLine
				break
			}
		}
		content = strings.Join(lines, "\n")
	} else {
		// Add to [env] section or create it
		if strings.Contains(content, "[env]") {
			// Find [env] section and add after it
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				if strings.TrimSpace(line) == "[env]" {
					// Insert after [env]
					newLines := make([]string, 0, len(lines)+1)
					newLines = append(newLines, lines[:i+1]...)
					newLines = append(newLines, envLine)
					newLines = append(newLines, lines[i+1:]...)
					lines = newLines
					break
				}
			}
			content = strings.Join(lines, "\n")
		} else {
			// Add [env] section
			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += fmt.Sprintf("\n[env]\n%s\n", envLine)
		}
	}

	if err := os.WriteFile(miseFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write mise.toml: %w", err)
	}

	fmt.Printf("Updated mise.toml with CLAUDE_CONFIG_DIR = \"%s\"\n", profilePath)
	return nil
}

func updateEnvrc(profilePath string) error {
	envrcFile := ".envrc"
	envLine := fmt.Sprintf("export CLAUDE_CONFIG_DIR=\"%s\"", profilePath)

	// Check if .envrc exists
	content := ""
	if data, err := os.ReadFile(envrcFile); err == nil {
		content = string(data)
	}

	// Check if already has CLAUDE_CONFIG_DIR
	if strings.Contains(content, "CLAUDE_CONFIG_DIR") {
		// Replace existing
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.Contains(line, "CLAUDE_CONFIG_DIR") {
				lines[i] = envLine
				break
			}
		}
		content = strings.Join(lines, "\n")
	} else {
		// Append
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += envLine + "\n"
	}

	if err := os.WriteFile(envrcFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write .envrc: %w", err)
	}

	fmt.Printf("Updated .envrc with CLAUDE_CONFIG_DIR\n")
	fmt.Println("Run 'direnv allow' to apply changes")
	return nil
}
