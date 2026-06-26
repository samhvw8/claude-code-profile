package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundleSaveAndLoad(t *testing.T) {
	bundlesDir := t.TempDir()

	want := &Bundle{
		Name:        "impeccable",
		Description: "Design-aware coding bundle",
		Version:     "1.0.0",
		Members: ComponentList{
			Skills: []string{"impeccable"},
			Agents: []string{"impeccable"},
			Hooks:  []string{"impeccable"},
		},
	}

	if err := want.Save(bundlesDir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// bundle.yaml must land inside hub/bundles/<name>/
	if _, err := os.Stat(filepath.Join(bundlesDir, "impeccable", BundleManifestFile)); err != nil {
		t.Fatalf("manifest not written: %v", err)
	}

	got, err := LoadBundle(bundlesDir, "impeccable")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}

	if got.Name != want.Name || got.Description != want.Description || got.Version != want.Version {
		t.Errorf("metadata mismatch: got %+v want %+v", got, want)
	}
	if len(got.Members.Skills) != 1 || got.Members.Skills[0] != "impeccable" {
		t.Errorf("skills mismatch: got %v", got.Members.Skills)
	}
	if len(got.Members.Agents) != 1 || len(got.Members.Hooks) != 1 {
		t.Errorf("member counts mismatch: %+v", got.Members)
	}
}

func TestLoadBundleNameFallback(t *testing.T) {
	bundlesDir := t.TempDir()
	dir := filepath.Join(bundlesDir, "named-by-dir")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// manifest with no name field
	if err := os.WriteFile(filepath.Join(dir, BundleManifestFile), []byte("description: x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	b, err := LoadBundle(bundlesDir, "named-by-dir")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if b.Name != "named-by-dir" {
		t.Errorf("expected name to fall back to dir name, got %q", b.Name)
	}
}

func TestListBundles(t *testing.T) {
	bundlesDir := t.TempDir()

	for _, name := range []string{"alpha", "beta"} {
		b := &Bundle{Name: name, Members: ComponentList{Skills: []string{name}}}
		if err := b.Save(bundlesDir); err != nil {
			t.Fatal(err)
		}
	}
	// a non-bundle dir (no manifest) and a hidden dir must be skipped
	if err := os.MkdirAll(filepath.Join(bundlesDir, "not-a-bundle"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bundlesDir, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}

	bundles, err := ListBundles(bundlesDir)
	if err != nil {
		t.Fatalf("ListBundles: %v", err)
	}
	if len(bundles) != 2 {
		t.Fatalf("expected 2 bundles, got %d", len(bundles))
	}
}

func TestListBundlesMissingDir(t *testing.T) {
	bundles, err := ListBundles(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected no error for missing dir, got %v", err)
	}
	if bundles != nil {
		t.Errorf("expected nil bundles, got %v", bundles)
	}
}
