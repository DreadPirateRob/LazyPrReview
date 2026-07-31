package ui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// rateLimitWarnThreshold is the remaining-GraphQL-budget floor: below it the
// Status line warns and auto-refresh pauses (SPEC §rate budget).
const rateLimitWarnThreshold = 100

var rateLimitWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

// statusSummary is the collapsed one-liner shown when the Status panel is not
// focused: repo, plus review decision and rate budget when a PR is open.
func statusSummary(m Model) string {
	s := fmt.Sprintf("%s/%s", m.Repo.Owner, m.Repo.Name)
	if m.PRDetail != nil {
		if m.PRDetail.ReviewDecision != "" {
			s += " · " + m.PRDetail.ReviewDecision
		}
		if m.PRDetail.RateLimitRemaining > 0 {
			rl := fmt.Sprintf("rate %d", m.PRDetail.RateLimitRemaining)
			if m.PRDetail.RateLimitRemaining < rateLimitWarnThreshold {
				rl = rateLimitWarnStyle.Render(rl + " ⚠")
			}
			s += " · " + rl
		}
	}
	return s
}

// StatusView renders the Status panel: a one-line summary when unfocused,
// or the full repo/viewer/review/reviewers/rate/pending view when focused.
func StatusView(m Model) string {
	if m.CurrentFocus() != FocusStatus {
		return statusSummary(m) + "\n"
	}
	var sb strings.Builder

	// Repo
	sb.WriteString(fmt.Sprintf("repo: %s/%s\n", m.Repo.Owner, m.Repo.Name))

	// Viewer
	if m.ViewerLogin != "" {
		sb.WriteString(fmt.Sprintf("viewer: %s\n", m.ViewerLogin))
	}

	// PR-level info (only when a PR is open)
	if m.PRDetail != nil {
		// Review decision
		if m.PRDetail.ReviewDecision != "" {
			sb.WriteString(fmt.Sprintf("review: %s\n", m.PRDetail.ReviewDecision))
		}

		// Requested reviewers
		if len(m.PRDetail.RequestedReviewers) > 0 {
			names := make([]string, len(m.PRDetail.RequestedReviewers))
			for i, r := range m.PRDetail.RequestedReviewers {
				if r.Kind == "team" {
					names[i] = "@" + r.Name
				} else {
					names[i] = r.Name
				}
			}
			sb.WriteString(fmt.Sprintf("reviewers: %s\n", strings.Join(names, ", ")))
		}

		// Rate limit
		if m.PRDetail.RateLimitRemaining > 0 {
			line := fmt.Sprintf("rate limit: %d", m.PRDetail.RateLimitRemaining)
			if m.PRDetail.RateLimitRemaining < rateLimitWarnThreshold {
				line = rateLimitWarnStyle.Render(line + " ⚠ low")
			}
			sb.WriteString(line + "\n")
		}

		// Pending review banner
		if n := m.PRDetail.PendingReviewCount + optimisticUnconfirmed(m); n > 0 {
			sb.WriteString(fmt.Sprintf("PENDING review: %d draft comment(s) (S to submit)\n", n))
		}
	}

	return sb.String()
}

// UpdateStatus handles keystrokes when the Status panel is focused.
func UpdateStatus(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.Keystroke() {
	case "j", "down":
		m.Status.Cursor++
	case "k", "up":
		if m.Status.Cursor > 0 {
			m.Status.Cursor--
		}
	case "e":
		return handleEditConfig(m)
	}
	return m, nil
}

// handleEditConfig implements the `e` binding: suspend TUI and run $EDITOR on
// the global config path, or show a toast if $EDITOR is unset.
func handleEditConfig(m Model) (Model, tea.Cmd) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		m.Toast = NewToast(ToastSetEditorForConfig)
		return m, nil
	}
	// Phase 1: emit a tea.Cmd that suspends the program, runs $EDITOR, and
	// returns a configReloadMsg. The actual exec is hooked by cmd/lazypr in
	// step 7.1; here we just emit the intent message so the shell compiles.
	return m, func() tea.Msg { return configEditRequestMsg{Editor: editor} }
}

// configEditRequestMsg is sent when the user presses `e` in the Status panel
// and $EDITOR is set. The CLI entry point (cmd/lazypr) handles the suspension
// and reload; the message is forwarded up through the Update loop.
type configEditRequestMsg struct {
	Editor string
}
