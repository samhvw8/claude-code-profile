package claudemd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ParseImports extracts @path/to/file references from a CLAUDE.md file.
// Returns relative paths as they appear in the file.
func ParseImports(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var imports []string
	seen := make(map[string]bool)
	inCodeBlock := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Track code fences
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		// Match @path at start of line (Claude Code import syntax)
		if !strings.HasPrefix(line, "@") {
			continue
		}

		ref := strings.TrimSpace(line[1:])
		if ref == "" {
			continue
		}

		// Skip obvious non-imports (annotations like @Composable, @Module, etc.)
		// Real imports are file paths with slashes or extensions
		if !strings.Contains(ref, "/") && !strings.Contains(ref, ".") {
			continue
		}

		if !seen[ref] {
			seen[ref] = true
			imports = append(imports, ref)
		}
	}

	return imports, scanner.Err()
}

// ResolveImports takes parsed import paths and resolves them to parent
// directories that need to be tracked. For example, @principles/se.md
// yields "principles" as the directory to track.
func ResolveImports(imports []string) []string {
	dirs := make(map[string]bool)
	for _, imp := range imports {
		dir := filepath.Dir(imp)
		if dir != "." && dir != "" {
			dirs[dir] = true
		}
	}

	var result []string
	for dir := range dirs {
		// Use the top-level directory only
		top := strings.SplitN(dir, string(filepath.Separator), 2)[0]
		if !contains(result, top) {
			result = append(result, top)
		}
	}
	return result
}

// LinkedDirs parses a CLAUDE.md file and returns the directories that
// should be tracked alongside it (e.g., "principles").
func LinkedDirs(claudeMDPath string) ([]string, error) {
	imports, err := ParseImports(claudeMDPath)
	if err != nil {
		return nil, err
	}
	return ResolveImports(imports), nil
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
