package source

import "testing"

func TestSkillsShRegistryCanHandle(t *testing.T) {
	r := &SkillsShRegistry{}

	tests := []struct {
		id   string
		want bool
	}{
		{"skills.sh/owner/name", true},
		{"owner/name", true},
		{"github:owner/name", false},
		{"https://github.com/owner/repo.git", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := r.CanHandle(tt.id); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestGitHubRegistryCanHandle(t *testing.T) {
	r := &GitHubRegistry{}

	tests := []struct {
		id   string
		want bool
	}{
		{"github:owner/name", true},
		{"owner/name", false},
		{"skills.sh/owner/name", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := r.CanHandle(tt.id); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestDetectRegistry(t *testing.T) {
	tests := []struct {
		id       string
		wantName string
	}{
		{"owner/name", "skills.sh"},
		{"skills.sh/owner/name", "skills.sh"},
		{"github:owner/name", "github"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			r := DetectRegistry(tt.id)
			if r == nil {
				t.Errorf("DetectRegistry(%q) returned nil", tt.id)
				return
			}
			if r.Name() != tt.wantName {
				t.Errorf("DetectRegistry(%q).Name() = %q, want %q", tt.id, r.Name(), tt.wantName)
			}
		})
	}
}
