package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// outputStyleTestProfile links one hub style into profile "p" and selects
// outputStyle in its settings.json.
func outputStyleTestProfile(t *testing.T, selected string) (*config.Paths, *Manager, *Profile) {
	t.Helper()
	paths, mgr := setupBundleTest(t)
	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubOutputStyles), "ste-lite.md"),
		"---\nname: STE Lite\ndescription: Plain English\nkeep-coding-instructions: true\n---\n\n# STE Lite\n\nShort sentences.\n")
	if err := mgr.LinkHubItem("p", config.HubOutputStyles, "ste-lite.md"); err != nil {
		t.Fatal(err)
	}
	p, _ := mgr.Get("p")
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": "`+selected+`"}`)
	return paths, mgr, p
}

func TestActiveOutputStyle(t *testing.T) {
	tests := []struct {
		selected string
		want     string // style name, "" for none
		note     bool
	}{
		{"STE Lite", "STE Lite", false},
		{"ste lite", "STE Lite", false}, // case-insensitive fallback
		{"Explanatory", "", false},      // built-in: nothing to carry
		{"", "", false},
		{"Missing", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.selected, func(t *testing.T) {
			_, _, p := outputStyleTestProfile(t, tt.selected)
			sel := ActiveOutputStyle(p)
			style, note := sel.Style, sel.Note
			if sel.Unknown {
				t.Fatalf("unknown selection: %+v", sel)
			}
			got := ""
			if style != nil {
				got = style.Name
				if style.Body != "# STE Lite\n\nShort sentences." || style.Ref != "output-styles/ste-lite.md" {
					t.Errorf("style = %+v", style)
				}
			}
			if got != tt.want || (note != "") != tt.note {
				t.Errorf("ActiveOutputStyle = %q, note %q", got, note)
			}
		})
	}
}

// A style with no name in its frontmatter is selected by its filename.
func TestActiveOutputStyle_FilenameName(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubOutputStyles), "terse.md"), "Be terse.\n")
	if err := mgr.LinkHubItem("p", config.HubOutputStyles, "terse.md"); err != nil {
		t.Fatal(err)
	}
	p, _ := mgr.Get("p")
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": "terse"}`)
	if sel := ActiveOutputStyle(p); sel.Style == nil || sel.Style.Body != "Be terse." {
		t.Fatalf("selection = %+v", sel)
	}
}

// ompStyleEnv points omp, pi and opencode at temp directories. omp's carrier
// path ignores PI_CODING_AGENT_DIR, so HOME decides where omp reads.
func ompStyleEnv(t *testing.T) (home, piDir string) {
	t.Helper()
	home = t.TempDir()
	piDir = filepath.Join(t.TempDir(), "pi-agent")
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", piDir)
	t.Setenv("PI_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.Unsetenv("OMP_PROFILE")
	os.Unsetenv("PI_PROFILE")
	return home, piDir
}

// A carrier is written and recorded, rewritten while it holds what ccp wrote,
// left alone once edited by hand, and never taken over just because it starts
// with ccp's marker.
func TestStyleCarriers_Ownership(t *testing.T) {
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	path := filepath.Join(t.TempDir(), "APPEND_SYSTEM.md")
	sync := func() []string {
		t.Helper()
		lines, _, err := syncStyleCarriers(paths, p, "test", path)
		if err != nil {
			t.Fatal(err)
		}
		return lines
	}

	if lines := sync(); len(lines) != 1 || !strings.Contains(lines[0], "STE Lite") {
		t.Fatalf("first sync = %q", lines)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), generatedMarker) || !strings.Contains(string(data), "Short sentences.") ||
		strings.Contains(string(data), "keep-coding-instructions") {
		t.Errorf("carrier = %q", data)
	}
	if lines := sync(); len(lines) != 0 {
		t.Errorf("second sync changed something: %q", lines)
	}

	mustWrite(t, path, string(data)+"\nmy own note\n")
	if lines := sync(); len(lines) != 1 || !strings.Contains(lines[0], "edited by hand") {
		t.Errorf("edited sync = %q", lines)
	}
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": "default"}`)
	sync()
	if after, _ := os.ReadFile(path); !strings.Contains(string(after), "my own note") {
		t.Error("removed or rewrote a carrier edited by hand")
	}

	planted := generatedMarker + " planted -->\nnot ccp's\n"
	mustWrite(t, path, planted)
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": "STE Lite"}`)
	if lines := sync(); len(lines) != 1 || !strings.Contains(lines[0], "occupied") {
		t.Errorf("planted sync = %q", lines)
	}
	if after, _ := os.ReadFile(path); string(after) != planted {
		t.Error("took over a file ccp never wrote because it carries the marker")
	}

	os.Remove(path)
	sync()
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": "default"}`)
	if lines := sync(); len(lines) != 1 || !strings.Contains(lines[0], "removed") {
		t.Errorf("deselected sync = %q", lines)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("carrier not removed when the profile selects a built-in style")
	}
}

