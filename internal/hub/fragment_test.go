package hub

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestFragmentReader_Read(t *testing.T) {
	hubDir := t.TempDir()
	fragmentsDir := filepath.Join(hubDir, string(config.HubSettingFragments))
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test fragment
	fragmentContent := `name: test-fragment
description: A test fragment
key: testKey
value: testValue
`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "test.yaml"), []byte(fragmentContent), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewFragmentReader()
	fragment, err := reader.Read(hubDir, "test")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if fragment.Name != "test-fragment" {
		t.Errorf("expected name 'test-fragment', got %q", fragment.Name)
	}
	if fragment.Key != "testKey" {
		t.Errorf("expected key 'testKey', got %q", fragment.Key)
	}
	if fragment.Value != "testValue" {
		t.Errorf("expected value 'testValue', got %v", fragment.Value)
	}
}

func TestFragmentReader_ReadAll(t *testing.T) {
	hubDir := t.TempDir()
	fragmentsDir := filepath.Join(hubDir, string(config.HubSettingFragments))
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test fragments
	fragment1 := `name: frag1
key: key1
value: value1
`
	fragment2 := `name: frag2
key: key2
value: 42
`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "frag1.yaml"), []byte(fragment1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fragmentsDir, "frag2.yaml"), []byte(fragment2), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewFragmentReader()
	fragments, err := reader.ReadAll(hubDir, []string{"frag1", "frag2"})
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(fragments) != 2 {
		t.Errorf("expected 2 fragments, got %d", len(fragments))
	}
}

func TestMergeFragments(t *testing.T) {
	fragments := []*Fragment{
		{Key: "key1", Value: "value1"},
		{Key: "key2", Value: 42},
		{Key: "key3", Value: true},
	}

	settings := MergeFragments(fragments)

	if len(settings) != 3 {
		t.Errorf("expected 3 settings, got %d", len(settings))
	}

	if settings["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %v", settings["key1"])
	}
	if settings["key2"] != 42 {
		t.Errorf("expected key2=42, got %v", settings["key2"])
	}
	if settings["key3"] != true {
		t.Errorf("expected key3=true, got %v", settings["key3"])
	}
}

func TestMergeFragmentsFromHub(t *testing.T) {
	hubDir := t.TempDir()
	fragmentsDir := filepath.Join(hubDir, string(config.HubSettingFragments))
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test fragment
	fragmentContent := `name: test
key: testKey
value: testValue
`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "test.yaml"), []byte(fragmentContent), 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := MergeFragmentsFromHub(hubDir, []string{"test"})
	if err != nil {
		t.Fatalf("MergeFragmentsFromHub() error = %v", err)
	}

	if val, ok := settings["testKey"]; !ok || val != "testValue" {
		t.Errorf("expected settings[testKey] = 'testValue', got %v", settings["testKey"])
	}
}

func TestFragmentReader_Read_NotFound(t *testing.T) {
	hubDir := t.TempDir()

	reader := NewFragmentReader()
	_, err := reader.Read(hubDir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent fragment")
	}
}

func TestFragmentReader_Read_ComplexValues(t *testing.T) {
	hubDir := t.TempDir()
	fragmentsDir := filepath.Join(hubDir, string(config.HubSettingFragments))
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create fragment with complex nested value
	fragmentContent := `name: complex-fragment
key: apiSettings
value:
  timeout: 30
  retries: 3
  endpoints:
    - /api/v1
    - /api/v2
`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "complex.yaml"), []byte(fragmentContent), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewFragmentReader()
	fragment, err := reader.Read(hubDir, "complex")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if fragment.Name != "complex-fragment" {
		t.Errorf("expected name 'complex-fragment', got %q", fragment.Name)
	}
	if fragment.Key != "apiSettings" {
		t.Errorf("expected key 'apiSettings', got %q", fragment.Key)
	}

	// Value should be a map
	valueMap, ok := fragment.Value.(map[string]interface{})
	if !ok {
		t.Fatalf("expected Value to be map, got %T", fragment.Value)
	}
	if valueMap["timeout"] != 30 {
		t.Errorf("expected timeout=30, got %v", valueMap["timeout"])
	}
}

func TestFragmentReader_Read_InvalidYAML(t *testing.T) {
	hubDir := t.TempDir()
	fragmentsDir := filepath.Join(hubDir, string(config.HubSettingFragments))
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create invalid YAML
	invalidContent := `name: broken
key: test
value: [invalid yaml`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "invalid.yaml"), []byte(invalidContent), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewFragmentReader()
	_, err := reader.Read(hubDir, "invalid")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestFragmentReader_ReadAll_Empty(t *testing.T) {
	hubDir := t.TempDir()

	reader := NewFragmentReader()
	fragments, err := reader.ReadAll(hubDir, []string{})
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(fragments) != 0 {
		t.Errorf("expected 0 fragments, got %d", len(fragments))
	}
}

func TestFragmentReader_ReadAll_PartialFailure(t *testing.T) {
	hubDir := t.TempDir()
	fragmentsDir := filepath.Join(hubDir, string(config.HubSettingFragments))
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create only one fragment
	fragmentContent := `name: exists
key: key1
value: value1
`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "exists.yaml"), []byte(fragmentContent), 0644); err != nil {
		t.Fatal(err)
	}

	reader := NewFragmentReader()
	// Try to read existing and non-existing fragments
	_, err := reader.ReadAll(hubDir, []string{"exists", "missing"})
	if err == nil {
		t.Error("expected error when one fragment is missing")
	}
}

func TestMergeFragments_Empty(t *testing.T) {
	fragments := []*Fragment{}
	settings := MergeFragments(fragments)

	if len(settings) != 0 {
		t.Errorf("expected empty settings, got %d entries", len(settings))
	}
}

func TestMergeFragments_Override(t *testing.T) {
	// Later fragments should override earlier ones with same key
	fragments := []*Fragment{
		{Key: "model", Value: "old-model"},
		{Key: "model", Value: "new-model"},
	}

	settings := MergeFragments(fragments)

	if settings["model"] != "new-model" {
		t.Errorf("expected model='new-model' (override), got %v", settings["model"])
	}
}
