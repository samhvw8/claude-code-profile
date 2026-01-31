package source

import "testing"

func TestGitProviderCanHandle(t *testing.T) {
	p := &GitProvider{}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://github.com/user/repo", true},
		{"github.com/user/repo", true},
		{"https://gitlab.com/user/repo", true},
		{"git@github.com:user/repo.git", true},
		{"https://example.com/file.tar.gz", false},
		{"https://example.com/page", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := p.CanHandle(tt.url); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestHTTPProviderCanHandle(t *testing.T) {
	p := &HTTPProvider{}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/file.tar.gz", true},
		{"https://example.com/file.tgz", true},
		{"https://example.com/file.zip", true},
		{"https://github.com/user/repo", false},
		{"github.com/user/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := p.CanHandle(tt.url); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		url      string
		wantType string
	}{
		{"https://github.com/user/repo", "git"},
		{"github.com/user/repo", "git"},
		{"https://example.com/file.tar.gz", "http"},
		{"https://example.com/file.zip", "http"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			p := DetectProvider(tt.url)
			if p == nil {
				t.Errorf("DetectProvider(%q) returned nil", tt.url)
				return
			}
			if p.Type() != tt.wantType {
				t.Errorf("DetectProvider(%q).Type() = %q, want %q", tt.url, p.Type(), tt.wantType)
			}
		})
	}
}

func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/user/repo", "https://github.com/user/repo.git"},
		{"https://github.com/user/repo", "https://github.com/user/repo.git"},
		{"https://github.com/user/repo.git", "https://github.com/user/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeGitURL(tt.input); got != tt.want {
				t.Errorf("normalizeGitURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
