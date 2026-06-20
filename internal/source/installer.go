package source

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samhoang/ccp/internal/config"
	"gopkg.in/yaml.v3"
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

type marketplaceJSON struct {
	Plugins []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	} `json:"plugins"`
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

		if err := CopyTree(srcPath, dstPath); err != nil {
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

		// Try Codex directories (.agents/, .codex/)
		for _, codexDir := range codexDirs {
			candidate := filepath.Join(sourceDir, codexDir, itemType, itemName)
			if _, statErr := os.Stat(candidate); statErr == nil {
				srcPath = candidate
				dstItem = item
				return
			}
			if filePath, ext := i.tryFileExtensions(filepath.Join(sourceDir, codexDir), itemType, itemName); filePath != "" {
				srcPath = filePath
				dstItem = fmt.Sprintf("%s/%s%s", itemType, itemName, ext)
				return
			}
		}

		// Try marketplace plugin subdirs (.claude-plugin/ and .codex-plugin/)
		if found := i.findInMarketplace(sourceDir, itemType, itemName); found != "" {
			srcPath = found
			dstItem = item
			return
		}

		// Bare skill repo: SKILL.md at the repository root. The whole repo is
		// the skill, so the source path is the repo root itself.
		if itemType == "skills" && rootSkillName(sourceDir) == itemName {
			srcPath = sourceDir
			dstItem = item
			return
		}

		// Not found
		err = fmt.Errorf("item not found: %s (checked %s/)", item, itemType)
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

// InstallToDir copies items from a source to an arbitrary target directory.
// Unlike Install, it does not track items in the registry and filters by allowed types.
func (i *Installer) InstallToDir(sourceID string, items []string, targetDir string, allowedTypes map[string]bool) ([]string, error) {
	_, err := i.registry.GetSource(sourceID)
	if err != nil {
		return nil, err
	}

	sourceDir := i.paths.SourceDir(sourceID)
	if _, err := os.Stat(sourceDir); err != nil {
		return nil, &SourceError{Op: "install", Source: sourceID,
			Err: fmt.Errorf("source not downloaded: %s", sourceDir)}
	}

	var installed []string

	for _, item := range items {
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

		if allowedTypes != nil && !allowedTypes[itemType] {
			return installed, &SourceError{Op: "install", Source: sourceID,
				Err: fmt.Errorf("type not allowed for project install: %s", itemType)}
		}

		dstPath := filepath.Join(targetDir, itemType, itemName)

		if _, err := os.Stat(dstPath); err == nil {
			if err := os.RemoveAll(dstPath); err != nil {
				return installed, &SourceError{Op: "install", Source: sourceID, Err: err}
			}
		}

		if err := CopyTree(srcPath, dstPath); err != nil {
			return installed, &SourceError{Op: "install", Source: sourceID, Err: err}
		}

		installed = append(installed, dstItem)
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

// codexDirs are Codex-format directories to scan for items
var codexDirs = []string{".agents", ".codex"}

// DiscoverItems scans a source directory for installable items
// Supports multiple structures:
// 1. Root-level: skills/, agents/, commands/, hooks/
// 2. Claude Code plugin: .claude-plugin/plugin.json
// 3. Codex format: .agents/skills/, .codex/skills/
// 4. Codex plugin: .codex-plugin/plugin.json, .codex-plugin/marketplace.json
// 5. Plugin subdirs: plugins/<name>/<type>/
// 6. Bare skill repo: SKILL.md at the repository root (the whole repo IS one skill)
func (i *Installer) DiscoverItems(sourceDir string) []string {
	var items []string
	seen := make(map[string]bool)

	itemTypes := []string{"skills", "agents", "commands", "rules", "hooks", "settings-templates"}

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

	// 1b. Bare skill repo: SKILL.md at the repository root (the whole repo IS
	// one skill, with no skills/<name>/ wrapper). Name comes from frontmatter.
	if name := rootSkillName(sourceDir); name != "" {
		addItem("skills/" + name)
	}

	// 2. Check for .claude-plugin/plugin.json and parse custom paths (plugin format)
	pluginJSON := filepath.Join(sourceDir, ".claude-plugin", "plugin.json")
	if data, err := os.ReadFile(pluginJSON); err == nil {
		i.discoverFromPluginJSON(sourceDir, data, itemTypes, addItem)
	}

	// 3. Check for .claude-plugin/marketplace.json and scan plugin subdirs
	marketplaceFile := filepath.Join(sourceDir, ".claude-plugin", "marketplace.json")
	if data, err := os.ReadFile(marketplaceFile); err == nil {
		i.discoverFromMarketplace(sourceDir, data, itemTypes, addItem)
	}

	// 4. Scan Codex-format directories (.agents/, .codex/)
	for _, codexDir := range codexDirs {
		for _, itemType := range itemTypes {
			typeDir := filepath.Join(sourceDir, codexDir, itemType)
			i.scanItemDir(typeDir, itemType, addItem)
		}
	}

	// 5. Check for .codex-plugin/plugin.json (Codex plugin format)
	codexPluginJSON := filepath.Join(sourceDir, ".codex-plugin", "plugin.json")
	if data, err := os.ReadFile(codexPluginJSON); err == nil {
		i.discoverFromPluginJSON(sourceDir, data, itemTypes, addItem)
	}

	// 6. Check for .codex-plugin/marketplace.json
	codexMarketplace := filepath.Join(sourceDir, ".codex-plugin", "marketplace.json")
	if data, err := os.ReadFile(codexMarketplace); err == nil {
		i.discoverFromMarketplace(sourceDir, data, itemTypes, addItem)
	}

	// 7. Scan plugins/<plugin-name>/<type>/ structure (legacy/marketplace)
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

// rootSkillName returns the skill name for a "bare" skill repo whose SKILL.md
// lives at the repository root (the whole repo IS the skill, with no
// skills/<name>/ wrapper). The name is taken from the frontmatter `name:`
// field, falling back to the source directory's base name. Returns "" when
// there is no root-level SKILL.md.
func rootSkillName(sourceDir string) string {
	data, err := os.ReadFile(filepath.Join(sourceDir, "SKILL.md"))
	if err != nil {
		return ""
	}
	if name := parseFrontmatterName(data); name != "" {
		return name
	}
	return filepath.Base(sourceDir)
}

// parseFrontmatterName extracts the `name:` value from a Markdown file's YAML
// frontmatter (the block delimited by leading and trailing `---` fences).
// Returns "" when there is no frontmatter or no name field.
func parseFrontmatterName(data []byte) string {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	rest := content[len("---"):]
	front, _, found := strings.Cut(rest, "\n---")
	if !found {
		return ""
	}
	var fm struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return ""
	}
	return strings.TrimSpace(fm.Name)
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

// discoverFromMarketplace parses marketplace.json and scans each plugin subdir
func (i *Installer) discoverFromMarketplace(sourceDir string, data []byte, itemTypes []string, addItem func(string)) {
	var marketplace marketplaceJSON
	if err := json.Unmarshal(data, &marketplace); err != nil {
		return
	}

	for _, plugin := range marketplace.Plugins {
		if plugin.Source == "" {
			continue
		}
		pluginDir := filepath.Join(sourceDir, strings.TrimPrefix(plugin.Source, "./"))

		// Scan standard item dirs within the plugin subdir
		for _, itemType := range itemTypes {
			typeDir := filepath.Join(pluginDir, itemType)
			i.scanItemDir(typeDir, itemType, addItem)
		}

		// Also check for plugin.json within the plugin subdir
		pJSON := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
		if pData, err := os.ReadFile(pJSON); err == nil {
			i.discoverFromPluginJSON(pluginDir, pData, itemTypes, addItem)
		}
	}
}

// findInMarketplace searches marketplace plugin subdirs for an item.
// Checks both .claude-plugin/ and .codex-plugin/ marketplace files.
func (i *Installer) findInMarketplace(sourceDir, itemType, itemName string) string {
	marketplaceFiles := []string{
		filepath.Join(sourceDir, ".claude-plugin", "marketplace.json"),
		filepath.Join(sourceDir, ".codex-plugin", "marketplace.json"),
	}

	for _, marketplaceFile := range marketplaceFiles {
		data, err := os.ReadFile(marketplaceFile)
		if err != nil {
			continue
		}

		var marketplace marketplaceJSON
		if err := json.Unmarshal(data, &marketplace); err != nil {
			continue
		}

		for _, plugin := range marketplace.Plugins {
			if plugin.Source == "" {
				continue
			}
			pluginDir := filepath.Join(sourceDir, strings.TrimPrefix(plugin.Source, "./"))
			candidate := filepath.Join(pluginDir, itemType, itemName)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return ""
}

// CopyTree copies a file or directory tree from src to dst.
// If src is a directory, it copies recursively. If src is a file, it copies the single file.
func CopyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return CopyDir(src, dst)
	}
	return CopyFileItem(src, dst)
}

// CopyDir copies a directory recursively from src to dst, resolving symlinks.
func CopyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		// Never copy version-control metadata into a hub item (matters when
		// the source is a whole repo, e.g. a bare root-level SKILL.md skill).
		if entry.Name() == ".git" {
			continue
		}

		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Use os.Stat to resolve symlinks and get actual type
		info, err := os.Stat(srcPath)
		if err != nil {
			return err
		}

		if info.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := CopyFileItem(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyFileItem copies a single file from src to dst, preserving permissions.
func CopyFileItem(src, dst string) error {
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
