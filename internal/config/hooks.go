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

// HookConfig represents the configuration for a hook
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
