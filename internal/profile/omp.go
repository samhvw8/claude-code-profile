package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/symlink"
	"gopkg.in/yaml.v3"
)

// errOmpNoHome is returned when omp's agent directory cannot be resolved
// because the user's home directory is unknown.
var errOmpNoHome = errors.New("cannot resolve omp agent directory: no home directory")

// OmpAgentDir returns omp's native user config directory.
//
// It mirrors omp's own resolution: a named profile (OMP_PROFILE, falling back
// to the legacy PI_PROFILE) relocates the whole user base to
// ~/.omp/profiles/<name>/agent and ignores PI_CODING_AGENT_DIR; otherwise
// PI_CODING_AGENT_DIR overrides ~/.omp/agent. "default", empty, and whitespace
// select the default profile. The config root name comes from PI_CONFIG_DIR
// (default ".omp"). A profile name that is not a single path element is ignored,
// so a typo or a crafted value cannot write outside the config root.
func OmpAgentDir() string {
	root := ompConfigRoot()
	if root == "" {
		return ""
	}

	profile := ompProfileName()
	if profile != "" {
		return filepath.Join(root, "profiles", profile, "agent")
	}

	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(root, "agent")
}

// ompConfigRoot returns the omp config root (~/.omp, or $HOME/$PI_CONFIG_DIR).
func ompConfigRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	configDir := os.Getenv("PI_CONFIG_DIR")
	if configDir == "" {
		configDir = ".omp"
	}
	return filepath.Join(home, configDir)
}

// ompProfileName returns the selected named profile, or "" for the default one.
// A name that is not a single path element is ignored, so a typo or a crafted
// value cannot redirect writes out of the config root; the command layer rejects
// an invalid --profile with an error rather than silently writing elsewhere.
func ompProfileName() string {
	profile, ok := os.LookupEnv("OMP_PROFILE")
	if !ok {
		profile = os.Getenv("PI_PROFILE")
	}

	name, err := ValidateOmpProfile(profile)
	if err != nil {
		return ""
	}
	return name
}

// ValidateOmpProfile vets a named omp profile, as 'ccp omp --profile' takes it.
// An empty name and "default" both select the default profile; a name that is
// not a single path element is an error here, because the caller named it
// explicitly and ccp must not silently write somewhere else.
func ValidateOmpProfile(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "default" {
		return "", nil
	}
	if name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid omp profile %q: must be one path element (no slashes, not . or ..)", name)
	}
	return name, nil
}

// OmpLinkTypes are the hub item types ccp mirrors into omp's agent directory,
// one directory per type — skills/, agents/, commands/. omp's roots have the
// same shape as the hub's, so linking is a one-to-one copy of the layout.
//
// rules/ is deliberately absent: omp surfaces a rule only when its frontmatter
// routes it somewhere (a description into the rulebook, alwaysApply into every
// session, a condition into TTSR), so linking one means predicting which keys
// omp honors. A wrong prediction is invisible — the rule is in omp's rules
// directory, omp never reads it, and the user loses it without a word. Every
// profile rule therefore travels in RULES.md, which ccp generates and fully
// controls: the same "every rule always applies" semantics Claude Code gives
// rules, and no prediction on the path that decides whether a rule exists.
//
// hooks/ is deliberately absent: omp imports hook *modules* from hooks/pre and
// hooks/post, where real omp hooks live, and a Claude hooks.json can never be
// imported. Linking hooks would mean scanning and pruning a directory ccp has no
// business owning, so ccp never links one and never touches that directory.
// Settings templates are absent for the same kind of reason: config.yml and
// settings.json are different schemas.
var OmpLinkTypes = []config.HubItemType{
	config.HubSkills,
	config.HubAgents,
	config.HubCommands,
}

// ompRetiredTypes are item types ccp used to link into omp and no longer links.
// Leftovers do real harm: a linked rule omp honors is injected twice, once where
// omp decided it belongs and once from RULES.md. Any pass removes them.
var ompRetiredTypes = []config.HubItemType{config.HubRules}

// OmpLinkResult reports what a link or sync pass changed.
type OmpLinkResult struct {
	Linked     []string // items linked to omp now (type/name)
	Skipped    []string // items already linked to the same hub item
	Notes      []string // remarks no sync can fix: items omp ignores, occupied paths, unreadable items
	Removed    []string // stale or retired ccp links pruned (sync only)
	LeftBehind []string // links an additive pass left in place (follow only)
	Context    []string // profile-level context files set up (sync only)
}

// OmpDiff reports how omp's links differ from a profile.
type OmpDiff struct {
	Missing []string // the profile wants these linked; omp has no link
	Stale   []string // omp has a ccp link that 'sync' would remove or re-point
	Ignored []string // linked, but omp ignores the item; each with the reason
}

// Empty reports whether omp already matches the profile.
func (d OmpDiff) Empty() bool {
	return len(d.Missing) == 0 && len(d.Stale) == 0
}

