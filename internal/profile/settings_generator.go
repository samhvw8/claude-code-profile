package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/hub"
)

// GenerateSettingsHooks generates the hooks section for settings.json from linked hub hooks
// Uses $HOME-based absolute paths for portability
func GenerateSettingsHooks(paths *config.Paths, profileDir string, manifest *Manifest) (map[config.HookType][]config.SettingsHookEntry, error) {
	hooks := make(map[config.HookType][]config.SettingsHookEntry)
	profileHooksDir := filepath.Join(profileDir, "hooks")

	for _, hookName := range manifest.Hub.Hooks {
		hookDir := filepath.Join(profileHooksDir, hookName)

		// Try hooks.json first (official format)
		hooksJSON, err := hub.GetHooksJSON(hookDir)
		if err == nil && hooksJSON != nil {
			processHooksJSON(hooksJSON, hookDir, hooks)
			continue
		}

		// Fall back to hook.yaml (legacy format via GetHookManifest)
		hookManifest, err := hub.GetHookManifest(paths.HubDir, hookName)
		if err != nil {
			// Skip hooks that don't have a manifest
			continue
		}

		processLegacyHook(hookManifest, profileHooksDir, hookName, hooks)
	}

	// Legacy hooks: old-style manifests list hooks in manifest.Hooks rather
	// than linking hub hooks. Generate them the way SyncHooksFromManifest does,
	// so no rewrite drops them.
	for hookType, entries := range legacyManifestHooks(profileDir, manifest) {
		hooks[hookType] = append(hooks[hookType], entries...)
	}

	// Bundle hooks: bundle members are tracked only by bundle name (never in
	// Hub.Hooks), so resolve each linked bundle and process its hook members.
	// The members are symlinked into profile/hooks/<member>, so the same
	// hooks.json path resolution applies.
	for _, bundleName := range manifest.Hub.Bundles {
		bundle, err := hub.LoadBundle(paths.BundlesDir(), bundleName)
		if err != nil {
			continue
		}
		for _, hookName := range bundle.Members.Hooks {
			hookDir := filepath.Join(profileHooksDir, hookName)
			if hooksJSON, err := hub.GetHooksJSON(hookDir); err == nil && hooksJSON != nil {
				processHooksJSON(hooksJSON, hookDir, hooks)
			}
		}
	}

	return hooks, nil
}

// processHooksJSON processes hooks.json format entries
func processHooksJSON(hooksJSON *config.HooksJSON, hookDir string, hooks map[config.HookType][]config.SettingsHookEntry) {
	for hookType, entries := range hooksJSON.Hooks {
		for _, hookEntry := range entries {
			for _, cmd := range hookEntry.Hooks {
				command := resolvePluginRootPath(cmd.Command, hookDir)
				timeout := cmd.Timeout
				if timeout == 0 {
					timeout = config.DefaultHookTimeout()
				}

				entry := config.NewSettingsHookEntry(hookEntry.Matcher, command, timeout)
				// Preserve the original type if specified
				if cmd.Type != "" {
					entry.Hooks[0].Type = cmd.Type
				}
				hooks[hookType] = append(hooks[hookType], entry)
			}
		}
	}
}

// processLegacyHook processes legacy hook.yaml format
func processLegacyHook(hookManifest *hub.HookManifest, profileHooksDir, hookName string, hooks map[config.HookType][]config.SettingsHookEntry) {
	command := buildLegacyCommand(hookManifest, profileHooksDir, hookName)

	// Prepend interpreter if specified
	if hookManifest.Interpreter != "" && hookManifest.Inline == "" {
		command = hookManifest.Interpreter + " " + command
	}

	timeout := hookManifest.Timeout
	if timeout == 0 {
		timeout = config.DefaultHookTimeout()
	}

	entry := config.NewSettingsHookEntry(hookManifest.Matcher, command, timeout)
	hookType := hookManifest.Type
	hooks[hookType] = append(hooks[hookType], entry)
}

// resolvePluginRootPath replaces ${CLAUDE_PLUGIN_ROOT} with the portable hook directory path
func resolvePluginRootPath(command, hookDir string) string {
	if !strings.Contains(command, "${CLAUDE_PLUGIN_ROOT}") {
		return command
	}
	portablePath := config.ToPortablePath(hookDir)
	return strings.ReplaceAll(command, "${CLAUDE_PLUGIN_ROOT}", portablePath)
}

// buildLegacyCommand builds the command string for legacy hook.yaml format
func buildLegacyCommand(hookManifest *hub.HookManifest, profileHooksDir, hookName string) string {
	if hookManifest.Inline != "" {
		return hookManifest.Inline
	}
	if len(hookManifest.Command) > 0 && hookManifest.Command[0] == '/' {
		// Absolute path - use as-is
		return hookManifest.Command
	}
	// Build portable path using profile's hooks directory
	absPath := filepath.Join(profileHooksDir, hookName, hookManifest.Command)
	return config.ToPortablePath(absPath)
}

