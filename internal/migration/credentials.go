package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
)

// Claude Code keeps its OAuth token per config folder. On macOS it is a
// Keychain generic password whose service is "Claude Code-credentials" for
// the default folder (~/.claude) and "Claude Code-credentials-<8 hex>" for a
// CLAUDE_CONFIG_DIR, the hex being the start of sha256 of that path string.
// Elsewhere it is <config dir>/.credentials.json.

const keychainService = "Claude Code-credentials"

// ConfigDirHash returns the first 8 hex digits of sha256(dir), the suffix
// Claude Code appends to its Keychain service for a CLAUDE_CONFIG_DIR.
func ConfigDirHash(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return hex.EncodeToString(sum[:])[:8]
}

// KeychainServiceFor returns the Keychain service Claude Code uses for a
// config folder; global selects the default ~/.claude login.
func KeychainServiceFor(dir string, global bool) string {
	if global {
		return keychainService
	}
	return keychainService + "-" + ConfigDirHash(dir)
}

// LoginLocation names where Claude Code keeps the login of a non-default
// config folder on goos, for messages.
func LoginLocation(goos, dir string) string {
	if goos == "darwin" {
		return fmt.Sprintf("Keychain item %q", KeychainServiceFor(dir, false))
	}
	return filepath.Join(dir, ".credentials.json")
}

// LoginKey returns where Claude Code keeps the login of the config folder dir
// (a CLAUDE_CONFIG_DIR) on goos: a Keychain service on macOS, the credentials
// file elsewhere.
func LoginKey(goos, dir string) string {
	if goos == "darwin" {
		return KeychainServiceFor(dir, false)
	}
	return filepath.Join(dir, ".credentials.json")
}

// NewCredentialStore returns the store Claude Code keeps logins in on goos.
func NewCredentialStore(goos string) (CredentialStore, error) {
	switch goos {
	case "darwin":
		return NewKeychainStore(), nil
	case "linux":
		return FileStore{}, nil
	}
	return nil, fmt.Errorf("reading Claude Code logins is not supported on %s", goos)
}

// ClaudeLogin is the claude.ai OAuth login in a credential secret. The secret
// can also hold tokens of MCP servers (mcpOAuth), which are not a login.
type ClaudeLogin struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"` // Unix milliseconds
}

// ParseLogin returns the claude.ai login in secret; ok is false when the
// secret holds none (only MCP server tokens, say) or is not JSON.
func ParseLogin(secret []byte) (login ClaudeLogin, ok bool) {
	var s struct {
		Oauth *ClaudeLogin `json:"claudeAiOauth"`
	}
	if json.Unmarshal(secret, &s) != nil || s.Oauth == nil {
		return ClaudeLogin{}, false
	}
	if s.Oauth.AccessToken == "" && s.Oauth.RefreshToken == "" {
		return ClaudeLogin{}, false
	}
	return *s.Oauth, true
}

// HasLogin reports whether secret holds a claude.ai login. A secret that is
// not JSON counts as one: its layout is unknown, so it is never overwritten.
func HasLogin(secret []byte) bool {
	if !json.Valid(secret) {
		return len(bytes.TrimSpace(secret)) > 0
	}
	_, ok := ParseLogin(secret)
	return ok
}

