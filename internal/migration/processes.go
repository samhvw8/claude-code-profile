package migration

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// ClaudeProcess is a running Claude Code process and the CLAUDE_CONFIG_DIR it
// was started with.
type ClaudeProcess struct {
	PID       int
	ConfigDir string // "" when unset
	EnvKnown  bool   // false when the environment could not be read
}

// ProcessLister finds running Claude Code processes.
type ProcessLister interface {
	ClaudeProcesses() ([]ClaudeProcess, error)
}

// NewProcessLister returns this machine's process lister: sysctl on macOS,
// /proc on Linux, nil elsewhere (callers must treat nil as "cannot tell").
func NewProcessLister() ProcessLister {
	switch runtime.GOOS {
	case "darwin":
		return newSysctlLister()
	case "linux":
		return procLister{root: "/proc"}
	}
	return nil
}

var jsRuntime = regexp.MustCompile(`^(node|nodejs|bun|deno)[0-9.]*$`)

// isClaudeProcess reports whether a process is Claude Code: a native "claude"
// binary (mise's .../claude/<ver>/claude included), a native install started
// through its resolved path (exec path under .../claude/versions/ or
// .../installs/claude/, such as ~/.local/share/claude/versions/2.1.77), or a
// JS runtime running the npm package anywhere in its arguments (node
// /usr/local/bin/claude, node --flag .../@anthropic-ai/claude-code/cli.js).
// execPath may be "" when unknown.
//
// A script started through its "#!/usr/bin/env node" shebang shows argv
// [/usr/bin/env, node, .../claude] right after exec: whatever follows env
// (options, NAME=value assignments) is skipped and the rest is checked the
// same way.
// Other wrappers (bash -c, npx) are not seen through.
func isClaudeProcess(execPath string, argv []string) bool {
	if strings.Contains(execPath, "/claude/versions/") || strings.Contains(execPath, "/installs/claude/") {
		return true
	}
	if len(argv) == 0 {
		return false
	}
	if filepath.Base(argv[0]) == "env" {
		// env's options differ by system and some take an argument (-u NAME,
		// -C DIR, -S "node --flag" as one shebang token), so its grammar is
		// not parsed: the arguments are split on spaces and every tail is
		// checked. A false match only makes the folder count as busy.
		var words []string
		for _, a := range argv[1:] {
			if v, ok := strings.CutPrefix(a, "--split-string="); ok {
				a = v
			}
			words = append(words, strings.Fields(a)...)
		}
		for i := range words {
			if isClaudeArgv(words[i:]) {
				return true
			}
		}
		return false
	}
	return isClaudeArgv(argv)
}

// isClaudeArgv is isClaudeProcess for an argv that is not started through
// env: it never recurses, so a long run of env words stays quadratic.
func isClaudeArgv(argv []string) bool {
	if filepath.Base(argv[0]) == "claude" {
		return true
	}
	if !jsRuntime.MatchString(filepath.Base(argv[0])) {
		return false
	}
	for _, a := range argv[1:] {
		if filepath.Base(a) == "claude" || strings.Contains(a, "@anthropic-ai/claude-code") {
			return true
		}
	}
	return false
}

// mayBeClaude reports whether a bare executable name could be Claude Code,
// for processes whose arguments cannot be read.
func mayBeClaude(comm string) bool {
	return comm == "claude" || jsRuntime.MatchString(comm)
}

func configDirOf(env []string) string {
	dir := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "CLAUDE_CONFIG_DIR="); ok {
			dir = v
		}
	}
	return dir
}

// procEntry is one process from the kernel's process table.
type procEntry struct {
	pid  int
	comm string // executable name, possibly truncated
}

// errProcessGone marks a process that exited while it was being read.
var errProcessGone = errors.New("process exited")

// sysctlLister reads every process's exact argv and environment from the
// kernel (macOS kern.procargs2), so paths with spaces stay intact.
type sysctlLister struct {
	procs    func() ([]procEntry, error)
	procargs func(pid int) ([]byte, error) // returns errProcessGone for ESRCH
}

func (l sysctlLister) ClaudeProcesses() ([]ClaudeProcess, error) {
	entries, err := l.procs()
	if err != nil {
		return nil, err
	}
	var out []ClaudeProcess
	for _, e := range entries {
		if e.pid <= 0 {
			continue
		}
		buf, err := l.procargs(e.pid)
		if errors.Is(err, errProcessGone) {
			continue
		}
		var exec string
		var argv, env []string
		if err == nil {
			exec, argv, env, err = parseProcargs2(buf)
		}
		if err != nil {
			// Unreadable (another user's process, or a malformed buffer): a
			// process that may be Claude counts as using every login.
			if mayBeClaude(e.comm) {
				out = append(out, ClaudeProcess{PID: e.pid})
			}
			continue
		}
		if isClaudeProcess(exec, argv) {
			out = append(out, ClaudeProcess{PID: e.pid, ConfigDir: configDirOf(env), EnvKnown: true})
		}
	}
	return out, nil
}

// parseProcargs2 decodes a kern.procargs2 buffer: int32 argc, the exec path
// and NUL padding, argc NUL-terminated arguments, then NUL-terminated
// environment strings up to an empty one or the end.
func parseProcargs2(buf []byte) (execPath string, argv, env []string, err error) {
	if len(buf) < 4 {
		return "", nil, nil, fmt.Errorf("procargs2: short buffer")
	}
	argc := int(binary.NativeEndian.Uint32(buf[:4]))
	rest := buf[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return "", nil, nil, fmt.Errorf("procargs2: no exec path")
	}
	execPath = string(rest[:i])
	rest = rest[i:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	next := func() (string, bool) {
		if len(rest) == 0 {
			return "", false
		}
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			s := string(rest)
			rest = nil
			return s, true
		}
		s := string(rest[:j])
		rest = rest[j+1:]
		return s, true
	}
	for range argc {
		s, ok := next()
		if !ok {
			return "", nil, nil, fmt.Errorf("procargs2: %d of %d arguments", len(argv), argc)
		}
		argv = append(argv, s)
	}
	for {
		s, ok := next()
		if !ok || s == "" {
			break
		}
		env = append(env, s)
	}
	return execPath, argv, env, nil
}

// procLister lists processes from /proc (Linux).
type procLister struct {
	root string
}

func (l procLister) ClaudeProcesses() ([]ClaudeProcess, error) {
	entries, err := os.ReadDir(l.root)
	if err != nil {
		return nil, err
	}
	var procs []ClaudeProcess
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join(l.root, e.Name())
		cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline"))
		if errors.Is(err, fs.ErrNotExist) {
			continue // exited
		}
		if err != nil {
			comm, cerr := os.ReadFile(filepath.Join(dir, "comm"))
			if cerr != nil || mayBeClaude(strings.TrimSpace(string(comm))) {
				procs = append(procs, ClaudeProcess{PID: pid})
			}
			continue
		}
		if len(cmdline) == 0 {
			continue // kernel thread or zombie
		}
		exe, _ := os.Readlink(filepath.Join(dir, "exe")) // unreadable for other users' processes
		if !isClaudeProcess(exe, strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")) {
			continue
		}
		environ, err := os.ReadFile(filepath.Join(dir, "environ"))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue // exited
		case err != nil:
			procs = append(procs, ClaudeProcess{PID: pid})
		default:
			procs = append(procs, ClaudeProcess{PID: pid, ConfigDir: configDirOf(strings.Split(string(environ), "\x00")), EnvKnown: true})
		}
	}
	return procs, nil
}
