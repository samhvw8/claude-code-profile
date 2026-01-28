package hub

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestHub(t *testing.T) {
	h := New("/test/hub")

	if h.Path != "/test/hub" {
		t.Errorf("Path = %q, want %q", h.Path, "/test/hub")
	}

	if h.ItemCount() != 0 {
		t.Errorf("ItemCount() = %d, want 0", h.ItemCount())
	}
}

func TestHubItems(t *testing.T) {
	h := New("/test/hub")

	h.Items[config.HubSkills] = []Item{
		{Name: "skill1", Type: config.HubSkills, Path: "/test/hub/skills/skill1", IsDir: true},
		{Name: "skill2", Type: config.HubSkills, Path: "/test/hub/skills/skill2", IsDir: true},
	}

	if h.ItemCount() != 2 {
		t.Errorf("ItemCount() = %d, want 2", h.ItemCount())
	}

	if !h.HasItem(config.HubSkills, "skill1") {
		t.Error("HasItem(skills, skill1) = false, want true")
	}

	if h.HasItem(config.HubSkills, "nonexistent") {
		t.Error("HasItem(skills, nonexistent) = true, want false")
	}

	item := h.GetItem(config.HubSkills, "skill1")
	if item == nil {
		t.Fatal("GetItem() returned nil")
	}
	if item.Name != "skill1" {
		t.Errorf("item.Name = %q, want %q", item.Name, "skill1")
	}
}

func TestScanner(t *testing.T) {
	// Create test hub structure
	testDir := t.TempDir()
	hubDir := filepath.Join(testDir, "hub")

	// Create skills directory with items
	skillsDir := filepath.Join(hubDir, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "test-skill"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create hooks directory with a file
	hooksDir := filepath.Join(hubDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "pre-commit.sh"), []byte("#!/bin/bash"), 0755); err != nil {
		t.Fatal(err)
	}

	// Scan
	scanner := NewScanner()
	h, err := scanner.Scan(hubDir)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if h.ItemCount() != 2 {
		t.Errorf("ItemCount() = %d, want 2", h.ItemCount())
	}

	// Check skill
	skill := h.GetItem(config.HubSkills, "test-skill")
	if skill == nil {
		t.Error("skill not found")
	} else if !skill.IsDir {
		t.Error("skill.IsDir = false, want true")
	}

	// Check hook
	hook := h.GetItem(config.HubHooks, "pre-commit.sh")
	if hook == nil {
		t.Error("hook not found")
	} else if hook.IsDir {
		t.Error("hook.IsDir = true, want false")
	}
}

func TestScannerSkipsHiddenFiles(t *testing.T) {
	testDir := t.TempDir()
	skillsDir := filepath.Join(testDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create visible and hidden items
	if err := os.MkdirAll(filepath.Join(skillsDir, "visible"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner()
	h, err := scanner.Scan(testDir)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(h.GetItems(config.HubSkills)) != 1 {
		t.Errorf("expected 1 skill (hidden should be skipped), got %d", len(h.GetItems(config.HubSkills)))
	}
}