// loginPart returns the compact claudeAiOauth value of a secret.
func loginPart(secret []byte) ([]byte, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(secret, &obj) != nil {
		return nil, false
	}
	raw, ok := obj["claudeAiOauth"]
	if !ok || string(raw) == "null" {
		return nil, false
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// sameLogin reports whether two secrets hold the same claude.ai login; other
// keys (MCP server tokens) may differ. Secrets that are not JSON objects must
// be equal byte for byte.
func sameLogin(a, b []byte) bool {
	la, okA := loginPart(a)
	lb, okB := loginPart(b)
	if okA && okB {
		return bytes.Equal(la, lb)
	}
	return bytes.Equal(a, b)
}

// mergeLogin returns src with the MCP server tokens of target added: target's
// own entry wins for a server both hold, being the newer one. Other keys come
// from src.
func mergeLogin(target, src []byte) ([]byte, error) {
	var t, s map[string]json.RawMessage
	if err := json.Unmarshal(target, &t); err != nil || t == nil {
		return nil, fmt.Errorf("the account's credential is not a JSON object")
	}
	if err := json.Unmarshal(src, &s); err != nil || s == nil {
		return nil, fmt.Errorf("the source credential is not a JSON object")
	}
	if tm, ok := t["mcpOAuth"]; ok {
		merged := map[string]json.RawMessage{}
		for _, raw := range []json.RawMessage{s["mcpOAuth"], tm} {
			var m map[string]json.RawMessage
			if len(raw) > 0 && json.Unmarshal(raw, &m) == nil {
				for k, v := range m {
					merged[k] = v
				}
			}
		}
		out, err := json.Marshal(merged)
		if err != nil {
			return nil, err
		}
		s["mcpOAuth"] = out
	}
	return json.Marshal(s)
}

// CredentialStore reads and writes one login secret by key: a Keychain
// service on macOS, a file path elsewhere. Delete removes the login of acct
// and returns a warning when another login under the same key remains.
type CredentialStore interface {
	Get(key string) (secret []byte, acct string, found bool, err error)
	Set(key, acct string, secret []byte) error
	Delete(key, acct string) (warning string, err error)
}

// KeychainInspector queries Keychain item attributes without reading secrets.
type KeychainInspector interface {
	Exists(service, acct string) (bool, error)
	ListServices(prefix string) ([]string, error)
}

// commandRunner runs a program with stdin and returns its stdout, stderr and
// exit code; err is set only when the program could not run.
type commandRunner func(stdin, name string, args ...string) (stdout, stderr string, code int, err error)

func execRunner(stdin, name string, args ...string) (string, string, int, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out.String(), errOut.String(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return "", "", -1, err
	}
	return out.String(), errOut.String(), 0, nil
}

// keychainNotFound is the exit code of security(1) for a missing item.
const keychainNotFound = 44

// securityBin is run by absolute path: the login token passes through it.
const securityBin = "/usr/bin/security"

// maxSecurityLine is the longest command line sent to "security -i" on stdin;
// its line buffer is 4096 bytes.
const maxSecurityLine = 4000

// maxKeychainDeletes caps the delete loop; security deletes one match per run.
const maxKeychainDeletes = 10

// KeychainStore is the macOS login Keychain through the security CLI. The
// secret never appears in argv: writes go through "security -i" on stdin.
type KeychainStore struct {
	run  commandRunner
	user string // the account Claude Code writes its items under
}

// NewKeychainStore returns a store backed by /usr/bin/security.
func NewKeychainStore() *KeychainStore { return &KeychainStore{run: execRunner, user: currentUser()} }

// currentUser is the login name Claude Code uses as the Keychain account.
func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

// Get reads the item of service the way Claude Code does: the one under the
// current user first, then, only if there is none, the first item of service
// under any account (reported in acct).
func (k *KeychainStore) Get(service string) ([]byte, string, bool, error) {
	if k.user != "" {
		secret, acct, found, err := k.find(service, "-a", k.user)
		if err != nil || found {
			return secret, acct, found, err
		}
	}
	return k.find(service)
}

func (k *KeychainStore) find(service string, extra ...string) ([]byte, string, bool, error) {
	args := append([]string{"find-generic-password", "-g"}, extra...)
	out, errOut, code, err := k.run("", securityBin, append(args, "-s", service)...)
	if err != nil {
		return nil, "", false, err
	}
	if code == keychainNotFound {
		return nil, "", false, nil
	}
	if code != 0 {
		return nil, "", false, fmt.Errorf("security find-generic-password -s %q: exit %d: %s", service, code, strings.TrimSpace(errOut))
	}
	var acct string
	for line := range strings.SplitSeq(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), `"acct"<blob>=`); ok {
			if acct, err = parseKeychainValue(v); err != nil {
				return nil, "", false, fmt.Errorf("account of %q: %w", service, err)
			}
		}
	}
	for line := range strings.SplitSeq(errOut, "\n") {
		if v, ok := strings.CutPrefix(line, "password: "); ok {
			s, err := parseKeychainValue(v)
			if err != nil {
				return nil, "", false, fmt.Errorf("password of %q: %w", service, err)
			}
			return []byte(s), acct, true, nil
		}
	}
	return nil, "", false, fmt.Errorf("security printed no password for %q", service)
}

// parseKeychainValue decodes a value as security -g prints it: "text",
// 0x<HEX>  "text" for non-printable data, or <NULL>.
func parseKeychainValue(v string) (string, error) {
	v = strings.TrimRight(v, "\r")
	switch {
	case v == "<NULL>" || v == "":
		return "", nil
	case strings.HasPrefix(v, "0x"):
		h, _, _ := strings.Cut(v[2:], " ")
		b, err := hex.DecodeString(h)
		if err != nil {
			return "", fmt.Errorf("bad hex value (%d bytes)", len(v)) // never echo the value
		}
		return string(b), nil
	case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
		return v[1 : len(v)-1], nil
	}
	return "", fmt.Errorf("unrecognized value (%d bytes)", len(v)) // never echo the value
}