// OmpLink symlinks hub items into omp's native skills/ and commands/
// directories. omp only loads foreign harness config (~/.claude) when its
// `enabledProviders` opts in, but it always loads its own agent directory, so
// linking there needs no omp configuration.
func OmpLink(paths *config.Paths, items []string) (OmpLinkResult, error) {
	var result OmpLinkResult

	agentDir := OmpAgentDir()
	if agentDir == "" {
		return result, errOmpNoHome
	}
	symMgr := symlink.New()

	for _, ref := range items {
		itemType, itemName, err := parseOmpItem(ref)
		if err != nil {
			return result, err
		}

		hubPath := paths.HubItemPath(itemType, itemName)
		if _, err := os.Stat(hubPath); err != nil {
			return result, fmt.Errorf("hub item not found: %s", ref)
		}
		// A link omp will not load costs nothing and stays in place for a later
		// omp version, so it is linked and reported, never refused.
		state, reason := ompLoadabilityOf(itemType, hubPath)

		linkPath := filepath.Join(agentDir, string(itemType), itemName)
		linked, err := ompReplaceLink(symMgr, linkPath, hubPath, paths.CcpDir)
		if err != nil {
			return result, fmt.Errorf("link %s: %w", ref, err)
		}
		if linked {
			result.Linked = append(result.Linked, ref)
		} else {
			result.Skipped = append(result.Skipped, ref)
		}
		if state != ompLoads {
			result.Notes = append(result.Notes, fmt.Sprintf("%s: %s", ref, reason))
		}
	}

	return result, nil
}

// OmpUnlink removes ccp-created skill and command links from omp. Links that
// point somewhere other than the hub, and real files, are left untouched.
func OmpUnlink(paths *config.Paths, items []string) ([]string, error) {
	agentDir := OmpAgentDir()
	if agentDir == "" {
		return nil, errOmpNoHome
	}
	symMgr := symlink.New()

	var unlinked []string
	for _, ref := range items {
		itemType, itemName, err := parseOmpItem(ref)
		if err != nil {
			return unlinked, err
		}

		linkPath := filepath.Join(agentDir, string(itemType), itemName)
		if _, _, ok := resolveCcpLink(symMgr, linkPath, paths.CcpDir); !ok {
			continue
		}
		if err := os.Remove(linkPath); err != nil {
			return unlinked, fmt.Errorf("unlink %s: %w", ref, err)
		}
		unlinked = append(unlinked, ref)
	}

	return unlinked, nil
}

// OmpSync reconciles omp with a profile: it links what the profile wants, and
// removes links the profile does not want — or that omp cannot load.
func OmpSync(paths *config.Paths, p *Profile) (OmpLinkResult, error) {
	return ompSync(paths, p, true, nil)
}

// ompSync mirrors a profile into omp. prune removes every link the profile does
// not want (an explicit `ccp omp sync`); previous, when set, additionally owns
// the items the profile being left behind wanted, so a profile switch can retire
// its items without touching links nobody's profile asked for.
func ompSync(paths *config.Paths, p *Profile, prune bool, previous *Profile) (OmpLinkResult, error) {
	var result OmpLinkResult

	agentDir := OmpAgentDir()
	if agentDir == "" {
		return result, errOmpNoHome
	}
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		result.Context = append(result.Context, fmt.Sprintf("created %s", agentDir))
	}
	symMgr := symlink.New()

	var prevDesired map[config.HubItemType]map[string]string
	if previous != nil {
		prevDesired = ompDesiredLinks(paths, previous.Manifest)
	}
	prevWants := func(itemType config.HubItemType, name string) bool {
		_, ok := prevDesired[itemType][name]
		return ok
	}

	desired := ompDesiredLinks(paths, p.Manifest)
	reported := make(map[string]bool)

	links, err := ompScanLinks(paths)
	if err != nil {
		return result, err
	}
	for _, link := range links {
		// A link from an older ccp that no longer links this type: it duplicates
		// what the profile's own files carry now, so any pass retires it.
		if link.retired {
			if err := os.Remove(link.path); err != nil {
				return result, err
			}
			result.Removed = append(result.Removed, fmt.Sprintf("%s (rules travel in RULES.md now)", link.ref()))
			continue
		}

		desiredSource, wanted := desired[link.itemType][link.name]
		if !wanted {
			owned := prune || (previous != nil && prevWants(link.itemType, link.name))
			if !owned {
				// Not this profile's item and not something an explicit sync owns.
				result.LeftBehind = append(result.LeftBehind, link.ref())
				continue
			}
			if err := os.Remove(link.path); err != nil {
				return result, err
			}
			result.Removed = append(result.Removed, link.ref())
			continue
		}

		state, reason := ompLoadabilityOf(link.itemType, desiredSource)
		// A missing or unreadable item is not proof the link is dead: keep it and
		// report, so a transient read failure cannot delete a working link. An
		// item omp ignores is kept for the same reason — omp may load it later,
		// and a link nobody reads costs nothing meanwhile.
		if state == ompUnknown {
			result.Notes = append(result.Notes, fmt.Sprintf("%s: %s (kept)", link.ref(), reason))
			reported[link.ref()] = true
			continue
		}
		if ompLinkTargets(link.target, desiredSource) {
			if state == ompInert {
				result.Notes = append(result.Notes, fmt.Sprintf("%s: %s", link.ref(), reason))
				reported[link.ref()] = true
			}
			continue // already exactly right
		}

		// Right item, wrong target (hand-edited or moved): re-point it.
		linked, err := ompReplaceLink(symMgr, link.path, desiredSource, paths.CcpDir)
		if err != nil {
			return result, err
		}
		if linked {
			result.Linked = append(result.Linked, link.ref())
		}
		if state == ompInert {
			result.Notes = append(result.Notes, fmt.Sprintf("%s: %s", link.ref(), reason))
			reported[link.ref()] = true
		}
	}

	for _, itemType := range OmpLinkTypes {
		for _, name := range sortedNames(desired[itemType]) {
			ref := string(itemType) + "/" + name
			source := desired[itemType][name]
			state, reason := ompLoadabilityOf(itemType, source)
			if state == ompUnknown {
				// Linking what ccp cannot read would create a broken link.
				if !reported[ref] {
					result.Notes = append(result.Notes, fmt.Sprintf("%s: %s", ref, reason))
				}
				continue
			}

			linkPath := filepath.Join(agentDir, string(itemType), name)
			linked, err := ompReplaceLink(symMgr, linkPath, source, paths.CcpDir)
			if err != nil {
				if errors.Is(err, errOmpPathOccupied) {
					result.Notes = append(result.Notes,
						fmt.Sprintf("%s: %v — remove or rename it to link the hub item", ref, err))
					continue
				}
				return result, fmt.Errorf("link %s: %w", ref, err)
			}
			if linked {
				result.Linked = append(result.Linked, ref)
			} else {
				result.Skipped = append(result.Skipped, ref)
			}
			if state == ompInert && !reported[ref] {
				result.Notes = append(result.Notes, fmt.Sprintf("%s: %s", ref, reason))
			}
		}
	}

	if err := ompSyncContext(paths, p, &result); err != nil {
		return result, err
	}

	return result, nil
}