// A carrier written while the harness read elsewhere is kept current, and
// removed with the others, instead of loading a stale style forever.
func TestStyleCarriers_FollowRecordedPaths(t *testing.T) {
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	a := filepath.Join(t.TempDir(), "APPEND_SYSTEM.md")
	b := filepath.Join(t.TempDir(), "APPEND_SYSTEM.md")
	for _, path := range []string{a, b} {
		if _, _, err := syncStyleCarriers(paths, p, carrierGroupPi, path); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{}`)
	if _, _, err := syncStyleCarriers(paths, p, carrierGroupPi, b); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{a, b} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s still carries a deselected style", path)
		}
	}
}

// omp reads APPEND_SYSTEM.md from its config root even when PI_CODING_AGENT_DIR
// is set, so that is where ccp writes it; teardown removes it again.
func TestOmpSync_WritesWhereOmpReads(t *testing.T) {
	home, piDir := ompStyleEnv(t)
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatal(err)
	}
	ompPath := filepath.Join(home, ".omp", "agent", "APPEND_SYSTEM.md")
	if data, _ := os.ReadFile(ompPath); !strings.Contains(string(data), "Short sentences.") {
		t.Fatalf("omp carrier = %q", data)
	}
	if _, err := os.Stat(filepath.Join(piDir, "APPEND_SYSTEM.md")); !os.IsNotExist(err) {
		t.Error("omp's carrier landed in PI_CODING_AGENT_DIR, where omp does not read it")
	}
	status, err := OmpContextStatus(paths, p)
	if err != nil || status.OutOfDate() {
		t.Errorf("status after sync = %+v, %v", status, err)
	}
	removed, err := OmpTeardown(paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ompPath); !os.IsNotExist(err) {
		t.Errorf("teardown left the carrier; removed %v", removed)
	}
}

// pi follows only after an explicit sync, and a pi carrier is no omp opt-in.
func TestPiSync_OptIn(t *testing.T) {
	_, piDir := ompStyleEnv(t)
	paths, _, p := outputStyleTestProfile(t, "STE Lite")

	if lines, err := PiFollowProfile(paths, p); err != nil || len(lines) != 0 {
		t.Fatalf("follow before opt-in = %q, %v", lines, err)
	}
	if _, err := os.Stat(filepath.Join(piDir, "APPEND_SYSTEM.md")); !os.IsNotExist(err) {
		t.Fatal("wrote into pi before the user opted in")
	}
	if _, err := PiSync(paths, p); err != nil {
		t.Fatal(err)
	}
	if !PiOptedIn(paths) {
		t.Error("not opted in after a sync")
	}
	if optedIn, err := OmpOptedIn(paths); err != nil || optedIn {
		t.Errorf("OmpOptedIn = %v, %v after only 'ccp pi sync'", optedIn, err)
	}
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{}`)
	lines, err := PiFollowProfile(paths, p)
	if err != nil || len(lines) != 1 || !strings.Contains(lines[0], "removed") {
		t.Errorf("follow after deselecting = %q, %v", lines, err)
	}
	if !PiOptedIn(paths) {
		t.Error("opt-in lost when the carrier was removed")
	}
}

