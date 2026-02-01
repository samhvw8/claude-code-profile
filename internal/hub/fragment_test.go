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
