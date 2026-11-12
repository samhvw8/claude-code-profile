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

func opencodeStatePath(paths *config.Paths) string {
	return filepath.Join(paths.CcpDir, "state", "opencode.json")
}

// copyState records the files ccp wrote into another harness's directory
// (opencode, agy), relative to it, with the hash of what was written. The
// state file's existence is that harness's opt-in marker.
type copyState struct {
	Files map[string]string `json:"files"`
}

func loadCopyState(path string) (*copyState, error) {
	state := &copyState{Files: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if state.Files == nil {
		state.Files = map[string]string{}
	}
	return state, nil
}

func saveCopyState(path string, state *copyState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, string(data)+"\n")
}

// copyReport is what syncCopies did, as paths relative to the harness's
// directory.
type copyReport struct {
	Written  []string // created or updated
	Removed  []string // ccp's files no longer wanted
	Edited   []string // ccp's files changed by hand since: left alone
	Occupied []string // a file ccp did not write sits where a copy would go
}

// syncCopies writes the desired converted copies into dir and removes ccp's
// copies that are no longer desired. It only ever replaces or removes a file
// that still holds exactly what ccp wrote; anything else is reported. frozen,
// when set, names paths this pass must neither write nor remove (their
// directory was refused); their state entries are kept for a later pass.
func syncCopies(dir string, state *copyState, desired map[string][]byte, frozen func(rel string) bool) (copyReport, error) {
	var r copyReport
	isFrozen := func(rel string) bool { return frozen != nil && frozen(rel) }

	for _, rel := range sortedKeys(desired) {
		if isFrozen(rel) {
			continue
		}
		content := desired[rel]
		path := filepath.Join(dir, rel)
		onDisk, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
		case err != nil:
			return r, err
		case string(onDisk) == string(content):
			state.Files[rel] = hashOf(content) // already right, possibly adopted
			continue
		case state.Files[rel] == "":
			r.Occupied = append(r.Occupied, rel)
			continue
		case state.Files[rel] != hashOf(onDisk):
			r.Edited = append(r.Edited, rel)
			continue
		}
		if err := writeFileAtomic(path, string(content)); err != nil {
			return r, err
		}
		state.Files[rel] = hashOf(content)
		r.Written = append(r.Written, rel)
	}

	for _, rel := range sortedKeys(state.Files) {
		if _, ok := desired[rel]; ok || isFrozen(rel) {
			continue
		}
		path := filepath.Join(dir, rel)
		onDisk, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
		case err != nil:
			return r, err
		case hashOf(onDisk) != state.Files[rel]:
			r.Edited = append(r.Edited, rel) // edited by hand: no longer ccp's to remove
		default:
			if err := os.Remove(path); err != nil {
				return r, err
			}
			r.Removed = append(r.Removed, rel)
		}
		delete(state.Files, rel)
	}
	return r, nil
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
	Style    []string // output-style carrier changes and notes
	// Instructions is the path opencode.json's "instructions" list must name
	// for the output style to load; empty when it already does.
	Instructions string
}

// opencodeStyleFile carries the profile's output style. opencode has no output
// styles, so the file reaches the model through the "instructions" list.
const opencodeStyleFile = "output-style.md"

// OpencodeSync mirrors the profile's agents, commands and output style into
// opencode.
func OpencodeSync(paths *config.Paths, p *Profile) (OpencodeResult, error) {
	var result OpencodeResult
	dir := OpencodeConfigDir()
	if dir == "" {
		return result, fmt.Errorf("cannot locate opencode's config directory")
	}
	state, err := loadCopyState(opencodeStatePath(paths))
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

	copies, err := syncCopies(dir, state, desired, nil)
	result.Written, result.Removed, result.Edited, result.Occupied = copies.Written, copies.Removed, copies.Edited, copies.Occupied
	if err != nil {
		return result, err
	}

	result.Hooks = append(result.Hooks, p.Manifest.Hub.Hooks...)
	if err := saveCopyState(opencodeStatePath(paths), state); err != nil {
		return result, err
	}

	// The output style is a carrier like omp's and pi's, owned through the
	// carrier ledger. Only a file ccp owns is worth pointing opencode at.
	stylePath := filepath.Join(dir, opencodeStyleFile)
	lines, owned, err := syncStyleCarriers(paths, p, carrierGroupOpencode, stylePath)
	result.Style = lines
	if err != nil {
		return result, err
	}
	if owned && !opencodeInstructionsName(dir, opencodeStyleFile) {
		result.Instructions = stylePath
	}
	return result, nil
}

// opencodeInstructionsName reports whether opencode's global config mentions a
// file. It only decides whether to print a hint, so a plain text search is
// enough: ccp never edits opencode.json.
func opencodeInstructionsName(dir, file string) bool {
	for _, name := range []string{"opencode.json", "opencode.jsonc"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil && strings.Contains(string(data), file) {
			return true
		}
	}
	return false
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
		for name := range strings.SplitSeq(tools, ",") {
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
	block, body, err := cutFrontmatter(data)
	if err != nil {
		return nil, "", err
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return nil, "", fmt.Errorf("frontmatter is not valid YAML: %w", err)
	}
	return fm, body, nil
}

// cutFrontmatter separates a markdown file's frontmatter block from its body.
// A file without frontmatter yields an empty block and the whole text.
func cutFrontmatter(data []byte) (block, body string, err error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return "", text, nil
	}
	rest := text[strings.Index(text, "\n")+1:]
	end := frontmatterFence.FindStringIndex(rest)
	if end == nil {
		return "", "", fmt.Errorf("unterminated frontmatter")
	}
	body = strings.TrimPrefix(rest[end[1]:], "\r")
	return rest[:end[0]], strings.TrimPrefix(body, "\n"), nil
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
