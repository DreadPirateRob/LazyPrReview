package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

var (
	checkOKStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	checkFailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	checkRunStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	checkOffStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// checkGlyphStyle maps a CheckGlyph result to its severity color (SPEC §8):
// ✓ green ok · ✗/± red blocking · ◔ yellow running · ⊘ gray inactive/skipped.
func checkGlyphStyle(glyph string) lipgloss.Style {
	switch glyph {
	case "✓":
		return checkOKStyle
	case "✗", "±":
		return checkFailStyle
	case "◔":
		return checkRunStyle
	default:
		return checkOffStyle
	}
}

// ChecksTabChecks and ChecksTabTimeline are the two tab identifiers for the
// Checks panel. They are stored in ChecksPanel.Tab.
const (
	ChecksTabChecks   = "Checks"
	ChecksTabTimeline = "Timeline"
)

// checksTabOrder is the canonical tab cycle order.
var checksTabOrder = []string{ChecksTabChecks, ChecksTabTimeline}

// CheckGlyph returns the NF/UTF-8 glyph for a check run or status context.
// Rules from SPEC.md §8:
//
//   - completed + success/neutral          → ✓  (green)
//   - completed + failure/error/timed_out  → ✗  (red)
//   - completed + cancelled/stale          → ⊘  (gray)
//   - completed + action_required          → ±  (red)
//   - in_progress / queued / waiting       → ◔  (yellow)
//   - skipped                              → ⊘  (gray)
//
// For commit-status contexts (Kind=="status") map State directly:
//
//	success → ✓, failure/error → ✗, pending → ◔, "" → ⊘
func CheckGlyph(c domain.Check) string {
	if c.Kind == "status" {
		switch strings.ToLower(c.State) {
		case "success":
			return "✓"
		case "failure", "error":
			return "✗"
		case "pending":
			return "◔"
		default:
			return "⊘"
		}
	}
	// CheckRun: examine status + conclusion
	switch strings.ToLower(c.Status) {
	case "completed":
		switch strings.ToLower(c.Conclusion) {
		case "success", "neutral":
			return "✓"
		case "failure", "error", "timed_out", "startup_failure":
			return "✗"
		case "action_required":
			return "±"
		case "cancelled", "stale", "skipped":
			return "⊘"
		default:
			return "⊘"
		}
	case "in_progress":
		return "◔"
	case "queued", "waiting", "pending":
		return "◔"
	default:
		return "⊘"
	}
}

// checksTitle composes the Checks panel border title. When the panel is
// focused it shows both tabs (active bracketed) + filter; unfocused it
// collapses to a plain "Checks" label to match the one-line summary body.
func checksTitle(m Model) string {
	if m.CurrentFocus() != FocusChecks {
		return "Checks"
	}
	return tabsTitle(checksTabOrder, checksActiveTab(m)) + filterSuffix(m, FocusChecks, m.ChecksPanel)
}

// ChecksView renders the Checks panel body. Unless the panel is focused it
// shows a one-line CI health summary; focused, it expands to the full
// Checks/Timeline tab view.
func ChecksView(m Model) string {
	if m.CurrentFocus() != FocusChecks {
		return checksSummary(m) + "\n"
	}
	var sb strings.Builder
	switch checksActiveTab(m) {
	case ChecksTabTimeline:
		renderTimelineRows(&sb, m)
	default:
		renderCheckRows(&sb, m)
	}
	return sb.String()
}

// checksSummary is the collapsed one-liner: overall CI health with a count of
// failing (or running) checks, e.g. "✗ 3 checks failing" / "✓ CI passing".
func checksSummary(m Model) string {
	if m.PRDetail == nil || len(m.PRDetail.Checks) == 0 {
		return "  (no checks)"
	}
	fail, running := 0, 0
	for _, c := range m.PRDetail.Checks {
		switch CheckGlyph(c) {
		case "✗", "±":
			fail++
		case "◔":
			running++
		}
	}
	switch {
	case fail > 0:
		return fmt.Sprintf("  %s %d %s failing", checkFailStyle.Render("✗"), fail, checkNoun(fail))
	case running > 0:
		return fmt.Sprintf("  %s %d %s running", checkRunStyle.Render("◔"), running, checkNoun(running))
	default:
		return "  " + checkOKStyle.Render("✓") + " CI passing"
	}
}

func checkNoun(n int) string {
	if n == 1 {
		return "check"
	}
	return "checks"
}

// filteredCheckIndices returns indices into PRDetail.Checks whose name or
// description matches the Checks panel filter.
func filteredCheckIndices(m Model) []int {
	if m.PRDetail == nil {
		return nil
	}
	out := make([]int, 0, len(m.PRDetail.Checks))
	for i, c := range m.PRDetail.Checks {
		if panelFilterMatch(m, c.Name+" "+c.Description, m.ChecksPanel.Filter) {
			out = append(out, i)
		}
	}
	return out
}

// filteredTimeline returns the timeline items matching the Checks panel
// filter (author, kind, and body).
func filteredTimeline(m Model) []domain.TimelineItem {
	if m.PRDetail == nil {
		return nil
	}
	if m.ChecksPanel.Filter == "" {
		return m.PRDetail.Timeline
	}
	out := make([]domain.TimelineItem, 0, len(m.PRDetail.Timeline))
	for _, item := range m.PRDetail.Timeline {
		if panelFilterMatch(m, item.Author+" "+item.Kind+" "+item.Body, m.ChecksPanel.Filter) {
			out = append(out, item)
		}
	}
	return out
}

func renderCheckRows(sb *strings.Builder, m Model) {
	vis := filteredCheckIndices(m)
	if len(vis) == 0 {
		sb.WriteString("  (no checks)\n")
		return
	}
	for pos, real := range vis {
		c := m.PRDetail.Checks[real]
		cursor := "  "
		if pos == m.ChecksPanel.Cursor {
			cursor = "> "
		}
		glyph := CheckGlyph(c)
		desc := c.Name
		if c.Description != "" {
			desc = fmt.Sprintf("%s — %s", c.Name, c.Description)
		}
		sb.WriteString(fmt.Sprintf("%s%s %s\n", cursor, checkGlyphStyle(glyph).Render(glyph), desc))
	}
}

func renderTimelineRows(sb *strings.Builder, m Model) {
	items := filteredTimeline(m)
	if len(items) == 0 {
		sb.WriteString("  (no timeline events)\n")
		return
	}
	for _, item := range items {
		when := item.SortAt.Format("2006-01-02 15:04")
		body := item.Body
		if len(body) > 60 {
			body = body[:57] + "..."
		}
		// Single-line: "YYYY-MM-DD HH:MM author [kind] body"
		sb.WriteString(fmt.Sprintf("  %s %s [%s] %s\n", when, item.Author, item.Kind, body))
	}
}

// UpdateChecks handles keystrokes when the Checks panel is focused.
func UpdateChecks(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	ks := msg.Keystroke()
	switch ks {
	case "/":
		m = startPanelFilter(m, &m.ChecksPanel)
		return m, nil
	case "k", "<up>":
		if m.ChecksPanel.Cursor > 0 {
			m.ChecksPanel.Cursor--
		}
	case "j", "<down>":
		max := checksMaxCursor(m, checksActiveTab(m))
		if m.ChecksPanel.Cursor < max {
			m.ChecksPanel.Cursor++
		}
	case "[":
		m = cycleChecksTab(m, -1)
	case "]":
		m = cycleChecksTab(m, +1)
	case "<":
		m.ChecksPanel.Cursor = 0
	case ">":
		m.ChecksPanel.Cursor = checksMaxCursor(m, checksActiveTab(m))
	case "o":
		// Open check detail URL in browser
		if m.PRDetail != nil && checksActiveTab(m) == ChecksTabChecks {
			vis := filteredCheckIndices(m)
			if m.ChecksPanel.Cursor < len(vis) {
				c := m.PRDetail.Checks[vis[m.ChecksPanel.Cursor]]
				url := c.DetailsURL
				if url == "" {
					url = c.TargetURL
				}
				if url != "" {
					return m, openBrowserCmd(url)
				}
			}
		}
	}
	return m, nil
}

// cycleChecksTab advances or retreats the Checks panel tab index.
func cycleChecksTab(m Model, delta int) Model {
	activeTab := checksActiveTab(m)
	cur := 0
	for i, t := range checksTabOrder {
		if t == activeTab {
			cur = i
			break
		}
	}
	next := (cur + delta + len(checksTabOrder)) % len(checksTabOrder)
	m.ChecksPanel.Tab = checksTabOrder[next]
	m.ChecksPanel.Cursor = 0
	m.ChecksPanel.Filter = ""
	return m
}

func checksActiveTab(m Model) string {
	if m.ChecksPanel.Tab == "" {
		return ChecksTabChecks
	}
	return m.ChecksPanel.Tab
}

func checksMaxCursor(m Model, tab string) int {
	if m.PRDetail == nil {
		return 0
	}
	switch tab {
	case ChecksTabTimeline:
		if n := len(filteredTimeline(m)); n > 0 {
			return n - 1
		}
	default:
		if n := len(filteredCheckIndices(m)); n > 0 {
			return n - 1
		}
	}
	return 0
}

// openBrowserCmd is a stub Tea command; the actual open-URL behavior is
// provided by cmd/lazypr, which hooks the message in step 7.
func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg { return openBrowserMsg{URL: url} }
}

// openBrowserMsg requests that the terminal shell open a URL in the default browser.
type openBrowserMsg struct {
	URL string
}
