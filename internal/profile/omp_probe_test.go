package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// ompProbeFrames builds the answer omp gives to get_state, with the given
// prompt parts, preceded by a frame of another shape.
func ompProbeFrames(parts ...string) string {
	body, err := json.Marshal(map[string]any{
		"type":    "command",
		"id":      "ccp-probe",
		"success": true,
		"data":    map[string]any{"systemPrompt": parts},
	})
	if err != nil {
		panic(err)
	}
	return "{\"type\":\"rpc.hello\",\"protocolVersion\":1}\n" + string(body) + "\n"
}

func TestOmpPromptFromFrames(t *testing.T) {
	got, err := ompPromptFromFrames(ompProbeFrames("first part", "second part"))
	if err != nil {
		t.Fatalf("ompPromptFromFrames() error: %v", err)
	}
	if want := "first part\nsecond part"; got != want {
		t.Errorf("ompPromptFromFrames() = %q, want %q", got, want)
	}

	for _, frames := range []string{
		"{\"type\":\"rpc.hello\"}\n",
		"not json at all\n",
		"",
	} {
		if _, err := ompPromptFromFrames(frames); err == nil {
			t.Errorf("ompPromptFromFrames(%q) error = nil, want an error", frames)
		}
	}
}

func TestOmpPromptNeedle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"frontmatter is skipped", "---\ndescription: d\n---\n\n# Documentation Convention\n", "# Documentation Convention"},
		{"a long body line is the check", "---\ndescription: d\n---\n\n# Tone\nBe terse at all times.\n", "Be terse at all times."},
		{"no frontmatter", "# Documentation Convention\n", "# Documentation Convention"},
		{"short lines are no check", "# Tone\nBe terse.\n", ""},
		{"an empty file is no check", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ompPromptNeedle(tt.content); got != tt.want {
				t.Errorf("ompPromptNeedle(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestOmpVerifyProfileChecksWhatOmpCarried(t *testing.T) {
	paths, _ := ompTestEnv(t)
	manifest := NewManifest("dev", "")
	manifest.Hub.Skills = []string{"validate"}
	manifest.Hub.Rules = []string{"codegraph.md", "tone.md"}
	p := ompTestProfile(t, paths, manifest)
	ompTestSkill(t, paths.HubDir, "validate")
	ompTestWrite(t, filepath.Join(p.Path, "CLAUDE.md"), "# Global instructions\n\nBe terse.\n")
	ompTestWrite(t, filepath.Join(p.Path, "rules", "codegraph.md"),
		"---\nalwaysApply: true\n---\n\n# Codegraph\n\nRoute structural questions to the graph tools.\n")
	ompTestWrite(t, filepath.Join(p.Path, "rules", "tone.md"), "# Tone\n\nBe terse and concrete at all times.\n")
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatalf("OmpSync() error: %v", err)
	}

	restore := ompProbeGetState
	t.Cleanup(func() { ompProbeGetState = restore })

	// omp's prompt carries the context file, the skill and one rule, but not the
	// other rule — exactly the silent absence the probe exists to find.
	prompt := "# Global instructions\n\nvalidate\nRoute structural questions to the graph tools.\n"
	ompProbeGetState = func() (string, error) { return ompProbeFrames(prompt), nil }

	report, err := OmpVerifyProfile(paths, p)
	if err != nil {
		t.Fatalf("OmpVerifyProfile() error: %v", err)
	}
	if report.Verified() {
		t.Errorf("Verified() = true, want the absent rule reported: %+v", report.Checks)
	}
	if want := []string{"rules/tone.md"}; !reflect.DeepEqual(report.Missing, want) {
		t.Errorf("Missing = %v, want %v", report.Missing, want)
	}
	if report.Chars != len(prompt) {
		t.Errorf("Chars = %d, want %d", report.Chars, len(prompt))
	}

	found := make(map[string]bool)
	for _, check := range report.Checks {
		found[check.Item] = check.Found
	}
	for _, item := range []string{"rules/codegraph.md", "rules/tone.md", "AGENTS.md", "skills/validate"} {
		got, ok := found[item]
		if !ok {
			t.Errorf("no check for %s (%+v)", item, report.Checks)
		}
		if item != "rules/tone.md" && !got {
			t.Errorf("check for %s = false, want true (needle %q)", item, checkNeedle(report, item))
		}
	}
	// The skill's name is a naming check, not a carrier: it must not be able to
	// fail the verification, only be reported.
	if report.Items != 1 || report.Named != 1 || len(report.Unnamed) != 0 {
		t.Errorf("Items = %d, Named = %d, Unnamed = %v, want 1 named item and no unnamed", report.Items, report.Named, report.Unnamed)
	}
	if report.Carriers != 3 {
		t.Errorf("Carriers = %d, want 3 (two rules and AGENTS.md)", report.Carriers)
	}

	// A linked item omp never names is reported, not treated as broken.
	ompProbeGetState = func() (string, error) {
		return ompProbeFrames("# Global instructions\n\nRoute structural questions to the graph tools.\nBe terse and concrete at all times.\n"), nil
	}
	report, err = OmpVerifyProfile(paths, p)
	if err != nil {
		t.Fatalf("OmpVerifyProfile() error: %v", err)
	}
	if !report.Verified() {
		t.Errorf("Verified() = false, want true (missing %v)", report.Missing)
	}
	if want := []string{"skills/validate"}; !reflect.DeepEqual(report.Unnamed, want) {
		t.Errorf("Unnamed = %v, want %v", report.Unnamed, want)
	}
}

func TestOmpVerifyProfileReportsAProbeFailure(t *testing.T) {
	paths, _ := ompTestEnv(t)
	p := ompTestProfile(t, paths, NewManifest("dev", ""))

	restore := ompProbeGetState
	t.Cleanup(func() { ompProbeGetState = restore })
	ompProbeGetState = func() (string, error) { return "", os.ErrDeadlineExceeded }

	if _, err := OmpVerifyProfile(paths, p); err == nil {
		t.Error("OmpVerifyProfile() error = nil, want the probe failure reported")
	}
}

// checkNeedle reports the needle recorded for an item, for failure messages.
func checkNeedle(report OmpPromptReport, item string) string {
	for _, check := range report.Checks {
		if check.Item == item {
			return check.What
		}
	}
	return ""
}
