package claudemd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseImports(t *testing.T) {
	tmpDir := t.TempDir()

	content := `# My CLAUDE.md

Some instructions here.

@principles/delegation-protocol.md
@principles/cognitive-framework.md
@principles/se.md

More text.
`
	path := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	imports, err := ParseImports(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"principles/delegation-protocol.md",
		"principles/cognitive-framework.md",
		"principles/se.md",
	}

	if len(imports) != len(expected) {
		t.Fatalf("expected %d imports, got %d: %v", len(expected), len(imports), imports)
	}

	for i, exp := range expected {
		if imports[i] != exp {
			t.Errorf("import[%d] = %q, want %q", i, imports[i], exp)
		}
	}
}

func TestParseImports_SkipsCodeBlocks(t *testing.T) {
	tmpDir := t.TempDir()

	content := "# Test\n\n```\n@inside/code-block.md\n```\n\n@real/import.md\n"
	path := filepath.Join(tmpDir, "CLAUDE.md")
	os.WriteFile(path, []byte(content), 0644)

	imports, err := ParseImports(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(imports) != 1 || imports[0] != "real/import.md" {
		t.Errorf("expected [real/import.md], got %v", imports)
	}
}

func TestParseImports_SkipsAnnotations(t *testing.T) {
	tmpDir := t.TempDir()

	content := "@Composable\n@Module\n@principles/se.md\n"
	path := filepath.Join(tmpDir, "CLAUDE.md")
	os.WriteFile(path, []byte(content), 0644)

	imports, err := ParseImports(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(imports) != 1 || imports[0] != "principles/se.md" {
		t.Errorf("expected [principles/se.md], got %v", imports)
	}
}

func TestParseImports_Deduplicates(t *testing.T) {
	tmpDir := t.TempDir()

	content := "@principles/se.md\n@principles/se.md\n"
	path := filepath.Join(tmpDir, "CLAUDE.md")
	os.WriteFile(path, []byte(content), 0644)

	imports, err := ParseImports(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(imports) != 1 {
		t.Errorf("expected 1 import (deduped), got %d", len(imports))
	}
}

func TestParseImports_NonExistentFile(t *testing.T) {
	imports, err := ParseImports("/nonexistent/CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}
	if imports != nil {
		t.Errorf("expected nil for nonexistent file, got %v", imports)
	}
}

func TestResolveImports(t *testing.T) {
	imports := []string{
		"principles/delegation-protocol.md",
		"principles/cognitive-framework.md",
		"principles/se.md",
		"references/api.md",
	}

	dirs := ResolveImports(imports)

	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d: %v", len(dirs), dirs)
	}

	has := make(map[string]bool)
	for _, d := range dirs {
		has[d] = true
	}
	if !has["principles"] {
		t.Error("expected 'principles' in dirs")
	}
	if !has["references"] {
		t.Error("expected 'references' in dirs")
	}
}

func TestResolveImports_TopLevelFiles(t *testing.T) {
	imports := []string{"standalone.md"}

	dirs := ResolveImports(imports)

	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs for top-level file, got %v", dirs)
	}
}

func TestLinkedDirs(t *testing.T) {
	tmpDir := t.TempDir()

	content := "@principles/se.md\n@references/api.md\n"
	path := filepath.Join(tmpDir, "CLAUDE.md")
	os.WriteFile(path, []byte(content), 0644)

	dirs, err := LinkedDirs(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d: %v", len(dirs), dirs)
	}
}
