package ui

import (
	"testing"
)

func TestHelpAllEntriesFromKeymap(t *testing.T) {
	entries := AllHelpEntries(nil)
	if len(entries) == 0 {
		t.Fatal("AllHelpEntries returned empty slice")
	}
	for _, e := range entries {
		if e.Action == "" {
			t.Fatal("found HelpEntry with empty Action")
		}
		if e.Context == "" {
			t.Fatalf("entry %q has empty Context", e.Action)
		}
		if e.Description == "" {
			t.Fatalf("entry %q has empty Description", e.Action)
		}
		if len(e.Keys) == 0 {
			t.Fatalf("entry %q has no keys", e.Action)
		}
	}
}

func TestHelpAllEntriesIsolated(t *testing.T) {
	// Mutating the returned slice or its key slices must not affect a second call.
	a := AllHelpEntries(nil)
	b := AllHelpEntries(nil)
	if len(a) != len(b) {
		t.Fatalf("repeated calls returned different lengths: %d vs %d", len(a), len(b))
	}
	a[0].Keys[0] = "MUTATED"
	fresh := AllHelpEntries(nil)
	if fresh[0].Keys[0] == "MUTATED" {
		t.Fatal("AllHelpEntries shares key slice memory between calls")
	}
}

func TestHelpFilterEmptyQuery(t *testing.T) {
	entries := AllHelpEntries(nil)
	got := FilterHelpEntries(entries, "")
	if len(got) != len(entries) {
		t.Fatalf("empty query: expected %d entries, got %d", len(entries), len(got))
	}
}

func TestHelpFilterByActionName(t *testing.T) {
	entries := AllHelpEntries(nil)
	got := FilterHelpEntries(entries, "quit")
	if len(got) == 0 {
		t.Fatal("expected at least one entry matching action name 'quit'")
	}
	for _, e := range got {
		if !containsCI(e.Action, "quit") &&
			!containsCI(e.Description, "quit") &&
			!keysContainCI(e.Keys, "quit") {
			t.Errorf("entry %q should not match 'quit'", e.Action)
		}
	}
}

func TestHelpFilterByDescription(t *testing.T) {
	entries := AllHelpEntries(nil)
	// "hunk" appears only in main-panel descriptions.
	got := FilterHelpEntries(entries, "hunk")
	if len(got) == 0 {
		t.Fatal("expected entries matching description 'hunk'")
	}
	for _, e := range got {
		if !containsCI(e.Description, "hunk") &&
			!containsCI(e.Action, "hunk") &&
			!keysContainCI(e.Keys, "hunk") {
			t.Errorf("entry %q should not match 'hunk'", e.Action)
		}
	}
}

func TestHelpFilterByKey(t *testing.T) {
	entries := AllHelpEntries(nil)
	// <enter> is a key on several actions.
	got := FilterHelpEntries(entries, "<enter>")
	if len(got) == 0 {
		t.Fatal("expected entries with key '<enter>'")
	}
	for _, e := range got {
		if !keysContainCI(e.Keys, "<enter>") &&
			!containsCI(e.Action, "<enter>") &&
			!containsCI(e.Description, "<enter>") {
			t.Errorf("entry %q should not match '<enter>'", e.Action)
		}
	}
}

func TestHelpFilterCaseInsensitive(t *testing.T) {
	entries := AllHelpEntries(nil)
	lower := FilterHelpEntries(entries, "quit")
	upper := FilterHelpEntries(entries, "QUIT")
	mixed := FilterHelpEntries(entries, "Quit")
	if len(lower) == 0 {
		t.Fatal("no results for 'quit'")
	}
	if len(lower) != len(upper) || len(lower) != len(mixed) {
		t.Fatalf("case sensitivity mismatch: lower=%d upper=%d mixed=%d",
			len(lower), len(upper), len(mixed))
	}
}

func TestHelpFilterNoMatch(t *testing.T) {
	entries := AllHelpEntries(nil)
	got := FilterHelpEntries(entries, "xyzzy_no_such_thing")
	if len(got) != 0 {
		t.Fatalf("expected no results for impossible query, got %d", len(got))
	}
}

func TestHelpFilterPreservesOrder(t *testing.T) {
	entries := AllHelpEntries(nil)
	got := FilterHelpEntries(entries, "jump")
	if len(got) < 2 {
		t.Skip("need at least 2 matching entries to verify order")
	}
	lastIdx := -1
	for _, g := range got {
		found := -1
		for i, e := range entries {
			if e.Action == g.Action && e.Context == g.Context {
				found = i
				break
			}
		}
		if found <= lastIdx {
			t.Fatalf("source order violated: entry %q at source index %d, previous was %d",
				g.Action, found, lastIdx)
		}
		lastIdx = found
	}
}

// helpers ─ simple ASCII-safe implementations that avoid importing strings
// to keep the test file self-contained.

func containsCI(s, q string) bool {
	if len(q) == 0 {
		return true
	}
	sl, ql := asciiLower(s), asciiLower(q)
	for i := range len(sl) - len(ql) + 1 {
		if sl[i:i+len(ql)] == ql {
			return true
		}
	}
	return false
}

func keysContainCI(keys []string, q string) bool {
	for _, k := range keys {
		if containsCI(k, q) {
			return true
		}
	}
	return false
}

func asciiLower(s string) string {
	b := make([]byte, len(s))
	for i := range len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