func TestOpencodeSyncCarriesOutputStyle(t *testing.T) {
	ompStyleEnv(t)
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	oc := OpencodeConfigDir()

	r, err := OpencodeSync(paths, p)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(oc, "output-style.md")); !strings.Contains(string(data), "Short sentences.") {
		t.Errorf("output-style.md = %q", data)
	}
	if r.Instructions != filepath.Join(oc, "output-style.md") {
		t.Errorf("no instructions hint: %+v", r)
	}

	mustWrite(t, filepath.Join(oc, "opencode.json"), `{"instructions": ["{env:HOME}/.config/opencode/output-style.md"]}`)
	if r, _ = OpencodeSync(paths, p); r.Instructions != "" {
		t.Errorf("hint repeated after opencode.json names the file: %+v", r)
	}

	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{}`)
	if r, _ = OpencodeSync(paths, p); len(r.Style) != 1 || !strings.Contains(r.Style[0], "removed") {
		t.Errorf("style file not removed after deselecting: %+v", r)
	}
}

// A user's own output-style.md, or ccp's edited by hand, gets no hint to load it.
func TestOpencodeSync_NoHintForFilesCcpDoesNotOwn(t *testing.T) {
	ompStyleEnv(t)
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	stylePath := filepath.Join(OpencodeConfigDir(), "output-style.md")
	mustWrite(t, stylePath, "mine\n")
	if r, err := OpencodeSync(paths, p); err != nil || r.Instructions != "" {
		t.Errorf("occupied: %+v, %v", r, err)
	}

	os.Remove(stylePath)
	if _, err := OpencodeSync(paths, p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(stylePath)
	mustWrite(t, stylePath, string(data)+"edited\n")
	if r, _ := OpencodeSync(paths, p); r.Instructions != "" {
		t.Errorf("edited: %+v", r)
	}
}

// A type directory replaced by a hand-made symlink to the hub must not be
// linked into: the links would land in the hub itself.
func TestLinkRefusesSymlinkedTypeDir(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	hubStyles := paths.HubItemDir(config.HubOutputStyles)
	mustWrite(t, filepath.Join(hubStyles, "a.md"), "a\n")
	mustWrite(t, filepath.Join(hubStyles, "b.md"), "b\n")
	p, _ := mgr.Get("p")
	dir := filepath.Join(p.Path, "output-styles")
	os.RemoveAll(dir)
	if err := os.Symlink(hubStyles, dir); err != nil {
		t.Fatal(err)
	}

	if err := mgr.LinkHubItem("p", config.HubOutputStyles, "a.md"); err == nil {
		t.Error("linked into a symlinked type directory")
	}
	p.Manifest.AddHubItem(config.HubOutputStyles, "b.md")
	if err := SyncLinks(paths, p, nil); err == nil {
		t.Error("SyncLinks accepted a symlinked type directory")
	}
	entries, _ := os.ReadDir(hubStyles)
	if len(entries) != 2 {
		t.Errorf("hub output-styles changed: %d entries", len(entries))
	}
}

// A symlinked type directory is refused before anything changes, by every path
// that links or fixes.
func TestSymlinkedTypeDir_RefusedEverywhere(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubOutputStyles), "a.md"), "a\n")
	p, _ := mgr.Get("p")
	p.Manifest.AddHubItem(config.HubOutputStyles, "a.md")
	skills := filepath.Join(p.Path, "skills")
	os.RemoveAll(skills)
	if err := os.Symlink(paths.HubItemDir(config.HubSkills), skills); err != nil {
		t.Fatal(err)
	}

	if err := SyncLinks(paths, p, nil); err == nil {
		t.Error("SyncLinks accepted a symlinked skills directory")
	}
	if _, err := os.Lstat(filepath.Join(p.Path, "output-styles", "a.md")); !os.IsNotExist(err) {
		t.Error("SyncLinks changed output-styles before refusing")
	}
	if err := mgr.LinkHubBundle("p", "impeccable"); err == nil {
		t.Error("LinkHubBundle accepted a symlinked skills directory")
	}
	if _, err := NewDetector(paths).Detect(p); err == nil {
		t.Error("drift detection accepted a symlinked skills directory")
	}
}

// In output-styles, ccp removes only unlisted links into the hub; the user's
// own files and links elsewhere stay and are not reported as drift.
func TestOutputStyles_ForeignEntriesKept(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	hubStyles := paths.HubItemDir(config.HubOutputStyles)
	mustWrite(t, filepath.Join(hubStyles, "stale.md"), "stale\n")
	own := filepath.Join(t.TempDir(), "dotfiles-style.md")
	mustWrite(t, own, "mine\n")
	p, _ := mgr.Get("p")
	dir := filepath.Join(p.Path, "output-styles")
	mustWrite(t, filepath.Join(dir, "written.md"), "real file\n")
	if err := os.Symlink(own, filepath.Join(dir, "linked.md")); err != nil {
		t.Fatal(err)
	}
	// ccp writes relative links; isHubLink must recognize that shape.
	if err := mgr.symMgr.Create(filepath.Join(dir, "stale.md"), filepath.Join(hubStyles, "stale.md")); err != nil {
		t.Fatal(err)
	}

	report, err := NewDetector(paths).Detect(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range report.Issues {
		if issue.ItemType == config.HubOutputStyles && issue.ItemName != "stale.md" {
			t.Errorf("foreign entry reported as drift: %+v", issue)
		}
	}

	if err := SyncLinks(paths, p, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"written.md", "linked.md"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s removed: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "stale.md")); !os.IsNotExist(err) {
		t.Error("unlisted hub link kept")
	}
}

// One unreadable style must not hide the selected one, and when the selected
// style cannot be found because something is unreadable, carriers stay as they
// are rather than being removed.
func TestOutputStyle_UnreadableSibling(t *testing.T) {
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	if err := os.Symlink(filepath.Join(paths.HubDir, "output-styles", "gone.md"), filepath.Join(p.Path, "output-styles", "alpha.md")); err != nil {
		t.Fatal(err)
	}
	if sel := ActiveOutputStyle(p); sel.Style == nil || sel.Unknown {
		t.Fatalf("a broken sibling hid the selected style: %+v", sel)
	}

	path := filepath.Join(t.TempDir(), "APPEND_SYSTEM.md")
	if _, _, err := syncStyleCarriers(paths, p, "test", path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": "alpha"}`)
	if sel := ActiveOutputStyle(p); !sel.Unknown {
		t.Fatalf("selection = %+v, want unknown", sel)
	}
	lines, _, err := syncStyleCarriers(paths, p, "test", path)
	if err != nil || len(lines) != 1 || !strings.Contains(lines[0], "unreadable") {
		t.Errorf("sync = %q, %v", lines, err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("carrier changed while the selection was unknown")
	}
}

