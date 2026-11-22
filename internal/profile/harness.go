package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/symlink"
)

// Custom harness plugins extend ccp's profile mirroring without an embedded
// scripting runtime or rebuilding Go. A harness plugin is either:
//
//   - a declarative TOML manifest (~/.ccp/harnesses/<id>.toml), or
//   - an executable (~/.ccp/harnesses/<id> or ccp-harness-<id> on PATH) that
//     reads a HarnessInput JSON snapshot on stdin and writes a HarnessPlan JSON
//     on stdout.
//
// Both produce the same HarnessPlan. ccp validates every path and executes the
// plan itself through the shared atomic copy ledger, symlink guards and
// output-style carrier ledger.

const (
	// HarnessKindTOML marks a declarative TOML manifest plugin.
	HarnessKindTOML = "toml"
	// HarnessKindExec marks an executable stdin/stdout JSON plugin.
	HarnessKindExec = "exec"

	// HarnessAPIVersion is the current JSON protocol version for exec plugins.
	HarnessAPIVersion = 1

	harnessExecPrefix    = "ccp-harness-"
	harnessExecTimeout   = 10 * time.Second
	harnessExecWaitDelay = 2 * time.Second
	harnessMaxOutput     = 16 << 20 // 16 MiB cap on plugin stdout
	harnessMaxStderr     = 4 << 10  // 4 KiB cap on captured stderr for error messages
)

var (
	harnessIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	// reservedHarnessIDs are built-in harnesses with dedicated commands.
	reservedHarnessIDs = []string{"claude", "omp", "opencode", "pi", "agy"}
	envExprRe          = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)
)

// ValidateHarnessID checks that id is a valid custom harness identifier and
// does not collide with a built-in harness.
func ValidateHarnessID(id string) error {
	if !harnessIDRe.MatchString(id) {
		return fmt.Errorf("invalid harness id %q: use 1-32 lowercase letters, digits, or '-' starting with a letter", id)
	}
	if slices.Contains(reservedHarnessIDs, id) {
		return fmt.Errorf("harness id %q is reserved for a built-in harness", id)
	}
	return nil
}

// ExpandHarnessPath expands ~, ~/..., ${env:NAME}, and ${env:NAME:-default}
// into an absolute, clean path. Relative paths and unset variables without a
// default are rejected.
func ExpandHarnessPath(tmpl string) (string, error) {
	s := strings.TrimSpace(tmpl)
	if s == "" {
		return "", errors.New("harness root path is empty")
	}
	var expandErr error
	s = envExprRe.ReplaceAllStringFunc(s, func(match string) string {
		if expandErr != nil {
			return match
		}
		parts := envExprRe.FindStringSubmatch(match)
		name, hasDefault, defVal := parts[1], parts[2] != "", parts[3]
		if v := os.Getenv(name); v != "" {
			return v
		}
		if hasDefault {
			return defVal
		}
		expandErr = fmt.Errorf("environment variable %s is unset or empty in path %q", name, tmpl)
		return ""
	})
	if expandErr != nil {
		return "", expandErr
	}
	if strings.Contains(s, "${") {
		return "", fmt.Errorf("invalid variable expression in path %q", tmpl)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("harness root path %q expanded to empty string", tmpl)
	}
	if s == "~" || strings.HasPrefix(s, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		if s == "~" {
			s = home
		} else {
			s = filepath.Join(home, s[2:])
		}
	}
	if !filepath.IsAbs(s) {
		return "", fmt.Errorf("harness root path %q must be absolute (or start with ~/ or ${env:...})", tmpl)
	}
	return filepath.Clean(s), nil
}

// HarnessPlugin describes one discovered custom harness plugin.
type HarnessPlugin struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Kind    string `json:"kind"` // "toml" or "exec"
	Path    string `json:"path"` // absolute path to .toml file or executable
	Root    string `json:"root,omitempty"`
	OptedIn bool   `json:"opted_in"`
	Err     error  `json:"-"`
}

// HarnessManifest is the declarative TOML schema for Option A plugins.
type HarnessManifest struct {
	ID          string              `toml:"id"`
	Name        string              `toml:"name"`
	Root        string              `toml:"root"`
	Context     *HarnessContextSpec `toml:"context"`
	Skills      *HarnessItemSpec    `toml:"skills"`
	Commands    *HarnessItemSpec    `toml:"commands"`
	Agents      *HarnessItemSpec    `toml:"agents"`
	Rules       *HarnessRulesSpec   `toml:"rules"`
	OutputStyle *HarnessStyleSpec   `toml:"output_style"`
}

// HarnessContextSpec links the profile's CLAUDE.md into the harness root.
type HarnessContextSpec struct {
	Link string `toml:"link"` // relative path under root, e.g. "AGENTS.md"
}

// HarnessItemSpec mirrors one hub item type as links or copies.
type HarnessItemSpec struct {
	Mode string `toml:"mode"` // "link" (skills) or "link"|"copy" (commands, agents)
	Dir  string `toml:"dir"`  // relative directory under root
}

// HarnessRulesSpec mirrors rules as links, copies, or one merged markdown file.
type HarnessRulesSpec struct {
	Mode string `toml:"mode"` // "link", "copy", or "merge"
	Dir  string `toml:"dir"`  // required for "link" or "copy"
	File string `toml:"file"` // required for "merge", e.g. "RULES.md"
}

// HarnessStyleSpec carries the active output style through ccp's carrier ledger.
type HarnessStyleSpec struct {
	File string `toml:"file"` // relative path under root, e.g. "STYLE.md"
}

