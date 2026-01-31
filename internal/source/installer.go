package source

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samhoang/ccp/internal/config"
)

// Installer handles copying items from sources to hub
type Installer struct {
	paths    *config.Paths
	registry *Registry
}

// NewInstaller creates a new installer
func NewInstaller(paths *config.Paths, registry *Registry) *Installer {
	return &Installer{
		paths:    paths,
		registry: registry,
	}
}

// Install copies items from a source to the hub
func (i *Installer) Install(sourceID string, items []string) ([]string, error) {
	source, err := i.registry.GetSource(sourceID)
	if err != nil {
		return nil, err
	}
	_ = source // Used for validation

	sourceDir := i.paths.SourceDir(sourceID)
	if _, err := os.Stat(sourceDir); err != nil {
		return nil, &SourceError{Op: "install", Source: sourceID,
			Err: fmt.Errorf("source not downloaded: %s", sourceDir)}
	}

	var installed []string

	for _, item := range items {
		parts := strings.SplitN(item, "/", 2)
		if len(parts) != 2 {
			return installed, &SourceError{Op: "install", Source: sourceID,
				Err: fmt.Errorf("invalid item format: %s", item)}
		}
		itemType, itemName := parts[0], parts[1]

		srcPath := filepath.Join(sourceDir, itemType, itemName)
		if _, err := os.Stat(srcPath); err != nil {
			return installed, &SourceError{Op: "install", Source: sourceID,
				Err: fmt.Errorf("item not found: %s", item)}
		}

		dstPath := filepath.Join(i.paths.HubDir, itemType, itemName)

		if _, err := os.Stat(dstPath); err == nil {
			return installed, &SourceError{Op: "install", Source: sourceID,
				Err: fmt.Errorf("item already exists: %s", item)}
		}

		if err := copyTree(srcPath, dstPath); err != nil {
			return installed, &SourceError{Op: "install", Source: sourceID, Err: err}
		}

		if err := i.registry.AddInstalled(sourceID, item); err != nil {
			os.RemoveAll(dstPath)
			return installed, err
		}

		installed = append(installed, item)
	}

	return installed, nil
}

// Uninstall removes items from hub
func (i *Installer) Uninstall(items []string) error {
	for _, item := range items {
		parts := strings.SplitN(item, "/", 2)
		if len(parts) != 2 {
			return &SourceError{Op: "uninstall", Err: fmt.Errorf("invalid item: %s", item)}
		}
		itemType, itemName := parts[0], parts[1]

		itemPath := filepath.Join(i.paths.HubDir, itemType, itemName)
		if err := os.RemoveAll(itemPath); err != nil && !os.IsNotExist(err) {
			return &SourceError{Op: "uninstall", Err: err}
		}

		entry := i.registry.FindSourceByItem(item)
		if entry != nil {
			i.registry.RemoveInstalled(entry.ID, item)
		}
	}

	return nil
}

// DiscoverItems scans a source directory for installable items
func (i *Installer) DiscoverItems(sourceDir string) []string {
	var items []string

	itemTypes := []string{"skills", "agents", "commands", "rules", "hooks", "setting-fragments"}

	for _, itemType := range itemTypes {
		typeDir := filepath.Join(sourceDir, itemType)
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				items = append(items, fmt.Sprintf("%s/%s", itemType, entry.Name()))
			}
		}
	}

	return items
}

func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFileItem(src, dst)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFileItem(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFileItem(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
