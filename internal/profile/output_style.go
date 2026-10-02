package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Output styles are Claude Code's way to change the system prompt: a linked
// output-styles/<name>.md becomes selectable, and the outputStyle setting picks
// one. Other harnesses have no such setting, so ccp carries the active style's
// prompt to them in a file it generates: APPEND_SYSTEM.md for omp and pi, and
// an instructions file for opencode. style_carriers.go owns those files.

// OutputStyle is the style a profile's settings select.
type OutputStyle struct {
	Name string // the name outputStyle uses
	Ref  string // output-styles/<file>
	Body string // the prompt, without frontmatter
}

// claudeBuiltinOutputStyles are the styles Claude Code ships. They have no file,
// so there is nothing to carry to another harness.
var claudeBuiltinOutputStyles = map[string]bool{"default": true, "explanatory": true, "learning": true}

// StyleSelection is what a profile's settings select, as far as ccp can tell.
type StyleSelection struct {
	Style *OutputStyle // nil: no linked style is selected
	Note  string       // a gap or read problem worth reporting
	// Unknown means ccp could not tell what is selected — settings.json or a
	// style file is unreadable, or a name could only be guessed. Carriers are
	// then left exactly as they are: a read failure is not proof the style was
	// deselected.
	Unknown bool
}

// ActiveOutputStyle returns the linked output style that the profile's
// settings.json selects. Style is nil when it selects none or a built-in one,
// and when two linked styles answer to the selected name: carrying either would
// let a style that merely reuses a name replace the one the user chose.
// Nothing here is fatal: a sync of agents, rules or links must not fail
// because the style lookup did.
func ActiveOutputStyle(p *Profile) StyleSelection {
	selected, err := selectedOutputStyle(p)
	if err != nil {
		return StyleSelection{Note: err.Error(), Unknown: true}
	}
	if selected == "" || claudeBuiltinOutputStyles[strings.ToLower(selected)] {
		return StyleSelection{}
	}

	dir := filepath.Join(p.Path, "output-styles")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return StyleSelection{Note: fmt.Sprintf("output-styles/ is unreadable: %v", err), Unknown: true}
	}
	var files []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") && !strings.HasPrefix(entry.Name(), ".") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	// An exact match wins over a case-insensitive one. An unreadable file is
	// skipped, not fatal: one broken link must not hide the style that works.
	var exact, folded []*OutputStyle
	var unsure []string
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			unsure = append(unsure, "output-styles/"+file+" is unreadable")
			continue
		}
		name, certain := outputStyleName(file, data)
		if !certain {
			unsure = append(unsure, "output-styles/"+file+" has frontmatter that is not valid YAML")
		}
		style := &OutputStyle{Name: name, Ref: "output-styles/" + file, Body: stripFrontmatter(string(data))}
		switch {
		case name == selected:
			exact = append(exact, style)
		case strings.EqualFold(name, selected):
			folded = append(folded, style)
		}
	}

	for _, matches := range [][]*OutputStyle{exact, folded} {
		switch {
		case len(matches) == 1:
			return StyleSelection{Style: matches[0]}
		case len(matches) > 1:
			refs := make([]string, len(matches))
			for i, m := range matches {
				refs[i] = m.Ref
			}
			return StyleSelection{Note: fmt.Sprintf("outputStyle '%s' matches %s — carried nowhere until only one answers to it",
				selected, strings.Join(refs, " and "))}
		}
	}
	if len(unsure) > 0 {
		// One of those files may be the selected style.
		return StyleSelection{
			Note:    fmt.Sprintf("outputStyle '%s' not found, and %s — left as it was", selected, strings.Join(unsure, "; ")),
			Unknown: true,
		}
	}
	return StyleSelection{Note: fmt.Sprintf("outputStyle '%s' matches no linked output style — link it with 'ccp link %s output-styles/<file>'", selected, p.Name)}
}

// outputStyleName returns the name a style answers to: frontmatter name, else
// the filename without .md. certain is false when the frontmatter is not valid
// YAML and the name came from a line-by-line guess.
func outputStyleName(file string, data []byte) (name string, certain bool) {
	name = strings.TrimSuffix(file, ".md")
	fm, _, err := splitFrontmatter(data)
	if err == nil {
		if v := stringField(fm, "name"); v != "" {
			name = v
		}
		return name, true
	}
	for _, line := range strings.Split(string(data), "\n")[1:] {
		if ompFence(line) {
			break
		}
		if m := ompFrontmatterFieldLine.FindStringSubmatch(line); m != nil && m[1] == "name" {
			if v := unquote(strings.TrimSpace(m[2])); v != "" {
				name = v
			}
		}
	}
	return name, false
}

// unquote removes one pair of matching YAML quotes.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// selectedOutputStyle reads the outputStyle setting from the profile's
// settings.json, the file Claude Code reads at user level. A project's
// /output-style choice lives in that project's .claude/settings.local.json and
// is deliberately not followed: it is per project, and the other harnesses'
// carriers are global.
func selectedOutputStyle(p *Profile) (string, error) {
	path := filepath.Join(p.Path, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("cannot read settings.json for the output style: %w", err)
	}
	var settings struct {
		OutputStyle string `json:"outputStyle"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return "", fmt.Errorf("settings.json is not valid JSON, so the output style is left as it was: %w", err)
	}
	return strings.TrimSpace(settings.OutputStyle), nil
}

// stripFrontmatter returns a markdown file's body. The fence rules are omp's,
// which tolerate a BOM, CRLF and prose that is not valid YAML.
func stripFrontmatter(text string) string {
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && ompFence(lines[0]) {
		for i := 1; i < len(lines); i++ {
			if ompFence(lines[i]) {
				text = strings.Join(lines[i+1:], "\n")
				break
			}
		}
	}
	return strings.TrimSpace(text)
}

// outputStyleCarrier is the file ccp generates to carry a style to a harness
// without output styles. Its header names the source so a reader knows where to
// edit; ownership is decided by the carrier ledger, not by this text. The
// content does not name the harness, so omp and pi sharing a directory cannot
// make their syncs undo each other.
func outputStyleCarrier(p *Profile, s *OutputStyle) string {
	safe := strings.NewReplacer("-->", "-- >").Replace // keep the header one comment
	return fmt.Sprintf("%s — output style '%s' of profile '%s'; edit %s in the hub, not this file. -->\n\n%s\n",
		generatedMarker, safe(s.Name), safe(p.Name), safe(s.Ref), s.Body)
}
