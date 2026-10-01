package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// opencode reads ~/.claude/CLAUDE.md and ~/.claude/skills/ itself, and takes
// rules through the instructions list in its opencode.json, so ccp mirrors
// only what it cannot read: agents and commands. Their frontmatter differs
// from Claude Code's (model IDs, tool lists), so ccp writes converted copies
// rather than links, and records what it wrote so it only ever replaces or
// removes its own files.

// OpencodeConfigDir returns opencode's global config directory.
func OpencodeConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "opencode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "opencode")
}

// opencodeState records the files ccp wrote into opencode's config directory,
// relative to it, with the hash of what was written.
type opencodeState struct {
	Files map[string]string `json:"files"`
}

func opencodeStatePath(paths *config.Paths) string {
	return filepath.Join(paths.CcpDir, "state", "opencode.json")
}

func loadOpencodeState(paths *config.Paths) (*opencodeState, error) {
	state := &opencodeState{Files: map[string]string{}}
	data, err := os.ReadFile(opencodeStatePath(paths))
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", opencodeStatePath(paths), err)
	}
	if state.Files == nil {
		state.Files = map[string]string{}
	}
	return state, nil
}

func saveOpencodeState(paths *config.Paths, state *opencodeState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return ompWriteFileAtomic(opencodeStatePath(paths), string(data)+"\n")
}

// OpencodeOptedIn reports whether ccp has synced to opencode before: the test
// for following automatically on a profile switch, link or bootstrap.
func OpencodeOptedIn(paths *config.Paths) bool {
	_, err := os.Stat(opencodeStatePath(paths))
	return err == nil
}

// OpencodeResult reports what a sync did, as paths relative to opencode's
// config directory.
type OpencodeResult struct {
	Written  []string // created or updated
	Removed  []string // ccp's files no longer in the profile
	Edited   []string // ccp's files changed by hand since: left alone
	Occupied []string // a file ccp did not write sits where a copy would go
	Skipped  []string // profile items that could not be converted, with the reason
	Hooks    []string // linked hooks, which opencode runs only as plugins
}

// OpencodeSync mirrors the profile's agents and commands into opencode.
func OpencodeSync(paths *config.Paths, p *Profile) (OpencodeResult, error) {
	var result OpencodeResult
	dir := OpencodeConfigDir()
	if dir == "" {
		return result, fmt.Errorf("cannot locate opencode's config directory")
	}
	state, err := loadOpencodeState(paths)
	if err != nil {
		return result, err
	}

	desired := map[string][]byte{}
	for _, kind := range []struct {
		itemType config.HubItemType
		target   string
		convert  func([]byte) ([]byte, error)
	}{
		{config.HubAgents, "agent", convertOpencodeAgent},
		{config.HubCommands, "command", convertOpencodeCommand},
	} {
		entries, _ := os.ReadDir(filepath.Join(p.Path, string(kind.itemType)))
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			ref := string(kind.itemType) + "/" + e.Name()
			data, err := os.ReadFile(filepath.Join(p.Path, string(kind.itemType), e.Name()))
			if err != nil {
				result.Skipped = append(result.Skipped, ref+": "+err.Error())
				continue
			}
			out, err := kind.convert(data)
			if err != nil {
				result.Skipped = append(result.Skipped, ref+": "+err.Error())
				continue
			}
			desired[kind.target+"/"+e.Name()] = out
		}
	}

	for _, rel := range sortedKeys(desired) {
		content := desired[rel]
		path := filepath.Join(dir, rel)
		onDisk, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
		case err != nil:
			return result, err
		case string(onDisk) == string(content):
			state.Files[rel] = hashOf(content) // already right, possibly adopted
			continue
		case state.Files[rel] == "":
			result.Occupied = append(result.Occupied, rel)
			continue
		case state.Files[rel] != hashOf(onDisk):
			result.Edited = append(result.Edited, rel)
			continue
		}
		if err := ompWriteFileAtomic(path, string(content)); err != nil {
			return result, err
		}
		state.Files[rel] = hashOf(content)
		result.Written = append(result.Written, rel)
	}

	for _, rel := range sortedKeys(state.Files) {
		if _, ok := desired[rel]; ok {
			continue
		}
		path := filepath.Join(dir, rel)
		onDisk, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
		case err != nil:
			return result, err
		case hashOf(onDisk) != state.Files[rel]:
			result.Edited = append(result.Edited, rel) // edited by hand: no longer ccp's to remove
		default:
			if err := os.Remove(path); err != nil {
				return result, err
			}
			result.Removed = append(result.Removed, rel)
		}
		delete(state.Files, rel)
	}

	result.Hooks = append(result.Hooks, p.Manifest.Hub.Hooks...)
	return result, saveOpencodeState(paths, state)
}