// HarnessInput is the read-only JSON snapshot piped to an exec plugin's stdin.
type HarnessInput struct {
	APIVersion         int                 `json:"api_version"`
	Harness            string              `json:"harness"`
	Profile            string              `json:"profile"`
	ProfileDir         string              `json:"profile_dir"`
	CcpDir             string              `json:"ccp_dir"`
	Home               string              `json:"home"`
	ClaudeMD           *HarnessFileInput   `json:"claude_md,omitempty"`
	OutputStyle        *HarnessStyleInput  `json:"output_style,omitempty"`
	OutputStyleUnknown bool                `json:"output_style_unknown,omitempty"`
	Skills             []HarnessSkillInput `json:"skills,omitempty"`
	Agents             []HarnessFileInput  `json:"agents,omitempty"`
	Rules              []HarnessFileInput  `json:"rules,omitempty"`
	Commands           []HarnessFileInput  `json:"commands,omitempty"`
	Hooks              []string            `json:"hooks,omitempty"`
}

// HarnessFileInput describes one markdown file in the profile.
type HarnessFileInput struct {
	Name    string `json:"name,omitempty"`
	Path    string `json:"path"`
	HubPath string `json:"hub_path,omitempty"`
	Content string `json:"content"`
}

// HarnessSkillInput describes one skill directory in the profile.
type HarnessSkillInput struct {
	Name    string `json:"name"`
	Path    string `json:"path"`     // path inside profile
	HubPath string `json:"hub_path"` // resolved path inside ~/.ccp/hub
}

// HarnessStyleInput describes the active output style of the profile.
type HarnessStyleInput struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// HarnessPlan is the declarative sync plan produced by both TOML manifests and
// executable plugins. ccp validates every path before applying it.
type HarnessPlan struct {
	APIVersion int               `json:"api_version"`
	Root       string            `json:"root"`
	Links      map[string]string `json:"links,omitempty"`
	Copies     map[string]string `json:"copies,omitempty"`
	StyleFile  string            `json:"style_file,omitempty"`
	Skipped    []string          `json:"skipped,omitempty"`
	Notes      []string          `json:"notes,omitempty"`
}

// HarnessResult reports what a custom harness sync or remove did.
type HarnessResult struct {
	Root     string
	Written  []string
	Linked   []string
	Removed  []string
	Edited   []string
	Occupied []string
	Skipped  []string
	Notes    []string
	Style    []string
}

// HasChanges reports whether anything changed or needs attention.
func (r HarnessResult) HasChanges() bool {
	return len(r.Written)+len(r.Linked)+len(r.Removed)+len(r.Edited)+
		len(r.Occupied)+len(r.Skipped)+len(r.Notes)+len(r.Style) > 0
}

// harnessState persists ccp's ownership ledger for one custom harness in
// ~/.ccp/state/harnesses/<id>.json. Its existence is the opt-in marker.
type harnessState struct {
	Root  string            `json:"root,omitempty"`
	Files map[string]string `json:"files,omitempty"`
	Links []string          `json:"links,omitempty"`
}

func harnessStatePath(paths *config.Paths, id string) string {
	return filepath.Join(paths.HarnessStateDir(), id+".json")
}

func harnessCarrierGroup(id string) string {
	return "harness:" + id + ":"
}

// HarnessOptedIn reports whether ccp has synced to custom harness id before.
func HarnessOptedIn(paths *config.Paths, id string) bool {
	_, err := os.Stat(harnessStatePath(paths, id))
	return err == nil
}

func loadHarnessState(paths *config.Paths, path string) (*harnessState, error) {
	st := &harnessState{Files: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if st.Files == nil {
		st.Files = map[string]string{}
	}
	if st.Root != "" {
		if !filepath.IsAbs(st.Root) {
			return nil, fmt.Errorf("invalid state %s: root %q must be absolute", path, st.Root)
		}
		st.Root = filepath.Clean(st.Root)
		if err := validateHarnessRoot(paths, st.Root); err != nil {
			return nil, fmt.Errorf("invalid state %s: %w", path, err)
		}
	}
	for rel := range st.Files {
		if err := validateRelHarnessPath(rel); err != nil {
			return nil, fmt.Errorf("invalid state %s file entry %q: %w", path, rel, err)
		}
	}
	for _, rel := range st.Links {
		if err := validateRelHarnessPath(rel); err != nil {
			return nil, fmt.Errorf("invalid state %s link entry %q: %w", path, rel, err)
		}
	}
	return st, nil
}

func saveHarnessState(path string, st *harnessState) error {
	sort.Strings(st.Links)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, string(data)+"\n")
}

// DiscoverHarnesses finds custom harness plugins in ~/.ccp/harnesses/ (TOML
// manifests and executables) and ccp-harness-<id> executables on absolute PATH
// directories. Local entries in ~/.ccp/harnesses/ take precedence over PATH.
func DiscoverHarnesses(paths *config.Paths) ([]HarnessPlugin, error) {
	byID := map[string]HarnessPlugin{}

	// 1. Scan ~/.ccp/harnesses/
	entries, err := os.ReadDir(paths.HarnessesDir())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(paths.HarnessesDir(), name)
		info, statErr := os.Stat(fullPath)
		if statErr != nil || info.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".toml") {
			id := strings.TrimSuffix(name, ".toml")
			p := HarnessPlugin{
				ID:      id,
				Kind:    HarnessKindTOML,
				Path:    fullPath,
				OptedIn: HarnessOptedIn(paths, id),
			}
			if err := ValidateHarnessID(id); err != nil {
				p.Err = err
			} else if existing, dup := byID[id]; dup {
				existing.Err = fmt.Errorf("ambiguous harness %q: both %s and %s exist", id, existing.Path, fullPath)
				byID[id] = existing
				continue
			} else if m, mErr := loadHarnessManifest(fullPath, id); mErr != nil {
				p.Err = mErr
			} else {
				p.Name = m.Name
				if root, rErr := ExpandHarnessPath(m.Root); rErr == nil {
					p.Root = root
				} else {
					p.Err = rErr
				}
			}
			byID[id] = p
			continue
		}
		// Non-.toml file: treat as exec plugin if executable.
		if info.Mode()&0111 == 0 {
			continue
		}
		id := name
		p := HarnessPlugin{
			ID:      id,
			Kind:    HarnessKindExec,
			Path:    fullPath,
			OptedIn: HarnessOptedIn(paths, id),
		}
		if err := ValidateHarnessID(id); err != nil {
			p.Err = err
		} else if existing, dup := byID[id]; dup {
			existing.Err = fmt.Errorf("ambiguous harness %q: both %s and %s exist", id, existing.Path, fullPath)
			byID[id] = existing
			continue
		}
		byID[id] = p
	}

	// 2. Scan absolute PATH directories for ccp-harness-<id> executables
	// (lower precedence than ~/.ccp/harnesses; relative PATH entries like "." are skipped).
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range dirEntries {
			name := e.Name()
			id, ok := strings.CutPrefix(name, harnessExecPrefix)
			if !ok || id == "" {
				continue
			}
			if _, exists := byID[id]; exists {
				continue
			}
			if ValidateHarnessID(id) != nil {
				continue
			}
			fullPath := filepath.Join(dir, name)
			info, err := os.Stat(fullPath)
			if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
				continue
			}
			byID[id] = HarnessPlugin{
				ID:      id,
				Kind:    HarnessKindExec,
				Path:    fullPath,
				OptedIn: HarnessOptedIn(paths, id),
			}
		}
	}

	out := make([]HarnessPlugin, 0, len(byID))
	for _, p := range byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// FindHarness returns the discovered plugin for id.