// A malformed settings.json is reported, never fatal, and changes no carrier.
func TestOutputStyle_InvalidSettingsDoesNotFailSyncs(t *testing.T) {
	home, _ := ompStyleEnv(t)
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	mustWrite(t, filepath.Join(p.Path, "CLAUDE.md"), "# context\n")
	if _, err := OmpSync(paths, p); err != nil {
		t.Fatal(err)
	}
	if _, err := OpencodeSync(paths, p); err != nil {
		t.Fatal(err)
	}
	ompPath := filepath.Join(home, ".omp", "agent", "APPEND_SYSTEM.md")
	ocPath := filepath.Join(OpencodeConfigDir(), "output-style.md")
	ompBefore, _ := os.ReadFile(ompPath)
	ocBefore, _ := os.ReadFile(ocPath)

	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": `)
	r, err := OmpSync(paths, p)
	if err != nil {
		t.Fatalf("omp sync failed on a bad settings.json: %v", err)
	}
	if !strings.Contains(strings.Join(r.Context, "\n"), "settings.json") {
		t.Errorf("bad settings.json not reported: %v", r.Context)
	}
	if _, err := OmpContextStatus(paths, p); err != nil {
		t.Errorf("status failed: %v", err)
	}
	if _, err := OpencodeSync(paths, p); err != nil {
		t.Fatalf("opencode sync failed: %v", err)
	}
	if after, _ := os.ReadFile(ompPath); string(after) != string(ompBefore) || len(ompBefore) == 0 {
		t.Error("omp carrier changed or missing")
	}
	if after, _ := os.ReadFile(ocPath); string(after) != string(ocBefore) || len(ocBefore) == 0 {
		t.Error("opencode carrier changed or missing")
	}
}

// Two linked styles answering to the selected name: neither is carried, and
// both are named, so a style reusing a name cannot replace the user's choice.
func TestActiveOutputStyle_DuplicateNames(t *testing.T) {
	paths, mgr, p := outputStyleTestProfile(t, "STE Lite")
	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubOutputStyles), "aaa-copy.md"), "---\nname: STE Lite\n---\nOther body.\n")
	if err := mgr.LinkHubItem("p", config.HubOutputStyles, "aaa-copy.md"); err != nil {
		t.Fatal(err)
	}
	sel := ActiveOutputStyle(p)
	if sel.Style != nil || sel.Unknown || !strings.Contains(sel.Note, "aaa-copy.md") || !strings.Contains(sel.Note, "ste-lite.md") {
		t.Errorf("selection = %+v", sel)
	}
}

