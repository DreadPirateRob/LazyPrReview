package ui

import (
	"strings"

	"github.com/DreadPirateRob/LazyPrReview/pkg/ui/keymap"
)

// HintBarView returns the one-line hint string for the currently active focus
// context. It always reflects the ACTIVE TAB/CONTEXT, not just the panel.
//
// The returned string is suitable for the bottom line of the terminal; it does
// not include a trailing newline.
func HintBarView(m Model) string {
	if !m.Config.GUI.ShowBottomLine {
		return ""
	}
	ctx := hintContext(m)
	entries := hintEntries(ctx)
	if len(entries) == 0 {
		return ""
	}
	return buildHintLine(entries)
}

// hintContext maps the current model focus to the keymap context name used for
// the hint bar. The active tab within a panel may further narrow the context.
func hintContext(m Model) string {
	switch m.CurrentFocus() {
	case FocusStatus:
		return "status"
	case FocusPRs:
		return "prs"
	case FocusFiles:
		return "files"
	case FocusThreads:
		return "threads"
	case FocusChecks:
		return "checks"
	case FocusMain:
		return "main"
	case FocusThread:
		return "thread"
	default:
		return "universal"
	}
}

// hintEntries returns the hint entries for the given context, always prefixing
// with the universal entries so the most common bindings are visible.
func hintEntries(ctx string) []hintEntry {
	all := keymap.Actions()
	var out []hintEntry

	// Per-context entries first (they are the most specific), minus a few demoted
	// bindings. The bar has a hard width budget: when a context overflows it gets
	// truncated, which silently drops ARBITRARY entries — so niche bindings are
	// demoted explicitly here and stay fully documented in `?` help.
	//
	// Demoted: `zz` centering (a vim-ism power users know) and the REVERSE half of
	// the thread/mention navigation pairs — once `t` and `m` are visible, `T`/`M`
	// follow the universal shift-reverses convention (n/N, t/T).
	demoted := map[string]bool{
		"centerCursor":         true,
		"prevUnresolvedThread": true,
		"prevMention":          true,
	}
	if ctx != "universal" {
		for _, a := range all {
			if a.Context == ctx && len(a.Keys) > 0 && !demoted[a.Name] {
				out = append(out, hintEntry{key: a.Keys[0], desc: shortDesc(a.Description)})
			}
		}
	}

	// Append a curated subset of universal entries.
	universalShortlist := []string{"showHelp"}
	for _, name := range universalShortlist {
		for _, a := range all {
			if a.Context == "universal" && a.Name == name && len(a.Keys) > 0 {
				out = append(out, hintEntry{key: a.Keys[0], desc: shortDesc(a.Description)})
			}
		}
	}
	return out
}

type hintEntry struct {
	key  string
	desc string
}

// buildHintLine formats hint entries as "key desc  key desc  …" joined by two spaces.
func buildHintLine(entries []hintEntry) string {
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = e.key + " " + e.desc
	}
	return strings.Join(parts, "  ")
}

// shortDesc trims a long description to a short label for the hint bar.
func shortDesc(s string) string {
	words := strings.Fields(s)
	if len(words) <= 3 {
		return s
	}
	// Trim to first 3 words + ellipsis if too long.
	candidate := strings.Join(words[:3], " ")
	if len(candidate) > 20 {
		return words[0]
	}
	return candidate
}