func FindHarness(paths *config.Paths, id string) (HarnessPlugin, error) {
	if err := ValidateHarnessID(id); err != nil {
		return HarnessPlugin{}, err
	}
	plugins, err := DiscoverHarnesses(paths)
	if err != nil {
		return HarnessPlugin{}, err
	}
	for _, p := range plugins {
		if p.ID == id {
			if p.Err != nil {
				return p, p.Err
			}
			return p, nil
		}
	}
	return HarnessPlugin{}, fmt.Errorf("harness %q not found in %s or on PATH (%s%s)", id, paths.HarnessesDir(), harnessExecPrefix, id)
}

func loadHarnessManifest(path, expectedID string) (*HarnessManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m HarnessManifest
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("invalid harness manifest %s: %w", path, err)
	}
	if m.ID != "" && m.ID != expectedID {
		return nil, fmt.Errorf("harness manifest %s declares id %q, expected %q", path, m.ID, expectedID)
	}
	m.ID = expectedID
	if strings.TrimSpace(m.Root) == "" {
		return nil, fmt.Errorf("harness manifest %s must set root", path)
	}
	if err := validateManifestSpec(&m); err != nil {
		return nil, fmt.Errorf("harness manifest %s: %w", path, err)
	}
	return &m, nil
}

func validateManifestSpec(m *HarnessManifest) error {
	checkItem := func(label string, spec *HarnessItemSpec, allowCopy bool) error {
		if spec == nil {
			return nil
		}
		switch spec.Mode {
		case "link":
		case "copy":
			if !allowCopy {
				return fmt.Errorf("[%s].mode must be \"link\", got %q", label, spec.Mode)
			}
		default:
			if allowCopy {
				return fmt.Errorf("[%s].mode must be \"link\" or \"copy\", got %q", label, spec.Mode)
			}
			return fmt.Errorf("[%s].mode must be \"link\", got %q", label, spec.Mode)
		}
		if err := validateRelHarnessPath(spec.Dir); err != nil {
			return fmt.Errorf("[%s].dir: %w", label, err)
		}
		return nil
	}
	if m.Context != nil {
		if err := validateRelHarnessPath(m.Context.Link); err != nil {
			return fmt.Errorf("[context].link: %w", err)
		}
	}
	if err := checkItem("skills", m.Skills, false); err != nil {
		return err
	}
	if err := checkItem("commands", m.Commands, true); err != nil {
		return err
	}
	if err := checkItem("agents", m.Agents, true); err != nil {
		return err
	}
	if m.Rules != nil {
		switch m.Rules.Mode {
		case "link", "copy":
			if m.Rules.File != "" {
				return fmt.Errorf("[rules].file is only valid when mode = \"merge\"")
			}
			if err := validateRelHarnessPath(m.Rules.Dir); err != nil {
				return fmt.Errorf("[rules].dir: %w", err)
			}
		case "merge":
			if m.Rules.Dir != "" {
				return fmt.Errorf("[rules].dir is not valid when mode = \"merge\"")
			}
			if err := validateRelHarnessPath(m.Rules.File); err != nil {
				return fmt.Errorf("[rules].file: %w", err)
			}
		default:
			return fmt.Errorf("[rules].mode must be \"link\", \"copy\", or \"merge\", got %q", m.Rules.Mode)
		}
	}
	if m.OutputStyle != nil {
		if err := validateRelHarnessPath(m.OutputStyle.File); err != nil {
			return fmt.Errorf("[output_style].file: %w", err)
		}
	}
	return nil
}

