package source

import "testing"

func TestParseGitWebURL(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantRepo string
		wantRef  string
		wantSub  string
	}{
		{
			"blob root SKILL.md",
			"https://github.com/colbymchenry/frontend-audit-skill/blob/main/SKILL.md",
			"https://github.com/colbymchenry/frontend-audit-skill", "main", "SKILL.md",
		},
		{
			"blob nested skill",
			"https://github.com/o/r/blob/dev/skills/x/SKILL.md",
			"https://github.com/o/r", "dev", "skills/x/SKILL.md",
		},
		{
			"tree directory",
			"https://github.com/o/r/tree/main/skills/x",
			"https://github.com/o/r", "main", "skills/x",
		},
		{
			"raw githubusercontent",
			"https://raw.githubusercontent.com/o/r/main/SKILL.md",
			"https://github.com/o/r", "main", "SKILL.md",
		},
		{
			"gitlab /-/blob/",
			"https://gitlab.com/o/r/-/blob/main/SKILL.md",
			"https://gitlab.com/o/r", "main", "SKILL.md",
		},
		{"plain repo URL", "https://github.com/o/r", "", "", ""},
		{"repo .git URL", "https://github.com/o/r.git", "", "", ""},
		{"owner/repo shorthand", "o/r", "", "", ""},
		{"tree without path", "https://github.com/o/r/tree/main", "", "", ""},
		{"non-git host", "https://example.com/o/r/blob/main/x", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, ref, sub := ParseGitWebURL(tt.in)
			if repo != tt.wantRepo || ref != tt.wantRef || sub != tt.wantSub {
				t.Errorf("ParseGitWebURL(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.in, repo, ref, sub, tt.wantRepo, tt.wantRef, tt.wantSub)
			}
		})
	}
}
