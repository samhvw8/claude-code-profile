package config

// Settings represents the Claude Code settings.json structure
type Settings struct {
	Hooks map[HookType][]SettingsHookEntry `json:"hooks,omitempty"`
}

// SettingsHookEntry represents a hook entry in settings.json
type SettingsHookEntry struct {
	Matcher string                `json:"matcher,omitempty"`
	Hooks   []SettingsHookCommand `json:"hooks"`
}

// SettingsHookCommand represents a command in settings.json
type SettingsHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// NewSettings creates an empty Settings structure
func NewSettings() *Settings {
	return &Settings{
		Hooks: make(map[HookType][]SettingsHookEntry),
	}
}

// AddHookEntry adds a hook entry to the settings
func (s *Settings) AddHookEntry(hookType HookType, entry SettingsHookEntry) {
	s.Hooks[hookType] = append(s.Hooks[hookType], entry)
}

// NewSettingsHookEntry creates a hook entry for settings.json
func NewSettingsHookEntry(matcher, command string, timeout int) SettingsHookEntry {
	return SettingsHookEntry{
		Matcher: matcher,
		Hooks: []SettingsHookCommand{{
			Type:    "command",
			Command: command,
			Timeout: timeout,
		}},
	}
}
