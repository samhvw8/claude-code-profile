package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/symlink"
)

// Antigravity CLI (agy) reads its global config from ~/.gemini/config. It does
// not read ~/.claude, so ccp mirrors the profile there:
//
//   - AGENTS.md links to the profile's CLAUDE.md: agy loads it always-on, with
//     no frontmatter, which is what CLAUDE.md is.
//   - GEMINI.md carries the output style, through the carrier ledger: agy also
//     loads it always-on, and nothing else of ccp's competes for it.
//   - rules/ and agents/ get converted copies: agy loads a rule only with its
//     own trigger frontmatter, and an agent only with its own tool names, so a
//     link to the Claude file would be a file agy ignores or misreads.
//   - skills/ gets links: agy reads a Claude SKILL.md as it is.
//
// Hooks use agy's own schema and tool names, and agy has no commands; both are
// reported, not mirrored. Copies are owned through ~/.ccp/state/agy.json the
// way opencode's are; links are owned by pointing into ~/.ccp.

// AgyConfigDir returns agy's global config directory.
func AgyConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini", "config")
}

func agyStatePath(paths *config.Paths) string {
	return filepath.Join(paths.CcpDir, "state", "agy.json")
}

// AgyOptedIn reports whether ccp has synced to agy before: the test for
// following automatically on a profile switch, link or bootstrap.
func AgyOptedIn(paths *config.Paths) bool {
	_, err := os.Stat(agyStatePath(paths))
	return err == nil
}

// AgyResult reports what a sync did, as paths relative to agy's config
// directory.
type AgyResult struct {
	Written  []string // converted copies created or updated
	Linked   []string // links created or re-pointed
	Removed  []string // ccp's copies and links no longer in the profile
	Edited   []string // ccp's copies changed by hand since: left alone
	Occupied []string // a file or link ccp did not make sits where ccp's would go
	Skipped  []string // profile items that could not be mirrored, with the reason
	Notes    []string // refused directories, oversized rules
	Style    []string // output-style carrier changes and notes
	Hooks    []string // linked hooks, which agy cannot run
	Commands []string // linked commands, which agy does not have
}

// agyRuleLimit is the size at which agy truncates a rule file.
const agyRuleLimit = 24000

// agyBuiltinAgents are agy's own agent names; a profile agent with one of them
// is renamed so it neither shadows nor is shadowed by agy's.
var agyBuiltinAgents = []string{"self", "browser", "research", "deepagent", "review-agent"}

// agyTools are the tool names an agy agent can be granted, in the order ccp
// writes them.
var agyTools = []string{"read_file", "edit_file", "write_file", "run_command", "manage_task", "search_web"}

// claudeToAgyTools maps Claude Code tool names to agy's. Tools with no agy
// counterpart (Task, TodoWrite, Skill, mcp__*) are dropped: agy adds its MCP
// and messaging tools to every agent itself.
var claudeToAgyTools = map[string][]string{
	"Read": {"read_file"}, "Glob": {"read_file"}, "Grep": {"read_file"}, "LS": {"read_file"}, "NotebookRead": {"read_file"},
	"Edit": {"edit_file"}, "MultiEdit": {"edit_file"}, "NotebookEdit": {"edit_file"},
	"Write":      {"write_file"},
	"Bash":       {"run_command", "manage_task"},
	"BashOutput": {"run_command", "manage_task"},
	"KillShell":  {"run_command", "manage_task"},
	"WebSearch":  {"search_web"}, "WebFetch": {"search_web"},
}

