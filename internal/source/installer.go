package source

import (
	"encoding/json"
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

// pluginJSON represents .claude-plugin/plugin.json structure
type pluginJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Skills      any    `json:"skills"`   // string or []string
	Commands    any    `json:"commands"` // string or []string
	Agents      any    `json:"agents"`   // string or []string
	Hooks       any    `json:"hooks"`    // string or object
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

// validExtensions are the file extensions to check when resolving flat file items
var validExtensions = []string{".md", ".yaml", ".yml", ".json", ".toml"}

// tryFileExtensions checks if an item exists as a file with any valid extension
// Returns the full path and the extension found (e.g., ".md")
func (i *Installer) tryFileExtensions(baseDir, itemType, itemName string) (filePath, ext string) {
	for _, e := range validExtensions {
		candidate := filepath.Join(baseDir, itemType, itemName+e)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, e
		}
	}
	return "", ""
}

// formatPluginDstItem formats the destination item name for plugin items
// ext is optional - pass empty string for directories, or ".md" etc for files
func formatPluginDstItem(pluginName, itemType, itemName, ext string) string {
	if itemName == pluginName {
		return fmt.Sprintf("%s/%s%s", itemType, itemName, ext)
	}
	return fmt.Sprintf("%s/%s-%s%s", itemType, pluginName, itemName, ext)
}

// resolveItemPaths resolves source path and destination item name
// Handles multiple structures:
// - Direct items: skills/name -> source/skills/name or source/.claude/skills/name
// - Plugin items: plugins/plugin/skills/name -> source/plugins/plugin/skills/name
func (i *Installer) resolveItemPaths(sourceDir, item string) (srcPath, dstItem string, err error) {
	parts := strings.Split(item, "/")

	// Direct item: skills/name
	if len(parts) == 2 {
		itemType, itemName := parts[0], parts[1]

		// Try root level first (directory)
		srcPath = filepath.Join(sourceDir, itemType, itemName)
		if _, statErr := os.Stat(srcPath); statErr == nil {
			dstItem = item
			return
		}

		// Try root level as file with extension
		if filePath, ext := i.tryFileExtensions(sourceDir, itemType, itemName); filePath != "" {
			srcPath = filePath
			dstItem = fmt.Sprintf("%s/%s%s", itemType, itemName, ext)
			return
		}

		// Try .claude/ folder (directory)
		srcPath = filepath.Join(sourceDir, ".claude", itemType, itemName)
		if _, statErr := os.Stat(srcPath); statErr == nil {
			dstItem = item
			return
		}

		// Try .claude/ folder as file
		if filePath, ext := i.tryFileExtensions(filepath.Join(sourceDir, ".claude"), itemType, itemName); filePath != "" {
			srcPath = filePath
			dstItem = fmt.Sprintf("%s/%s%s", itemType, itemName, ext)
			return
		}

		// Not found in either location
		err = fmt.Errorf("item not found: %s (checked %s/ and .claude/%s/)", item, itemType, itemType)
		return
	}

	// Plugin item: plugins/plugin/skills/name -> source/plugins/plugin/skills/name -> skills/name or skills/plugin-name
	if len(parts) == 4 && (parts[0] == "plugins" || parts[0] == "external_plugins") {
		pluginName := parts[1]
		itemType := parts[2]
		itemName := parts[3]

		baseDir := filepath.Join(sourceDir, parts[0], pluginName)

		// Try as directory first
		srcPath = filepath.Join(baseDir, itemType, itemName)
		if _, statErr := os.Stat(srcPath); statErr == nil {
			dstItem = formatPluginDstItem(pluginName, itemType, itemName, "")
			return
		}

		// Try as file with extension
		if filePath, ext := i.tryFileExtensions(baseDir, itemType, itemName); filePath != "" {
			srcPath = filePath
			dstItem = formatPluginDstItem(pluginName, itemType, itemName, ext)
			return
		}

		// Return path for error message (will fail os.Stat in caller)
		srcPath = filepath.Join(baseDir, itemType, itemName)
		dstItem = formatPluginDstItem(pluginName, itemType, itemName, "")
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
// Supports multiple structures:
// 1. Root-level: skills/, agents/, commands/, hooks/
// 2. Claude Code plugin: .claude-plugin/plugin.json with skills/, commands/, agents/
// 3. .claude folder: .claude/skills/, .claude/commands/
func (i *Installer) DiscoverItems(sourceDir string) []string {
	var items []string
	seen := make(map[string]bool)

	itemTypes := []string{"skills", "agents", "commands", "rules", "hooks", "setting-fragments"}

	// Helper to add items without duplicates
	addItem := func(item string) {
		if !seen[item] {
			seen[item] = true
			items = append(items, item)
		}
	}

	// 1. Scan root-level item directories (skills/, agents/, etc.)
	for _, itemType := range itemTypes {
		typeDir := filepath.Join(sourceDir, itemType)
		i.scanItemDir(typeDir, itemType, addItem)
	}

	// 2. Scan .claude/ folder (Claude Code standalone config)
	claudeDir := filepath.Join(sourceDir, ".claude")
	for _, itemType := range itemTypes {
		typeDir := filepath.Join(claudeDir, itemType)
		i.scanItemDir(typeDir, itemType, addItem)
	}

	// 3. Check for .claude-plugin/plugin.json and parse custom paths
	pluginJSON := filepath.Join(sourceDir, ".claude-plugin", "plugin.json")
	if data, err := os.ReadFile(pluginJSON); err == nil {
		i.discoverFromPluginJSON(sourceDir, data, itemTypes, addItem)
	}

	// 4. Scan plugins/<plugin-name>/<type>/ structure (legacy/marketplace)
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
						addItem(fmt.Sprintf("%s/%s/%s/%s", pluginDir, plugin.Name(), itemType, entry.Name()))
					} else if isValidItemFile(entry.Name()) {
						// Also scan for flat files (e.g., agents/foo.md, commands/bar.md)
						name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
						addItem(fmt.Sprintf("%s/%s/%s/%s", pluginDir, plugin.Name(), itemType, name))
					}
				}
			}
		}
	}

	return items
}