// ompSyncContext sets up the two profile-level files omp reads globally:
// AGENTS.md (omp's user context file, mirroring the profile's CLAUDE.md) and
// RULES.md (omp's always-apply rule file, generated from the profile's rules).
func ompSyncContext(paths *config.Paths, p *Profile, result *OmpLinkResult) error {
	agentDir := OmpAgentDir()
	if agentDir == "" {
		return errOmpNoHome
	}
	symMgr := symlink.New()

	agentsPath := filepath.Join(agentDir, "AGENTS.md")
	claudeMd := filepath.Join(p.Path, "CLAUDE.md")
	if _, err := os.Stat(claudeMd); err == nil {
		linked, err := ompReplaceLink(symMgr, agentsPath, claudeMd, paths.CcpDir)
		switch {
		case err != nil && errors.Is(err, errOmpPathOccupied):
			result.Context = append(result.Context, fmt.Sprintf("AGENTS.md: %v", err))
		case err != nil:
			return err
		case linked:
			result.Context = append(result.Context, "AGENTS.md → CLAUDE.md")
		}
	} else if _, _, isCcpLink := resolveCcpLink(symMgr, agentsPath, paths.CcpDir); isCcpLink {
		// No CLAUDE.md in the profile: drop the link ccp made earlier.
		if err := os.Remove(agentsPath); err != nil {
			return err
		}
		result.Context = append(result.Context, "AGENTS.md (removed: profile has no CLAUDE.md)")
	}

	return ompWriteRulesFile(p, filepath.Join(agentDir, "RULES.md"), result)
}

// ompRulesMarker identifies a RULES.md that ccp generated, so a user's own file
// is never overwritten.
const ompRulesMarker = "<!-- generated by ccp"

// ompRulesContent concatenates every rule in the profile into omp's sticky
// RULES.md, which omp injects into every session. Linking rules into omp's
// rules/ directory is deliberately not done: omp surfaces a rule only when its
// frontmatter routes it somewhere (a description into the rulebook, alwaysApply
// into every session, a condition into TTSR), so a link whose frontmatter omp
// does not honor is a rule nobody loads — silently. RULES.md is generated by
// ccp, so what it carries is what the model sees.
//
// An unreadable rule is reported rather than fatal: one unreadable file must not
// abort a sync, and RULES.md is regenerated from scratch on the next one.
func ompRulesContent(p *Profile) (content string, count int, unreadable []string, err error) {
	dir := filepath.Join(p.Path, "rules")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, nil, nil
		}
		return "", 0, nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "%s — every rule in profile '%s'; regenerate with 'ccp omp sync'. -->\n", ompRulesMarker, p.Name)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			unreadable = append(unreadable, fmt.Sprintf("%s (%v)", name, err))
			continue
		}
		fmt.Fprintf(&b, "\n---\n\n%s\n", strings.TrimSpace(string(data)))
	}

	return b.String(), len(names) - len(unreadable), unreadable, nil
}