// OpencodeFollowProfile re-syncs after the active profile changed, once the
// user has opted in by running 'ccp opencode sync'.
func OpencodeFollowProfile(paths *config.Paths, p *Profile) (OpencodeResult, bool, error) {
	if !OpencodeOptedIn(paths) {
		return OpencodeResult{}, false, nil
	}
	result, err := OpencodeSync(paths, p)
	return result, true, err
}

// claudeToOpencodeTools maps Claude Code tool names to opencode's.
var claudeToOpencodeTools = map[string]string{
	"Read": "read", "Write": "write", "Edit": "edit", "MultiEdit": "edit",
	"Bash": "bash", "Glob": "glob", "Grep": "grep", "WebFetch": "webfetch",
	"WebSearch": "websearch", "Task": "task", "Agent": "task", "TodoWrite": "todowrite",
}

var (
	hexColor         = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	frontmatterFence = regexp.MustCompile(`(?m)^---[ \t]*\r?$`)
)

// convertOpencodeAgent turns a Claude Code agent file into an opencode one.
// The body is kept; the frontmatter keeps only fields opencode understands,
// because opencode passes unknown fields to the model provider as options.
func convertOpencodeAgent(data []byte) ([]byte, error) {
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	description := stringField(fm, "description")
	if description == "" {
		return nil, fmt.Errorf("no description, which opencode requires")
	}

	out := struct {
		Description string          `yaml:"description"`
		Mode        string          `yaml:"mode"`
		Model       string          `yaml:"model,omitempty"`
		Color       string          `yaml:"color,omitempty"`
		Tools       map[string]bool `yaml:"tools,omitempty"`
	}{Description: description, Mode: "subagent", Model: opencodeModel(fm), Color: stringField(fm, "color")}
	if !hexColor.MatchString(out.Color) {
		out.Color = "" // Claude Code color names are not opencode colors
	}

	// A Claude Code tool list is an allowlist: grant those, deny the rest.
	if tools := stringField(fm, "tools"); tools != "" {
		out.Tools = map[string]bool{}
		for _, name := range claudeToOpencodeTools {
			out.Tools[name] = false
		}
		for _, name := range strings.Split(tools, ",") {
			if oc, ok := claudeToOpencodeTools[strings.TrimSpace(name)]; ok {
				out.Tools[oc] = true
			}
		}
	}
	return joinFrontmatter(out, body)
}

// convertOpencodeCommand turns a Claude Code command file into an opencode one.
// Both use $ARGUMENTS in the body; allowed-tools and argument-hint have no
// opencode equivalent.
func convertOpencodeCommand(data []byte) ([]byte, error) {
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	out := struct {
		Description string `yaml:"description,omitempty"`
		Model       string `yaml:"model,omitempty"`
	}{Description: stringField(fm, "description"), Model: opencodeModel(fm)}
	return joinFrontmatter(out, body)
}

// opencodeModel keeps a model only when it is already an opencode
// provider/model ID; Claude Code aliases (inherit, opus, sonnet) are dropped so
// the agent uses opencode's configured model.
func opencodeModel(fm map[string]any) string {
	model := stringField(fm, "model")
	if strings.Contains(model, "/") {
		return model
	}
	return ""
}

func splitFrontmatter(data []byte) (map[string]any, string, error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	fm := map[string]any{}
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return fm, text, nil
	}
	rest := text[strings.Index(text, "\n")+1:]
	end := frontmatterFence.FindStringIndex(rest)
	if end == nil {
		return nil, "", fmt.Errorf("unterminated frontmatter")
	}
	if err := yaml.Unmarshal([]byte(rest[:end[0]]), &fm); err != nil {
		return nil, "", fmt.Errorf("frontmatter is not valid YAML: %w", err)
	}
	body := strings.TrimPrefix(rest[end[1]:], "\r")
	return fm, strings.TrimPrefix(body, "\n"), nil
}

func joinFrontmatter(fm any, body string) ([]byte, error) {
	head, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(head) + "---\n" + body), nil
}

func stringField(fm map[string]any, key string) string {
	s, _ := fm[key].(string)
	return strings.TrimSpace(s)
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
