package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
)

const (
	menuKindCopy             = "copy"
	menuKindToggleCommandLog = "toggle-command-log"
	menuKindFocusCommandLog  = "focus-command-log"
	menuKindReview           = "review"          // Value = review event
	menuKindDiscardReview    = "discard-review"  // opens the confirm menu
	menuKindDiscardConfirm   = "discard-confirm" // performs the discard
	menuKindClose            = "close"           // no-op, just closes the menu
	menuKindDeleteConfirm    = "delete-confirm"  // deletes DeleteTarget
)

func openCopyMenu(m Model) Model {
	m.MenuTitle = "Copy"
	m.MenuItems = copyMenuItems(m)
	m.MenuCursor = 0
	if len(m.MenuItems) > 0 {
		m = m.PushFocus(FocusMenu)
	}
	return m
}

func openCommandLogMenu(m Model) Model {
	m.MenuTitle = "Command log"
	m.MenuItems = []MenuItem{
		{Label: "show/hide", Key: "s", Kind: menuKindToggleCommandLog},
		{Label: "focus", Key: "f", Kind: menuKindFocusCommandLog},
	}
	m.MenuCursor = 0
	m = m.PushFocus(FocusMenu)
	return m
}

// resolvePendingReviewID returns the review to act on: the locally-cached id
// (set as soon as an ensure succeeds, covering the gap before reconcile) or the
// server's adopted pending review, else "".
func resolvePendingReviewID(m Model) string {
	if m.PendingReviewID != "" {
		return m.PendingReviewID
	}
	if m.PRDetail != nil && m.PRDetail.PendingReview != nil {
		return m.PRDetail.PendingReview.ID
	}
	return ""
}

// openSubmitMenu shows the review-submit choices (SPEC §9). Always available in
// an open PR — Approve can create a review on the fly.
func openSubmitMenu(m Model) Model {
	if m.PRDetail == nil {
		return m
	}
	n := m.PRDetail.PendingReviewCount + optimisticUnconfirmed(m)
	m.MenuTitle = fmt.Sprintf("Submit review — %d draft comment(s)", n)
	m.MenuItems = []MenuItem{
		{Label: "Approve", Key: "a", Kind: menuKindReview, Value: "APPROVE"},
		{Label: "Request changes", Key: "r", Kind: menuKindReview, Value: "REQUEST_CHANGES"},
		{Label: "Comment", Key: "c", Kind: menuKindReview, Value: "COMMENT"},
		{Label: "Discard pending review", Key: "d", Kind: menuKindDiscardReview},
	}
	m.MenuCursor = 0
	return m.PushFocus(FocusMenu)
}

// openDiscardConfirmMenu is the destructive-action confirm step for discard.
func openDiscardConfirmMenu(m Model) Model {
	m.MenuTitle = "Discard pending review? This deletes all drafts"
	m.MenuItems = []MenuItem{
		{Label: "Yes, discard", Key: "y", Kind: menuKindDiscardConfirm},
		{Label: "No, keep", Key: "n", Kind: menuKindClose},
	}
	m.MenuCursor = 1 // default to the safe choice
	return m.PushFocus(FocusMenu)
}

// openDeleteConfirmMenu confirms deleting the comment stored in DeleteTarget.
func openDeleteConfirmMenu(m Model) Model {
	m.MenuTitle = "Delete this comment?"
	m.MenuItems = []MenuItem{
		{Label: "Yes, delete", Key: "y", Kind: menuKindDeleteConfirm},
		{Label: "No, keep", Key: "n", Kind: menuKindClose},
	}
	m.MenuCursor = 1 // default to the safe choice
	return m.PushFocus(FocusMenu)
}