func validateRelHarnessPath(rel string) error {
	if rel == "" {
		return errors.New("path cannot be empty")
	}
	if strings.Contains(rel, "\\") {
		return fmt.Errorf("path %q must use '/' separators", rel)
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return fmt.Errorf("path %q must be relative to harness root", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes harness root", rel)
	}
	return nil
}

// harnessDesiredLinks resolves every hub item of itemType linked by manifest
// (including bundle members), returning linkName -> absolute hub path.
func harnessDesiredLinks(paths *config.Paths, manifest *Manifest, itemType config.HubItemType) map[string]string {
	out := BundleMemberLinks(paths, manifest, itemType)
	for _, name := range manifest.GetHubItems(itemType) {
		linkName := name
		if itemType == config.HubRules {
			linkName = filepath.Base(name)
		}
		out[linkName] = paths.HubItemPath(itemType, name)
	}
	return out
}

// HarnessBuildPlan produces and validates a HarnessPlan for plugin and profile
// without modifying the filesystem.
func HarnessBuildPlan(paths *config.Paths, plugin HarnessPlugin, p *Profile) (HarnessPlan, error) {
	if plugin.Err != nil {
		return HarnessPlan{}, plugin.Err
	}
	var (
		plan HarnessPlan
		err  error
	)
	switch plugin.Kind {
	case HarnessKindTOML:
		plan, err = buildTOMLHarnessPlan(paths, plugin, p)
	case HarnessKindExec:
		plan, err = buildExecHarnessPlan(paths, plugin, p)
	default:
		return HarnessPlan{}, fmt.Errorf("unknown harness kind %q", plugin.Kind)
	}
	if err != nil {
		return HarnessPlan{}, err
	}
	if err := ValidateHarnessPlan(paths, &plan); err != nil {
		return HarnessPlan{}, err
	}
	return plan, nil
}

func buildTOMLHarnessPlan(paths *config.Paths, plugin HarnessPlugin, p *Profile) (HarnessPlan, error) {
	m, err := loadHarnessManifest(plugin.Path, plugin.ID)
	if err != nil {
		return HarnessPlan{}, err
	}
	root, err := ExpandHarnessPath(m.Root)
	if err != nil {
		return HarnessPlan{}, err
	}
	plan := HarnessPlan{
		APIVersion: HarnessAPIVersion,
		Root:       root,
		Links:      map[string]string{},
		Copies:     map[string]string{},
	}

	if m.Context != nil {
		claudeMD := filepath.Join(p.Path, "CLAUDE.md")
		if _, err := os.Stat(claudeMD); err == nil {
			plan.Links[filepath.ToSlash(filepath.Clean(m.Context.Link))] = absClean(claudeMD)
		}
	}

	addItemSpec := func(itemType config.HubItemType, spec *HarnessItemSpec) {
		if spec == nil {
			return
		}
		dir := filepath.ToSlash(filepath.Clean(spec.Dir))
		switch spec.Mode {
		case "link":
			desired := harnessDesiredLinks(paths, p.Manifest, itemType)
			for _, name := range sortedNames(desired) {
				target := absClean(desired[name])
				if _, err := os.Stat(target); err != nil {
					plan.Skipped = append(plan.Skipped, fmt.Sprintf("%s/%s: target missing or unreadable", itemType, name))
					continue
				}
				plan.Links[dir+"/"+name] = target
			}
		case "copy":
			for _, name := range agyProfileFiles(p, itemType) {
				data, err := os.ReadFile(filepath.Join(p.Path, string(itemType), name))
				if err != nil {
					plan.Skipped = append(plan.Skipped, fmt.Sprintf("%s/%s: %v", itemType, name, err))
					continue
				}
				plan.Copies[dir+"/"+name] = string(data)
			}
		}
	}
	addItemSpec(config.HubSkills, m.Skills)
	addItemSpec(config.HubCommands, m.Commands)
	addItemSpec(config.HubAgents, m.Agents)

	if m.Rules != nil {
		switch m.Rules.Mode {
		case "link", "copy":
			addItemSpec(config.HubRules, &HarnessItemSpec{Mode: m.Rules.Mode, Dir: m.Rules.Dir})
		case "merge":
			merged, skipped := buildMergedRules(p)
			plan.Skipped = append(plan.Skipped, skipped...)
			if merged != "" {
				plan.Copies[filepath.ToSlash(filepath.Clean(m.Rules.File))] = merged
			}
		}
	}

	if m.OutputStyle != nil {
		plan.StyleFile = filepath.ToSlash(filepath.Clean(m.OutputStyle.File))
	}
	return plan, nil
}

func buildMergedRules(p *Profile) (string, []string) {
	names := agyProfileFiles(p, config.HubRules)
	if len(names) == 0 {
		return "", nil
	}
	var (
		b       strings.Builder
		skipped []string
		written int
	)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(p.Path, string(config.HubRules), name))
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("rules/%s: %v", name, err))
			continue
		}
		if written == 0 {
			fmt.Fprintf(&b, "<!-- generated by ccp from profile %s rules -->\n\n", p.Name)
		} else {
			b.WriteString("\n---\n\n")
		}
		b.WriteString(strings.TrimSpace(string(data)))
		b.WriteString("\n")
		written++
	}
	return b.String(), skipped
}

func buildHarnessInput(paths *config.Paths, harnessID string, p *Profile) HarnessInput {
	home, _ := os.UserHomeDir()
	in := HarnessInput{
		APIVersion: HarnessAPIVersion,
		Harness:    harnessID,
		Profile:    p.Name,
		ProfileDir: absClean(p.Path),
		CcpDir:     absClean(paths.CcpDir),
		Home:       home,
	}
	if desiredHooks := harnessDesiredLinks(paths, p.Manifest, config.HubHooks); len(desiredHooks) > 0 {
		in.Hooks = sortedNames(desiredHooks)
	}

	claudeMD := filepath.Join(p.Path, "CLAUDE.md")
	if data, err := os.ReadFile(claudeMD); err == nil {
		in.ClaudeMD = &HarnessFileInput{Name: "CLAUDE.md", Path: absClean(claudeMD), Content: string(data)}
	}

	sel := ActiveOutputStyle(p)
	in.OutputStyleUnknown = sel.Unknown
	if sel.Style != nil {
		in.OutputStyle = &HarnessStyleInput{
			Name:    sel.Style.Name,
			Path:    absClean(filepath.Join(p.Path, sel.Style.Ref)),
			Content: sel.Style.Body,
		}
	}

	desiredSkills := harnessDesiredLinks(paths, p.Manifest, config.HubSkills)
	for _, name := range sortedNames(desiredSkills) {
		in.Skills = append(in.Skills, HarnessSkillInput{
			Name:    name,
			Path:    absClean(filepath.Join(p.Path, string(config.HubSkills), name)),
			HubPath: absClean(desiredSkills[name]),
		})
	}

	readFiles := func(itemType config.HubItemType) []HarnessFileInput {
		desired := harnessDesiredLinks(paths, p.Manifest, itemType)
		var out []HarnessFileInput
		for _, name := range agyProfileFiles(p, itemType) {
			fp := filepath.Join(p.Path, string(itemType), name)
			data, err := os.ReadFile(fp)
			if err != nil {
				continue
			}
			item := HarnessFileInput{
				Name:    name,
				Path:    absClean(fp),
				Content: string(data),
			}
			if hubPath, ok := desired[name]; ok {
				item.HubPath = absClean(hubPath)
			}
			out = append(out, item)
		}
		return out
	}
	in.Agents = readFiles(config.HubAgents)
	in.Rules = readFiles(config.HubRules)
	in.Commands = readFiles(config.HubCommands)
	return in
}

