package migration

import (
	"errors"
	"slices"
	"testing"
)

func TestKeychainStore_ExistsAttributeOnly(t *testing.T) {
	var gotArgs []string
	k := &KeychainStore{
		run: func(_ string, name string, args ...string) (string, string, int, error) {
			if name != securityBin {
				t.Fatalf("binary = %q, want %q", name, securityBin)
			}
			gotArgs = args
			return `"svce"<blob>="gemini"` + "\n", "", 0, nil
		},
	}

	ok, err := k.Exists("gemini", "antigravity")
	if err != nil || !ok {
		t.Fatalf("Exists = %v, %v; want true, nil", ok, err)
	}
	for _, flag := range []string{"-g", "-w"} {
		if slices.Contains(gotArgs, flag) {
			t.Fatalf("Exists passed secret-reading flag %q in %v", flag, gotArgs)
		}
	}
	want := []string{"find-generic-password", "-a", "antigravity", "-s", "gemini"}
	if !slices.Equal(gotArgs, want) {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}

	// Exit 44 (not found) returns false, nil.
	k.run = func(string, string, ...string) (string, string, int, error) {
		return "", "SecKeychainSearchCopyNext: The specified item could not be found in the keychain.", keychainNotFound, nil
	}
	if ok, err := k.Exists("gemini", "antigravity"); err != nil || ok {
		t.Errorf("not found = %v, %v; want false, nil", ok, err)
	}

	// Other non-zero exit returns an error.
	k.run = func(string, string, ...string) (string, string, int, error) {
		return "", "denied", 36, nil
	}
	if ok, err := k.Exists("gemini", "antigravity"); err == nil || ok {
		t.Errorf("error = %v, %v; want false, error", ok, err)
	}

	// Runner error propagates.
	k.run = func(string, string, ...string) (string, string, int, error) {
		return "", "", -1, errors.New("boom")
	}
	if ok, err := k.Exists("gemini", "antigravity"); err == nil || ok {
		t.Errorf("runner error = %v, %v; want false, error", ok, err)
	}
}

func TestKeychainStore_ListServicesAttributeOnly(t *testing.T) {
	var gotArgs []string
	dump := `keychain: "/Users/test/Library/Keychains/login.keychain-db"
version: 512
class: "genp"
attributes:
    "acct"<blob>="test"
    "svce"<blob>="Claude Code-credentials"
keychain: "/Users/test/Library/Keychains/login.keychain-db"
class: "genp"
attributes:
    "acct"<blob>="test"
    "svce"<blob>="Claude Code-credentials-b2c3d4e5"
class: "genp"
attributes:
    "acct"<blob>="other"
    "svce"<blob>="Claude Code-credentials-a15378a4"
class: "genp"
attributes:
    "acct"<blob>="dup"
    "svce"<blob>=0x436c6175646520436f64652d63726564656e7469616c732d6131353337386134  "Claude Code-credentials-a15378a4"
class: "genp"
attributes:
    "svce"<blob>="unrelated-item"
class: "genp"
attributes:
    "svce"<blob>=<unparseable
`
	k := &KeychainStore{
		run: func(_ string, name string, args ...string) (string, string, int, error) {
			if name != securityBin {
				t.Fatalf("binary = %q, want %q", name, securityBin)
			}
			gotArgs = args
			return dump, "", 0, nil
		},
	}

	got, err := k.ListServices("Claude Code-credentials-")
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"-d", "-g", "-w"} {
		if slices.Contains(gotArgs, flag) {
			t.Fatalf("ListServices passed %q in %v", flag, gotArgs)
		}
	}
	want := []string{"Claude Code-credentials-a15378a4", "Claude Code-credentials-b2c3d4e5"}
	if !slices.Equal(got, want) {
		t.Errorf("ListServices = %v, want %v", got, want)
	}

	k.run = func(string, string, ...string) (string, string, int, error) {
		return "", "locked", 36, nil
	}
	if _, err := k.ListServices("Claude Code-credentials-"); err == nil {
		t.Error("expected error on non-zero dump-keychain exit")
	}
}