// ompWriteRulesFile writes RULES.md for a profile, leaving any file ccp did not
// generate untouched and removing ccp's own file when the profile has no rules.
func ompWriteRulesFile(p *Profile, rulesPath string, result *OmpLinkResult) error {
	info, err := os.Lstat(rulesPath)
	switch {
	case err == nil && !info.Mode().IsRegular():
		result.Context = append(result.Context, "RULES.md: occupied by a file ccp did not create")
		return nil
	case err != nil && !os.IsNotExist(err):
		return err
	}

	content, count, unreadable, err := ompRulesContent(p)
	if err != nil {
		return err
	}
	if len(unreadable) > 0 {
		result.Context = append(result.Context,
			fmt.Sprintf("RULES.md: skipped unreadable %s", strings.Join(unreadable, ", ")))
	}

	if info != nil {
		existing, err := os.ReadFile(rulesPath)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(string(existing), ompRulesMarker) {
			result.Context = append(result.Context, "RULES.md: occupied by a file ccp did not create")
			return nil
		}
		if string(existing) == content {
			return nil
		}
		if count == 0 {
			if err := os.Remove(rulesPath); err != nil {
				return err
			}
			result.Context = append(result.Context, "RULES.md (removed: profile has no rules)")
			return nil
		}
	} else if count == 0 {
		return nil
	}

	// omp injects RULES.md into every session, so a session starting mid-write
	// must never read a truncated file: replace it atomically.
	if err := ompWriteFileAtomic(rulesPath, content); err != nil {
		return err
	}
	result.Context = append(result.Context,
		fmt.Sprintf("RULES.md (%d rules, %s injected every session)", count, ompHumanSize(len(content))))
	return nil
}

// ompWriteFileAtomic replaces a file through a temporary sibling.
func ompWriteFileAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ccp-rules-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ompHumanSize formats a byte count for the one place ccp reports the cost of
// injecting a file into every omp session.
func ompHumanSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

// ompAgentRoots returns every directory ccp may have written into: the default
// agent directory, each named omp profile's agent directory, and the
// PI_CODING_AGENT_DIR override when one points elsewhere. Teardown has to cover
// all of them, since `OMP_PROFILE=x ccp omp sync` writes into a tree the next
// plain invocation would not look at.
//
// Roots are de-duplicated by their resolved path: the same directory reached
// twice — a symlinked ~/.omp, or the selected profile appearing both as the
// active directory and under profiles/ — would otherwise be torn down twice, and
// the second removal fails.
func ompAgentRoots() []string {
	var candidates []string
	if base := ompConfigRoot(); base != "" {
		candidates = append(candidates, filepath.Join(base, "agent"))
		// Named profiles live beside the default one, under the config root.
		if entries, err := os.ReadDir(filepath.Join(base, "profiles")); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					candidates = append(candidates, filepath.Join(base, "profiles", entry.Name(), "agent"))
				}
			}
		}
	}
	// OmpAgentDir also covers a PI_CODING_AGENT_DIR override, which may point
	// anywhere at all.
	candidates = append(candidates, OmpAgentDir())

	seen := make(map[string]bool, len(candidates))
	roots := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		key := ompRealPath(dir)
		if seen[key] {
			continue
		}
		seen[key] = true
		roots = append(roots, dir)
	}

	sort.Strings(roots)
	return roots
}

// OmpManaged returns every path ccp owns in omp: the hub item links in omp's
// type directories, the AGENTS.md link, and a RULES.md carrying ccp's marker,
// across the default agent directory and every named omp profile. Everything
// else omp holds is left out.
func OmpManaged(paths *config.Paths) ([]string, error) {
	return ompManagedInRoots(paths, ompAgentRoots())
}

// ompManagedInRoots returns the paths ccp owns under the given agent
// directories. OmpManaged asks about every tree ccp could have written into;
// a profile switch asks about the one tree it would write into now, so the
// decision to touch omp and the write itself share a scope.
func ompManagedInRoots(paths *config.Paths, roots []string) ([]string, error) {
	symMgr := symlink.New()

	var managed []string
	for _, agentDir := range roots {
		if agentDir == "" {
			continue
		}
		for _, itemType := range ompScannedTypes {
			dir := filepath.Join(agentDir, string(itemType))
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			for _, entry := range entries {
				linkPath := filepath.Join(dir, entry.Name())
				if _, _, ok := resolveCcpLink(symMgr, linkPath, paths.CcpDir); ok {
					managed = append(managed, linkPath)
				}
			}
		}

		agentsPath := filepath.Join(agentDir, "AGENTS.md")
		if _, _, isCcpLink := resolveCcpLink(symMgr, agentsPath, paths.CcpDir); isCcpLink {
			managed = append(managed, agentsPath)
		}

		rulesPath := filepath.Join(agentDir, "RULES.md")
		if info, err := os.Lstat(rulesPath); err == nil && info.Mode().IsRegular() {
			if data, err := os.ReadFile(rulesPath); err == nil && strings.HasPrefix(string(data), ompRulesMarker) {
				managed = append(managed, rulesPath)
			}
		}
	}

	sort.Strings(managed)
	return managed, nil
}

