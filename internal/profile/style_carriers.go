package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// Style carriers are the files ccp writes so harnesses without output styles
// get the active style: omp's and pi's APPEND_SYSTEM.md, opencode's
// output-style.md. One ledger in ~/.ccp/state records every carrier ccp wrote —
// its absolute path and the hash of what was written — and is the only
// evidence of ownership: a file is ccp's to rewrite or remove only while it
// still holds exactly what ccp wrote there. A file edited by hand, or one ccp
// never wrote, is left alone and reported, whatever its first line says.
//
// Carriers are grouped. A sync updates every recorded carrier of its group, not
// only the one at today's path, so a carrier written while an environment
// variable pointed elsewhere (PI_CODING_AGENT_DIR, XDG_CONFIG_HOME) is kept
// current or removed instead of loading a stale style forever. pi and opencode
// follow the active profile, so each is one group; omp's named profiles mirror
// different ccp profiles on purpose, so every omp directory is its own group.

const (
	carrierGroupPi       = "pi"
	carrierGroupOpencode = "opencode"
	carrierGroupOmp      = "omp:" // + the agent directory
)

type carrierRecord struct {
	Group string `json:"group"`
	Hash  string `json:"hash"`
}

type carrierLedger struct {
	// Follow lists groups that follow the active profile automatically once
	// synced by hand. omp and opencode have their own opt-in.
	Follow   []string                 `json:"follow,omitempty"`
	Carriers map[string]carrierRecord `json:"carriers"`
}

func carrierLedgerPath(paths *config.Paths) string {
	return filepath.Join(paths.CcpDir, "state", "output-style-carriers.json")
}

// errCorruptLedger marks a ledger file that exists but does not parse.
var errCorruptLedger = errors.New("output-style ledger is not valid JSON")

// loadCarrierLedger reads the ledger for a look that changes nothing. A missing
// file is an empty ledger; a corrupt one is an error wrapping errCorruptLedger,
// which the next update repairs (updateCarrierLedger).
func loadCarrierLedger(paths *config.Paths) (*carrierLedger, error) {
	ledger := &carrierLedger{Carriers: map[string]carrierRecord{}}
	data, err := os.ReadFile(carrierLedgerPath(paths))
	if err != nil {
		if os.IsNotExist(err) {
			return ledger, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, ledger); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", errCorruptLedger, carrierLedgerPath(paths), err)
	}
	if ledger.Carriers == nil {
		ledger.Carriers = map[string]carrierRecord{}
	}
	return ledger, nil
}

// updateCarrierLedger is the only way the ledger changes: it locks the ledger,
// loads it, lets update change it, and saves it — also when update fails, so a
// carrier written before a later failure stays recorded. A corrupt ledger is
// moved aside to .bak and replaced by an empty one, with a note saying what was
// lost: ownership proof, never a file. notes are lines for the report.
func updateCarrierLedger(paths *config.Paths, update func(*carrierLedger) error) (notes []string, err error) {
	unlock, err := lockCarrierLedger(paths)
	if err != nil {
		return nil, err
	}
	defer unlock()

	ledger, err := loadCarrierLedger(paths)
	if errors.Is(err, errCorruptLedger) {
		bak := carrierLedgerPath(paths) + ".bak"
		if renameErr := os.Rename(carrierLedgerPath(paths), bak); renameErr != nil {
			return nil, err
		}
		notes = append(notes, fmt.Sprintf("the output-style ledger was not valid JSON and was moved to %s; "+
			"ccp starts a new one. Carriers that still hold ccp's text are adopted again, any other is left alone "+
			"and reported; pi stops following until 'ccp pi sync' is run again", bak))
		ledger, err = &carrierLedger{Carriers: map[string]carrierRecord{}}, nil
	}
	if err != nil {
		return nil, err
	}

	before, _ := json.Marshal(ledger)
	updateErr := update(ledger)
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		if saveErr := ledger.save(paths); saveErr != nil && updateErr == nil {
			updateErr = saveErr
		}
	}
	return notes, updateErr
}

// RepairStyleLedger repairs a corrupt ledger before a profile change reaches
// the harnesses, so the loss is reported once and up front. It changes nothing
// when the ledger is fine or absent.
func RepairStyleLedger(paths *config.Paths) ([]string, error) {
	if _, err := loadCarrierLedger(paths); !errors.Is(err, errCorruptLedger) {
		return nil, nil
	}
	return updateCarrierLedger(paths, func(*carrierLedger) error { return nil })
}

// carrierLockWait is how long an update waits for another ccp's lock.
var carrierLockWait = 5 * time.Second

