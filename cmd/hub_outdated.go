package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var hubOutdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "Show hub items with available updates",
	Long: `Check hub items against their source repositories for updates.

Compares the installed commit SHA against the latest commit on the remote branch.

Examples:
  ccp hub outdated                 # Check all items for updates`,
	RunE: runHubOutdated,
}

func init() {
	hubCmd.AddCommand(hubOutdatedCmd)
}

type outdatedItem struct {
	item      hub.Item
	localSHA  string
	remoteSHA string
}

func runHubOutdated(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// Find all items with source tracking
	var checkable []hub.Item
	var local []hub.Item

	for _, item := range h.AllItems() {
		if item.Source != nil && item.Source.Type == hub.SourceTypeGitHub && item.Source.GitHub != nil {
			checkable = append(checkable, item)
		} else if item.IsDir {
			local = append(local, item)
		}
	}

	if len(checkable) == 0 {
		fmt.Println("No items with GitHub source tracking found")
		if len(local) > 0 {
			fmt.Printf("  %d items without source tracking (manually added)\n", len(local))
		}
		return nil
	}

	fmt.Printf("Checking %d items for updates...\n\n", len(checkable))

	var outdated []outdatedItem
	var upToDate []hub.Item
	var errors []string

	for _, item := range checkable {
		src := item.Source.GitHub
		remoteSHA, err := getRemoteCommit(src.Owner, src.Repo, src.Ref)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s/%s: %v", item.Type, item.Name, err))
			continue
		}

		localSHA := src.Commit
		if localSHA == "" {
			// No local commit tracked, consider it outdated
			outdated = append(outdated, outdatedItem{
				item:      item,
				localSHA:  "(unknown)",
				remoteSHA: remoteSHA,
			})
		} else if !strings.HasPrefix(remoteSHA, localSHA) && !strings.HasPrefix(localSHA, remoteSHA) {
			outdated = append(outdated, outdatedItem{
				item:      item,
				localSHA:  localSHA,
				remoteSHA: remoteSHA,
			})
		} else {
			upToDate = append(upToDate, item)
		}
	}

	// Display results
	if len(outdated) > 0 {
		fmt.Println("Outdated items:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "TYPE\tNAME\tSOURCE\tLOCAL\tREMOTE\n")
		fmt.Fprintf(w, "----\t----\t------\t-----\t------\n")
		for _, o := range outdated {
			localShort := shortenSHA(o.localSHA)
			remoteShort := shortenSHA(o.remoteSHA)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				o.item.Type, o.item.Name, o.item.Source.SourceInfo(), localShort, remoteShort)
		}
		w.Flush()
		fmt.Println()
		fmt.Printf("Run 'ccp hub update --all' to update all items\n")
	} else {
		fmt.Println("All items are up to date")
	}

	if len(upToDate) > 0 && len(outdated) > 0 {
		fmt.Printf("\n%d items up to date\n", len(upToDate))
	}

	if len(errors) > 0 {
		fmt.Printf("\n%d items could not be checked:\n", len(errors))
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	return nil
}

// getRemoteCommit fetches the latest commit SHA for a GitHub repo
func getRemoteCommit(owner, repo, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	cmd := exec.Command("git", "ls-remote", repoURL, ref)
	cmd.Stderr = io.Discard

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to fetch remote: %w", err)
	}

	// Output format: "<sha>\t<ref>"
	parts := strings.Fields(string(output))
	if len(parts) < 1 {
		return "", fmt.Errorf("no commit found for ref: %s", ref)
	}

	return parts[0], nil
}

// shortenSHA returns first 7 chars of a SHA
func shortenSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
