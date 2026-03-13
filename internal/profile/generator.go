package profile

import (
	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

// HookProcessor processes hooks from the hub and generates settings entries
type HookProcessor interface {
	// ProcessAll processes all hooks from the manifest and returns settings hook entries
	ProcessAll(manifest *Manifest) (map[config.HookType][]config.SettingsHookEntry, error)
}

// FragmentProcessor processes setting fragments from the hub
type FragmentProcessor interface {
	// ProcessAll processes all fragments from the manifest and returns a settings map
	ProcessAll(manifest *Manifest) (map[string]interface{}, error)
}

// TemplateProcessor loads a settings template from the hub
type TemplateProcessor interface {
	// Process loads the settings template referenced by the manifest
	Process(manifest *Manifest) (map[string]interface{}, error)
}

// SettingsBuilder orchestrates the generation of settings.json content
type SettingsBuilder interface {
	// Build creates a complete settings map from the manifest
	Build(manifest *Manifest) (map[string]interface{}, error)
}

// DefaultHookProcessor implements HookProcessor using the existing logic
type DefaultHookProcessor struct {
	paths      *config.Paths
	profileDir string
}

// NewHookProcessor creates a new hook processor
func NewHookProcessor(paths *config.Paths, profileDir string) HookProcessor {
	return &DefaultHookProcessor{
		paths:      paths,
		profileDir: profileDir,
	}
}

// ProcessAll processes all hooks and returns settings entries
func (p *DefaultHookProcessor) ProcessAll(manifest *Manifest) (map[config.HookType][]config.SettingsHookEntry, error) {
	return GenerateSettingsHooks(p.paths, p.profileDir, manifest)
}

// DefaultFragmentProcessor implements FragmentProcessor using hub.MergeFragmentsFromHub
type DefaultFragmentProcessor struct {
	hubDir string
}

// NewFragmentProcessor creates a new fragment processor
func NewFragmentProcessor(hubDir string) FragmentProcessor {
	return &DefaultFragmentProcessor{hubDir: hubDir}
}

// ProcessAll processes all fragments and returns a settings map
func (p *DefaultFragmentProcessor) ProcessAll(manifest *Manifest) (map[string]interface{}, error) {
	if len(manifest.Hub.SettingFragments) == 0 {
		return make(map[string]interface{}), nil
	}
	return mergeSettingFragments(p.hubDir, manifest.Hub.SettingFragments)
}

// DefaultTemplateProcessor implements TemplateProcessor using hub.TemplateManager
type DefaultTemplateProcessor struct {
	hubDir string
}

// NewTemplateProcessor creates a new template processor
func NewTemplateProcessor(hubDir string) TemplateProcessor {
	return &DefaultTemplateProcessor{hubDir: hubDir}
}

// Process loads the settings template referenced by the manifest
func (p *DefaultTemplateProcessor) Process(manifest *Manifest) (map[string]interface{}, error) {
	if manifest.SettingsTemplate == "" {
		return make(map[string]interface{}), nil
	}
	mgr := hub.NewTemplateManager(p.hubDir)
	t, err := mgr.Load(manifest.SettingsTemplate)
	if err != nil {
		return nil, err
	}
	return t.Settings, nil
}

// DefaultSettingsBuilder implements SettingsBuilder with hook, fragment, and template processors
type DefaultSettingsBuilder struct {
	hookProcessor     HookProcessor
	fragmentProcessor FragmentProcessor
	templateProcessor TemplateProcessor
}

// NewSettingsBuilder creates a new settings builder with the given processors
func NewSettingsBuilder(hookProcessor HookProcessor, fragmentProcessor FragmentProcessor, templateProcessor TemplateProcessor) SettingsBuilder {
	return &DefaultSettingsBuilder{
		hookProcessor:     hookProcessor,
		fragmentProcessor: fragmentProcessor,
		templateProcessor: templateProcessor,
	}
}

// Build creates a complete settings map from the manifest
func (b *DefaultSettingsBuilder) Build(manifest *Manifest) (map[string]interface{}, error) {
	settings := make(map[string]interface{})

	// Process settings template (new system — takes priority)
	templateSettings, err := b.templateProcessor.Process(manifest)
	if err != nil {
		return nil, err
	}
	for key, value := range templateSettings {
		settings[key] = value
	}

	// Process fragments (legacy — will be removed after migration)
	fragmentSettings, err := b.fragmentProcessor.ProcessAll(manifest)
	if err != nil {
		return nil, err
	}
	for key, value := range fragmentSettings {
		settings[key] = value
	}

	// Process hooks
	hooks, err := b.hookProcessor.ProcessAll(manifest)
	if err != nil {
		return nil, err
	}
	if len(hooks) > 0 {
		settings["hooks"] = hooks
	}

	return settings, nil
}

// BuilderFromPaths creates a SettingsBuilder from paths configuration
func BuilderFromPaths(paths *config.Paths, profileDir string) SettingsBuilder {
	hookProcessor := NewHookProcessor(paths, profileDir)
	fragmentProcessor := NewFragmentProcessor(paths.HubDir)
	templateProcessor := NewTemplateProcessor(paths.HubDir)
	return NewSettingsBuilder(hookProcessor, fragmentProcessor, templateProcessor)
}