func buildExecHarnessPlan(paths *config.Paths, plugin HarnessPlugin, p *Profile) (HarnessPlan, error) {
	input := buildHarnessInput(paths, plugin.ID, p)
	payload, err := json.Marshal(input)
	if err != nil {
		return HarnessPlan{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), harnessExecTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, plugin.Path)
	cmd.WaitDelay = harnessExecWaitDelay
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	stderr := &truncatingBuffer{max: harnessMaxStderr}
	cmd.Stdout = &limitedBuffer{buf: &stdout, max: harnessMaxOutput}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return HarnessPlan{}, fmt.Errorf("harness plugin %q timed out after %s", plugin.ID, harnessExecTimeout)
		}
		msg := sanitizeStderr(stderr.String())
		if msg != "" {
			return HarnessPlan{}, fmt.Errorf("harness plugin %q failed: %v (%s)", plugin.ID, err, msg)
		}
		return HarnessPlan{}, fmt.Errorf("harness plugin %q failed: %w", plugin.ID, err)
	}

	var plan HarnessPlan
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(stdout.Bytes())))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&plan); err != nil {
		return HarnessPlan{}, fmt.Errorf("harness plugin %q returned invalid JSON: %w", plugin.ID, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return HarnessPlan{}, fmt.Errorf("harness plugin %q returned trailing data after JSON object", plugin.ID)
	}
	if plan.APIVersion != HarnessAPIVersion {
		return HarnessPlan{}, fmt.Errorf("harness plugin %q returned api_version %d, expected %d", plugin.ID, plan.APIVersion, HarnessAPIVersion)
	}
	if strings.TrimSpace(plan.Root) == "" {
		return HarnessPlan{}, fmt.Errorf("harness plugin %q returned empty root", plugin.ID)
	}
	root, err := ExpandHarnessPath(plan.Root)
	if err != nil {
		return HarnessPlan{}, fmt.Errorf("harness plugin %q root: %w", plugin.ID, err)
	}
	plan.Root = root
	return plan, nil
}

type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.buf.Len()+len(p) > l.max {
		return 0, fmt.Errorf("output exceeded %d bytes", l.max)
	}
	return l.buf.Write(p)
}

// truncatingBuffer retains up to max bytes and silently discards the rest so
// verbose stderr never aborts a successful plugin run.
type truncatingBuffer struct {
	buf bytes.Buffer
	max int
}

func (t *truncatingBuffer) Write(p []byte) (int, error) {
	if rem := t.max - t.buf.Len(); rem > 0 {
		if len(p) > rem {
			t.buf.Write(p[:rem])
		} else {
			t.buf.Write(p)
		}
	}
	return len(p), nil
}

func (t *truncatingBuffer) String() string { return t.buf.String() }

func sanitizeStderr(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var clean strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || (r >= 0x20 && r != 0x7f) {
			clean.WriteRune(r)
		}
	}
	lines := strings.Split(clean.String(), "\n")
	if len(lines) > 3 {
		lines = append(lines[:3], "...")
	}
	out := strings.Join(lines, " | ")
	if runes := []rune(out); len(runes) > 300 {
		out = string(runes[:300]) + "..."
	}
	return out
}

// ValidateHarnessPlan enforces path safety on a plan before execution:
//   - root is absolute, clean, and not a dangerous directory (/, $HOME, ~/.ccp, ~/.claude)
//   - every relative path stays inside root with no duplicate or ancestor/descendant collisions
//   - every link target resolves strictly inside paths.CcpDir (after EvalSymlinks)
func ValidateHarnessPlan(paths *config.Paths, plan *HarnessPlan) error {
	if plan.Root == "" || !filepath.IsAbs(plan.Root) {
		return fmt.Errorf("harness plan root %q must be an absolute path", plan.Root)
	}
	plan.Root = filepath.Clean(plan.Root)
	if err := validateHarnessRoot(paths, plan.Root); err != nil {
		return err
	}

	normLinks := make(map[string]string, len(plan.Links))
	for _, rel := range sortedKeys(plan.Links) {
		target := plan.Links[rel]
		if err := validateRelHarnessPath(rel); err != nil {
			return fmt.Errorf("link %q: %w", rel, err)
		}
		cleanRel := filepath.ToSlash(filepath.Clean(rel))
		if _, dup := normLinks[cleanRel]; dup {
			return fmt.Errorf("duplicate link path %q after normalization", cleanRel)
		}
		if !filepath.IsAbs(target) {
			return fmt.Errorf("link %q target %q must be an absolute path", rel, target)
		}
		cleanTarget := filepath.Clean(target)
		if !targetInsideCcp(paths, cleanTarget) {
			return fmt.Errorf("link %q target %q must resolve strictly inside %s", rel, target, paths.CcpDir)
		}
		normLinks[cleanRel] = cleanTarget
	}
	plan.Links = normLinks

	normCopies := make(map[string]string, len(plan.Copies))
	for _, rel := range sortedKeys(plan.Copies) {
		content := plan.Copies[rel]
		if err := validateRelHarnessPath(rel); err != nil {
			return fmt.Errorf("copy %q: %w", rel, err)
		}
		cleanRel := filepath.ToSlash(filepath.Clean(rel))
		if _, dup := normCopies[cleanRel]; dup {
			return fmt.Errorf("duplicate copy path %q after normalization", cleanRel)
		}
		if _, conflict := plan.Links[cleanRel]; conflict {
			return fmt.Errorf("path %q appears in both links and copies", cleanRel)
		}
		normCopies[cleanRel] = content
	}
	plan.Copies = normCopies

	if plan.StyleFile != "" {
		if err := validateRelHarnessPath(plan.StyleFile); err != nil {
			return fmt.Errorf("style_file %q: %w", plan.StyleFile, err)
		}
		plan.StyleFile = filepath.ToSlash(filepath.Clean(plan.StyleFile))
		if _, conflict := plan.Links[plan.StyleFile]; conflict {
			return fmt.Errorf("style_file %q conflicts with a planned link", plan.StyleFile)
		}
		if _, conflict := plan.Copies[plan.StyleFile]; conflict {
			return fmt.Errorf("style_file %q conflicts with a planned copy", plan.StyleFile)
		}
	}

	// Reject ancestor/descendant overlaps across all planned paths (e.g. "a" and "a/b").
	pathSet := make(map[string]bool, len(plan.Links)+len(plan.Copies)+1)
	for rel := range plan.Links {
		pathSet[rel] = true
	}
	for rel := range plan.Copies {
		pathSet[rel] = true
	}
	if plan.StyleFile != "" {
		pathSet[plan.StyleFile] = true
	}
	for _, rel := range mapKeysSorted(pathSet) {
		for dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel))); dir != "." && dir != ""; dir = filepath.ToSlash(filepath.Dir(filepath.FromSlash(dir))) {
			if pathSet[dir] {
				return fmt.Errorf("conflicting nested paths %q and %q in harness plan", dir, rel)
			}
		}
	}
	return nil
}

