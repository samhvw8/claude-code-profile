package cmd

import (
	"os"
	"strings"
	"testing"
)

// omp --profile must not silently ignore a name: ccp resolves an omp named
// profile through the environment, so a rejected flag would otherwise write into
// the default tree while the user asked for another one.
func TestOmpProfileFlagValidation(t *testing.T) {
	t.Setenv("OMP_PROFILE", "from-env")
	t.Cleanup(func() { ompProfile = "" })

	ompProfile = "../evil"
	if err := ompCmd.PersistentPreRunE(ompCmd, nil); err == nil || !strings.Contains(err.Error(), "invalid omp profile") {
		t.Fatalf("PersistentPreRunE(--profile ../evil) error = %v, want an invalid-profile error", err)
	}
	if got := os.Getenv("OMP_PROFILE"); got != "from-env" {
		t.Errorf("OMP_PROFILE = %q, want the environment untouched after a rejected flag", got)
	}

	ompProfile = "work"
	if err := ompCmd.PersistentPreRunE(ompCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE(--profile work) error: %v", err)
	}
	if got := os.Getenv("OMP_PROFILE"); got != "work" {
		t.Errorf("OMP_PROFILE = %q, want work", got)
	}

	// An explicit 'default' selects the default tree, so it must clear a named
	// profile from the environment rather than keep writing into it.
	ompProfile = "default"
	if err := ompCmd.PersistentPreRunE(ompCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE(--profile default) error: %v", err)
	}
	if got, ok := os.LookupEnv("OMP_PROFILE"); ok {
		t.Errorf("OMP_PROFILE = %q, want unset", got)
	}
}
