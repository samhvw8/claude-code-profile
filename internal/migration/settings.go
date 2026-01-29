package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samhoang/ccp/internal/config"
)

// SettingsHookEntry represents a single hook command in settings.json
type SettingsHookEntry struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
	Type    string `json:"type"`
}

// SettingsHook represents a hook configuration with optional matcher
type SettingsHook struct {
	Hooks   []SettingsHookEntry `json:"hooks"`
	Matcher string              `json:"matcher,omitempty"`
}

// SettingsFile represents the settings.json structure (partial)
type SettingsFile struct {
	Hooks map[string][]SettingsHook `json:"hooks,omitempty"`
	// Store raw JSON for other fields to preserve them
	raw map[string]json.RawMessage
}

// ExtractedHook represents a parsed hook with extracted path information
type ExtractedHook struct {
	HookType    config.HookType // SessionStart, PreToolUse, etc.
	Command     string          // Original command
	FilePath    string          // Extracted file path (may be empty for inline)
	Interpreter string          // Interpreter prefix (e.g., "bash", "node")
	IsInside    bool            // true if path inside ~/.claude
	IsInline    bool            // true if command is inline script (no file)
	Matcher     string          // Optional matcher
	Timeout     int
	EntryIndex  int // Index within the parent hook array
	HookIndex   int // Index of the hook entry
}

// ParseSettings reads and parses settings.json
func ParseSettings(path string) (*SettingsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var settings SettingsFile
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	// Also keep raw for later rewriting
	if err := json.Unmarshal(data, &settings.raw); err != nil {
		settings.raw = make(map[string]json.RawMessage)
	}

	return &settings, nil
}

// ExtractHookPaths extracts all hooks from settings and classifies their paths
func ExtractHookPaths(settings *SettingsFile, claudeDir string) []ExtractedHook {
	var hooks []ExtractedHook

	if settings.Hooks == nil {
		return hooks
	}

	for hookTypeName, hookArray := range settings.Hooks {
		hookType := config.HookType(hookTypeName)

		for entryIdx, hook := range hookArray {
			for hookIdx, entry := range hook.Hooks {
				extracted := ExtractedHook{
					HookType:   hookType,
					Command:    entry.Command,
					Matcher:    hook.Matcher,
					Timeout:    entry.Timeout,
					EntryIndex: entryIdx,
					HookIndex:  hookIdx,
				}

				// Extract file path and interpreter from command
				interpreter, filePath := extractInterpreterAndPath(entry.Command)
				extracted.Interpreter = interpreter
				if filePath == "" {
					extracted.IsInline = true
				} else {
					extracted.FilePath = filePath
					extracted.IsInside = isInsideDir(filePath, claudeDir)
				}

				hooks = append(hooks, extracted)
			}
		}
	}

	return hooks
}

// extractInterpreterAndPath extracts the interpreter prefix and file path from a hook command
// Returns: interpreter (e.g., "bash", "node"), filePath (expanded path)
func extractInterpreterAndPath(command string) (interpreter, filePath string) {
	// Common interpreter prefixes
	prefixes := []string{
		"bash ", "sh ", "/bin/bash ", "/bin/sh ",
		"node ", "python ", "python3 ", "ruby ",
		"/usr/bin/node ", "/usr/bin/python ", "/usr/bin/python3 ",
	}

	// Check for interpreter prefix
	cmdLower := strings.ToLower(command)
	remainingCmd := command
	for _, prefix := range prefixes {
		if strings.HasPrefix(cmdLower, prefix) {
			// Extract just the interpreter name (e.g., "bash" from "/bin/bash ")
			interpreter = strings.TrimSpace(prefix)
			if strings.Contains(interpreter, "/") {
				interpreter = filepath.Base(interpreter)
			}
			remainingCmd = command[len(prefix):]
			break
		}
	}

	// Expand $HOME and ~ in the remaining command
	remainingCmd = expandHome(remainingCmd)

	// Handle shell builtins and inline scripts
	if isInlineScript(remainingCmd) {
		return interpreter, ""
	}

	// Extract the first path-like token
	tokens := strings.Fields(remainingCmd)
	if len(tokens) == 0 {
		return interpreter, ""
	}

	path := tokens[0]

	// Validate it looks like a file path
	if !strings.Contains(path, "/") && !strings.HasPrefix(path, ".") {
		return interpreter, ""
	}

	return interpreter, path
}

// extractFilePath extracts the file path from a hook command (legacy, calls extractInterpreterAndPath)
// Handles: bash /path/to/script.sh, node $HOME/.claude/script.js, etc.
func extractFilePath(command string) string {
	_, filePath := extractInterpreterAndPath(command)
	return filePath
}

// expandHome expands $HOME and ~ to the actual home directory
func expandHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	// Replace $HOME
	path = strings.ReplaceAll(path, "$HOME", home)

	// Replace ~ at the start
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, path[2:])
	} else if path == "~" {
		path = home
	}

	return path
}

// isInsideDir checks if a path is inside the given directory
func isInsideDir(path, dir string) bool {
	// Expand and clean paths
	path = expandHome(path)
	dir = expandHome(dir)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}

	// Normalize with trailing separator
	if !strings.HasSuffix(absDir, string(filepath.Separator)) {
		absDir += string(filepath.Separator)
	}

	return strings.HasPrefix(absPath, absDir)
}

// isInlineScript checks if the command is an inline script rather than a file reference
func isInlineScript(command string) bool {
	// Patterns that indicate inline scripts
	inlinePatterns := []string{
		"<<", // heredoc
		"; ",
		" && ",
		" || ",
		"echo ",
		"cat ",
		"printf ",
	}

	for _, pattern := range inlinePatterns {
		if strings.Contains(command, pattern) {
			return true
		}
	}

	// Check for common shell constructs
	shellConstructs := regexp.MustCompile(`\$\([^)]+\)|` + // $(...) command substitution
		"`[^`]+`|" + // `...` command substitution
		`\$\{[^}]+\}`) // ${...} variable with modifiers

	return shellConstructs.MatchString(command)
}

// GenerateHookName generates a unique name for a hook based on its file path or command
func GenerateHookName(hook ExtractedHook) string {
	if hook.FilePath != "" {
		// Use the file name without extension
		base := filepath.Base(hook.FilePath)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)
		return sanitizeName(name)
	}

	// For inline hooks, generate from hook type and matcher
	name := strings.ToLower(string(hook.HookType))
	if hook.Matcher != "" {
		name = name + "-" + strings.ToLower(hook.Matcher)
	}
	return sanitizeName(name)
}

// sanitizeName converts a string to a valid directory name
func sanitizeName(name string) string {
	// Replace spaces and underscores with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove any characters that aren't alphanumeric or hyphens
	reg := regexp.MustCompile(`[^a-zA-Z0-9-]`)
	name = reg.ReplaceAllString(name, "")

	// Convert to lowercase and collapse multiple hyphens
	name = strings.ToLower(name)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	if name == "" {
		name = "hook"
	}

	return name
}