// AgySync mirrors the profile into agy: context files, rules, agents, skills
// and the output style.
func AgySync(paths *config.Paths, p *Profile) (AgyResult, error) {
	var result AgyResult
	dir := AgyConfigDir()
	if dir == "" {
		return result, fmt.Errorf("cannot locate agy's config directory")
	}
	state, err := loadCopyState(agyStatePath(paths))
	if err != nil {
		return result, err
	}

	// A type directory that is a link (a dotfiles-managed agents/, say) or not a
	// directory is refused: writing through it would put ccp's files in a tree
	// someone else manages.
	refused := map[string]bool{}
	for _, sub := range []string{"rules", "agents", "skills"} {
		if problem := agyDirProblem(filepath.Join(dir, sub), sub); problem != "" {
			refused[sub] = true
			result.Notes = append(result.Notes, problem)
		}
	}

	desired := map[string][]byte{}
	for _, kind := range []struct {
		itemType config.HubItemType
		convert  func(name string, data []byte) (string, []byte, error)
	}{
		{config.HubRules, func(name string, data []byte) (string, []byte, error) {
			out, err := convertAgyRule(data)
			return name, out, err
		}},
		{config.HubAgents, convertAgyAgent},
	} {
		if refused[string(kind.itemType)] {
			continue
		}
		for _, name := range agyProfileFiles(p, kind.itemType) {
			ref := string(kind.itemType) + "/" + name
			data, err := os.ReadFile(filepath.Join(p.Path, string(kind.itemType), name))
			if err != nil {
				result.Skipped = append(result.Skipped, ref+": "+err.Error())
				continue
			}
			target, out, err := kind.convert(strings.TrimSuffix(name, ".md"), data)
			if err != nil {
				result.Skipped = append(result.Skipped, ref+": "+err.Error())
				continue
			}
			rel := string(kind.itemType) + "/" + target
			if !strings.HasSuffix(rel, ".md") {
				rel += ".md"
			}
			if _, dup := desired[rel]; dup {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s: another profile item already becomes %s", ref, rel))
				continue
			}
			if kind.itemType == config.HubRules && len(out) > agyRuleLimit {
				result.Notes = append(result.Notes, fmt.Sprintf("%s: %s, over agy's %d-byte rule limit — agy truncates it",
					rel, ompHumanSize(len(out)), agyRuleLimit))
			}
			desired[rel] = out
		}
	}

	frozen := func(rel string) bool { return refused[strings.SplitN(rel, "/", 2)[0]] }
	copies, err := syncCopies(dir, state, desired, frozen)
	result.Written, result.Removed, result.Edited, result.Occupied = copies.Written, copies.Removed, copies.Edited, copies.Occupied
	if err != nil {
		return result, err
	}
	if err := saveCopyState(agyStatePath(paths), state); err != nil {
		return result, err
	}

	if !refused["skills"] {
		if err := agySyncSkills(paths, p, filepath.Join(dir, "skills"), &result); err != nil {
			return result, err
		}
	}
	if err := agySyncAgentsMd(paths, p, dir, &result); err != nil {
		return result, err
	}

	result.Hooks = append(result.Hooks, p.Manifest.Hub.Hooks...)
	for _, name := range agyProfileFiles(p, config.HubCommands) {
		result.Commands = append(result.Commands, strings.TrimSuffix(name, ".md"))
	}

	lines, _, err := syncStyleCarriers(paths, p, carrierGroupAgy, filepath.Join(dir, "GEMINI.md"))
	result.Style = lines
	return result, err
}

// AgyFollowProfile re-syncs after the active profile changed, once the user has
// opted in by running 'ccp agy sync'.
func AgyFollowProfile(paths *config.Paths, p *Profile) (AgyResult, bool, error) {
	if !AgyOptedIn(paths) {
		return AgyResult{}, false, nil
	}
	result, err := AgySync(paths, p)
	return result, true, err
}

// agyDirProblem says why ccp must not write into a type directory, or "" when
// it may (including when the directory does not exist yet).
func agyDirProblem(path, name string) string {
	info, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		return ""
	case err != nil:
		return fmt.Sprintf("%s: %v — not mirrored", name, err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Sprintf("%s: %s is a symlink — remove the %s link first; ccp does not write through it", name, path, name)
	case !info.IsDir():
		return fmt.Sprintf("%s: %s is not a directory — remove it first", name, path)
	}
	return ""
}

// agyProfileFiles lists the .md files of one type in the profile, sorted.
func agyProfileFiles(p *Profile, itemType config.HubItemType) []string {
	entries, _ := os.ReadDir(filepath.Join(p.Path, string(itemType)))
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	return names // os.ReadDir sorts by name
}

// agySyncSkills links each of the profile's skills into agy's skills/ and
// removes ccp's links the profile no longer has. Entries ccp did not make are
// left alone.
func agySyncSkills(paths *config.Paths, p *Profile, skillsDir string, result *AgyResult) error {
	symMgr := symlink.New()
	want := map[string]string{}
	keep := map[string]bool{}
	for name, source := range ompDesiredLinks(paths, p.Manifest)[config.HubSkills] {
		ref := "skills/" + name
		fm, err := ompFrontmatter(filepath.Join(source, "SKILL.md"))
		if err != nil {
			// Unreadable is not proof the skill is gone: keep any link ccp made.
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: cannot read SKILL.md", ref))
			keep[name] = true
			continue
		}
		if strings.TrimSpace(ompText(fm["description"])) == "" {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: SKILL.md has no description, which agy requires", ref))
			continue
		}
		want[name] = source
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		linkPath := filepath.Join(skillsDir, e.Name())
		if _, _, ok := resolveCcpLink(symMgr, linkPath, paths.CcpDir); !ok {
			continue // hand-made: not ccp's
		}
		if _, wanted := want[e.Name()]; wanted || keep[e.Name()] {
			continue
		}
		if err := os.Remove(linkPath); err != nil {
			return err
		}
		result.Removed = append(result.Removed, "skills/"+e.Name())
	}

	for _, name := range sortedNames(want) {
		linked, err := ompReplaceLink(symMgr, filepath.Join(skillsDir, name), want[name], paths.CcpDir)
		switch {
		case errors.Is(err, errOmpPathOccupied):
			result.Occupied = append(result.Occupied, "skills/"+name)
		case err != nil:
			return fmt.Errorf("link skills/%s: %w", name, err)
		case linked:
			result.Linked = append(result.Linked, "skills/"+name)
		}
	}
	return nil
}