// A name in quotes is still found when other frontmatter is not valid YAML;
// a file whose name can only be guessed makes a miss unknown, not a removal.
func TestActiveOutputStyle_LenientFrontmatter(t *testing.T) {
	paths, mgr := setupBundleTest(t)
	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubOutputStyles), "terse.md"),
		"---\nname: \"Terse Mode\"\ndescription: Use this: always\n---\nBe terse.\n")
	if err := mgr.LinkHubItem("p", config.HubOutputStyles, "terse.md"); err != nil {
		t.Fatal(err)
	}
	p, _ := mgr.Get("p")
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": "Terse Mode"}`)
	if sel := ActiveOutputStyle(p); sel.Style == nil || sel.Style.Body != "Be terse." {
		t.Errorf("quoted name not matched: %+v", sel)
	}
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{"outputStyle": "Other"}`)
	if sel := ActiveOutputStyle(p); !sel.Unknown {
		t.Errorf("miss beside a guessed name = %+v, want unknown", sel)
	}
}

// Reset lists carriers while the ledger exists and removes them after, keeping
// any that changed since ccp wrote them.
func TestRemoveStyleCarriers_KeepsChangedFiles(t *testing.T) {
	_, piDir := ompStyleEnv(t)
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	if _, err := PiSync(paths, p); err != nil {
		t.Fatal(err)
	}
	if _, err := OpencodeSync(paths, p); err != nil {
		t.Fatal(err)
	}
	carriers, err := OwnedStyleCarriers(paths, "")
	if err != nil || len(carriers) != 2 {
		t.Fatalf("owned = %+v, %v", carriers, err)
	}
	ocPath := filepath.Join(OpencodeConfigDir(), "output-style.md")
	data, _ := os.ReadFile(ocPath)
	mustWrite(t, ocPath, string(data)+"edited\n")
	os.RemoveAll(paths.CcpDir) // what the resetter does to the ledger

	removed, kept := RemoveStyleCarriers(carriers)
	if len(removed) != 1 || removed[0] != filepath.Join(piDir, "APPEND_SYSTEM.md") || len(kept) != 1 || kept[0] != ocPath {
		t.Errorf("removed %v, kept %v", removed, kept)
	}
}

func TestOutputStyleCarrier_HeaderStaysOneComment(t *testing.T) {
	out := outputStyleCarrier(&Profile{Name: "p"}, &OutputStyle{Name: "a-->b", Ref: "output-styles/x.md", Body: "body"})
	header, _, _ := strings.Cut(out, "\n")
	if strings.Count(header, "-->") != 1 || !strings.HasSuffix(header, "-->") {
		t.Errorf("header = %q", header)
	}
}

// A carrier written before a later carrier fails stays recorded: the ledger is
// saved on the error path too, so the next sync still knows the file is ccp's.
func TestStyleCarriers_SavedWhenALaterCarrierFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	dirA := t.TempDir()
	a := filepath.Join(dirA, "APPEND_SYSTEM.md")
	b := filepath.Join(t.TempDir(), "APPEND_SYSTEM.md")
	for _, path := range []string{a, b} {
		if _, _, err := syncStyleCarriers(paths, p, carrierGroupPi, path); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubOutputStyles), "ste-lite.md"),
		"---\nname: STE Lite\n---\nChanged body.\n")
	os.Chmod(dirA, 0555)
	defer os.Chmod(dirA, 0755)
	if _, _, err := syncStyleCarriers(paths, p, carrierGroupPi, b); err == nil {
		t.Fatal("a write into a read-only directory did not fail")
	}
	os.Chmod(dirA, 0755)

	// The style changes again before the next sync: only a ledger saved after
	// the failure still proves b holds ccp's text.
	mustWrite(t, filepath.Join(paths.HubItemDir(config.HubOutputStyles), "ste-lite.md"),
		"---\nname: STE Lite\n---\nChanged again.\n")
	lines, _, err := syncStyleCarriers(paths, p, carrierGroupPi, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if strings.Contains(line, "occupied") || strings.Contains(line, "edited") {
			t.Errorf("ccp lost track of a carrier it wrote before the failure: %q", lines)
		}
	}
	for _, path := range []string{a, b} {
		if data, _ := os.ReadFile(path); !strings.Contains(string(data), "Changed again.") {
			t.Errorf("%s not brought up to date: %q", path, data)
		}
	}
}

// A corrupt ledger is moved aside and rebuilt: carriers that still hold ccp's
// text are adopted, and the loss is reported once.
func TestStyleCarriers_CorruptLedgerRepaired(t *testing.T) {
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	path := filepath.Join(t.TempDir(), "APPEND_SYSTEM.md")
	if _, _, err := syncStyleCarriers(paths, p, "test", path); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, carrierLedgerPath(paths), "{not json")

	if line, stale, _, err := styleCarrierStatus(paths, p, path); err != nil || !stale || !strings.Contains(line, "ownership unknown") {
		t.Errorf("status on a corrupt ledger = %q, %v, %v", line, stale, err)
	}
	notes, err := RepairStyleLedger(paths)
	if err != nil || len(notes) != 1 || !strings.Contains(notes[0], ".bak") {
		t.Fatalf("repair = %q, %v", notes, err)
	}
	if _, err := os.Stat(carrierLedgerPath(paths) + ".bak"); err != nil {
		t.Error("corrupt ledger not kept as .bak")
	}
	if notes, _ := RepairStyleLedger(paths); len(notes) != 0 {
		t.Errorf("repair of a healthy ledger reported %q", notes)
	}
	if _, owned, err := syncStyleCarriers(paths, p, "test", path); err != nil || !owned {
		t.Errorf("unchanged carrier not adopted after repair: owned=%v, %v", owned, err)
	}
}

// Two ccp processes never update the ledger at once; a lock left by a killed
// process is taken over.
func TestStyleCarriers_Lock(t *testing.T) {
	paths, _, _ := outputStyleTestProfile(t, "STE Lite")
	old := carrierLockWait
	carrierLockWait = 100 * time.Millisecond
	defer func() { carrierLockWait = old }()

	lock := carrierLedgerPath(paths) + ".lock"
	mustWrite(t, lock, "")
	if _, err := updateCarrierLedger(paths, func(*carrierLedger) error { return nil }); err == nil || !strings.Contains(err.Error(), "another ccp") {
		t.Errorf("update with a held lock = %v", err)
	}
	stale := time.Now().Add(-2 * time.Minute)
	os.Chtimes(lock, stale, stale)
	if _, err := updateCarrierLedger(paths, func(*carrierLedger) error { return nil }); err != nil {
		t.Errorf("stale lock not taken over: %v", err)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Error("lock not released")
	}
}

// A carrier the user deleted is forgotten once nothing is wanted; one whose
// directory is missing (an unmounted drive) keeps its entry.
func TestStyleCarriers_Pruning(t *testing.T) {
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	gone := filepath.Join(t.TempDir(), "APPEND_SYSTEM.md")
	unmounted := filepath.Join(t.TempDir(), "drive", "APPEND_SYSTEM.md")
	current := filepath.Join(t.TempDir(), "APPEND_SYSTEM.md")
	for _, path := range []string{gone, unmounted, current} {
		if _, _, err := syncStyleCarriers(paths, p, carrierGroupPi, path); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(gone)
	os.RemoveAll(filepath.Dir(unmounted))
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{}`)
	if _, _, err := syncStyleCarriers(paths, p, carrierGroupPi, current); err != nil {
		t.Fatal(err)
	}
	ledger, _ := loadCarrierLedger(paths)
	if _, ok := ledger.Carriers[gone]; ok {
		t.Error("entry for a deleted carrier kept")
	}
	if _, ok := ledger.Carriers[unmounted]; !ok {
		t.Error("entry for a carrier on a missing drive dropped")
	}
}

