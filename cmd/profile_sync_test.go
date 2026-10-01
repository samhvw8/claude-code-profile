package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/hub"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// Sync must keep (and restore) the member links of a linked bundle.
func TestSyncProfile_KeepsBundleMembers(t *testing.T) {
	paths, profileDir := setupDoctorTest(t)
	bundleDir := paths.BundleDir("kit")
	os.MkdirAll(filepath.Join(bundleDir, "agents"), 0755)
	os.WriteFile(filepath.Join(bundleDir, "agents", "helper.md"), []byte("# helper"), 0644)
	bundle := &hub.Bundle{Name: "kit", Version: "1.0.0", Members: hub.ComponentList{Agents: []string{"helper.md"}}}
	if err := bundle.Save(paths.BundlesDir()); err != nil {
		t.Fatal(err)
	}
	mgr := profile.NewManager(paths)
	if err := mgr.LinkHubBundle("p", "kit"); err != nil {
		t.Fatalf("LinkHubBundle: %v", err)
	}
	member := filepath.Join(profileDir, "agents", "helper.md")

	p, _ := mgr.Get("p")
	if err := syncProfile(paths, p, false); err != nil {
		t.Fatalf("syncProfile: %v", err)
	}
	if _, err := os.Stat(member); err != nil {
		t.Errorf("sync removed bundle member link: %v", err)
	}

	os.Remove(member)
	if err := syncProfile(paths, p, false); err != nil {
		t.Fatalf("syncProfile: %v", err)
	}
	if _, err := os.Stat(member); err != nil {
		t.Errorf("sync did not restore bundle member link: %v", err)
	}
}
