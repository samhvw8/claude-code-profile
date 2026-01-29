package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRollback_AddDir(t *testing.T) {
	r := NewRollback()

	r.AddDir("/path/to/dir1")
	r.AddDir("/path/to/dir2")

	if len(r.dirs) != 2 {
		t.Errorf("expected 2 dirs, got %d", len(r.dirs))
	}
}

func TestRollback_AddMove(t *testing.T) {
	r := NewRollback()

	r.AddMove("/new/path", "/original/path")
	r.AddMove("/new/path2", "/original/path2")

	if len(r.moves) != 2 {
		t.Errorf("expected 2 moves, got %d", len(r.moves))
	}
}

func TestRollback_Clear(t *testing.T) {
	r := NewRollback()
	r.AddDir("/path")
	r.AddMove("/new", "/old")

	r.Clear()

	if len(r.dirs) != 0 {
		t.Error("dirs should be empty after Clear")
	}
	if len(r.moves) != 0 {
		t.Error("moves should be empty after Clear")
	}
}

func TestRollback_Execute_Dirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directories
	dir1 := filepath.Join(tmpDir, "level1")
	dir2 := filepath.Join(dir1, "level2")
	if err := os.MkdirAll(dir2, 0755); err != nil {
		t.Fatal(err)
	}

	r := NewRollback()
	r.AddDir(dir1)
	r.AddDir(dir2)

	// Execute rollback - should remove in reverse order
	if err := r.Execute(); err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// Verify directories are removed
	if _, err := os.Stat(dir1); !os.IsNotExist(err) {
		t.Error("dir1 should be removed")
	}
}

func TestRollback_Execute_Moves(t *testing.T) {
	tmpDir := t.TempDir()

	// Create original file
	originalPath := filepath.Join(tmpDir, "original.txt")
	newPath := filepath.Join(tmpDir, "moved.txt")

	if err := os.WriteFile(originalPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Simulate a move operation
	if err := os.Rename(originalPath, newPath); err != nil {
		t.Fatal(err)
	}

	r := NewRollback()
	r.AddMove(newPath, originalPath)

	// Execute rollback - should restore original
	if err := r.Execute(); err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// Verify file is back at original location
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		t.Error("original file should be restored")
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Error("moved file should not exist")
	}
}

func TestRollback_Execute_ReverseOrder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create: dir1/file1 -> moved to dir2/file1
	// Rollback must restore file1 before removing dir1
	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")

	if err := os.MkdirAll(dir1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir2, 0755); err != nil {
		t.Fatal(err)
	}

	originalFile := filepath.Join(dir1, "file.txt")
	movedFile := filepath.Join(dir2, "file.txt")

	if err := os.WriteFile(originalFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(originalFile, movedFile); err != nil {
		t.Fatal(err)
	}

	r := NewRollback()
	// Order matters: dir created first, then file moved
	r.AddDir(dir2)
	r.AddMove(movedFile, originalFile)

	// Execute should: 1) restore file, 2) remove dir2
	if err := r.Execute(); err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// Verify
	if _, err := os.Stat(originalFile); os.IsNotExist(err) {
		t.Error("original file should be restored")
	}
	if _, err := os.Stat(dir2); !os.IsNotExist(err) {
		t.Error("dir2 should be removed")
	}
}

func TestRollback_Execute_EmptyIsNoOp(t *testing.T) {
	r := NewRollback()
	if err := r.Execute(); err != nil {
		t.Errorf("Execute on empty rollback should not error: %v", err)
	}
}

func TestRollback_Execute_ContinuesOnError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a real dir
	realDir := filepath.Join(tmpDir, "real")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := NewRollback()
	// Note: RemoveAll doesn't fail for nonexistent paths, so we add a dir that exists
	// The point is to verify all dirs are processed
	r.AddDir("/nonexistent/path/that/does/not/exist") // This won't error - RemoveAll is idempotent
	r.AddDir(realDir)

	// Execute - should process all dirs
	if err := r.Execute(); err != nil {
		t.Errorf("Execute should not error for nonexistent paths: %v", err)
	}

	// Real dir should be removed
	if _, statErr := os.Stat(realDir); !os.IsNotExist(statErr) {
		t.Error("real dir should be removed")
	}
}
