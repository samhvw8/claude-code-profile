package migration

import (
	"fmt"
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

// isClaudeArgv0 reports whether argv[0] is Claude Code: "claude", or a path
// ending in /claude such as mise's .../installs/claude/<ver>/claude.
func isClaudeArgv0(argv0 string) bool {
	return filepath.Base(argv0) == "claude"
}

// psLister lists processes with ps(1) (macOS): one ps for the command lines,
// then one per Claude process for its environment.
type psLister struct {
	run commandRunner
}

var envVarStart = regexp.MustCompile(` [A-Za-z_][A-Za-z0-9_]*=`)

func (l psLister) ClaudeProcesses() ([]ClaudeProcess, error) {
	out, errOut, code, err := l.run("", "ps", "-Axww", "-o", "pid=,command=")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("ps failed: %s", strings.TrimSpace(errOut))
	}
	var procs []ClaudeProcess
	for line := range strings.SplitSeq(out, "\n") {
		pidStr, command, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		command = strings.TrimSpace(command)
		argv0, _, _ := strings.Cut(command, " ")
		if !isClaudeArgv0(argv0) {
			continue
		}
		p := ClaudeProcess{PID: pid}
		envOut, _, code, err := l.run("", "ps", "eww", "-o", "command=", "-p", strconv.Itoa(pid))
		envOut = strings.TrimSpace(envOut)
		if err == nil && code == 0 && len(envOut) > len(command) && strings.HasPrefix(envOut, command) {
			p.EnvKnown = true
			p.ConfigDir = envValue(envOut[len(command):], "CLAUDE_CONFIG_DIR")
		} else if code != 0 && envOut == "" {
			continue // exited between the two ps calls
		}
		procs = append(procs, p)
	}
	return procs, nil
}

// envValue finds KEY=VALUE in ps's environment text; a value runs until the
// next " NAME=" or the end, so paths with spaces survive.
func envValue(env, key string) string {
	env = " " + strings.TrimSpace(env)
	i := strings.Index(env, " "+key+"=")
	if i < 0 {
		return ""
	}
	rest := env[i+len(key)+2:]
	if loc := envVarStart.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	return rest
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
		cmdline, err := os.ReadFile(filepath.Join(l.root, e.Name(), "cmdline"))
		if err != nil || len(cmdline) == 0 {
			continue
		}
		argv0, _, _ := strings.Cut(string(cmdline), "\x00")
		if !isClaudeArgv0(argv0) {
			continue
		}
		p := ClaudeProcess{PID: pid}
		if environ, err := os.ReadFile(filepath.Join(l.root, e.Name(), "environ")); err == nil {
			p.EnvKnown = true
			for kv := range strings.SplitSeq(string(environ), "\x00") {
				if v, ok := strings.CutPrefix(kv, "CLAUDE_CONFIG_DIR="); ok {
					p.ConfigDir = v
				}
			}
		}
		procs = append(procs, p)
	}
	return procs, nil
}