// A style carrier is omp's own footprint: with nothing else ccp owns in omp,
// it still opts omp in, so deselecting the style reaches omp.
func TestOmpOptedIn_CarrierOnlyFootprint(t *testing.T) {
	home, _ := ompStyleEnv(t)
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	var result OmpLinkResult
	if err := ompSyncStyle(paths, p, &result); err != nil {
		t.Fatal(err)
	}
	if optedIn, err := OmpOptedIn(paths); err != nil || !optedIn {
		t.Fatalf("OmpOptedIn = %v, %v with an owned carrier", optedIn, err)
	}
	mustWrite(t, filepath.Join(p.Path, "settings.json"), `{}`)
	if _, err := OmpRefreshStyle(paths, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".omp", "agent", "APPEND_SYSTEM.md")); !os.IsNotExist(err) {
		t.Error("deselected style still injected into omp")
	}
}

func TestStyleCarriers_ReportSize(t *testing.T) {
	paths, _, p := outputStyleTestProfile(t, "STE Lite")
	lines, _, err := syncStyleCarriers(paths, p, "test", filepath.Join(t.TempDir(), "APPEND_SYSTEM.md"))
	if err != nil || len(lines) != 1 || !strings.Contains(lines[0], " B in every session") {
		t.Errorf("sync line = %q, %v", lines, err)
	}
}
