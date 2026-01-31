package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const skillsAPIBase = "https://skills.sh"

type searchSkill struct {
	Name     string `json:"name"`
	Slug     string `json:"id"`
	Source   string `json:"topSource"`
	Installs int    `json:"installs"`
}

type searchResponse struct {
	Skills []searchSkill `json:"skills"`
}

var skillsFindCmd = &cobra.Command{
	Use:   "find [query]",
	Short: "Search for skills on skills.sh",
	Long: `Search for skills from the open agent skills ecosystem.

Examples:
  ccp skills find react          # Search for React-related skills
  ccp skills find testing        # Search for testing skills
  ccp skills find "pr review"    # Search for PR review skills

After finding a skill, install it with:
  ccp skills add <owner/repo@skill>

Browse all skills at: https://skills.sh/`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSkillsFind,
}

func init() {
	skillsCmd.AddCommand(skillsFindCmd)
}

func runSkillsFind(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	// Show migration hint
	fmt.Printf("Hint: ccp source find %s\n\n", query)

	results, err := searchSkillsAPI(query)
	if err != nil {
		return fmt.Errorf("failed to search skills: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No skills found for \"%s\"\n", query)
		fmt.Println()
		fmt.Println("Browse all skills at: https://skills.sh/")
		return nil
	}

	fmt.Println("Install with: ccp skills add <owner/repo@skill>")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, skill := range results {
		pkg := skill.Source
		if pkg == "" {
			pkg = skill.Slug
		}
		fmt.Fprintf(w, "%s@%s\t(%d installs)\n", pkg, skill.Name, skill.Installs)
		fmt.Fprintf(w, "  └ https://skills.sh/%s/%s\n", pkg, skill.Slug)
	}
	w.Flush()

	return nil
}

func searchSkillsAPI(query string) ([]searchSkill, error) {
	searchURL := fmt.Sprintf("%s/api/search?q=%s&limit=10", skillsAPIBase, url.QueryEscape(query))

	resp, err := http.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Skills, nil
}
