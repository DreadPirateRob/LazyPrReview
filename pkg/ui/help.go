package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/ui/keymap"
)

// AllHelpEntries builds the canonical help table from keymap metadata.
// The returned slice mirrors the registration order in keymap.Actions().
// Call this once when the help overlay is opened rather than keeping a permanent
// copy. b is the effective binding table; pass nil to describe the shipped
// defaults. Actions config disabled are omitted, since `?` must advertise only
// what actually works.
func AllHelpEntries(b *keymap.Bindings) []HelpEntry {
	actions := keymap.Actions()
	entries := make([]HelpEntry, 0, len(actions))
	for _, a := range actions {
		keys := a.Keys
		if b != nil {
			keys = b.DisplayKeys(a.Context, a.Name)
			if len(keys) == 0 {
				// Disabled in config. Dropping the row entirely is the point: a help
				// entry with no keys still reads as "this exists", which is exactly
				// the mismatch between `?` and reality this change removes.
				continue
			}
		}
		entries = append(entries, HelpEntry{
			Context:     a.Context,
			Action:      a.Name,
			Keys:        append([]string(nil), keys...),
			Description: a.Description,
		})
	}
	return entries
}

// FilterHelpEntries returns the subset of entries that match query as a
// case-insensitive substring of any of: action name, any rendered key label,
// or description. Source order is preserved. An empty query returns entries
// unchanged (no allocation).
func FilterHelpEntries(entries []HelpEntry, query string) []HelpEntry {
	if query == "" {
		return entries
	}
	q := strings.ToLower(query)
	out := entries[:0:0] // nil-like but typed; avoids returning nil vs empty
	for _, e := range entries {
		if matchesHelp(e, q) {
			out = append(out, e)
		}
	}
	return out
}

// matchesHelp reports whether e contains q in any searchable field.
// q must already be lower-cased.
func matchesHelp(e HelpEntry, q string) bool {
	if strings.Contains(strings.ToLower(e.Action), q) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Description), q) {
		return true
	}
	for _, k := range e.Keys {
		if strings.Contains(strings.ToLower(k), q) {
			return true
		}
	}
	return false
}

// UpdateHelp scrolls the help overlay within its bounded viewport.
func UpdateHelp(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	max := len(FilterHelpEntries(scopedHelpEntries(m), m.HelpQuery)) - 1
	if max < 0 {
		max = 0
	}
	switch msg.Keystroke() {
	case "j", "down":
		if m.HelpCursor < max {
			m.HelpCursor++
		}
	case "k", "up":
		if m.HelpCursor > 0 {
			m.HelpCursor--
		}
	}
	return m, nil
}

func scopedHelpEntries(m Model) []HelpEntry {
	entries := m.HelpEntries
	if len(entries) == 0 {
		entries = AllHelpEntries(m.Keys)
	}
	scoped := entries[:0:0]
	active := string(m.CurrentFocus())
	if active == string(FocusHelp) && len(m.FocusStack) > 1 {
		active = string(m.FocusStack[len(m.FocusStack)-2])
	}
	for _, entry := range entries {
		if entry.Context == "universal" || entry.Context == active {
			scoped = append(scoped, entry)
		}
	}
	return scoped
}