func validateHarnessRoot(paths *config.Paths, root string) error {
	clean := filepath.Clean(root)
	resolved := resolveDeepestPath(clean)

	checkPair := func(candidate string) error {
		if candidate == string(filepath.Separator) || filepath.Dir(candidate) == candidate {
			return fmt.Errorf("harness root %q cannot be the filesystem root", root)
		}
		if home, err := os.UserHomeDir(); err == nil {
			for _, h := range pathVariants(home) {
				if SamePath(candidate, h) {
					return fmt.Errorf("harness root %q cannot be the home directory", root)
				}
			}
		}
		for _, c := range pathVariants(paths.CcpDir) {
			if SamePath(candidate, c) || isStrictlyUnderDir(candidate, c) {
				return fmt.Errorf("harness root %q cannot be inside %s", root, paths.CcpDir)
			}
		}
		if paths.GlobalClaudeDir != "" {
			for _, g := range pathVariants(paths.GlobalClaudeDir) {
				if SamePath(candidate, g) || isStrictlyUnderDir(candidate, g) {
					return fmt.Errorf("harness root %q cannot be inside %s", root, paths.GlobalClaudeDir)
				}
			}
		}
		return nil
	}
	if err := checkPair(clean); err != nil {
		return err
	}
	if resolved != clean {
		if err := checkPair(resolved); err != nil {
			return err
		}
	}
	return nil
}