// snapshotPath is where ccp records what it last wrote to a profile's
// settings.json, so later runs can tell edits made since then (by hand or by
// Claude Code) apart from changes to the template, fragment or hooks. It lives
// under ~/.ccp/state, outside the profile, because it describes this machine's
// last write: a copy carried to another machine by dotfiles would misjudge edits.
func snapshotPath(paths *config.Paths, profileDir string) string {
	return filepath.Join(paths.CcpDir, "state", "settings", filepath.Base(profileDir)+".json")
}

// MoveSettingsSnapshot follows a profile rename; RemoveSettingsSnapshot a delete.
func MoveSettingsSnapshot(paths *config.Paths, oldDir, newDir string) error {
	err := os.Rename(snapshotPath(paths, oldDir), snapshotPath(paths, newDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func RemoveSettingsSnapshot(paths *config.Paths, profileDir string) error {
	err := os.Remove(snapshotPath(paths, profileDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// buildSettings returns the settings.json ccp would write now and the part of
// it that ccp generated. They differ by hooks that ccp did not create — added
// by hand or with Claude Code's /hooks — which are carried over from disk. An
// unreadable settings.json contributes nothing.
func buildSettings(paths *config.Paths, profileDir string, manifest *Manifest) (final, generated map[string]interface{}, err error) {
	raw, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		return nil, nil, err
	}
	if generated, err = toGeneric(raw); err != nil {
		return nil, nil, err
	}
	if final, err = toGeneric(raw); err != nil {
		return nil, nil, err
	}
	current, _ := readSettingsMap(profileDir)
	setHooks(final, mergeForeignHooks(hooksOf(final), foreignHooks(paths, profileDir, manifest, current)))
	return final, generated, nil
}

// PreviewSettings generates what settings.json would contain without writing it.
func PreviewSettings(paths *config.Paths, profileDir string, manifest *Manifest) ([]byte, error) {
	final, _, err := buildSettings(paths, profileDir, manifest)
	if err != nil {
		return nil, err
	}
	return marshalJSON(final)
}

// SettingsChanged returns true if the generated settings differ from the current settings.json.
func SettingsChanged(paths *config.Paths, profileDir string, manifest *Manifest) (bool, error) {
	settingsPath := filepath.Join(profileDir, "settings.json")

	newData, err := PreviewSettings(paths, profileDir, manifest)
	if err != nil {
		return false, err
	}

	oldData, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}

	return string(oldData) != string(newData), nil
}

// ApplySettings brings settings.json in line with the manifest without losing
// edits. When settings.json holds non-hook edits made since ccp last wrote it
// (UncapturedSettings), only the hooks key is rewritten and those keys are
// returned; template and fragment changes wait until the edits are captured or
// discarded. With force, or when there are none, the whole file is regenerated.
// Hooks ccp did not create survive either way.
func ApplySettings(paths *config.Paths, profileDir string, manifest *Manifest, force bool) ([]string, error) {
	if !force {
		uncaptured, err := UncapturedSettings(paths, profileDir, manifest)
		if err != nil {
			return nil, err
		}
		if len(uncaptured) > 0 {
			return uncaptured, SyncHooks(paths, profileDir, manifest)
		}
	}
	return nil, RegenerateSettings(paths, profileDir, manifest)
}

// UncapturedSettings returns the top-level non-hook keys of settings.json that
// regenerating would lose: edited since ccp last wrote the file (changed, added
// or deleted relative to the snapshot) and not reproduced by the template and
// fragment. A captured edit is reproduced by the fragment, so it no longer
// counts. Without a snapshot (written by an older ccp), any on-disk key that
// regeneration would change counts. A null value is treated as absent: ccp
// never writes nulls, and Claude Code reads one as unset.
func UncapturedSettings(paths *config.Paths, profileDir string, manifest *Manifest) ([]string, error) {
	current, err := readSettingsMap(profileDir)
	if err != nil || current == nil {
		return nil, err
	}
	_, generated, err := buildSettings(paths, profileDir, manifest)
	if err != nil {
		return nil, err
	}
	snapshot := loadSnapshot(paths, profileDir)
	current, generated, snapshot = stripNullsMap(current), stripNullsMap(generated), stripNullsMap(snapshot)
	delete(current, "hooks")
	delete(generated, "hooks")

	var keys []string
	for k, v := range current {
		if reflect.DeepEqual(generated[k], v) {
			continue // regeneration writes the same value
		}
		if snapshot != nil && reflect.DeepEqual(snapshot[k], v) {
			continue // unchanged since ccp wrote it: an input changed, not the file
		}
		keys = append(keys, k)
	}
	for k := range snapshot {
		_, onDisk := current[k]
		_, regenerated := generated[k]
		if k != "hooks" && !onDisk && regenerated {
			keys = append(keys, k) // deleted on disk; regeneration would restore it
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// SyncHooks rewrites only the hooks key of settings.json from the profile's
// linked hub hooks, leaving every other key — and hooks ccp did not create — as
// they are on disk.
func SyncHooks(paths *config.Paths, profileDir string, manifest *Manifest) error {
	current, err := readSettingsMap(profileDir)
	if err != nil {
		return err
	}
	if current == nil {
		return RegenerateSettings(paths, profileDir, manifest)
	}
	final, generated, err := buildSettings(paths, profileDir, manifest)
	if err != nil {
		return err
	}
	setHooks(current, hooksOf(final))
	if err := writeJSONFile(filepath.Join(profileDir, "settings.json"), current); err != nil {
		return err
	}

	// ccp now owns these hooks. Without a snapshot (an older ccp wrote this
	// profile), record the full generation: it is what ccp would have written,
	// so edits on disk still differ from it and stay protected.
	snapshot := loadSnapshot(paths, profileDir)
	if snapshot == nil {
		snapshot = generated
	}
	setHooks(snapshot, hooksOf(generated))
	return saveSnapshot(paths, profileDir, snapshot)
}

// foreignHooks returns the hook entries in settings.json that ccp did not
// generate. An entry is ccp's when it points into the profile's hooks
// directory or the hub's hooks or bundles (where 'ccp init' and migrations
// point), when ccp wrote
// it last time (per the snapshot), or when some hub hook would generate it —
// which recognises ccp's own entries on profiles that have no snapshot yet.
func foreignHooks(paths *config.Paths, profileDir string, manifest *Manifest, current map[string]interface{}) map[string][]interface{} {
	written := hooksOf(loadSnapshot(paths, profileDir))
	known := knownHookEntries(paths, profileDir, manifest)
	var dirs []string
	for _, dir := range []string{filepath.Join(profileDir, "hooks"), paths.HubItemDir(config.HubHooks), paths.BundlesDir()} {
		dir += string(filepath.Separator)
		dirs = append(dirs, dir, config.ToPortablePath(dir))
	}

	foreign := make(map[string][]interface{})
	for hookType, entries := range hooksOf(current) {
		list, _ := entries.([]interface{})
	entry:
		for _, entry := range list {
			data, _ := marshalJSON(entry)
			for _, dir := range dirs {
				if strings.Contains(string(data), dir) {
					continue entry
				}
			}
			if containsEntry(written[hookType], entry) || containsEntry(known[hookType], entry) {
				continue
			}
			foreign[hookType] = append(foreign[hookType], entry)
		}
	}
	return foreign
}

// knownHookEntries returns every entry a hub hook, bundle hook or legacy
// manifest hook would generate for this profile, linked or not.
func knownHookEntries(paths *config.Paths, profileDir string, manifest *Manifest) map[string]interface{} {
	hooks := make(map[config.HookType][]config.SettingsHookEntry)
	profileHooksDir := filepath.Join(profileDir, "hooks")
	entries, _ := os.ReadDir(paths.HubItemDir(config.HubHooks))
	for _, e := range entries {
		if hj, err := hub.GetHooksJSON(filepath.Join(paths.HubItemDir(config.HubHooks), e.Name())); err == nil && hj != nil {
			processHooksJSON(hj, filepath.Join(profileHooksDir, e.Name()), hooks)
		} else if hm, err := hub.GetHookManifest(paths.HubDir, e.Name()); err == nil {
			processLegacyHook(hm, profileHooksDir, e.Name(), hooks)
		}
	}
	bundles, _ := os.ReadDir(paths.BundlesDir())
	for _, b := range bundles {
		bundle, err := hub.LoadBundle(paths.BundlesDir(), b.Name())
		if err != nil {
			continue
		}
		for _, name := range bundle.Members.Hooks {
			if hj, err := hub.GetHooksJSON(filepath.Join(paths.BundleDir(b.Name()), "hooks", name)); err == nil && hj != nil {
				processHooksJSON(hj, filepath.Join(profileHooksDir, name), hooks)
			}
		}
	}
	for hookType, list := range legacyManifestHooks(profileDir, manifest) {
		hooks[hookType] = append(hooks[hookType], list...)
	}
	generic, _ := toGeneric(map[string]interface{}{"hooks": hooks})
	return hooksOf(generic)
}

// mergeForeignHooks appends foreign hook entries after the generated ones,
// skipping any the generation already produced.
func mergeForeignHooks(hooks map[string]interface{}, foreign map[string][]interface{}) map[string]interface{} {
	if len(foreign) == 0 {
		return hooks
	}
	if hooks == nil {
		hooks = make(map[string]interface{})
	}
	for hookType, entries := range foreign {
		for _, entry := range entries {
			if !containsEntry(hooks[hookType], entry) {
				list, _ := hooks[hookType].([]interface{})
				hooks[hookType] = append(list, entry)
			}
		}
	}
	return hooks
}

func containsEntry(list interface{}, entry interface{}) bool {
	entries, _ := list.([]interface{})
	for _, e := range entries {
		if reflect.DeepEqual(e, entry) {
			return true
		}
	}
	return false
}

// hooksOf returns the hooks object of a generic settings map, or nil.
func hooksOf(settings map[string]interface{}) map[string]interface{} {
	hooks, _ := settings["hooks"].(map[string]interface{})
	return hooks
}

// setHooks stores hooks in settings, removing the key when there are none.
func setHooks(settings map[string]interface{}, hooks map[string]interface{}) {
	if len(hooks) == 0 {
		delete(settings, "hooks")
		return
	}
	settings["hooks"] = hooks
}

// stripNullsMap returns m without null values, at any depth.
func stripNullsMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if v == nil {
			continue
		}
		if sub, ok := v.(map[string]interface{}); ok {
			v = stripNullsMap(sub)
		}
		out[k] = v
	}
	return out
}

// toGeneric round-trips a value through JSON so it compares equal to what
// reading settings.json back would produce.
func toGeneric(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = make(map[string]interface{})
	}
	return m, nil
}

// loadSnapshot returns what ccp last wrote, or nil. A missing or unreadable
// snapshot only costs precision (ccp falls back to comparing with a fresh
// generation), so it is never fatal; a corrupt one is reported and replaced on
// the next write.
func loadSnapshot(paths *config.Paths, profileDir string) map[string]interface{} {
	path := snapshotPath(paths, profileDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: ignoring %s: %v\n", path, err)
		}
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: ignoring corrupt %s: %v\n", path, err)
		return nil
	}
	if m == nil {
		m = make(map[string]interface{})
	}
	return m
}

func saveSnapshot(paths *config.Paths, profileDir string, snapshot map[string]interface{}) error {
	path := snapshotPath(paths, profileDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeJSONFile(path, snapshot)
}

// readSettingsMap reads settings.json as a generic map; nil when it does not exist.
func readSettingsMap(profileDir string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("settings.json is not valid JSON (%v): fix it, or regenerate it with 'ccp profile sync --force', which keeps the old file as settings.json.bak", err)
	}
	if m == nil {
		m = make(map[string]interface{})
	}
	return m, nil
}

// RegenerateSettings regenerates settings.json from the template, fragment and
// hub hooks, keeps hooks ccp did not create, and records what it generated. An
// unreadable settings.json is kept as settings.json.bak, not parsed.
func RegenerateSettings(paths *config.Paths, profileDir string, manifest *Manifest) error {
	settingsPath := filepath.Join(profileDir, "settings.json")
	if _, err := readSettingsMap(profileDir); err != nil {
		if data, readErr := os.ReadFile(settingsPath); readErr == nil {
			if err := os.WriteFile(settingsPath+".bak", data, 0644); err != nil {
				return fmt.Errorf("could not back up unreadable settings.json: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Warning: settings.json was not valid JSON; kept it as settings.json.bak\n")
		}
	}
	final, generated, err := buildSettings(paths, profileDir, manifest)
	if err != nil {
		return err
	}
	if err := writeJSONFile(settingsPath, final); err != nil {
		return err
	}
	return saveSnapshot(paths, profileDir, generated)
}

// marshalJSON serializes data as pretty JSON without HTML escaping
func marshalJSON(data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeJSONFile writes data as JSON without HTML escaping
func writeJSONFile(path string, data interface{}) error {
	b, err := marshalJSON(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// SettingsHaveHooks reports whether the profile's settings.json has a hooks key,
// which may need removing once its last hook is unlinked.
func SettingsHaveHooks(profileDir string) bool {
	current, err := readSettingsMap(profileDir)
	return err == nil && hooksOf(current) != nil
}

// CountHookEntries returns how many settings.json hook entries the profile's
// hub hooks and bundles generate.
func CountHookEntries(paths *config.Paths, profileDir string, manifest *Manifest) int {
	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		return 0
	}
	n := 0
	for _, entries := range hooks {
		n += len(entries)
	}
	return n
}