// OmpTeardown removes everything OmpManaged reports — what 'ccp reset' and
// 'ccp omp unlink --all' need to undo the integration without leaving links
// pointing into a deleted ~/.ccp.
func OmpTeardown(paths *config.Paths) ([]string, error) {
	managed, err := OmpManaged(paths)
	if err != nil {
		return nil, err
	}

	var removed []string
	for _, path := range managed {
		if err := os.Remove(path); err != nil {
			return removed, err
		}
		removed = append(removed, path)
	}
	return removed, nil
}

// OmpContextReport describes the profile-level files omp reads globally.
type OmpContextReport struct {
	Current    []string // up to date
	Stale      []string // 'ccp omp sync' would set these up or refresh them
	Occupied   []string // held by a file ccp did not create; sync cannot touch them
	Unreadable []string // rules that could not be read, so RULES.md leaves them out
}

// OutOfDate reports whether 'ccp omp sync' would change a context file.
func (r OmpContextReport) OutOfDate() bool {
	return len(r.Stale) > 0
}

// OmpContextStatus compares omp's global context files (AGENTS.md, RULES.md)
// against what a profile wants, so drift is visible without re-running sync.
// Occupied paths are reported but are not drift: nothing can make them linkable.
func OmpContextStatus(paths *config.Paths, p *Profile) (OmpContextReport, error) {
	var report OmpContextReport

	agentDir := OmpAgentDir()
	if agentDir == "" {
		return report, errOmpNoHome
	}
	symMgr := symlink.New()

	// AGENTS.md mirrors the profile's CLAUDE.md.
	agentsPath := filepath.Join(agentDir, "AGENTS.md")
	claudeMd := filepath.Join(p.Path, "CLAUDE.md")
	_, claudeMdErr := os.Stat(claudeMd)
	agentTarget, _, isAgentLink := resolveCcpLink(symMgr, agentsPath, paths.CcpDir)

	switch {
	case claudeMdErr == nil && isAgentLink:
		if ompLinkTargets(agentTarget, claudeMd) {
			report.Current = append(report.Current, "AGENTS.md → CLAUDE.md")
		} else {
			report.Stale = append(report.Stale, "AGENTS.md points at another profile")
		}
	case claudeMdErr == nil:
		if _, err := os.Lstat(agentsPath); err == nil {
			report.Occupied = append(report.Occupied, "AGENTS.md")
		} else {
			report.Stale = append(report.Stale, "AGENTS.md not set up")
		}
	case isAgentLink:
		report.Stale = append(report.Stale, "AGENTS.md left over (profile has no CLAUDE.md)")
	}

	// RULES.md is generated from the profile's rules.
	rulesPath := filepath.Join(agentDir, "RULES.md")
	content, count, unreadable, err := ompRulesContent(p)
	if err != nil {
		return report, err
	}
	report.Unreadable = unreadable

	info, statErr := os.Lstat(rulesPath)
	switch {
	case statErr == nil && !info.Mode().IsRegular():
		report.Occupied = append(report.Occupied, "RULES.md")
		return report, nil
	case statErr != nil && !os.IsNotExist(statErr):
		return report, statErr
	}

	existing, readErr := os.ReadFile(rulesPath)
	switch {
	case count == 0 && readErr != nil:
		// Nothing wanted, nothing there.
	case count == 0 && strings.HasPrefix(string(existing), ompRulesMarker):
		report.Stale = append(report.Stale, "RULES.md left over (profile has no rules)")
	case count == 0:
		report.Occupied = append(report.Occupied, "RULES.md")
	case readErr != nil:
		report.Stale = append(report.Stale, fmt.Sprintf("RULES.md missing (%d rules)", count))
	case !strings.HasPrefix(string(existing), ompRulesMarker):
		report.Occupied = append(report.Occupied, "RULES.md")
	case string(existing) != content:
		report.Stale = append(report.Stale, fmt.Sprintf("RULES.md outdated (%d rules, %s)", count, ompHumanSize(len(content))))
	default:
		report.Current = append(report.Current, fmt.Sprintf("RULES.md (%d rules, %s every session)", count, ompHumanSize(len(content))))
	}

	return report, nil
}

// OmpList returns the hub items currently linked into omp, as type/name.
func OmpList(paths *config.Paths) ([]string, error) {
	links, err := ompScanLinks(paths)
	if err != nil {
		return nil, err
	}

	items := make([]string, 0, len(links))
	for _, link := range links {
		items = append(items, link.ref())
	}
	sort.Strings(items)
	return items, nil
}

