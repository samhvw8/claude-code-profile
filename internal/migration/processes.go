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

// AgyProcess is a running process that carries an --app_data_dir flag, or a
// process whose arguments could not be read and whose executable name may be
// agy.
type AgyProcess struct {
	PID        int
	AppDataDir string // value of --app_data_dir / -app_data_dir, "" when unknown
	ArgsKnown  bool   // false when argv could not be read
}

// ProcessLister finds running Claude Code and agy processes.
type ProcessLister interface {
	ClaudeProcesses() ([]ClaudeProcess, error)
	AgyProcesses() ([]AgyProcess, error)
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

// mayBeAgy reports whether a bare executable name could be agy, for processes
// whose arguments cannot be read.
func mayBeAgy(comm string) bool {
	return comm == "agy" || strings.HasPrefix(comm, "antigravity")
}

// appDataDirOf extracts the value of --app_data_dir or -app_data_dir from argv
// (both --flag=value and --flag value forms, stopping at "--"), matching
// cmd.hasAppDataDirFlag. When the flag is repeated, the last value wins.
func appDataDirOf(argv []string) (string, bool) {
	if len(argv) < 2 {
		return "", false
	}
	args := argv[1:]
	val := ""
	found := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if v, ok := strings.CutPrefix(a, "--app_data_dir="); ok {
			val, found = v, true
		} else if v, ok := strings.CutPrefix(a, "-app_data_dir="); ok {
			val, found = v, true
		} else if (a == "--app_data_dir" || a == "-app_data_dir") && i+1 < len(args) {
			i++
			val, found = args[i], true
		}
	}
	return val, found
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

// sysctlProc is one process as the kernel reports it; readable is false when
// its arguments could not be read or parsed (another user's process, or a
// malformed buffer).
type sysctlProc struct {
	pid        int
	comm, exec string
	argv, env  []string
	readable   bool
}

// read walks the process table once for both ClaudeProcesses and AgyProcesses.
func (l sysctlLister) read() ([]sysctlProc, error) {
	entries, err := l.procs()
	if err != nil {
		return nil, err
	}
	var out []sysctlProc
	for _, e := range entries {
		if e.pid <= 0 {
			continue
		}
		buf, err := l.procargs(e.pid)
		if errors.Is(err, errProcessGone) {
			continue
		}
		p := sysctlProc{pid: e.pid, comm: e.comm}
		if err == nil {
			p.exec, p.argv, p.env, err = parseProcargs2(buf)
		}
		p.readable = err == nil
		out = append(out, p)
	}
	return out, nil
}

func (l sysctlLister) ClaudeProcesses() ([]ClaudeProcess, error) {
	procs, err := l.read()
	if err != nil {
		return nil, err
	}
	var out []ClaudeProcess
	for _, p := range procs {
		if !p.readable {
			// A process that may be Claude counts as using every login.
			if mayBeClaude(p.comm) {
				out = append(out, ClaudeProcess{PID: p.pid})
			}
			continue
		}
		if isClaudeProcess(p.exec, p.argv) {
			out = append(out, ClaudeProcess{PID: p.pid, ConfigDir: configDirOf(p.env), EnvKnown: true})
		}
	}
	return out, nil
}

func (l sysctlLister) AgyProcesses() ([]AgyProcess, error) {
	procs, err := l.read()
	if err != nil {
		return nil, err
	}
	var out []AgyProcess
	for _, p := range procs {
		if !p.readable {
			if mayBeAgy(p.comm) {
				out = append(out, AgyProcess{PID: p.pid})
			}
			continue
		}
		if dir, ok := appDataDirOf(p.argv); ok {
			out = append(out, AgyProcess{PID: p.pid, AppDataDir: dir, ArgsKnown: true})
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

// procCmdline is one /proc entry: its argv, or, when cmdline could not be
// read, its executable name (commKnown false when that failed too).
type procCmdline struct {
	pid       int
	dir       string
	argv      []string
	readable  bool
	comm      string
	commKnown bool
}

// read walks /proc once for both ClaudeProcesses and AgyProcesses.
func (l procLister) read() ([]procCmdline, error) {
	entries, err := os.ReadDir(l.root)
	if err != nil {
		return nil, err
	}
	var out []procCmdline
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
			out = append(out, procCmdline{pid: pid, dir: dir, comm: strings.TrimSpace(string(comm)), commKnown: cerr == nil})
			continue
		}
		if len(cmdline) == 0 {
			continue // kernel thread or zombie
		}
		out = append(out, procCmdline{pid: pid, dir: dir, readable: true,
			argv: strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")})
	}
	return out, nil
}

func (l procLister) ClaudeProcesses() ([]ClaudeProcess, error) {
	cmds, err := l.read()
	if err != nil {
		return nil, err
	}
	var procs []ClaudeProcess
	for _, c := range cmds {
		if !c.readable {
			if !c.commKnown || mayBeClaude(c.comm) {
				procs = append(procs, ClaudeProcess{PID: c.pid})
			}
			continue
		}
		exe, _ := os.Readlink(filepath.Join(c.dir, "exe")) // unreadable for other users' processes
		if !isClaudeProcess(exe, c.argv) {
			continue
		}
		environ, err := os.ReadFile(filepath.Join(c.dir, "environ"))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue // exited
		case err != nil:
			procs = append(procs, ClaudeProcess{PID: c.pid})
		default:
			procs = append(procs, ClaudeProcess{PID: c.pid, ConfigDir: configDirOf(strings.Split(string(environ), "\x00")), EnvKnown: true})
		}
	}
	return procs, nil
}

func (l procLister) AgyProcesses() ([]AgyProcess, error) {
	cmds, err := l.read()
	if err != nil {
		return nil, err
	}
	var procs []AgyProcess
	for _, c := range cmds {
		if !c.readable {
			if !c.commKnown || mayBeAgy(c.comm) {
				procs = append(procs, AgyProcess{PID: c.pid})
			}
			continue
		}
		if appDir, ok := appDataDirOf(c.argv); ok {
			procs = append(procs, AgyProcess{PID: c.pid, AppDataDir: appDir, ArgsKnown: true})
		}
	}
	return procs, nil
}