// resolveDeepestPath resolves symlinks along the deepest existing ancestor of p
// and re-appends the remaining non-existent suffix.
func resolveDeepestPath(p string) string {
	cur := filepath.Clean(p)
	var tail []string
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			parts := append([]string{real}, tail...)
			return filepath.Clean(filepath.Join(parts...))
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(p)
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

func pathVariants(p string) []string {
	clean := filepath.Clean(p)
	resolved := resolveDeepestPath(clean)
	if resolved != clean {
		return []string{clean, resolved}
	}
	return []string{clean}
}

// targetInsideCcp reports whether target resolves strictly inside paths.CcpDir
// after evaluating symlinks. A symlink inside ~/.ccp that escapes to /etc or
// target == paths.CcpDir itself is rejected.
func targetInsideCcp(paths *config.Paths, target string) bool {
	realCcp, errC := filepath.EvalSymlinks(paths.CcpDir)
	realTarget, errT := filepath.EvalSymlinks(target)
	if errC != nil || errT != nil {
		return false
	}
	return isStrictlyUnderDir(realTarget, realCcp)
}

func isStrictlyUnderDir(path, base string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// parentDirProblem walks every intermediate parent directory of rel under root
// and returns a non-empty reason if any ancestor is a symlink or non-directory.
func parentDirProblem(root, rel string) string {
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if dir == "." || dir == "" {
		return ""
	}
	parts := strings.Split(dir, "/")
	cur := root
	acc := ""
	for _, part := range parts {
		cur = filepath.Join(cur, part)
		if acc == "" {
			acc = part
		} else {
			acc += "/" + part
		}
		if problem := agyDirProblem(cur, acc); problem != "" {
			return problem
		}
	}
	return ""
}

// HarnessCheck validates plugin id against profile p without touching disk.
func HarnessCheck(paths *config.Paths, id string, p *Profile) (HarnessPlan, error) {
	plugin, err := FindHarness(paths, id)
	if err != nil {
		return HarnessPlan{}, err
	}
	return HarnessBuildPlan(paths, plugin, p)
}

// HarnessSync runs plugin id for profile p and applies its plan safely.
func HarnessSync(paths *config.Paths, id string, p *Profile) (HarnessResult, error) {
	plugin, err := FindHarness(paths, id)
	if err != nil {
		return HarnessResult{}, err
	}
	plan, err := HarnessBuildPlan(paths, plugin, p)
	if err != nil {
		return HarnessResult{}, err
	}
	return applyHarnessPlan(paths, id, p, plan)
}

func applyHarnessPlan(paths *config.Paths, id string, p *Profile, plan HarnessPlan) (result HarnessResult, retErr error) {
	result = HarnessResult{
		Root:    plan.Root,
		Skipped: append([]string(nil), plan.Skipped...),
		Notes:   append([]string(nil), plan.Notes...),
	}

	statePath := harnessStatePath(paths, id)
	st, err := loadHarnessState(paths, statePath)
	if err != nil {
		return result, err
	}

	dirty := false
	defer func() {
		if dirty {
			if saveErr := saveHarnessState(statePath, st); saveErr != nil && retErr == nil {
				retErr = saveErr
			}
		}
	}()

	// If the harness root changed since the previous sync, retire old copies
	// and links from the old root first so nothing is stranded there. If any
	// entries in the old root were frozen (symlinked parent), abort the root
	// change rather than leaking old-root entries into the new root.
	oldRoot := st.Root
	if oldRoot != "" && !SamePath(oldRoot, plan.Root) {
		oldRes, err := cleanHarnessRoot(paths, oldRoot, st)
		result.Removed = append(result.Removed, oldRes.Removed...)
		result.Edited = append(result.Edited, oldRes.Edited...)
		result.Notes = append(result.Notes, oldRes.Notes...)
		dirty = true
		if err != nil {
			return result, err
		}
		if len(st.Files) > 0 || len(st.Links) > 0 {
			return result, fmt.Errorf("cannot move harness %q from %s to %s: refused entries remain in %s (resolve symlinked directories first)", id, oldRoot, plan.Root, oldRoot)
		}
	}
	st.Root = plan.Root

	// 1. Prune stale ccp-owned links FIRST (checking each stale link's own
	// parent dirs) so switching from a directory link ("skills") to child paths
	// ("skills/alpha") or from link -> copy removes the old symlink before
	// parentDirProblem evaluates the new plan.
	symMgr := symlink.New()
	seenNotes := map[string]bool{}
	addNote := func(note string) {
		if note != "" && !seenNotes[note] {
			seenNotes[note] = true
			result.Notes = append(result.Notes, note)
		}
	}
	var keptLinks []string
	for i, rel := range st.Links {
		if problem := parentDirProblem(plan.Root, rel); problem != "" {
			addNote(problem)
			keptLinks = append(keptLinks, rel)
			continue
		}
		if _, wanted := plan.Links[rel]; wanted {
			keptLinks = append(keptLinks, rel)
			continue
		}
		linkPath := filepath.Join(plan.Root, filepath.FromSlash(rel))
		if _, _, ok := resolveCcpLink(symMgr, linkPath, paths.CcpDir); ok {
			if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
				st.Links = append(keptLinks, st.Links[i:]...)
				dirty = true
				return result, err
			}
			result.Removed = append(result.Removed, rel)
			dirty = true
		}
	}
	st.Links = keptLinks

	// Check every parent directory component of the new plan and remaining state.
	refusedProblems := map[string]string{}
	checkRel := func(rel string) {
		if problem := parentDirProblem(plan.Root, rel); problem != "" {
			refusedProblems[rel] = problem
			addNote(problem)
		}
	}
	for rel := range plan.Copies {
		checkRel(rel)
	}
	for rel := range plan.Links {
		checkRel(rel)
	}
	for rel := range st.Files {
		checkRel(rel)
	}
	for _, rel := range st.Links {
		checkRel(rel)
	}
	if plan.StyleFile != "" {
		checkRel(plan.StyleFile)
	}
	frozen := func(rel string) bool { return refusedProblems[rel] != "" }

	// 2. Retire any old output-style carrier no longer at plan.StyleFile BEFORE
	// syncCopies/ompReplaceLink so moving a path from style_file -> copy/link
	// does not hit Occupied. Check symlinked parents against both oldRoot and
	// plan.Root. If plan.StyleFile is frozen, keep its path so we never delete
	// through the symlink.
	group := harnessCarrierGroup(id)
	keepStylePath := ""
	if plan.StyleFile != "" {
		keepStylePath = filepath.Join(plan.Root, filepath.FromSlash(plan.StyleFile))
	}
	if err := retireOtherHarnessStyleCarriers(paths, group, []string{oldRoot, plan.Root}, keepStylePath, &result); err != nil {
		return result, err
	}

	// 3. Apply copies via shared hash-ledger syncCopies.
	desiredCopies := make(map[string][]byte, len(plan.Copies))
	for rel, content := range plan.Copies {
		desiredCopies[rel] = []byte(content)
	}
	cs := &copyState{Files: st.Files}
	copies, err := syncCopies(plan.Root, cs, desiredCopies, frozen)
	st.Files = cs.Files
	dirty = true
	result.Written = append(result.Written, copies.Written...)
	result.Removed = append(result.Removed, copies.Removed...)
	result.Edited = append(result.Edited, copies.Edited...)
	result.Occupied = append(result.Occupied, copies.Occupied...)
	if err != nil {
		return result, err
	}

	// 4. Create/update desired links. Seed nextLinks with all still-tracked
	// links (both frozen and already-wanted) so an error on one link never drops
	// existing links from state.
	nextLinks := map[string]bool{}
	for _, rel := range st.Links {
		nextLinks[rel] = true
	}
	for _, rel := range sortedKeys(plan.Links) {
		if frozen(rel) {
			continue
		}
		target := plan.Links[rel]
		linkPath := filepath.Join(plan.Root, filepath.FromSlash(rel))
		linked, err := ompReplaceLink(symMgr, linkPath, target, paths.CcpDir)
		switch {
		case errors.Is(err, errOmpPathOccupied):
			result.Occupied = append(result.Occupied, rel)
		case err != nil:
			st.Links = mapKeysSorted(nextLinks)
			return result, fmt.Errorf("link %s: %w", rel, err)
		default:
			nextLinks[rel] = true
			if linked {
				result.Linked = append(result.Linked, rel)
			}
		}
	}
	st.Links = mapKeysSorted(nextLinks)

	// 5. Sync active output-style carrier when configured and parent dir is safe.
	if plan.StyleFile != "" && !frozen(plan.StyleFile) {
		lines, _, err := syncStyleCarriers(paths, p, group, keepStylePath)
		result.Style = lines
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

func mapKeysSorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func retireOtherHarnessStyleCarriers(paths *config.Paths, group string, roots []string, keepPath string, result *HarnessResult) error {
	checkSafe := func(target string) string {
		for _, root := range roots {
			if root == "" {
				continue
			}
			if rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target)); err == nil &&
				rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				if problem := parentDirProblem(root, filepath.ToSlash(rel)); problem != "" {
					return problem
				}
			}
		}
		return ""
	}
	removed, edited, notes, err := retireStyleCarrierGroup(paths, group, keepPath, checkSafe)
	result.Removed = append(result.Removed, removed...)
	result.Edited = append(result.Edited, edited...)
	result.Notes = append(result.Notes, notes...)
	return err
}

