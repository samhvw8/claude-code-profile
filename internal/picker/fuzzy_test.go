package picker

import "testing"

func TestFuzzyScore(t *testing.T) {
	tests := []struct {
		str     string
		pattern string
		match   bool
	}{
		{"hello-world", "hw", true},
		{"hello-world", "hwd", true},
		{"mySkillName", "msn", true},
		{"mySkillName", "skill", true},
		{"mySkillName", "xyz", false},
		{"", "a", false},
		{"anything", "", true},
		{"abc", "abc", true},
		{"abc", "abcd", false},
		{"design-principles", "dp", true},
		{"design-principles", "depr", true},
		{"CamelCaseFunc", "ccf", true},
		{"CamelCaseFunc", "cafu", true},
	}

	for _, tt := range tests {
		ok, _ := fuzzyScore(tt.str, tt.pattern)
		if ok != tt.match {
			t.Errorf("fuzzyScore(%q, %q) = %v, want %v", tt.str, tt.pattern, ok, tt.match)
		}
	}
}

func TestFuzzyScoreRanking(t *testing.T) {
	// Exact prefix should score higher than scattered match
	_, prefixScore := fuzzyScore("profile-create", "pro")
	_, scatterScore := fuzzyScore("some-p-r-o-thing", "pro")
	if prefixScore <= scatterScore {
		t.Errorf("prefix match (%d) should score higher than scattered (%d)", prefixScore, scatterScore)
	}

	// Consecutive match should score higher than non-consecutive
	_, consec := fuzzyScore("abc-thing", "abc")
	_, nonConsec := fuzzyScore("axbxc-thing", "abc")
	if consec <= nonConsec {
		t.Errorf("consecutive (%d) should score higher than non-consecutive (%d)", consec, nonConsec)
	}

	// Word boundary match should score higher than mid-word
	_, boundary := fuzzyScore("my-skill", "ms")
	_, midWord := fuzzyScore("xmxs", "ms")
	if boundary <= midWord {
		t.Errorf("boundary match (%d) should score higher than mid-word (%d)", boundary, midWord)
	}
}

func TestSortByFuzzyScore(t *testing.T) {
	items := []Item{
		{ID: "z-scattered-x-y", Label: "z-scattered-x-y"},
		{ID: "xyz-exact", Label: "xyz-exact"},
		{ID: "no-match", Label: "no-match"},
		{ID: "xy-partial", Label: "xy-partial"},
	}

	result := sortByFuzzyScore(items, "xy")
	if len(result) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(result))
	}
	// "no-match" should be excluded
	for _, item := range result {
		if item.ID == "no-match" {
			t.Error("non-matching item should be excluded")
		}
	}
	// "xyz-exact" should rank first (consecutive + boundary)
	if result[0].ID != "xyz-exact" {
		t.Errorf("expected xyz-exact first, got %s", result[0].ID)
	}
}
