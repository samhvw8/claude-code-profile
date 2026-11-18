package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// accountShimDir is the project folder 'ccp use --accounts' puts on PATH.
const accountShimDir = ".ccp/bin"

// accountShim is the claude shim 'ccp use --accounts' writes. It names no
// profile: 'ccp run --account auto' takes the folder's profile from
// CLAUDE_CONFIG_DIR, so a later 'ccp use <other>' needs no rewrite.
//
// The real claude comes from 'mise which', never a mise shim: a mise shim
// reapplies the folder's mise.toml and resets CLAUDE_CONFIG_DIR to the
// profile, so the account would be dropped.
const accountShim = `#!/bin/sh
# Written by 'ccp use <profile> --accounts'. Do not edit; run it again instead.
# claude in this folder runs on the account of the folder's profile with the
# most plan limit left ('ccp account' lists them). A session already on an
# account keeps it; with no account logged in, claude runs on the profile.
here=$(cd "$(dirname "$0")" && pwd)
real=$(mise which claude 2>/dev/null)
if [ -z "$real" ]; then
	# Not installed with mise: the next claude on PATH, skipping this folder
	# and mise shims (see above).
	real=$(PATH=$(printf '%s\n' "$PATH" | tr : '\n' | grep -v -x -F "$here" | grep -v '/mise/shims$' | paste -s -d : -) command -v claude)
fi
if [ -z "$real" ]; then
	echo "claude: not found on PATH (ccp shim in $here)" >&2
	exit 127
fi
exec ccp run --account auto --or-profile -- "$real" "$@"
`

// installAccountShim writes .ccp/bin/claude in the current folder, puts
// .ccp/bin on PATH through mise.toml (or .envrc), and keeps .ccp/ out of git
// through .git/info/exclude, which is local and never committed.
func installAccountShim() error {
	if err := os.MkdirAll(accountShimDir, 0o755); err != nil {
		return err
	}
	shim := filepath.Join(accountShimDir, "claude")
	if err := os.WriteFile(shim, []byte(accountShim), 0o755); err != nil {
		return err
	}
	if err := os.Chmod(shim, 0o755); err != nil {
		return err
	}
	switch {
	case fileExists("mise.toml"):
		if err := ensureMisePath("mise.toml"); err != nil {
			return err
		}
	case fileExists(".envrc"):
		if err := ensureEnvrcPath(".envrc"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("--accounts needs mise.toml or .envrc to put %s on PATH; create one and run again", accountShimDir)
	}
	if err := excludeFromGit(".ccp/"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not add .ccp/ to .git/info/exclude: %v\n", err)
	}
	fmt.Printf("Wrote %s: claude here now picks the profile's account with the most limit left\n", shim)
	fmt.Println("Add accounts with 'ccp account add <name>' and log in once with 'ccp run --account <name>'")
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const misePathEntry = `"{{config_root}}/.ccp/bin"`

// ensureMisePath adds .ccp/bin to the [env] _.path of a mise.toml: a new
// _.path line, or the first entry of an existing one-line array or string.
// Other lines stay as they were.
func ensureMisePath(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	section, envAt := "", -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && !strings.HasPrefix(t, "[[") && strings.HasSuffix(t, "]") && !strings.Contains(t, "=") {
			section = t
			if t == "[env]" {
				envAt = i
			}
			continue
		}
		if section != "[env]" || !(strings.HasPrefix(t, "_.path ") || strings.HasPrefix(t, "_.path=")) {
			continue
		}
		if strings.Contains(t, ".ccp/bin") {
			return nil
		}
		value := strings.TrimSpace(t[strings.Index(t, "=")+1:])
		var updated string
		switch {
		case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
			inner := strings.TrimSpace(value[1 : len(value)-1])
			if inner == "" {
				updated = "[" + misePathEntry + "]"
			} else {
				updated = "[" + misePathEntry + ", " + inner + "]"
			}
		case strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'"):
			updated = "[" + misePathEntry + ", " + value + "]"
		default:
			return fmt.Errorf("%s: cannot add %s to this _.path; add %s to it by hand", path, accountShimDir, misePathEntry)
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + "_.path = " + updated
		return writeLines(path, lines)
	}
	entry := "_.path = [" + misePathEntry + "]"
	if envAt < 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "[env]", entry)
	} else {
		lines = append(lines[:envAt+1], append([]string{entry}, lines[envAt+1:]...)...)
	}
	return writeLines(path, lines)
}

// ensureEnvrcPath adds "PATH_add .ccp/bin" to an .envrc once.
func ensureEnvrcPath(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	line := "PATH_add " + accountShimDir
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == line {
			return nil
		}
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content+line+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Println("Run 'direnv allow' to apply changes")
	return nil
}

func writeLines(path string, lines []string) error {
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// excludeFromGit adds pattern to the repository's info/exclude when the
// current folder is inside a git work tree; elsewhere it does nothing.
func excludeFromGit(pattern string) error {
	out, err := exec.Command("git", "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		return nil // not a git work tree, or no git
	}
	exclude := strings.TrimSpace(string(out))
	data, err := os.ReadFile(exclude)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == pattern {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(exclude, []byte(content+pattern+"\n"), 0o644)
}
