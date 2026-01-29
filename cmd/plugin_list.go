package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type marketplaceOwner struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type marketplaceMetadata struct {
	Description string `json:"description"`
	Version     string `json:"version"`
}

type marketplacePlugin struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	Tags        []string `json:"tags"`
}

type marketplace struct {
	Name     string              `json:"name"`
	Owner    marketplaceOwner    `json:"owner"`
	Metadata marketplaceMetadata `json:"metadata"`
	Plugins  []marketplacePlugin `json:"plugins"`
}

var pluginListCmd = &cobra.Command{
	Use:   "list <owner/repo>",
	Short: "List plugins from a marketplace",
	Long: `List available plugins from a Claude Code marketplace repository.

Examples:
  ccp plugin list EveryInc/compound-engineering-plugin
  ccp plugin list https://github.com/EveryInc/compound-engineering-plugin`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginList,
}

func init() {
	pluginCmd.AddCommand(pluginListCmd)
}

func runPluginList(cmd *cobra.Command, args []string) error {
	source := args[0]

	// Parse source to owner/repo
	owner, repo, err := parseMarketplaceSource(source)
	if err != nil {
		return err
	}

	// Fetch marketplace.json
	mp, err := fetchMarketplace(owner, repo)
	if err != nil {
		return fmt.Errorf("failed to fetch marketplace: %w", err)
	}

	fmt.Printf("Marketplace: %s\n", mp.Name)
	if mp.Metadata.Description != "" {
		fmt.Printf("Description: %s\n", mp.Metadata.Description)
	}
	fmt.Printf("Owner: %s\n", mp.Owner.Name)
	fmt.Println()

	if len(mp.Plugins) == 0 {
		fmt.Println("No plugins found in this marketplace")
		return nil
	}

	fmt.Printf("Available plugins (%d):\n\n", len(mp.Plugins))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tVERSION\tDESCRIPTION\n")
	fmt.Fprintf(w, "----\t-------\t-----------\n")

	for _, p := range mp.Plugins {
		desc := p.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, p.Version, desc)
	}
	w.Flush()

	fmt.Println()
	fmt.Println("Install with:")
	fmt.Printf("  ccp plugin add %s/%s@<plugin-name>\n", owner, repo)

	return nil
}

func parseMarketplaceSource(source string) (owner, repo string, err error) {
	// Remove URL prefix if present
	source = strings.TrimPrefix(source, "https://github.com/")
	source = strings.TrimPrefix(source, "github.com/")
	source = strings.TrimSuffix(source, ".git")

	// Remove @ suffix if present (for plugin name)
	if atIdx := strings.LastIndex(source, "@"); atIdx > 0 {
		source = source[:atIdx]
	}

	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid source format: %s\n  Expected: owner/repo", source)
	}

	return parts[0], parts[1], nil
}

func fetchMarketplace(owner, repo string) (*marketplace, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/.claude-plugin/marketplace.json", owner, repo)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("marketplace.json not found in %s/%s\n  Make sure the repo has .claude-plugin/marketplace.json", owner, repo)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch marketplace.json: HTTP %d", resp.StatusCode)
	}

	var mp marketplace
	if err := json.NewDecoder(resp.Body).Decode(&mp); err != nil {
		return nil, fmt.Errorf("failed to parse marketplace.json: %w", err)
	}

	return &mp, nil
}
