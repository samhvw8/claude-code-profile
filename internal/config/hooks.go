package config

// HookType represents the type of hook event
type HookType string

const (
	HookSessionStart      HookType = "SessionStart"
	HookUserPromptSubmit  HookType = "UserPromptSubmit"
	HookPreToolUse        HookType = "PreToolUse"
	HookPostToolUse       HookType = "PostToolUse"
	HookStop              HookType = "Stop"
	HookSubagentStop      HookType = "SubagentStop"
)

// AllHookTypes returns all valid hook types
func AllHookTypes() []HookType {
	return []HookType{
		HookSessionStart,
		HookUserPromptSubmit,
		HookPreToolUse,
		HookPostToolUse,
		HookStop,
		HookSubagentStop,
	}
}

// HookConfig represents the configuration for a hook (legacy YAML format)
type HookConfig struct {
	// Name is the hook file name (without extension)
	Name string `yaml:"name" json:"name"`

	// Type is the hook event type (SessionStart, UserPromptSubmit, etc.)
	Type HookType `yaml:"type" json:"type"`

	// Command is the command to execute (defaults to the hook file path)
	Command string `yaml:"command,omitempty" json:"command,omitempty"`

	// Timeout in seconds (default: 60)
	Timeout int `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// Matcher for tool-specific hooks (PreToolUse, PostToolUse)
	Matcher string `yaml:"matcher,omitempty" json:"matcher,omitempty"`
}

// DefaultHookTimeout returns the default timeout for hooks
func DefaultHookTimeout() int {
	return 60
}

// HooksJSON represents the official Claude Code hooks.json format
// Used in plugins at hooks/hooks.json
type HooksJSON struct {
	Hooks map[HookType][]HookEntry `json:"hooks"`
}

// HookEntry represents a single hook entry with optional matcher
type HookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []HookCommand `json:"hooks"`
}

// HookCommand represents a single hook command
type HookCommand struct {
	Type    string `json:"type"`              // Always "command"
	Command string `json:"command"`           // Path to script or inline command
	Timeout int    `json:"timeout,omitempty"` // Timeout in seconds
}

// NewHooksJSON creates an empty HooksJSON structure
func NewHooksJSON() *HooksJSON {
	return &HooksJSON{
		Hooks: make(map[HookType][]HookEntry),
	}
}

// AddHook adds a hook command to the specified event type
func (h *HooksJSON) AddHook(hookType HookType, matcher string, command string, timeout int) {
	entry := HookEntry{
		Matcher: matcher,
		Hooks: []HookCommand{
			{
				Type:    "command",
				Command: command,
				Timeout: timeout,
			},
		},
	}
	h.Hooks[hookType] = append(h.Hooks[hookType], entry)
}

// GetHooks returns all hook entries for a given event type
func (h *HooksJSON) GetHooks(hookType HookType) []HookEntry {
	return h.Hooks[hookType]
}