// OmpStatus compares omp's links against a profile. An item omp ignores is still
// linked and reported, never removed: only a link the profile does not want is
// stale, because that — not a guess about omp's loader — is what sync acts on.
func OmpStatus(paths *config.Paths, manifest *Manifest) (OmpDiff, error) {
	var diff OmpDiff

	agentDir := OmpAgentDir()
	if agentDir == "" {
		return diff, errOmpNoHome
	}

	links, err := ompScanLinks(paths)
	if err != nil {
		return diff, err
	}
	linked := make(map[string]bool, len(links))
	for _, link := range links {
		linked[link.ref()] = true
	}

	desired := ompDesiredLinks(paths, manifest)

	for _, itemType := range OmpLinkTypes {
		for _, name := range sortedNames(desired[itemType]) {
			ref := string(itemType) + "/" + name
			state, reason := ompLoadabilityOf(itemType, desired[itemType][name])
			if state == ompInert {
				diff.Ignored = append(diff.Ignored, fmt.Sprintf("%s: %s", ref, reason))
			}
			if linked[ref] || state == ompUnknown {
				continue
			}
			// A path ccp did not create cannot be linked over, so it is not
			// "missing" either — OmpSync reports the collision instead.
			if _, err := os.Lstat(filepath.Join(agentDir, string(itemType), name)); err == nil {
				continue
			}
			diff.Missing = append(diff.Missing, ref)
		}
	}

	for _, link := range links {
		// Stale means "an explicit sync would remove or re-point this": the item
		// is gone from the profile — which also covers a type ccp no longer
		// links, like the rule links older versions made — or the link points
		// somewhere other than the item it claims to be.
		source, wanted := desired[link.itemType][link.name]
		if !wanted || !ompLinkTargets(link.target, source) {
			diff.Stale = append(diff.Stale, link.ref())
		}
	}

	sort.Strings(diff.Ignored)
	return diff, nil
}

// OmpFollowProfile brings omp in step with a profile switch: it links what the
// new profile wants and retires the items the previous profile wanted, without
// touching links no profile asked for (created by hand with 'ccp omp link') —
// those are reported as left behind instead.
//
// It runs only when ccp already owns something in omp: an untouched omp
// directory means the user never opted in, so switching profiles must not create
// links on its own.
//
// Callers should treat an error as a warning: this runs after a profile switch
// has already happened.
func OmpFollowProfile(paths *config.Paths, previous, next *Profile) (OmpLinkResult, error) {
	optedIn, err := OmpOptedIn(paths)
	if err != nil || !optedIn {
		return OmpLinkResult{}, err
	}
	return ompSync(paths, next, false, previous)
}

// OmpOptedIn reports whether ccp owns anything in the omp directory ccp would
// write into right now. That is the test for an automatic write — a profile
// switch or a bootstrap — and it is deliberately scoped to the current tree:
// opting into omp in one named profile is not a reason to start writing into
// the default one.
func OmpOptedIn(paths *config.Paths) (bool, error) {
	agentDir := OmpAgentDir()
	if agentDir == "" {
		return false, errOmpNoHome
	}
	managed, err := ompManagedInRoots(paths, []string{agentDir})
	if err != nil {
		return false, err
	}
	return len(managed) > 0, nil
}

// OmpBroken returns ccp links in omp whose hub target no longer exists.
func OmpBroken(paths *config.Paths) ([]string, error) {
	links, err := ompScanLinks(paths)
	if err != nil {
		return nil, err
	}

	var broken []string
	for _, link := range links {
		if link.broken {
			broken = append(broken, link.ref())
		}
	}
	return broken, nil
}

// OmpPruneBroken removes ccp links in omp whose hub target no longer exists,
// leaving every other link untouched.
func OmpPruneBroken(paths *config.Paths) ([]string, error) {
	links, err := ompScanLinks(paths)
	if err != nil {
		return nil, err
	}

	var pruned []string
	for _, link := range links {
		if !link.broken {
			continue
		}
		if err := os.Remove(link.path); err != nil {
			return pruned, err
		}
		pruned = append(pruned, link.ref())
	}
	return pruned, nil
}

// ompLink is a ccp-created symlink inside omp's agent directory.
type ompLink struct {
	itemType config.HubItemType
	name     string
	path     string // the symlink itself
	target   string // resolved hub path it points at
	broken   bool   // the hub target no longer exists
	retired  bool   // ccp no longer links this type; the link is a leftover
}

func (l ompLink) ref() string {
	return string(l.itemType) + "/" + l.name
}

// ompScannedTypes are the directories ompScanLinks reads: what ccp links now,
// plus the types older versions linked, so their leftovers can be retired.
var ompScannedTypes = append(append([]config.HubItemType{}, OmpLinkTypes...), ompRetiredTypes...)

// ompRetiredType reports whether ccp no longer links an item type.
func ompRetiredType(itemType config.HubItemType) bool {
	for _, retired := range ompRetiredTypes {
		if itemType == retired {
			return true
		}
	}
	return false
}

// ompScanLinks returns every ccp link omp currently holds, in type/name order.
func ompScanLinks(paths *config.Paths) ([]ompLink, error) {
	agentDir := OmpAgentDir()
	if agentDir == "" {
		return nil, errOmpNoHome
	}
	symMgr := symlink.New()

	var links []ompLink
	for _, itemType := range ompScannedTypes {
		dir := filepath.Join(agentDir, string(itemType))
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			linkPath := filepath.Join(dir, entry.Name())
			target, broken, ok := resolveCcpLink(symMgr, linkPath, paths.CcpDir)
			if !ok {
				continue
			}
			links = append(links, ompLink{
				itemType: itemType,
				name:     entry.Name(),
				path:     linkPath,
				target:   target,
				broken:   broken,
				retired:  ompRetiredType(itemType),
			})
		}
	}
	return links, nil
}

