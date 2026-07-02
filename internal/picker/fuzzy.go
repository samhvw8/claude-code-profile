package picker

import (
	"sort"
	"strings"
	"unicode"
)

// fuzzyScore returns whether pattern matches str (chars in order, case-insensitive)
// and a quality score. Higher score = better match.
// Scoring mimics fzf: consecutive chars, word boundaries, and start-of-string all boost score.
func fuzzyScore(str, pattern string) (bool, int) {
	if pattern == "" {
		return true, 0
	}

	sLower := strings.ToLower(str)
	pLower := strings.ToLower(pattern)

	// Quick reject: all pattern chars must exist in order
	pi := 0
	for i := 0; i < len(sLower) && pi < len(pLower); i++ {
		if sLower[i] == pLower[pi] {
			pi++
		}
	}
	if pi < len(pLower) {
		return false, 0
	}

	// Score the match
	score := 0
	pi = 0
	consecutive := 0

	for i := 0; i < len(sLower) && pi < len(pLower); i++ {
		if sLower[i] != pLower[pi] {
			consecutive = 0
			continue
		}

		score++
		consecutive++

		// Consecutive bonus — grows fast to strongly prefer adjacent chars
		if consecutive > 1 {
			score += consecutive * 3
		}

		// Word boundary bonus: start of string, after separator, or camelCase transition
		if i == 0 || isSep(str[i-1]) || (i > 0 && unicode.IsLower(rune(str[i-1])) && unicode.IsUpper(rune(str[i]))) {
			score += 8
		}

		// First pattern char at start of string
		if pi == 0 && i == 0 {
			score += 5
		}

		pi++
	}

	// Prefer shorter targets (tighter match density)
	score -= len(str) / 5

	return true, score
}

func isSep(b byte) bool {
	return b == '-' || b == '_' || b == '.' || b == '/' || b == ' '
}

// sortByFuzzyScore filters items by fuzzy match and returns them sorted best-first.
func sortByFuzzyScore(items []Item, query string) []Item {
	type scored struct {
		item  Item
		score int
	}
	var matches []scored
	for _, item := range items {
		bestScore := -1
		if ok, s := fuzzyScore(item.Label, query); ok && s > bestScore {
			bestScore = s
		}
		if ok, s := fuzzyScore(item.ID, query); ok && s > bestScore {
			bestScore = s
		}
		if bestScore >= 0 {
			matches = append(matches, scored{item, bestScore})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})
	result := make([]Item, len(matches))
	for i, m := range matches {
		result[i] = m.item
	}
	return result
}
