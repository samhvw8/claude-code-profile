package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/source"
)

var sourceUpdateCmd = &cobra.Command{
	Use:   "update [source]",
	Short: "Update sources",
	Long: `Update one or all sources to latest version.

Examples:
  ccp source update                  # Update all sources
  ccp source update samhoang/skills  # Update specific source`,
	RunE: runSourceUpdate,
}

func init() {
	sourceCmd.AddCommand(sourceUpdateCmd)
}

func runSourceUpdate(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	ctx := context.Background()
	var toUpdate []source.SourceEntry

	if len(args) > 0 {
		src, err := registry.GetSource(args[0])
		if err != nil {
			return err
		}
		toUpdate = []source.SourceEntry{{ID: args[0], Source: *src}}
	} else {
		toUpdate = registry.ListSources()
	}

	if len(toUpdate) == 0 {
		fmt.Println("No sources to update")
		return nil
	}

	updated := 0
	for _, entry := range toUpdate {
		provider := source.GetProvider(entry.Source.Provider)
		if provider == nil {
			fmt.Printf("  %s: unknown provider %s\n", entry.ID, entry.Source.Provider)
			continue
		}

		fmt.Printf("Updating %s...\n", entry.ID)

		result, err := provider.Update(ctx, entry.Source.Path, source.UpdateOptions{
			Ref: entry.Source.Ref,
		})
		if err != nil {
			fmt.Printf("  %s: %v\n", entry.ID, err)
			continue
		}

		if result.Updated {
			src := entry.Source
			src.Commit = result.NewCommit
			registry.UpdateSource(entry.ID, src)

			oldCommit := result.OldCommit
			newCommit := result.NewCommit
			if len(oldCommit) > 7 {
				oldCommit = oldCommit[:7]
			}
			if len(newCommit) > 7 {
				newCommit = newCommit[:7]
			}
			fmt.Printf("  %s: updated %s -> %s\n", entry.ID, oldCommit, newCommit)
			updated++
		} else {
			fmt.Printf("  %s: already up to date\n", entry.ID)
		}
	}

	if updated > 0 {
		if err := registry.Save(); err != nil {
			return err
		}
	}

	fmt.Printf("\nUpdated %d/%d sources\n", updated, len(toUpdate))
	return nil
}
