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

var jsRuntime = regexp.MustCompile(`^(node|nodejs|bun|deno)[0-9.]*$`)

// isClaudeArgv reports whether a command line is Claude Code: a native
// "claude" binary (mise's .../claude/<ver>/claude included), or a JS runtime
// running the npm package (node /usr/local/bin/claude, node .../@anthropic-ai/claude-code/cli.js).
func isClaudeArgv(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	if filepath.Base(argv[0]) == "claude" {
		return true
	}
	if !jsRuntime.MatchString(filepath.Base(argv[0])) {
		return false
	}
	for _, a := range argv[1:min(len(argv), 3)] {
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
		var argv, env []string
		if err == nil {
			argv, env, err = parseProcargs2(buf)
		}
		if err != nil {
			// Unreadable (another user's process, or a malformed buffer): a
			// process that may be Claude counts as using every login.
			if mayBeClaude(e.comm) {
				out = append(out, ClaudeProcess{PID: e.pid})
			}
			continue
		}
		if isClaudeArgv(argv) {
			out = append(out, ClaudeProcess{PID: e.pid, ConfigDir: configDirOf(env), EnvKnown: true})
		}
	}
	return out, nil
}

// parseProcargs2 decodes a kern.procargs2 buffer: int32 argc, the exec path
// and NUL padding, argc NUL-terminated arguments, then NUL-terminated
// environment strings up to an empty one or the end.
func parseProcargs2(buf []byte) (argv, env []string, err error) {
	if len(buf) < 4 {
		return nil, nil, fmt.Errorf("procargs2: short buffer")
	}
	argc := int(binary.NativeEndian.Uint32(buf[:4]))
	rest := buf[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil, nil, fmt.Errorf("procargs2: no exec path")
	}
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
			return nil, nil, fmt.Errorf("procargs2: %d of %d arguments", len(argv), argc)
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
	return argv, env, nil
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
		if !isClaudeArgv(strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")) {
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
