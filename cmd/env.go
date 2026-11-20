package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var (
	envFormat  string
	envAccount string
	envRelink  bool
)

var envCmd = &cobra.Command{
	Use:    "env [<profile>]",
	Hidden: true,
	Short:  "Configure project environment to use a profile",
	Long: `Update project configuration files to use a specific profile.

Supported formats:
  mise    - Update mise.toml with CLAUDE_CONFIG_DIR
  direnv  - Update .envrc with export CLAUDE_CONFIG_DIR
  shell   - Print shell export command (default)

With --account, CLAUDE_CONFIG_DIR is the account folder ~/.ccp/accounts/<name>,
which keeps its own Claude login and links everything else to the profile.

Examples:
  ccp env dev                    # Print shell export
  ccp env dev --format=mise      # Update mise.toml
  ccp env dev --format=direnv    # Update .envrc
  ccp env default --account work1 --format=mise
  eval "$(ccp env --account auto)"   # this terminal: the freest account

With a named account the profile may be left out. --account auto (shell
format only) picks the account with the most plan limit left once, now.`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runEnv,
}

func init() {
	envCmd.Flags().StringVarP(&envFormat, "format", "f", "shell", "Output format: shell, mise, direnv")
	addAccountFlags(envCmd, &envAccount, &envRelink)
	rootCmd.AddCommand(envCmd)
}

func runEnv(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	profileName := ""
	if len(args) > 0 {
		profileName = args[0]
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	switch envFormat {
	case "shell", "mise", "direnv":
	default:
		return fmt.Errorf("unknown format: %s (valid: shell, mise, direnv)", envFormat)
	}
	accountSet := cmd.Flags().Changed("account")
	if accountSet && envAccount == "" {
		return profile.ValidateAccountName("")
	}
	if envAccount == profile.AutoAccount && envFormat != "shell" {
		return fmt.Errorf("--account auto picks once, so it cannot go into %s; use 'ccp run --account auto' to pick at each start", envFormat)
	}
	if err := refuseAgyAccountForEnvOrSession(paths, envAccount); err != nil {
		return err
	}
	profileName, err = activationProfile(paths, profileName, envAccount)
	if err != nil {
		return err
	}
	account, err := chooseAccount(paths, profileName, envAccount, envRelink, profile.HarnessClaude)
	if err != nil {
		return err
	}
	profilePath, err := activationDir(paths, profileName, account, accountSet, envRelink)
	if err != nil {
		return err
	}
	if account != "" && envFormat == "shell" {
		markAccountUsed(profilePath)
	}

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

// updateMiseTomlMulti updates mise.toml with multiple env vars
func updateMiseTomlMulti(envVars map[string]string) error {
	miseFile := "mise.toml"

	content := ""
	if data, err := os.ReadFile(miseFile); err == nil {
		content = string(data)
	}

	lines := strings.Split(content, "\n")

	// Process each env var
	for key, value := range envVars {
		envLine := fmt.Sprintf("%s = \"%s\"", key, value)
		found := false

		// Check if already has this key
		for i, line := range lines {
			if strings.Contains(line, key+" =") || strings.Contains(line, key+"=") {
				lines[i] = envLine
				found = true
				break
			}
		}

		if !found {
			// Need to add it to [env] section
			envSectionFound := false
			for i, line := range lines {
				if strings.TrimSpace(line) == "[env]" {
					// Insert after [env]
					newLines := make([]string, 0, len(lines)+1)
					newLines = append(newLines, lines[:i+1]...)
					newLines = append(newLines, envLine)
					newLines = append(newLines, lines[i+1:]...)
					lines = newLines
					envSectionFound = true
					break
				}
			}

			if !envSectionFound {
				// Add [env] section at end
				if len(lines) > 0 && lines[len(lines)-1] != "" {
					lines = append(lines, "")
				}
				lines = append(lines, "[env]", envLine)
			}
		}
	}

	content = strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if err := os.WriteFile(miseFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write mise.toml: %w", err)
	}

	fmt.Printf("Updated mise.toml with Claude env vars\n")
	return nil
}

// updateEnvrcMulti updates .envrc with multiple env vars
func updateEnvrcMulti(envVars map[string]string) error {
	envrcFile := ".envrc"

	content := ""
	if data, err := os.ReadFile(envrcFile); err == nil {
		content = string(data)
	}

	lines := strings.Split(content, "\n")

	// Process each env var
	for key, value := range envVars {
		envLine := fmt.Sprintf("export %s=\"%s\"", key, value)
		found := false

		for i, line := range lines {
			if strings.Contains(line, key+"=") || strings.Contains(line, key+" =") {
				lines[i] = envLine
				found = true
				break
			}
		}

		if !found {
			// Append
			lines = append(lines, envLine)
		}
	}

	content = strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if err := os.WriteFile(envrcFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write .envrc: %w", err)
	}

	fmt.Printf("Updated .envrc with Claude env vars\n")
	fmt.Println("Run 'direnv allow' to apply changes")
	return nil
}