// ompDesiredLinks maps every hub item a profile wants in omp to the hub path it
// should point at. Bundle members resolve inside their bundle directory, since
// that is where a bundle keeps them; a bundle that carries a copy of a hub item
// the profile also links is the same item, so the first source wins.
func ompDesiredLinks(paths *config.Paths, manifest *Manifest) map[config.HubItemType]map[string]string {
	desired := make(map[config.HubItemType]map[string]string, len(OmpLinkTypes))
	for _, itemType := range OmpLinkTypes {
		desired[itemType] = make(map[string]string)
	}

	put := func(itemType config.HubItemType, name, source string) {
		if _, ok := desired[itemType][name]; ok {
			return // the same item reached through two sources
		}
		desired[itemType][name] = source
	}

	for _, itemType := range OmpLinkTypes {
		for _, name := range manifest.GetHubItems(itemType) {
			put(itemType, name, paths.HubItemPath(itemType, name))
		}
	}

	for _, bundleName := range manifest.GetHubItems(config.HubBundles) {
		bundle, err := hub.LoadBundle(paths.BundlesDir(), bundleName)
		if err != nil {
			continue // Missing bundle: drift detection reports it.
		}
		for _, member := range bundle.Members.AllComponents() {
			itemType := config.HubItemType(member.Type)
			if _, ok := desired[itemType]; !ok {
				continue
			}
			put(itemType, member.Name, filepath.Join(paths.BundleDir(bundleName), member.Type, member.Name))
		}
	}

	return desired
}

// errOmpPathOccupied reports that omp already holds a file or link ccp did not
// create. Overwriting it would destroy user content, so it is reported instead.
var errOmpPathOccupied = errors.New("occupied by a file ccp did not create")

// ompReplaceLink points linkPath at source, replacing an existing ccp link to a
// different target. It reports whether a new link was created, and errors
// rather than overwriting anything ccp did not create.
func ompReplaceLink(symMgr *symlink.Manager, linkPath, source, ccpDir string) (bool, error) {
	target, _, isCcpLink := resolveCcpLink(symMgr, linkPath, ccpDir)
	if isCcpLink {
		if ompLinkTargets(target, source) {
			return false, nil
		}
		if err := os.Remove(linkPath); err != nil {
			return false, err
		}
	} else if _, err := os.Lstat(linkPath); err == nil {
		return false, fmt.Errorf("%w: %s", errOmpPathOccupied, linkPath)
	}

	return true, ompCreateLink(linkPath, source)
}

// ompLinkTargets reports whether a link already points at the intended source.
// Both sides are resolved first: a link's recorded target is textual, and when
// any parent directory is itself a symlink (a dotfiles-managed ~/.omp, say) the
// text comparison disagrees with what the kernel resolves.
func ompLinkTargets(target, source string) bool {
	return ompRealPath(target) == ompRealPath(source)
}

// ompRealPath resolves symlinks in a path, falling back to the cleaned path when
// part of it does not exist yet.
func ompRealPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// ompCreateLink makes a relative link at linkPath pointing at source, computing
// the relative path from the *resolved* parent so the link still resolves when a
// parent directory is a symlink.
func ompCreateLink(linkPath, source string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}

	parent := filepath.Dir(linkPath)
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		parent = resolved
	}
	target := ompRealPath(source)

	rel, err := filepath.Rel(parent, target)
	if err != nil {
		return err
	}
	return os.Symlink(rel, linkPath)
}

// resolveCcpLink resolves the symlink at linkPath. ok is false when the path is
// not a symlink into a ccp directory (the hub, or a profile that a context file
// is mirrored from); broken reports that its target no longer exists.
//
// The recorded target is joined against the link's *resolved* parent, never its
// textual path: when a parent directory is itself a symlink (a dotfiles-managed
// ~/.omp, say), a relative target written against the real path lands at a depth
// that only exists on paper, and ccp would disown the links it wrote itself.
// ompCreateLink resolves the parent the same way, so the writer and the reader
// agree on what a link means.
func resolveCcpLink(symMgr *symlink.Manager, linkPath, ccpDir string) (target string, broken, ok bool) {
	info, err := symMgr.Info(linkPath)
	if err != nil || !info.Exists || !info.IsSymlink {
		return "", false, false
	}

	raw, err := os.Readlink(linkPath)
	if err != nil {
		return "", false, false
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(ompRealPath(filepath.Dir(linkPath)), raw)
	}
	target = ompRealPath(raw)

	if !underDir(target, ompRealPath(ccpDir)) {
		return "", false, false
	}
	return target, info.IsBroken, true
}

// underDir reports whether path is dir itself or lies inside it.
func underDir(path, dir string) bool {
	dir = filepath.Clean(dir)
	return path == dir || strings.HasPrefix(path, dir+string(os.PathSeparator))
}

// ompLoadability says whether omp loads an item from a path, and ccp's
// confidence in that answer.
type ompLoadability int

const (
	ompLoads   ompLoadability = iota // omp loads it
	ompInert                         // omp ignores it; reason is authoritative
	ompUnknown                       // could not be read: keep any existing link
)