func copyMenuItems(m Model) []MenuItem {
	items := []MenuItem{}
	if m.PRDetail != nil && m.PRDetail.URL != "" {
		items = append(items, MenuItem{Label: "PR URL", Key: "p", Kind: menuKindCopy, Value: m.PRDetail.URL, Toast: ToastCopiedPRURL})
		if m.PRDetail.HeadRefName != "" {
			items = append(items, MenuItem{Label: "Branch name", Key: "b", Kind: menuKindCopy, Value: m.PRDetail.HeadRefName, Toast: ToastCopiedBranchName})
		}
		if m.PRDetail.HeadRefOID != "" {
			items = append(items, MenuItem{Label: "Head SHA", Key: "s", Kind: menuKindCopy, Value: m.PRDetail.HeadRefOID, Toast: ToastCopiedHeadSHA})
		}
	}
	if m.PRDetail != nil && m.FilesPanel.Cursor >= 0 && m.FilesPanel.Cursor < len(m.PRDetail.Files) {
		items = append(items, MenuItem{Label: "File path", Key: "f", Kind: menuKindCopy, Value: m.PRDetail.Files[m.FilesPanel.Cursor].Path, Toast: ToastCopiedFilePath})
	}
	if url := currentThreadURL(m); url != "" {
		items = append(items, MenuItem{Label: "Thread permalink", Key: "t", Kind: menuKindCopy, Value: url, Toast: ToastCopiedThreadPermalink})
	}
	if body := currentThreadBody(m); body != "" {
		items = append(items, MenuItem{Label: "Comment body", Key: "c", Kind: menuKindCopy, Value: body, Toast: ToastCopiedCommentBody})
	}
	return items
}

func UpdateMenu(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if len(m.MenuItems) == 0 {
		return m, nil
	}
	switch msg.Keystroke() {
	case "j", "down":
		if m.MenuCursor < len(m.MenuItems)-1 {
			m.MenuCursor++
		}
		return m, nil
	case "k", "up":
		if m.MenuCursor > 0 {
			m.MenuCursor--
		}
		return m, nil
	case "enter":
		return applyMenuItem(m, m.MenuItems[m.MenuCursor])
	}
	for _, item := range m.MenuItems {
		if msg.Keystroke() == item.Key {
			return applyMenuItem(m, item)
		}
	}
	return m, nil
}

func applyMenuItem(m Model, item MenuItem) (Model, tea.Cmd) {
	m = m.PopFocus()
	switch item.Kind {
	case menuKindCopy:
		return m, copyCmd(item.Value, item.Toast)
	case menuKindToggleCommandLog:
		m.CommandLogOn = !m.CommandLogOn
		if !m.CommandLogOn {
			m.CommandLogFocused = false
		}
		return m, nil
	case menuKindFocusCommandLog:
		m.CommandLogOn = true
		m.CommandLogFocused = true
		return m, nil
	case menuKindReview:
		event := forge.ReviewEvent(item.Value)
		title := "Approve — optional message · ctrl+s submit"
		switch event {
		case "REQUEST_CHANGES":
			title = "Request changes — message required · ctrl+s submit"
		case "COMMENT":
			title = "Comment review — message required · ctrl+s submit"
		}
		return openComposer(m, composeState{Kind: composeReview, Event: event, Required: event != "APPROVE", Title: title}), nil
	case menuKindDiscardReview:
		return openDiscardConfirmMenu(m), nil
	case menuKindDiscardConfirm:
		m.Activity = "Discarding review…"
		return m, discardReviewCmd(m.Forge, resolvePendingReviewID(m))
	case menuKindDeleteConfirm:
		m.Activity = "Deleting comment…"
		return m, deleteCommentCmd(m.Forge, m.Repo, m.DeleteTarget)
	case menuKindClose:
		return m, nil
	default:
		return m, nil
	}
}

func MenuView(m Model) string {
	if len(m.MenuItems) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s menu\n", m.MenuTitle))
	for i, item := range m.MenuItems {
		cursor := " "
		if i == m.MenuCursor {
			cursor = ">"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] %s\n", cursor, item.Key, item.Label))
	}
	return sb.String()
}
