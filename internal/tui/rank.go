package tui

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/inf1nite-lo0p/op/internal/cache"
)

// Score tiers for where a token matched. Higher = better. Gaps are
// intentionally large: a hit on the *name* should always beat the
// same query happening to fuzzy-match scattered characters in some
// unrelated path. Without these gaps, typing "kit str" ranks a
// random `socket.io-react-hook` above the actual `kit` repo just
// because the letters appear in order.
//
// The "split-2" tiers handle queries like "frontendplatform" — a
// concatenation the user typed without a space. We split the query
// into two halves and check both as substrings; if both hit, the
// row is much more likely to be what the user meant than a fuzzy
// scatter-match of all 16 letters across an unrelated long path.
const (
	tierNameExact  = 1500 // name == token
	tierNamePrefix = 1200 // name starts with token
	tierNameSubstr = 1000 // name contains token verbatim
	tierNameSplit2 = 800  // name contains both halves of a 2-way split
	tierBranch     = 500  // branch contains token
	tierPathSubstr = 350  // path contains token verbatim
	tierPathSplit2 = 250  // path contains both halves of a 2-way split
	tierFuzzy      = 100  // last-resort: chars in order anywhere
)

// rankEntries returns indices into entries, sorted best-first under
// a multi-token + recency-aware scheme. Each whitespace-separated
// token must match every row (fzf's "AND" convention); the per-row
// score is the sum of per-token tier scores plus a recency bonus.
func rankEntries(entries []cache.Entry, query string) []int {
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		out := make([]int, len(entries))
		for i := range entries {
			out[i] = i
		}
		return out
	}

	type ranked struct {
		idx   int
		score int
		t     time.Time
	}
	results := make([]ranked, 0, len(entries))
	for i, e := range entries {
		score, ok := scoreEntry(e, tokens)
		if !ok {
			continue
		}
		results = append(results, ranked{
			idx:   i,
			score: score + recencyBonus(e.HeadMTime),
			t:     e.HeadMTime,
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].t.After(results[j].t)
	})

	out := make([]int, len(results))
	for i, r := range results {
		out[i] = r.idx
	}
	return out
}

// scoreEntry assigns a single score to a row for the *combined* set
// of tokens. Each token must match somewhere — if any one doesn't,
// the entry is dropped.
func scoreEntry(e cache.Entry, tokens []string) (int, bool) {
	name := strings.ToLower(e.Name)
	branch := strings.ToLower(e.Branch)
	// Strip $HOME from the path before matching. Every project under
	// the user's home dir would otherwise trivially "match" any token
	// that appears in the home prefix (most commonly the username),
	// making the path tier useless for narrowing.
	path := strings.ToLower(stripHome(e.Path))

	total := 0
	for _, tok := range tokens {
		var s int
		switch {
		case name == tok:
			s = tierNameExact
		case strings.HasPrefix(name, tok):
			s = tierNamePrefix
		case strings.Contains(name, tok):
			s = tierNameSubstr
		case splitMatch(tok, name):
			s = tierNameSplit2
		case strings.Contains(branch, tok):
			s = tierBranch
		case strings.Contains(path, tok):
			s = tierPathSubstr
		case splitMatch(tok, path):
			s = tierPathSplit2
		case fuzzyMatch(tok, name+" "+branch+" "+path):
			s = tierFuzzy
		default:
			return 0, false
		}
		total += s
	}
	return total, true
}

// splitMatch is true if tok can be split into two halves (each at
// least 2 chars) such that *both* halves appear as substrings of
// haystack. Catches queries the user typed without a space —
// e.g. "frontendplatform" against "technance-platform-frontend":
// "frontend" + "platform" are both in there even though the verbatim
// concatenation is not. We don't try 3+ splits — diminishing returns
// and an explosion of false positives.
func splitMatch(tok, haystack string) bool {
	if len(tok) < 4 {
		return false
	}
	for i := 2; i <= len(tok)-2; i++ {
		a, b := tok[:i], tok[i:]
		if strings.Contains(haystack, a) && strings.Contains(haystack, b) {
			return true
		}
	}
	return false
}

// stripHome returns p with the user's home directory prefix removed.
// Used by ranking so that, e.g., typing your username doesn't match
// every single project under your home dir (because every path starts
// with /home/<you>/). The display layer keeps using the full path —
// this transform is only consulted at score time.
func stripHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if strings.HasPrefix(p, home) {
		return p[len(home):]
	}
	return p
}

// fuzzyMatch returns true if every byte of needle appears in haystack
// in order — the classic fzf-style "all chars present in sequence"
// fallback. Both strings are expected to be already lower-cased.
func fuzzyMatch(needle, haystack string) bool {
	if needle == "" {
		return true
	}
	i := 0
	for j := 0; j < len(haystack); j++ {
		if haystack[j] == needle[i] {
			i++
			if i == len(needle) {
				return true
			}
		}
	}
	return false
}

// recencyBonus is the additive nudge given to recently-touched
// projects. Tuned so that on a tie within the same match tier,
// today's repo decisively beats last month's, but on a clear
// text-match difference (substring vs fuzzy) the better text still
// wins. Values are paired with the tier values above.
func recencyBonus(t time.Time) int {
	if t.IsZero() {
		return 0
	}
	age := time.Since(t)
	switch {
	case age < time.Hour:
		return 200
	case age < 6*time.Hour:
		return 150
	case age < 24*time.Hour:
		return 100
	case age < 3*24*time.Hour:
		return 60
	case age < 7*24*time.Hour:
		return 30
	case age < 30*24*time.Hour:
		return 12
	default:
		return 0
	}
}
