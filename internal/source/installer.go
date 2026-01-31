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
		// Resolve source and destination paths
		srcPath, dstItem, err := i.resolveItemPaths(sourceDir, item)
		if err != nil {
			return installed, &SourceError{Op: "install", Source: sourceID, Err: err}
		}

		if _, err := os.Stat(srcPath); err != nil {
			return installed, &SourceError{Op: "install", Source: sourceID,
				Err: fmt.Errorf("item not found: %s", item)}
		}

		parts := strings.SplitN(dstItem, "/", 2)
		if len(parts) != 2 {
			return installed, &SourceError{Op: "install", Source: sourceID,
				Err: fmt.Errorf("invalid item format: %s", dstItem)}
		}
		itemType, itemName := parts[0], parts[1]

		dstPath := filepath.Join(i.paths.HubDir, itemType, itemName)

		if _, err := os.Stat(dstPath); err == nil {
			return installed, &SourceError{Op: "install", Source: sourceID,
				Err: fmt.Errorf("item already exists: %s", dstItem)}
		}

		if err := copyTree(srcPath, dstPath); err != nil {
			return installed, &SourceError{Op: "install", Source: sourceID, Err: err}
		}

		if err := i.registry.AddInstalled(sourceID, dstItem); err != nil {
			os.RemoveAll(dstPath)
			return installed, err
		}

		installed = append(installed, dstItem)
	}

	return installed, nil
}

// resolveItemPaths resolves source path and destination item name
// Handles both direct items (skills/name) and plugin items (plugins/plugin/skills/name)
func (i *Installer) resolveItemPaths(sourceDir, item string) (srcPath, dstItem string, err error) {
	parts := strings.Split(item, "/")

	// Direct item: skills/name -> source/skills/name -> skills/name
	if len(parts) == 2 {
		srcPath = filepath.Join(sourceDir, parts[0], parts[1])
		dstItem = item
		return
	}

	// Plugin item: plugins/plugin/skills/name -> source/plugins/plugin/skills/name -> skills/name or skills/plugin-name
	if len(parts) == 4 && (parts[0] == "plugins" || parts[0] == "external_plugins") {
		pluginName := parts[1]
		itemType := parts[2]
		itemName := parts[3]

		srcPath = filepath.Join(sourceDir, parts[0], pluginName, itemType, itemName)

		// Avoid duplicate: if item name equals plugin name, just use item name
		// e.g., plugins/playground/skills/playground -> skills/playground (not skills/playground-playground)
		if itemName == pluginName {
			dstItem = fmt.Sprintf("%s/%s", itemType, itemName)
		} else {
			dstItem = fmt.Sprintf("%s/%s-%s", itemType, pluginName, itemName)
		}
		return
	}

	err = fmt.Errorf("invalid item format: %s", item)
	return
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

	// Scan root-level item directories (skills/, agents/, etc.)
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

	// Scan plugins/<plugin-name>/<type>/ structure
	pluginDirs := []string{"plugins", "external_plugins"}
	for _, pluginDir := range pluginDirs {
		pluginsPath := filepath.Join(sourceDir, pluginDir)
		plugins, err := os.ReadDir(pluginsPath)
		if err != nil {
			continue
		}

		for _, plugin := range plugins {
			if !plugin.IsDir() {
				continue
			}
			pluginPath := filepath.Join(pluginsPath, plugin.Name())

			// Scan each item type within the plugin
			for _, itemType := range itemTypes {
				typeDir := filepath.Join(pluginPath, itemType)
				entries, err := os.ReadDir(typeDir)
				if err != nil {
					continue
				}

				for _, entry := range entries {
					if entry.IsDir() {
						// Use plugin-prefixed name: plugins/<plugin>/<type>/<name>
						items = append(items, fmt.Sprintf("%s/%s/%s/%s", pluginDir, plugin.Name(), itemType, entry.Name()))
					}
				}
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
