package migration

import (
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
)

// HookLocation represents where a hook file is located
type HookLocation int

const (
	HookLocationInside  HookLocation = iota // Inside ~/.claude
	HookLocationOutside                     // Outside ~/.claude
	HookLocationInline                      // Inline script (no file)
)

// String returns the string representation of HookLocation
func (l HookLocation) String() string {
	switch l {
	case HookLocationInside:
		return "inside"
	case HookLocationOutside:
		return "outside"
	case HookLocationInline:
		return "inline"
	default:
		return "unknown"
	}
}

// ClassifiedHook is an ExtractedHook with its location classification
type ClassifiedHook struct {
	ExtractedHook
	Location     HookLocation
	RelativePath string   // Path relative to ~/.claude (for inside hooks)
	ParentDirs   []string // Parent directories to warn about (for outside hooks)
}

// HookClassification contains all classified hooks grouped by location
type HookClassification struct {
	Inside  []ClassifiedHook
	Outside []ClassifiedHook
	Inline  []ClassifiedHook
}

// ClassifyHook determines where a hook file is located relative to claudeDir
func ClassifyHook(hook ExtractedHook, claudeDir string) ClassifiedHook {
	classified := ClassifiedHook{
		ExtractedHook: hook,
	}

	if hook.IsInline {
		classified.Location = HookLocationInline
		return classified
	}

	if hook.FilePath == "" {
		classified.Location = HookLocationInline
		return classified
	}

	if hook.IsInside {
		classified.Location = HookLocationInside
		classified.RelativePath = getRelativePath(hook.FilePath, claudeDir)
	} else {
		classified.Location = HookLocationOutside
		classified.ParentDirs = getParentDirs(hook.FilePath)
	}

	return classified
}

// ClassifyHooks classifies all hooks and returns them grouped by location
func ClassifyHooks(hooks []ExtractedHook, claudeDir string) *HookClassification {
	result := &HookClassification{
		Inside:  make([]ClassifiedHook, 0),
		Outside: make([]ClassifiedHook, 0),
		Inline:  make([]ClassifiedHook, 0),
	}

	for _, hook := range hooks {
		classified := ClassifyHook(hook, claudeDir)
		switch classified.Location {
		case HookLocationInside:
			result.Inside = append(result.Inside, classified)
		case HookLocationOutside:
			result.Outside = append(result.Outside, classified)
		case HookLocationInline:
			result.Inline = append(result.Inline, classified)
		}
	}

	return result
}

// getRelativePath returns the path relative to the base directory
func getRelativePath(path, baseDir string) string {
	path = expandHome(path)
	baseDir = expandHome(baseDir)

	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return path
	}
	return rel
}

// getParentDirs extracts parent directories for warning display
func getParentDirs(path string) []string {
	path = expandHome(path)

	dir := filepath.Dir(path)
	var parents []string

	// Get up to 3 parent directories
	for i := 0; i < 3 && dir != "/" && dir != "." && dir != ""; i++ {
		parents = append([]string{dir}, parents...)
		dir = filepath.Dir(dir)
	}

	return parents
}

// HookMigrationChoice represents user's choice for handling an outside hook
type HookMigrationChoice int

const (
	HookChoiceCopy   HookMigrationChoice = iota // Copy to hub
	HookChoiceSkip                              // Skip (remove from settings)
	HookChoiceKeep                              // Keep external reference
)

// String returns the string representation of HookMigrationChoice
func (c HookMigrationChoice) String() string {
	switch c {
	case HookChoiceCopy:
		return "copy"
	case HookChoiceSkip:
		return "skip"
	case HookChoiceKeep:
		return "keep"
	default:
		return "unknown"
	}
}

// HookMigrationDecision combines a classified hook with the user's choice
type HookMigrationDecision struct {
	Hook   ClassifiedHook
	Choice HookMigrationChoice
}

// HookMigrationPlan contains all decisions for hook migration
type HookMigrationPlan struct {
	Inside    []ClassifiedHook        // Always migrate
	Inline    []ClassifiedHook        // Extract to hub
	Decisions []HookMigrationDecision // User decisions for outside hooks
}

// HookManifest represents the hook.yaml file in hub
type HookManifest struct {
	Name        string          `yaml:"name"`
	Type        config.HookType `yaml:"type"`
	Timeout     int             `yaml:"timeout,omitempty"`
	Command     string          `yaml:"command,omitempty"`     // Relative to hook folder
	Interpreter string          `yaml:"interpreter,omitempty"` // e.g., "bash", "node"
	Matcher     string          `yaml:"matcher,omitempty"`
	Inline      string          `yaml:"inline,omitempty"` // For inline hooks
}

// GetHooksToMigrate returns all hooks that should be migrated to hub
func (p *HookMigrationPlan) GetHooksToMigrate() []ClassifiedHook {
	var result []ClassifiedHook

	// Inside hooks are always migrated
	result = append(result, p.Inside...)

	// Inline hooks are always migrated
	result = append(result, p.Inline...)

	// Add outside hooks that user chose to copy
	for _, decision := range p.Decisions {
		if decision.Choice == HookChoiceCopy {
			result = append(result, decision.Hook)
		}
	}

	return result
}

// GetHooksToKeep returns outside hooks that user chose to keep as external references
func (p *HookMigrationPlan) GetHooksToKeep() []ClassifiedHook {
	var result []ClassifiedHook
	for _, decision := range p.Decisions {
		if decision.Choice == HookChoiceKeep {
			result = append(result, decision.Hook)
		}
	}
	return result
}
