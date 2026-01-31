package symlink

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager(t *testing.T) {
	testDir := t.TempDir()
	mgr := New()

	// Create a target file
	targetFile := filepath.Join(testDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink
	linkPath := filepath.Join(testDir, "link.txt")
	if err := mgr.Create(linkPath, targetFile); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Check IsSymlink
	isLink, err := mgr.IsSymlink(linkPath)
	if err != nil {
		t.Fatalf("IsSymlink() error: %v", err)
	}
	if !isLink {
		t.Error("IsSymlink() = false, want true")
	}

	// Check Info
	info, err := mgr.Info(linkPath)
	if err != nil {
		t.Fatalf("Info() error: %v", err)
	}
	if !info.Exists {
		t.Error("info.Exists = false, want true")
	}
	if !info.IsSymlink {
		t.Error("info.IsSymlink = false, want true")
	}
	if info.IsBroken {
		t.Error("info.IsBroken = true, want false")
	}

	// Check Validate
	valid, err := mgr.Validate(linkPath, targetFile)
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
	if !valid {
		t.Error("Validate() = false, want true")
	}

	// Check Validate with wrong target
	wrongTarget := filepath.Join(testDir, "other.txt")
	valid, err = mgr.Validate(linkPath, wrongTarget)
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
	if valid {
		t.Error("Validate(wrong target) = true, want false")
	}

	// Check ReadLink - now returns relative path
	target, err := mgr.ReadLink(linkPath)
	if err != nil {
		t.Fatalf("ReadLink() error: %v", err)
	}
	// Resolve relative path and compare
	resolvedTarget := target
	if !filepath.IsAbs(target) {
		resolvedTarget = filepath.Join(filepath.Dir(linkPath), target)
	}
	absResolved, _ := filepath.Abs(resolvedTarget)
	absExpected, _ := filepath.Abs(targetFile)
	if absResolved != absExpected {
		t.Errorf("ReadLink() resolved to %q, want %q", absResolved, absExpected)
	}

	// Remove
	if err := mgr.Remove(linkPath); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	// Verify removed
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("link still exists after Remove()")
	}
}

func TestManagerBrokenSymlink(t *testing.T) {
	testDir := t.TempDir()
	mgr := New()

	// Create symlink to non-existent target
	linkPath := filepath.Join(testDir, "broken-link")
	nonexistentTarget := filepath.Join(testDir, "nonexistent")

	if err := os.Symlink(nonexistentTarget, linkPath); err != nil {
		t.Fatal(err)
	}

	info, err := mgr.Info(linkPath)
	if err != nil {
		t.Fatalf("Info() error: %v", err)
	}

	if !info.Exists {
		t.Error("info.Exists = false, want true (link exists)")
	}
	if !info.IsSymlink {
		t.Error("info.IsSymlink = false, want true")
	}
	if !info.IsBroken {
		t.Error("info.IsBroken = false, want true")
	}
}

func TestManagerSwap(t *testing.T) {
	testDir := t.TempDir()
	mgr := New()

	// Create two targets
	target1 := filepath.Join(testDir, "target1")
	target2 := filepath.Join(testDir, "target2")
	if err := os.MkdirAll(target1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target2, 0755); err != nil {
		t.Fatal(err)
	}

	// Create initial symlink
	linkPath := filepath.Join(testDir, "link")
	if err := mgr.Create(linkPath, target1); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Verify initial target
	valid, _ := mgr.Validate(linkPath, target1)
	if !valid {
		t.Error("initial link doesn't point to target1")
	}

	// Swap to target2
	if err := mgr.Swap(linkPath, target2); err != nil {
		t.Fatalf("Swap() error: %v", err)
	}

	// Verify new target
	valid, _ = mgr.Validate(linkPath, target2)
	if !valid {
		t.Error("swapped link doesn't point to target2")
	}
}

func TestManagerInfoNonexistent(t *testing.T) {
	testDir := t.TempDir()
	mgr := New()

	nonexistent := filepath.Join(testDir, "nonexistent")
	info, err := mgr.Info(nonexistent)
	if err != nil {
		t.Fatalf("Info() error: %v", err)
	}

	if info.Exists {
		t.Error("info.Exists = true, want false")
	}
}
