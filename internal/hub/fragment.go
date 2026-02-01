package hub

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
)

// Fragment represents a setting fragment from the hub
type Fragment struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description,omitempty"`
	Key         string      `yaml:"key"`
	Value       interface{} `yaml:"value"`
}

// FragmentReader reads setting fragments from the hub
type FragmentReader interface {
	Read(hubDir string, name string) (*Fragment, error)
	ReadAll(hubDir string, names []string) ([]*Fragment, error)
}

// YAMLFragmentReader reads fragments from YAML files
type YAMLFragmentReader struct{}

// NewFragmentReader creates a new YAML fragment reader
func NewFragmentReader() FragmentReader {
	return &YAMLFragmentReader{}
}

// Read reads a single fragment by name
func (r *YAMLFragmentReader) Read(hubDir string, name string) (*Fragment, error) {
	fragmentPath := filepath.Join(hubDir, string(config.HubSettingFragments), name+".yaml")

	data, err := os.ReadFile(fragmentPath)
	if err != nil {
		return nil, err
	}

	var fragment Fragment
	if err := yaml.Unmarshal(data, &fragment); err != nil {
		return nil, err
	}

	return &fragment, nil
}

// ReadAll reads multiple fragments by name
func (r *YAMLFragmentReader) ReadAll(hubDir string, names []string) ([]*Fragment, error) {
	fragments := make([]*Fragment, 0, len(names))
	for _, name := range names {
		fragment, err := r.Read(hubDir, name)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, fragment)
	}
	return fragments, nil
}

// MergeFragments merges multiple fragments into a settings map
func MergeFragments(fragments []*Fragment) map[string]interface{} {
	settings := make(map[string]interface{})
	for _, fragment := range fragments {
		settings[fragment.Key] = fragment.Value
	}
	return settings
}

// MergeFragmentsFromHub is a convenience function that reads and merges fragments
func MergeFragmentsFromHub(hubDir string, names []string) (map[string]interface{}, error) {
	reader := NewFragmentReader()
	fragments, err := reader.ReadAll(hubDir, names)
	if err != nil {
		return nil, err
	}
	return MergeFragments(fragments), nil
}
