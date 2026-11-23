package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "ccp",
	Short: "Claude Code Profile manager",
	Long: `ccp (Claude Code Profile) manages a central hub of reusable components
and multiple profiles for Claude Code. Each profile is a complete Claude Code
configuration directory, activated via CLAUDE_CONFIG_DIR or symlink.`,
	Version: Version,
	Run:     runRoot,
}

func runRoot(cmd *cobra.Command, args []string) {
	// Check if ccp is initialized
	paths, err := config.ResolvePaths()
	if err != nil {
		cmd.Help()
		return
	}

	if !paths.IsInitialized() {
		fmt.Println("ccp - Claude Code Profile Manager")
		fmt.Println()
		fmt.Println("Not initialized. Get started with:")
		fmt.Println()
		fmt.Println("  ccp init       Initialize ccp (migrates existing ~/.claude)")
		fmt.Println("  ccp --help     Show all commands")
		fmt.Println()
		fmt.Println("Learn more: https://github.com/samhvw8/claude-code-profile")
		return
	}

	// If initialized, show help
	cmd.Help()
}

func Execute() {
	// Cobra would print the error too; print it once, here.
	rootCmd.SilenceErrors = true
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// harnessAnnotation marks a hidden command that mirrors a profile into
// another harness. The command surface keeps them out of the command list
// (decisions/command-surface.md); 'ccp --help' names them in a footer built
// from this mark, so a new harness command only needs the annotation.
const harnessAnnotation = "ccp-harness"

// harnessFooter lists the hidden harness commands of the root command, or ""
// below the root and when there are none.
func harnessFooter(c *cobra.Command) string {
	if c.HasParent() {
		return ""
	}
	var b strings.Builder
	for _, sub := range c.Commands() {
		if sub.Annotations[harnessAnnotation] == "" {
			continue
		}
		fmt.Fprintf(&b, "  %-*s %s\n", c.NamePadding(), sub.Name(), sub.Short)
	}
	if b.Len() == 0 {
		return ""
	}
	return "\nOther harnesses (hidden; after the first sync, 'ccp use -g', link and unlink keep them current):\n" + b.String()
}

func init() {
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	cobra.AddTemplateFunc("harnessFooter", harnessFooter)
	rootCmd.SetUsageTemplate(rootCmd.UsageTemplate() + "{{harnessFooter .}}")
}