// agySyncAgentsMd links AGENTS.md to the profile's CLAUDE.md, replacing only a
// link ccp made, and removes ccp's link when the profile has no CLAUDE.md.
func agySyncAgentsMd(paths *config.Paths, p *Profile, dir string, result *AgyResult) error {
	symMgr := symlink.New()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	claudeMd := filepath.Join(p.Path, "CLAUDE.md")
	if _, err := os.Stat(claudeMd); err == nil {
		linked, err := ompReplaceLink(symMgr, agentsPath, claudeMd, paths.CcpDir)
		switch {
		case errors.Is(err, errOmpPathOccupied):
			result.Occupied = append(result.Occupied, "AGENTS.md")
		case err != nil:
			return err
		case linked:
			result.Linked = append(result.Linked, "AGENTS.md → CLAUDE.md")
		}
		return nil
	}
	if _, _, isCcpLink := resolveCcpLink(symMgr, agentsPath, paths.CcpDir); isCcpLink {
		if err := os.Remove(agentsPath); err != nil {
			return err
		}
		result.Removed = append(result.Removed, "AGENTS.md (profile has no CLAUDE.md)")
	}
	return nil
}

// AgyManagedLinks lists the links ccp made in agy (skills and AGENTS.md), which
// point into ~/.ccp and would dangle once it is gone.
func AgyManagedLinks(paths *config.Paths) []string {
	dir := AgyConfigDir()
	if dir == "" {
		return nil
	}
	symMgr := symlink.New()
	var links []string
	if agyDirProblem(filepath.Join(dir, "skills"), "skills") == "" {
		entries, _ := os.ReadDir(filepath.Join(dir, "skills"))
		for _, e := range entries {
			path := filepath.Join(dir, "skills", e.Name())
			if _, _, ok := resolveCcpLink(symMgr, path, paths.CcpDir); ok {
				links = append(links, path)
			}
		}
	}
	agentsPath := filepath.Join(dir, "AGENTS.md")
	if _, _, ok := resolveCcpLink(symMgr, agentsPath, paths.CcpDir); ok {
		links = append(links, agentsPath)
	}
	return links
}

// convertAgyRule turns a Claude Code rule into an agy rule. A rule scoped by
// paths becomes a glob rule; any other rule is always on, as it is in Claude
// Code. The body is kept.
func convertAgyRule(data []byte) ([]byte, error) {
	fm, body, err := splitFrontmatterLenient(data)
	if err != nil {
		return nil, err
	}
	out := struct {
		Trigger     string `yaml:"trigger"`
		Globs       string `yaml:"globs,omitempty"`
		Description string `yaml:"description,omitempty"`
	}{Trigger: "always_on", Description: stringField(fm, "description")}
	if globs := agyGlobs(fm["paths"]); len(globs) > 0 {
		out.Trigger, out.Globs = "glob", strings.Join(globs, ", ")
	}
	return joinFrontmatter(out, body)
}

// agyGlobs converts Claude Code rule paths (a string or a list) into agy globs:
// brace sets expanded, since agy takes one comma-separated string, and relative
// globs prefixed with **/, since agy matches them against absolute paths.
func agyGlobs(v any) []string {
	var raw []string
	switch t := v.(type) {
	case string:
		raw = splitOutsideBraces(t)
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok {
				raw = append(raw, splitOutsideBraces(s)...)
			}
		}
	}
	var globs []string
	for _, pattern := range raw {
		for _, g := range expandBraces(pattern) {
			g = strings.TrimPrefix(strings.TrimSpace(g), "./")
			if g == "" {
				continue
			}
			if !strings.HasPrefix(g, "/") && !strings.HasPrefix(g, "**/") && g != "**" {
				g = "**/" + g
			}
			if !slices.Contains(globs, g) {
				globs = append(globs, g)
			}
		}
	}
	return globs
}

// splitOutsideBraces splits a comma-separated pattern list, leaving commas
// inside a brace set alone.
func splitOutsideBraces(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// expandBraces expands every brace set in a glob: "*.{ts,tsx}" becomes
// "*.ts" and "*.tsx". Nested and repeated sets are expanded too; an unclosed
// brace is left as it is.
func expandBraces(pattern string) []string {
	open := strings.Index(pattern, "{")
	if open < 0 {
		return []string{pattern}
	}
	depth := 0
	for i := open; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var out []string
				for _, alt := range splitOutsideBraces(pattern[open+1 : i]) {
					out = append(out, expandBraces(pattern[:open]+alt+pattern[i+1:])...)
				}
				return out
			}
		}
	}
	return []string{pattern}
}