func cleanHarnessRoot(paths *config.Paths, root string, st *harnessState) (HarnessResult, error) {
	var res HarnessResult
	refused := map[string]bool{}
	seenNotes := map[string]bool{}
	checkRel := func(rel string) {
		if problem := parentDirProblem(root, rel); problem != "" {
			refused[rel] = true
			if !seenNotes[problem] {
				seenNotes[problem] = true
				res.Notes = append(res.Notes, problem)
			}
		}
	}
	for rel := range st.Files {
		checkRel(rel)
	}
	for _, rel := range st.Links {
		checkRel(rel)
	}
	frozen := func(rel string) bool { return refused[rel] }

	cs := &copyState{Files: st.Files}
	copies, err := syncCopies(root, cs, nil, frozen)
	st.Files = cs.Files
	res.Removed = append(res.Removed, copies.Removed...)
	res.Edited = append(res.Edited, copies.Edited...)
	if err != nil {
		return res, err
	}

	symMgr := symlink.New()
	var remaining []string
	for i, rel := range st.Links {
		if frozen(rel) {
			remaining = append(remaining, rel)
			continue
		}
		linkPath := filepath.Join(root, filepath.FromSlash(rel))
		if _, _, ok := resolveCcpLink(symMgr, linkPath, paths.CcpDir); ok {
			if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
				st.Links = append(remaining, st.Links[i:]...)
				return res, err
			}
			res.Removed = append(res.Removed, rel)
		}
	}
	st.Links = remaining
	return res, nil
}

// HarnessRemove removes ccp-owned copies, links, style carriers and state for
// custom harness id (opting it out without deleting the plugin file itself).
func HarnessRemove(paths *config.Paths, id string) (HarnessResult, error) {
	if err := ValidateHarnessID(id); err != nil {
		return HarnessResult{}, err
	}
	var result HarnessResult
	statePath := harnessStatePath(paths, id)
	st, err := loadHarnessState(paths, statePath)
	if err != nil {
		return result, err
	}
	result.Root = st.Root
	if st.Root != "" {
		cleanRes, err := cleanHarnessRoot(paths, st.Root, st)
		result.Removed = append(result.Removed, cleanRes.Removed...)
		result.Edited = append(result.Edited, cleanRes.Edited...)
		result.Notes = append(result.Notes, cleanRes.Notes...)
		if err != nil {
			_ = saveHarnessState(statePath, st)
			return result, err
		}
	}

	// Remove or forget any output-style carriers belonging to this harness.
	if err := retireOtherHarnessStyleCarriers(paths, harnessCarrierGroup(id), []string{st.Root}, "", &result); err != nil {
		_ = saveHarnessState(statePath, st)
		return result, err
	}

	// Keep state if refused entries remain so a later remove or reset still tracks them.
	if len(st.Files) > 0 || len(st.Links) > 0 {
		if err := saveHarnessState(statePath, st); err != nil {
			return result, err
		}
		return result, fmt.Errorf("harness %q still has refused entries under %s; resolve symlinked directories and rerun remove", id, st.Root)
	}

	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return result, err
	}
	return result, nil
}

// HarnessFollowResult is the outcome of following the active profile for one
// opted-in custom harness.
type HarnessFollowResult struct {
	ID     string
	Result HarnessResult
	Err    error
}

// HarnessFollowAll re-syncs every opted-in custom harness for profile p.
func HarnessFollowAll(paths *config.Paths, p *Profile) []HarnessFollowResult {
	entries, err := os.ReadDir(paths.HarnessStateDir())
	if err != nil {
		return nil
	}
	var out []HarnessFollowResult
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if ValidateHarnessID(id) != nil {
			continue
		}
		res, err := HarnessSync(paths, id, p)
		out = append(out, HarnessFollowResult{ID: id, Result: res, Err: err})
	}
	return out
}

// HarnessLink records a ccp-owned symlink and its readlink target captured
// before 'ccp reset' deletes ~/.ccp, so reset can verify the link has not
// changed (TOCTOU) before removing it.
type HarnessLink struct {
	Path   string
	Target string
}

// HarnessManagedLinks returns every ccp-owned symlink across all opted-in
// custom harnesses, grouped by harness root directory (for 'ccp reset').
func HarnessManagedLinks(paths *config.Paths) map[string][]HarnessLink {
	entries, err := os.ReadDir(paths.HarnessStateDir())
	if err != nil {
		return nil
	}
	symMgr := symlink.New()
	out := map[string][]HarnessLink{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if ValidateHarnessID(id) != nil {
			continue
		}
		st, err := loadHarnessState(paths, filepath.Join(paths.HarnessStateDir(), e.Name()))
		if err != nil || st.Root == "" {
			continue
		}
		for _, rel := range st.Links {
			if parentDirProblem(st.Root, rel) != "" {
				continue
			}
			linkPath := filepath.Join(st.Root, filepath.FromSlash(rel))
			if _, _, ok := resolveCcpLink(symMgr, linkPath, paths.CcpDir); ok {
				if rawTarget, err := os.Readlink(linkPath); err == nil {
					out[st.Root] = append(out[st.Root], HarnessLink{Path: linkPath, Target: rawTarget})
				}
			}
		}
	}
	return out
}

// HarnessLinkUnchanged reports whether linkPath is still a symlink with the
// exact target recorded earlier (works after ~/.ccp has been deleted).
func HarnessLinkUnchanged(link HarnessLink) bool {
	info, err := os.Lstat(link.Path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	cur, err := os.Readlink(link.Path)
	return err == nil && cur == link.Target
}
