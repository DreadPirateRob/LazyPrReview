package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
)

// focusedFilterPanel returns the panel whose filter the `/` input edits for
// the current focus, or nil when the focused context has no filterable list.
func focusedFilterPanel(m *Model) *Panel {
	switch m.CurrentFocus() {
	case FocusPRs:
		return &m.PRPanel
	case FocusFiles:
		return &m.FilesPanel
	case FocusThreads:
		return &m.ThreadsPanel
	case FocusChecks:
		return &m.ChecksPanel
	}
	return nil
}

// handleFilterInput consumes every key while a panel filter is being typed:
// esc cancels (clears the query), enter commits (keeps it), backspace edits,
// and printable single characters append. The cursor resets on each mutation
// so it can never point past the shrinking filtered list.
func handleFilterInput(m Model, ks string) (Model, tea.Cmd) {
	p := focusedFilterPanel(&m)
	if p == nil {
		m.FilterActive = false
		return m, nil
	}
	switch ks {
	case "esc", "<esc>":
		m.FilterActive = false
		p.Filter = ""
		p.Cursor = 0
	case "enter", "<enter>":
		m.FilterActive = false
		p.Cursor = 0
		// On the PR Search tab, committing runs a repo-wide server search —
		// unless the identical query is still cached fresh.
		if m.CurrentFocus() == FocusPRs && m.PRFilter == forge.FilterSearch {
			if prs, ok := m.cachedSearch(p.Filter); ok {
				m.PRs = prs
				return m, nil
			}
			m.Activity = "Searching…"
			m.LoadingPRs = true
			return m, searchPRsCmd(m.Forge, m.Repo, p.Filter)
		}
	case "backspace", "<backspace>":
		if len(p.Filter) > 0 {
			runes := []rune(p.Filter)
			p.Filter = string(runes[:len(runes)-1])
			p.Cursor = 0
		}
	default:
		// Only accept printable single characters.
		if len(ks) == 1 {
			p.Filter += ks
			p.Cursor = 0
		}
	}
	// Every branch above resets the cursor to 0 because the visible row set just
	// changed. The panels whose selection drives Main therefore have to re-point it,
	// or the panel highlights one row while Main still shows the previous one.
	switch m.CurrentFocus() {
	case FocusFiles:
		m = followFilesSelection(m)
	case FocusThreads:
		m = followThreadsSelection(m)
	}
	return m, nil
}

// startPanelFilter arms the filter input for the given panel.
func startPanelFilter(m Model, p *Panel) Model {
	m.FilterActive = true
	p.Filter = ""
	p.Cursor = 0
	return m
}

// tabsTitle joins a panel's tabs lazygit-style for its border title, with the
// active tab bracketed: "[Unresolved] - All - Commits".
func tabsTitle(tabs []string, active string) string {
	parts := make([]string, len(tabs))
	for i, t := range tabs {
		if t == active {
			parts[i] = "[" + t + "]"
		} else {
			parts[i] = t
		}
	}
	return strings.Join(parts, " - ")
}

// filterSuffix is appended to a panel's border title: a live input cursor
// while typing, a compact marker for a committed query, empty otherwise.
func filterSuffix(m Model, ctx FocusContext, p Panel) string {
	if m.FilterActive && m.CurrentFocus() == ctx {
		return " /" + p.Filter + "█"
	}
	if p.Filter != "" {
		return " /" + p.Filter
	}
	return ""
}

// panelFilterMatch reports whether candidate matches the panel query under
// the configured filter mode (substring or fuzzy).
func panelFilterMatch(m Model, candidate, query string) bool {
	if query == "" {
		return true
	}
	return matchesFilter(strings.ToLower(candidate), strings.ToLower(query), m.Config.GUI.FilterMode)
}