// scanItemDir scans a directory for item subdirectories and valid item files
func (i *Installer) scanItemDir(typeDir, itemType string, addItem func(string)) {
	entries, err := os.ReadDir(typeDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			addItem(fmt.Sprintf("%s/%s", itemType, entry.Name()))
		} else if isValidItemFile(entry.Name()) {
			// Also scan for flat files (e.g., agents/foo.md, commands/bar.md)
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			addItem(fmt.Sprintf("%s/%s", itemType, name))
		}
	}
}

// isValidItemFile checks if a file is a valid item file (not hidden, valid extension)
func isValidItemFile(name string) bool {
	// Skip hidden files
	if strings.HasPrefix(name, ".") {
		return false
	}

	// Valid item file extensions
	ext := strings.ToLower(filepath.Ext(name))
	validExts := map[string]bool{
		".md":   true, // Markdown (skills, agents, commands)
		".yaml": true, // YAML config
		".yml":  true, // YAML config
		".json": true, // JSON config
		".toml": true, // TOML config
	}

	return validExts[ext]
}

// discoverFromPluginJSON parses plugin.json for custom component paths
func (i *Installer) discoverFromPluginJSON(sourceDir string, data []byte, _ []string, addItem func(string)) {
	var plugin pluginJSON
	if err := json.Unmarshal(data, &plugin); err != nil {
		return
	}

	// Parse custom paths from plugin.json
	pathFields := map[string]any{
		"skills":   plugin.Skills,
		"commands": plugin.Commands,
		"agents":   plugin.Agents,
	}

	for itemType, pathVal := range pathFields {
		if pathVal == nil {
			continue
		}

		var paths []string
		switch v := pathVal.(type) {
		case string:
			paths = []string{v}
		case []any:
			for _, p := range v {
				if s, ok := p.(string); ok {
					paths = append(paths, s)
				}
			}
		}

		for _, p := range paths {
			// Clean path: remove ./ prefix
			p = strings.TrimPrefix(p, "./")
			fullPath := filepath.Join(sourceDir, p)

			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			if info.IsDir() {
				// It's a directory - scan for items
				i.scanItemDir(fullPath, itemType, addItem)
			} else if strings.HasSuffix(p, ".md") {
				// It's a markdown file - extract item name from path
				// e.g., "./custom/cmd.md" -> commands/cmd
				name := strings.TrimSuffix(filepath.Base(p), ".md")
				addItem(fmt.Sprintf("%s/%s", itemType, name))
			}
		}
	}
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

		// Use os.Stat to resolve symlinks and get actual type
		info, err := os.Stat(srcPath)
		if err != nil {
			return err
		}

		if info.IsDir() {
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