func (k *KeychainStore) Set(service, acct string, secret []byte) error {
	for _, s := range []string{service, acct} {
		if strings.ContainsAny(s, "\"\\\n\r") {
			return fmt.Errorf("refusing Keychain value with quotes or newlines: %q", s)
		}
	}
	hexSecret := hex.EncodeToString(secret)
	line := fmt.Sprintf("add-generic-password -U -a \"%s\" -s \"%s\" -X %s\n", acct, service, hexSecret)
	// security -i reads each command line into a 4096-byte buffer, so a longer
	// line would be cut. Like Claude Code (which switches at 4032 bytes), a
	// line that does not fit goes through argv instead: the hex is then
	// visible to ps for the moment security runs, the trade-off Claude Code
	// itself makes for large logins.
	var out, errOut string
	var code int
	var err error
	if len(line) > maxSecurityLine {
		out, errOut, code, err = k.run("", securityBin, "add-generic-password", "-U", "-a", acct, "-s", service, "-X", hexSecret)
	} else {
		out, errOut, code, err = k.run(line, securityBin, "-i")
	}
	if err != nil {
		return err
	}
	// security -i exits 0 even when a command fails; it reports "returned N".
	// Only that line is quoted back: the rest could echo the hex secret.
	if code != 0 || strings.Contains(out+errOut, "returned") {
		status := fmt.Sprintf("exit %d", code)
		for l := range strings.SplitSeq(out+errOut, "\n") {
			if _, n, ok := strings.Cut(l, "returned "); ok {
				status = "returned " + strings.TrimSpace(n)
			}
		}
		return fmt.Errorf("security add-generic-password -s %q failed: %s", service, status)
	}
	return nil
}

// Delete removes every item of service and acct (security deletes one match
// per run), then reports an item of the same service under another account,
// which it leaves alone.
func (k *KeychainStore) Delete(service, acct string) (string, error) {
	for range maxKeychainDeletes {
		_, errOut, code, err := k.run("", securityBin, "delete-generic-password", "-s", service, "-a", acct)
		if err != nil {
			return "", err
		}
		if code == keychainNotFound {
			_, other, found, err := k.Get(service)
			if err != nil || !found {
				return "", err
			}
			return fmt.Sprintf("Keychain still holds %q for account %q; left as is", service, other), nil
		}
		if code != 0 {
			return "", fmt.Errorf("security delete-generic-password -s %q -a %q: exit %d: %s", service, acct, code, strings.TrimSpace(errOut))
		}
	}
	return "", fmt.Errorf("Keychain %q for account %q still present after %d deletes", service, acct, maxKeychainDeletes)
}

// Exists reports whether a generic password item exists for service (and acct
// when non-empty) using an attribute-only lookup (never -g or -w).
func (k *KeychainStore) Exists(service, acct string) (bool, error) {
	args := []string{"find-generic-password"}
	if acct != "" {
		args = append(args, "-a", acct)
	}
	args = append(args, "-s", service)
	_, errOut, code, err := k.run("", securityBin, args...)
	if err != nil {
		return false, err
	}
	if code == keychainNotFound {
		return false, nil
	}
	if code != 0 {
		return false, fmt.Errorf("security find-generic-password -s %q: exit %d: %s", service, code, strings.TrimSpace(errOut))
	}
	return true, nil
}

// ListServices lists generic password service names from an attribute-only
// keychain dump (never -d), filtered by prefix when non-empty.
func (k *KeychainStore) ListServices(prefix string) ([]string, error) {
	out, errOut, code, err := k.run("", securityBin, "dump-keychain")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("security dump-keychain: exit %d: %s", code, strings.TrimSpace(errOut))
	}
	seen := map[string]bool{}
	var services []string
	for line := range strings.SplitSeq(out, "\n") {
		v, ok := strings.CutPrefix(strings.TrimSpace(line), `"svce"<blob>=`)
		if !ok {
			continue
		}
		svc, err := parseKeychainValue(v)
		if err != nil || svc == "" {
			continue
		}
		if prefix != "" && !strings.HasPrefix(svc, prefix) {
			continue
		}
		if !seen[svc] {
			seen[svc] = true
			services = append(services, svc)
		}
	}
	sort.Strings(services)
	return services, nil
}

// FileStore keeps a login in a file (Claude Code's .credentials.json); the
// key is the file path.
type FileStore struct{}

func (FileStore) Get(path string) ([]byte, string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	return data, "", true, nil
}

func (FileStore) Set(path, _ string, secret []byte) error {
	return writeFileAtomic(path, secret, 0600)
}

func (FileStore) Delete(path, _ string) (string, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return "", nil
}

// writeFileAtomic writes data through a temp file and rename, so a crash
// leaves either the old file or the new one. A symlinked path is written at
// its target, keeping the link.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".ccp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
