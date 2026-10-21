package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/source"
)

var sourceUpdateCmd = &cobra.Command{
	Use:   "update [source]",
	Short: "Update sources",
	Long: `Update one or all sources to latest version.

Installed items are shadows (symlinks into the clone), so they update in
place. A clone with local edits (any change to a tracked file, or a new file
inside an installed item) is not updated, because the update would discard
them: keep an edit with 'ccp install <source> <item> --clone', or discard it
in the clone with git. These guards need a git source.

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

	installer := source.NewInstaller(paths, registry)
	updated, blocked := 0, 0
	for _, entry := range toUpdate {
		provider := source.GetProvider(entry.Source.Provider)
		if provider == nil {
			fmt.Printf("  %s: unknown provider %s\n", entry.ID, entry.Source.Provider)
			continue
		}

		fmt.Printf("Updating %s...\n", entry.ID)

		// Shadow items are edited inside the clone; the update's hard reset
		// would discard those edits without a word.
		dirty, err := installer.LocalEdits(entry.ID)
		if err != nil {
			fmt.Printf("  %s: %v\n", entry.ID, err)
			blocked++
			continue
		}
		if len(dirty) > 0 {
			fmt.Printf("  %s: not updated, local edits would be lost:\n", entry.ID)
			for _, f := range dirty {
				fmt.Printf("    %s\n", f)
			}
			fmt.Printf("  → keep them: ccp install %s <item> --clone\n", entry.ID)
			fmt.Printf("  → or discard them: git -C %s checkout . && git -C %s clean -fd\n", entry.Source.Path, entry.Source.Path)
			blocked++
			continue
		}

		result, err := provider.Update(ctx, entry.Source.Path, source.UpdateOptions{
			Ref: entry.Source.Ref,
			URL: entry.Source.URL,
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

	if updated > 0 {
		fmt.Printf("\n%d of %d sources had updates\n", updated, len(toUpdate))
	} else if blocked == 0 {
		fmt.Printf("\nAll %d sources up to date\n", len(toUpdate))
	}
	if blocked > 0 {
		return fmt.Errorf("%d source(s) not updated", blocked)
	}
	return nil
}