// lockCarrierLedger serializes ledger updates between ccp processes — a hook
// and a manual command, say — so neither saves over the other's changes. The
// lock is a file created exclusively; one older than a minute is left from a
// killed process and is taken over.
func lockCarrierLedger(paths *config.Paths) (unlock func(), err error) {
	lock := carrierLedgerPath(paths) + ".lock"
	if err := os.MkdirAll(filepath.Dir(lock), 0755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(carrierLockWait)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			f.Close()
			return func() { os.Remove(lock) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if info, statErr := os.Stat(lock); statErr == nil && time.Since(info.ModTime()) > time.Minute {
			os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another ccp is updating %s; if none is running, remove %s", carrierLedgerPath(paths), lock)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (l *carrierLedger) save(paths *config.Paths) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(carrierLedgerPath(paths), string(data)+"\n")
}

func (l *carrierLedger) follows(group string) bool {
	return slices.Contains(l.Follow, group)
}

// carrierAction is what a sync does to one carrier path. carrierPlan is the
// single decision table: sync executes it, status only describes it, so the
// two can never disagree.
type carrierAction int

const (
	carrierNothing  carrierAction = iota // absent and unwanted, or already right
	carrierWrite                         // missing, or ccp's file with old content
	carrierRemove                        // ccp's file, no longer wanted
	carrierAdopt                         // holds exactly what ccp would write
	carrierEdited                        // ccp wrote it, someone changed it since
	carrierOccupied                      // a file ccp never wrote
	carrierForget                        // recorded, gone, and not wanted: drop the entry
)

func carrierPlan(path, want string, rec carrierRecord, recorded bool) (carrierAction, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		switch {
		case want != "":
			return carrierWrite, nil
		case recorded:
			return carrierForget, nil
		default:
			return carrierNothing, nil
		}
	}
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		if want == "" && !recorded {
			return carrierNothing, nil
		}
		return carrierOccupied, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	owned := recorded && rec.Hash == hashOf(data)
	switch {
	case want != "" && string(data) == want:
		if owned {
			return carrierNothing, nil
		}
		return carrierAdopt, nil
	case owned && want == "":
		return carrierRemove, nil
	case owned:
		return carrierWrite, nil
	case recorded:
		return carrierEdited, nil
	case want == "":
		return carrierNothing, nil
	default:
		return carrierOccupied, nil
	}
}

// styleWant returns the carrier content a profile wants, or ok=false when the
// selection is unknown and every carrier must be left as it is.
func styleWant(p *Profile) (want string, sel StyleSelection, ok bool) {
	sel = ActiveOutputStyle(p)
	if sel.Unknown {
		return "", sel, false
	}
	if sel.Style != nil {
		want = outputStyleCarrier(p, sel.Style)
	}
	return want, sel, true
}

// syncStyleCarriers keeps every carrier of a group in step with the profile:
// the one at path (where the harness reads today) and any other path the ledger
// recorded for the group. It returns report lines and whether ccp owns the
// carrier at path afterwards. The ledger is saved even when a carrier fails,
// so what was written before the failure stays provably ccp's.
func syncStyleCarriers(paths *config.Paths, p *Profile, group, path string) (lines []string, owned bool, err error) {
	want, sel, ok := styleWant(p)
	if !ok {
		return []string{fmt.Sprintf("%s: %s", filepath.Base(path), sel.Note)}, false, nil
	}

	notes, err := updateCarrierLedger(paths, func(ledger *carrierLedger) error {
		targets := []string{path}
		for _, recorded := range sortedKeys(ledger.Carriers) {
			if ledger.Carriers[recorded].Group == group && recorded != path {
				targets = append(targets, recorded)
			}
		}

		for _, target := range targets {
			label := target
			if target == path {
				label = filepath.Base(target)
			} else if _, err := os.Stat(filepath.Dir(target)); err != nil {
				// Its directory is not there now — an unmounted drive, a removed
				// tree. Keep the entry: if it comes back, so does ccp's claim.
				continue
			}
			rec, recorded := ledger.Carriers[target]
			action, err := carrierPlan(target, want, rec, recorded)
			if err != nil {
				return err
			}
			switch action {
			case carrierWrite:
				// The harness reads this file into every session's system prompt,
				// so a session starting mid-write must never see a truncated file.
				if err := writeFileAtomic(target, want); err != nil {
					return err
				}
				ledger.Carriers[target] = carrierRecord{Group: group, Hash: hashOf([]byte(want))}
				lines = append(lines, fmt.Sprintf("%s (output style '%s', %s in every session)",
					label, sel.Style.Name, ompHumanSize(len(want))))
			case carrierAdopt:
				ledger.Carriers[target] = carrierRecord{Group: group, Hash: hashOf([]byte(want))}
			case carrierForget:
				delete(ledger.Carriers, target)
			case carrierRemove:
				if err := os.Remove(target); err != nil {
					return err
				}
				delete(ledger.Carriers, target)
				reason := "profile selects no linked output style"
				if sel.Note != "" {
					reason = sel.Note
				}
				lines = append(lines, fmt.Sprintf("%s (removed: %s)", label, reason))
			case carrierEdited:
				delete(ledger.Carriers, target) // edited by hand: no longer ccp's
				lines = append(lines, fmt.Sprintf("%s: left alone — edited by hand since ccp wrote it; remove it to let ccp manage it", label))
			case carrierOccupied:
				delete(ledger.Carriers, target)
				if want != "" {
					lines = append(lines, fmt.Sprintf("%s: occupied by a file ccp did not write (or wrote before its ledger was lost) — output style '%s' not carried", label, sel.Style.Name))
				}
			}
		}
		_, owned = ledger.Carriers[path]
		return nil
	})
	lines = append(notes, lines...)
	if sel.Note != "" && len(lines) == 0 && want == "" {
		lines = append(lines, fmt.Sprintf("%s: %s", filepath.Base(path), sel.Note))
	}
	return lines, owned, err
}

// styleCarrierStatus describes the carrier at path without changing anything.
// stale is true when a sync would change it.
func styleCarrierStatus(paths *config.Paths, p *Profile, path string) (line string, stale, occupied bool, err error) {
	file := filepath.Base(path)
	want, sel, ok := styleWant(p)
	if !ok {
		return "", false, false, nil // nothing a sync would change
	}
	ledger, err := loadCarrierLedger(paths)
	if errors.Is(err, errCorruptLedger) {
		return file + ": ownership unknown — the output-style ledger is not valid JSON; the next sync repairs it", true, false, nil
	}
	if err != nil {
		return "", false, false, err
	}
	rec, recorded := ledger.Carriers[path]
	action, err := carrierPlan(path, want, rec, recorded)
	if err != nil {
		return "", false, false, err
	}
	switch action {
	case carrierWrite:
		if _, statErr := os.Lstat(path); os.IsNotExist(statErr) {
			return fmt.Sprintf("%s missing (output style '%s')", file, sel.Style.Name), true, false, nil
		}
		return fmt.Sprintf("%s outdated (output style '%s')", file, sel.Style.Name), true, false, nil
	case carrierRemove:
		return fmt.Sprintf("%s left over (profile selects no linked output style)", file), true, false, nil
	case carrierForget:
		return "", false, false, nil
	case carrierEdited:
		return file + " (edited by hand)", false, true, nil
	case carrierOccupied:
		return file, false, true, nil
	case carrierAdopt, carrierNothing:
		if want != "" {
			return fmt.Sprintf("%s (output style '%s')", file, sel.Style.Name), false, false, nil
		}
	}
	return "", false, false, nil
}

// StyleCarrier is one carrier ccp still owns, with the hash that proves it.
type StyleCarrier struct {
	Path string
	hash string
}

// OwnedStyleCarriers returns the carriers that still hold exactly what ccp
// wrote, optionally only those whose group starts with prefix. A carrier
// edited by hand is not listed: it is the user's now.
func OwnedStyleCarriers(paths *config.Paths, prefix string) ([]StyleCarrier, error) {
	ledger, err := loadCarrierLedger(paths)
	if err != nil {
		return nil, err
	}
	var owned []StyleCarrier
	for _, path := range sortedKeys(ledger.Carriers) {
		rec := ledger.Carriers[path]
		if !strings.HasPrefix(rec.Group, prefix) {
			continue
		}
		if data, err := os.ReadFile(path); err == nil && hashOf(data) == rec.Hash {
			owned = append(owned, StyleCarrier{Path: path, hash: rec.Hash})
		}
	}
	return owned, nil
}

// RemoveStyleCarriers removes carriers listed earlier by OwnedStyleCarriers,
// re-checking each one's content right before removing it: 'ccp reset' lists
// them before ~/.ccp (and the ledger) is gone and removes them after. It
// returns the paths removed; a file changed since it was listed is kept.
func RemoveStyleCarriers(carriers []StyleCarrier) (removed, kept []string) {
	for _, c := range carriers {
		data, err := os.ReadFile(c.Path)
		switch {
		case os.IsNotExist(err):
			continue // already gone, e.g. a directory omp and pi share
		case err != nil || hashOf(data) != c.hash:
			kept = append(kept, c.Path)
		case os.Remove(c.Path) == nil:
			removed = append(removed, c.Path)
		default:
			kept = append(kept, c.Path)
		}
	}
	sort.Strings(removed)
	return removed, kept
}

// forgetStyleCarriers drops ledger entries for paths that no longer exist.
func forgetStyleCarriers(paths *config.Paths, removed []string) error {
	if len(removed) == 0 {
		return nil
	}
	_, err := updateCarrierLedger(paths, func(ledger *carrierLedger) error {
		for _, path := range removed {
			delete(ledger.Carriers, path)
		}
		return nil
	})
	return err
}
