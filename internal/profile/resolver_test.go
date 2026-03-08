package profile

import (
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestResolveManifestNoComposition(t *testing.T) {
	m := NewManifest("test", "")
	m.Hub.Skills = []string{"coding"}

	paths := &config.Paths{}

	resolved, err := ResolveManifest(m, paths)
	if err != nil {
		t.Fatalf("ResolveManifest failed: %v", err)
	}

	// Should return same manifest when no composition
	if resolved != m {
		t.Error("expected same manifest pointer when no composition")
	}
}

func TestResolveManifestWithEngineAndContext(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ContextsDir: filepath.Join(tmpDir, "contexts"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create engine
	engineMgr := NewEngineManager(paths)
	engine := NewEngine("test-engine", "")
	engine.Hub.SettingFragments = []string{"model-opus"}
	engine.Hub.Hooks = []string{"session-manager"}
	engine.Data.History = config.ShareModeIsolated
	engine.Data.Tasks = config.ShareModeShared
	engineMgr.Create("test-engine", engine)

	// Create context
	ctxMgr := NewContextManager(paths)
	ctx := NewContext("test-ctx", "")
	ctx.Hub.Skills = []string{"coding", "debugging"}
	ctx.Hub.Agents = []string{"reviewer"}
	ctx.Hub.Hooks = []string{"prompt-loader"}
	ctxMgr.Create("test-ctx", ctx)

	// Create manifest with engine + context + overrides
	m := NewManifest("test-profile", "")
	m.Engine = "test-engine"
	m.Context = "test-ctx"
	m.Hub.Skills = []string{"docker"} // Override
	m.Hub.Hooks = []string{"pre-commit"}

	resolved, err := ResolveManifest(m, paths)
	if err != nil {
		t.Fatalf("ResolveManifest failed: %v", err)
	}

	// Check merged skills: context (coding, debugging) + profile (docker)
	if len(resolved.Hub.Skills) != 3 {
		t.Errorf("Skills = %v, want 3 items", resolved.Hub.Skills)
	}

	// Check merged hooks: engine (session-manager) + context (prompt-loader) + profile (pre-commit)
	if len(resolved.Hub.Hooks) != 3 {
		t.Errorf("Hooks = %v, want 3 items", resolved.Hub.Hooks)
	}

	// Check setting fragments from engine
	if len(resolved.Hub.SettingFragments) != 1 || resolved.Hub.SettingFragments[0] != "model-opus" {
		t.Errorf("SettingFragments = %v, want [model-opus]", resolved.Hub.SettingFragments)
	}

	// Check agents from context
	if len(resolved.Hub.Agents) != 1 || resolved.Hub.Agents[0] != "reviewer" {
		t.Errorf("Agents = %v, want [reviewer]", resolved.Hub.Agents)
	}

	// Check data config from engine
	if resolved.Data.History != config.ShareModeIsolated {
		t.Errorf("Data.History = %v, want isolated", resolved.Data.History)
	}
	if resolved.Data.Tasks != config.ShareModeShared {
		t.Errorf("Data.Tasks = %v, want shared", resolved.Data.Tasks)
	}
}

func TestResolveManifestDedup(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ContextsDir: filepath.Join(tmpDir, "contexts"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create engine with a hook
	engineMgr := NewEngineManager(paths)
	engine := NewEngine("eng", "")
	engine.Hub.Hooks = []string{"shared-hook"}
	engineMgr.Create("eng", engine)

	// Create context with same hook
	ctxMgr := NewContextManager(paths)
	ctx := NewContext("ctx", "")
	ctx.Hub.Hooks = []string{"shared-hook"}
	ctxMgr.Create("ctx", ctx)

	// Profile also has the same hook
	m := NewManifest("test", "")
	m.Engine = "eng"
	m.Context = "ctx"
	m.Hub.Hooks = []string{"shared-hook"}

	resolved, err := ResolveManifest(m, paths)
	if err != nil {
		t.Fatalf("ResolveManifest failed: %v", err)
	}

	// Should be deduplicated
	if len(resolved.Hub.Hooks) != 1 {
		t.Errorf("Hooks = %v, want 1 item (deduplicated)", resolved.Hub.Hooks)
	}
}

func TestResolveManifestEngineOnly(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ContextsDir: filepath.Join(tmpDir, "contexts"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	engineMgr := NewEngineManager(paths)
	engine := NewEngine("eng", "")
	engine.Hub.SettingFragments = []string{"frag1"}
	engineMgr.Create("eng", engine)

	m := NewManifest("test", "")
	m.Engine = "eng"
	m.Hub.Skills = []string{"skill1"}

	resolved, err := ResolveManifest(m, paths)
	if err != nil {
		t.Fatalf("ResolveManifest failed: %v", err)
	}

	if len(resolved.Hub.SettingFragments) != 1 {
		t.Errorf("SettingFragments = %v, want 1", resolved.Hub.SettingFragments)
	}
	if len(resolved.Hub.Skills) != 1 {
		t.Errorf("Skills = %v, want 1", resolved.Hub.Skills)
	}
}

func TestResolveManifestContextOnly(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ContextsDir: filepath.Join(tmpDir, "contexts"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	ctxMgr := NewContextManager(paths)
	ctx := NewContext("ctx", "")
	ctx.Hub.Skills = []string{"skill1", "skill2"}
	ctxMgr.Create("ctx", ctx)

	m := NewManifest("test", "")
	m.Context = "ctx"

	resolved, err := ResolveManifest(m, paths)
	if err != nil {
		t.Fatalf("ResolveManifest failed: %v", err)
	}

	if len(resolved.Hub.Skills) != 2 {
		t.Errorf("Skills = %v, want 2", resolved.Hub.Skills)
	}
}

func TestDedup(t *testing.T) {
	tests := []struct {
		input []string
		want  int
	}{
		{nil, 0},
		{[]string{}, 0},
		{[]string{"a", "b", "c"}, 3},
		{[]string{"a", "b", "a", "c", "b"}, 3},
		{[]string{"x", "x", "x"}, 1},
	}

	for _, tt := range tests {
		result := dedup(tt.input)
		if len(result) != tt.want {
			t.Errorf("dedup(%v) = %v, want len %d", tt.input, result, tt.want)
		}
	}
}
