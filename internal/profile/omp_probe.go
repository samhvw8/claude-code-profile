package profile

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/samhoang/ccp/internal/config"
)

// ompProbeTimeout bounds the live probe: omp starts, builds its prompt and
// answers, which takes a couple of seconds on a warm install.
const ompProbeTimeout = 60 * time.Second

// ompProbeGetState runs omp in RPC mode and returns the frames it answers with,
// one JSON object per line. It is a variable so tests can replace the live
// process with a recorded answer.
var ompProbeGetState = ompGetStateFrames

// ompGetStateFrames asks a live omp for its state, which includes the system
// prompt it just built. omp needs configured models for this, starts no session
// of its own, and spends no model tokens.
func ompGetStateFrames() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ompProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "omp", "--mode", "rpc", "--no-session", "-c")
	cmd.Stdin = strings.NewReader("{\"id\":\"ccp-probe\",\"type\":\"get_state\"}\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("omp probe failed: %s", firstLine(msg))
	}
	return stdout.String(), nil
}

// firstLine keeps an error message to one line: omp's stderr is often a banner.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// OmpLoadCheck is one thing the probe looked for, and where it looked.
type OmpLoadCheck struct {
	Item  string // as ccp reports it: rules/tone.md, AGENTS.md, skills/validate
	What  string // the text or name that had to appear
	Where string // "system prompt" (ccp's own carrier) or "omp's answer"
	Found bool
}

// OmpPromptReport is what a live omp carried.
//
// Missing is what matters: the text of a rule or context file ccp generates and
// omp does not carry at all. Unnamed is informational — omp lists a skill's
// metadata under its own filters (`skillful`, `ignoredSkills`, `includeSkills`,
// custom directories) and names its agents through tool schemas, so an item
// omp's answer never mentions is worth knowing about, not proof of a bug.
type OmpPromptReport struct {
	Checks   []OmpLoadCheck
	Missing  []string // carriers the prompt did not carry
	Unnamed  []string // linked items omp's answer never names
	Named    int      // linked items omp's answer names
	Items    int      // linked items checked
	Carriers int      // rules and context files checked (text ccp generates)
	Chars    int      // size of the system prompt omp built
	Answer   int      // size of everything omp answered with
}

// Verified reports whether every carrier ccp generates reached omp's prompt.
func (r OmpPromptReport) Verified() bool { return len(r.Missing) == 0 }

// OmpVerifyProfile asks a live omp what it actually carries and compares that
// with the profile ccp mirrored into omp.
//
// This is the only ground truth ccp can get about omp's loader: everything else
// is a prediction about another program, and a prediction that is wrong makes an
// item silently absent rather than visible. It is a diagnostic, never a gate —
// it needs configured models and touches omp's own state — so it runs only when
// a user asks for it.
func OmpVerifyProfile(paths *config.Paths, p *Profile) (OmpPromptReport, error) {
	frames, err := ompProbeGetState()
	if err != nil {
		return OmpPromptReport{}, err
	}
	prompt, err := ompPromptFromFrames(frames)
	if err != nil {
		return OmpPromptReport{}, err
	}
	return ompCheckPrompt(paths, p, prompt, frames), nil
}

// ompPromptFromFrames picks the system prompt out of omp's RPC frames. The
// get_state answer carries it as the parts omp concatenates for the model.
func ompPromptFromFrames(frames string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(frames))
	// One frame carries the whole prompt, so the default 64 KB line limit is
	// not enough.
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)

	for scanner.Scan() {
		var frame struct {
			Data struct {
				SystemPrompt []string `json:"systemPrompt"`
			} `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue // frames of other shapes
		}
		if len(frame.Data.SystemPrompt) > 0 {
			return strings.Join(frame.Data.SystemPrompt, "\n"), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading omp's answer: %w", err)
	}
	return "", errors.New("omp returned no system prompt (is a model configured?)")
}

// ompCheckPrompt collects what omp carried: the text of every rule (RULES.md
// carries all of them) and of the profile's context file must be in the system
// prompt, and each linked item should be named somewhere in omp's answer.
func ompCheckPrompt(paths *config.Paths, p *Profile, prompt, answer string) OmpPromptReport {
	report := OmpPromptReport{Chars: len(prompt), Answer: len(answer)}

	check := func(item, needle, where, haystack string, required bool) {
		if needle == "" {
			return
		}
		found := strings.Contains(haystack, needle)
		report.Checks = append(report.Checks, OmpLoadCheck{Item: item, What: needle, Where: where, Found: found})
		if required {
			report.Carriers++
			if !found {
				report.Missing = append(report.Missing, item)
			}
			return
		}
		report.Items++
		if found {
			report.Named++
		} else {
			report.Unnamed = append(report.Unnamed, item)
		}
	}

	for _, name := range ompRuleNames(p) {
		content, err := os.ReadFile(filepath.Join(p.Path, "rules", name))
		if err != nil {
			continue // unreadable: RULES.md reports it as skipped
		}
		check("rules/"+name, ompPromptNeedle(string(content)), "system prompt", prompt, true)
	}

	if content, err := os.ReadFile(filepath.Join(p.Path, "CLAUDE.md")); err == nil {
		check("AGENTS.md", ompPromptNeedle(string(content)), "system prompt", prompt, true)
	}

	items, err := OmpList(paths)
	if err != nil {
		return report
	}
	for _, ref := range items {
		check(ref, ompItemNeedle(paths, ref), "omp's answer", answer, false)
	}

	return report
}

// ompItemNeedle is the name omp would list an item under: a skill's declared
// name when its SKILL.md has one (a hub directory can be named differently), and
// the file's base name otherwise.
func ompItemNeedle(paths *config.Paths, ref string) string {
	itemType, name, ok := strings.Cut(ref, "/")
	if !ok {
		return ""
	}
	path := paths.HubItemPath(config.HubItemType(itemType), name)

	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, "SKILL.md")
	}
	if fields, err := ompFrontmatter(path); err == nil {
		if declared, ok := fields["name"]; ok && ompIsText(declared) {
			return strings.TrimSpace(ompText(declared))
		}
	}
	return strings.TrimSuffix(filepath.Base(name), ".md")
}

// ompRuleNames lists the profile's rule files in the order RULES.md carries them.
func ompRuleNames(p *Profile) []string {
	entries, err := os.ReadDir(filepath.Join(p.Path, "rules"))
	if err != nil {
		return nil
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// ompPromptNeedle picks the line to look for in omp's prompt: the first line of
// the body, since frontmatter is not what reaches the model, and a line too
// short to be distinctive is no check at all.
func ompPromptNeedle(content string) string {
	lines := strings.Split(strings.TrimPrefix(content, "\ufeff"), "\n")
	inFrontmatter := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case i == 0 && trimmed == "---":
			inFrontmatter = true
		case inFrontmatter:
			if trimmed == "---" {
				inFrontmatter = false
			}
		case len([]rune(trimmed)) >= 12:
			return trimmed
		}
	}
	return ""
}
