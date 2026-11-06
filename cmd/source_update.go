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

After each update, a shadow whose item moved upstream is relinked, and every
installed item the update does not reach (a hub copy, your own link, an item
missing from the hub or gone upstream) is listed with the command to fix it.

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
	updated, blocked, behindCount := 0, 0, 0
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

		// Checked on every run, not only after new commits: a copy left behind
		// by an earlier update is still behind.
		relinked, behind, err := installer.FollowUpdate(entry.ID)
		for _, item := range relinked {
			fmt.Printf("  %s: relinked %s (moved in the source)\n", entry.ID, item)
		}
		if err != nil {
			fmt.Printf("  %s: %v\n", entry.ID, err)
		}
		if len(behind) > 0 {
			printBehind(entry.ID, behind)
			behindCount += len(behind)
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
	if behindCount > 0 {
		fmt.Printf("WARN: %d installed item(s) do not follow their source (listed above)\n", behindCount)
	}
	if blocked > 0 {
		return fmt.Errorf("%d source(s) not updated", blocked)
	}
	return nil
}

// printBehind lists the installed items of sourceID that the update did not
// reach, each with the command that makes it follow the source again or keeps
// it as the user's own.
func printBehind(sourceID string, behind []source.Behind) {
	fmt.Printf("  %s: not reached by the update:\n", sourceID)
	for _, b := range behind {
		switch {
		case b.Gone:
			fmt.Printf("    %s: gone from the source (its shadow is broken)\n", b.Item)
			fmt.Printf("      → install it again by its new path, or drop it from installed in ccp.toml and run: ccp hub remove %s\n", b.Item)
		case b.State == source.ItemMissing:
			fmt.Printf("    %s: not in the hub\n", b.Item)
			fmt.Println("      → recreate it as a shadow: ccp install")
		case b.State == source.ItemCopySame:
			fmt.Printf("    %s: a copy, not a shadow (same as the source)\n", b.Item)
			fmt.Println("      → make it a shadow: ccp doctor --fix")
		case b.State == source.ItemCopyDiffers:
			fmt.Printf("    %s: a copy, not a shadow (a local edit or an older version)\n", b.Item)
			fmt.Printf("      → take the source: ccp hub remove %s && ccp install\n", b.Item)
			fmt.Printf("      → keep it as yours: ccp install %s %s --clone\n", sourceID, b.Item)
		case b.State == source.ItemUnresolved:
			fmt.Printf("    %s: a copy whose path is gone from the source\n", b.Item)
			fmt.Printf("      → keep it as yours: ccp install %s %s --clone\n", sourceID, b.Item)
		case b.State == source.ItemForeignLink:
			fmt.Printf("    %s: a link outside sources/ (yours)\n", b.Item)
			fmt.Printf("      → keep it as yours: ccp install %s %s --clone\n", sourceID, b.Item)
		}
	}
}