// ompLoadabilityOf encodes omp's loader rules for the item types ccp links,
// verified against omp 18.1.19. It is a reporter: sync prints its verdict as a
// note and nothing else depends on it, so a verdict that is wrong for a future
// omp costs a misleading line of output, never a missing link.
//
// Values are tested, not key presence: an empty description is no description,
// and key spelling is normalized the way omp normalizes it.
func ompLoadabilityOf(itemType config.HubItemType, path string) (ompLoadability, string) {
	info, err := os.Stat(path)
	if err != nil {
		return ompUnknown, "not found in hub"
	}

	switch itemType {
	case config.HubSkills:
		if !info.IsDir() {
			return ompInert, "not a directory (omp only discovers skills/<name>/SKILL.md)"
		}
		skillPath := filepath.Join(path, "SKILL.md")
		if skill, err := os.Stat(skillPath); err != nil || skill.IsDir() {
			return ompInert, "no SKILL.md (omp only discovers skills/<name>/SKILL.md)"
		}
		fields, err := ompFrontmatter(skillPath)
		if err != nil {
			return ompUnknown, "SKILL.md is unreadable"
		}
		if v, ok := fields["enabled"]; ok && !ompIsTrue(v) {
			return ompInert, "SKILL.md is disabled (enabled: false)"
		}
		if desc, ok := fields["description"]; !ok || !ompIsText(desc) {
			return ompInert, "SKILL.md has no description (omp's native provider requires one)"
		}

	case config.HubCommands:
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return ompInert, "not a .md file (omp only discovers commands/*.md)"
		}

	case config.HubAgents:
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return ompInert, "not a .md file (omp discovers agents/*.md)"
		}
		fields, err := ompFrontmatter(path)
		if err != nil {
			return ompUnknown, "unreadable"
		}
		if name, ok := fields["name"]; !ok || !ompIsText(name) {
			return ompInert, "no name in frontmatter (omp rejects an agent without one)"
		}
		if desc, ok := fields["description"]; !ok || !ompIsText(desc) {
			return ompInert, "no description in frontmatter (omp rejects an agent without one)"
		}
		if model, ok := fields["model"]; ok && ompIsText(model) &&
			strings.EqualFold(strings.TrimSpace(ompText(model)), "inherit") {
			return ompInert, "model: inherit (omp resolves no such model, so the agent fails to spawn)"
		}
	}

	return ompLoads, ""
}

// ompIsTrue reports a frontmatter flag omp treats as on: it normalizes flags to
// booleans, so only a true value counts.
func ompIsTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	}
	return false
}

// ompIsText reports a non-empty string value.
func ompIsText(v any) bool {
	return strings.TrimSpace(ompText(v)) != ""
}

// ompText renders a frontmatter scalar for comparison.
func ompText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ompFence reports whether a line is an unindented frontmatter fence. Leading
// whitespace disqualifies it, so a fence inside an indented block scalar cannot
// truncate the block; trailing whitespace and a CRLF carriage return are fine.
func ompFence(line string) bool {
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return false
	}
	return strings.TrimRight(line, " \t\r") == "---"
}

// ompFrontmatterFieldLine matches the simple key/value form omp falls back to
// when a frontmatter document does not parse as YAML.
var ompFrontmatterFieldLine = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s*(.*)$`)

// ompFrontmatter reads a markdown file's YAML frontmatter. A file without
// frontmatter yields an empty map; an unreadable file yields an error.
func ompFrontmatter(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	text := strings.TrimPrefix(string(data), "\ufeff") // strip a BOM
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !ompFence(lines[0]) {
		return map[string]any{}, nil
	}
	for i := 1; i < len(lines); i++ {
		if !ompFence(lines[i]) {
			continue
		}

		block := lines[1:i]
		fields := map[string]any{}
		if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &fields); err != nil {
			// Prose descriptions routinely break YAML (`Examples: ...`), and omp
			// falls back to line-based parsing for exactly that reason. Read the
			// keys the same way rather than reporting a present field as absent.
			for _, line := range block {
				if m := ompFrontmatterFieldLine.FindStringSubmatch(line); m != nil {
					fields[m[1]] = strings.TrimSpace(m[2])
				}
			}
		}
		return fields, nil
	}
	return map[string]any{}, nil // unterminated frontmatter
}

// parseOmpItem splits a "type/name" reference and validates it against the
// types omp shares with Claude Code.
func parseOmpItem(ref string) (config.HubItemType, string, error) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid item: %s (expected type/name)", ref)
	}

	itemType := config.HubItemType(parts[0])
	for _, linkable := range OmpLinkTypes {
		if itemType == linkable {
			return itemType, parts[1], nil
		}
	}
	if ompRetiredType(itemType) {
		return "", "", fmt.Errorf("omp cannot link %s: omp rules travel in RULES.md, which 'ccp omp sync' writes", ref)
	}
	valid := make([]string, 0, len(OmpLinkTypes))
	for _, linkable := range OmpLinkTypes {
		valid = append(valid, string(linkable))
	}
	return "", "", fmt.Errorf("omp cannot link %s (omp mirrors: %s)", ref, strings.Join(valid, ", "))
}

// sortedNames returns a map's keys in sorted order.
func sortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