// convertAgyAgent turns a Claude Code agent into an agy subagent. It returns
// the agent's agy name, which is also its file name.
func convertAgyAgent(stem string, data []byte) (string, []byte, error) {
	fm, body, err := splitFrontmatterLenient(data)
	if err != nil {
		return "", nil, err
	}
	name := stringField(fm, "name")
	if name == "" {
		name = stem
	}
	if strings.ContainsAny(name, `/\`) {
		return "", nil, fmt.Errorf("name %q is not a file name", name)
	}
	if slices.Contains(agyBuiltinAgents, name) {
		name = "claude-" + name
	}
	description := stringField(fm, "description")
	if description == "" {
		return "", nil, fmt.Errorf("no description, which agy requires")
	}

	out := struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description"`
		Tools       []string `yaml:"tools"`
		Subagent    bool     `yaml:"subagent"`
		MainAgent   bool     `yaml:"mainAgent"`
		Model       string   `yaml:"model,omitempty"`
	}{Name: name, Description: description, Tools: agyAgentTools(fm), Subagent: true, Model: agyModel(stringField(fm, "model"))}
	content, err := joinFrontmatter(out, body)
	return name, content, err
}

// agyAgentTools grants an agy agent the tools its Claude Code counterpart has.
// A tools list is an allowlist; without one the agent has every tool except
// those disallowedTools names outright. The result is never nil, because an
// agy agent without tools gets agy's small read-only default instead.
func agyAgentTools(fm map[string]any) []string {
	grant := map[string]bool{}
	if allowed := claudeToolNames(fm["tools"]); len(allowed) > 0 {
		for _, name := range allowed {
			for _, t := range claudeToAgyTools[name] {
				grant[t] = true
			}
		}
	} else {
		for _, t := range agyTools {
			grant[t] = true
		}
		// Only a bare name removes a tool: Bash(rm:*) narrows Bash, it does not
		// take it away.
		denied := map[string]bool{}
		for _, raw := range claudeToolEntries(fm["disallowedTools"]) {
			if !strings.Contains(raw, "(") {
				denied[raw] = true
			}
		}
		if denied["Read"] {
			grant["read_file"] = false
		}
		if denied["Edit"] {
			grant["edit_file"] = false
		}
		if denied["Write"] {
			grant["write_file"] = false
		}
		if denied["Bash"] {
			grant["run_command"], grant["manage_task"] = false, false
		}
		if denied["WebSearch"] && denied["WebFetch"] {
			grant["search_web"] = false
		}
	}
	tools := []string{}
	for _, t := range agyTools {
		if grant[t] {
			tools = append(tools, t)
		}
	}
	return tools
}

// claudeToolEntries reads a Claude Code tool field — a comma-separated string
// or a YAML list — into trimmed entries, as written (Bash(git:*) included).
func claudeToolEntries(v any) []string {
	var raw []string
	switch t := v.(type) {
	case string:
		raw = strings.Split(t, ",")
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	}
	var entries []string
	for _, r := range raw {
		if r = strings.TrimSpace(r); r != "" {
			entries = append(entries, r)
		}
	}
	return entries
}

// claudeToolNames reads a tool field as bare tool names: Bash(git:*) is Bash.
func claudeToolNames(v any) []string {
	var names []string
	for _, entry := range claudeToolEntries(v) {
		if i := strings.Index(entry, "("); i >= 0 {
			entry = strings.TrimSpace(entry[:i])
		}
		names = append(names, entry)
	}
	return names
}

// agyModel maps a Claude Code model alias to agy's: haiku to flash, opus to
// pro. Anything else (inherit, sonnet, full model IDs) is left out, so the
// agent uses agy's own model.
func agyModel(model string) string {
	switch model {
	case "haiku":
		return "flash"
	case "opus":
		return "pro"
	}
	return ""
}

// splitFrontmatterLenient is splitFrontmatter with the fallback omp and agy
// apply: frontmatter that is not valid YAML (prose descriptions routinely
// break it) is read line by line as key: value.
func splitFrontmatterLenient(data []byte) (map[string]any, string, error) {
	block, body, err := cutFrontmatter(data)
	if err != nil {
		return nil, "", err
	}
	fm := map[string]any{}
	if yaml.Unmarshal([]byte(block), &fm) != nil {
		fm = map[string]any{}
		for _, line := range strings.Split(block, "\n") {
			if m := ompFrontmatterFieldLine.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil {
				fm[m[1]] = strings.Trim(strings.TrimSpace(m[2]), `"'`)
			}
		}
	}
	return fm, body, nil
}
