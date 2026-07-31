package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

type copyResultMsg struct {
	Toast string
	Err   error
}

func copyCmd(value, toast string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(value); err != nil {
			return copyResultMsg{Err: err}
		}
		return copyResultMsg{Toast: toast}
	}
}

func openURLCmd(url, configured string) tea.Cmd {
	return func() tea.Msg {
		cmdName := configured
		if cmdName == "" {
			switch runtime.GOOS {
			case "darwin":
				cmdName = "open"
			case "windows":
				cmdName = "cmd"
			default:
				cmdName = "xdg-open"
			}
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" && configured == "" {
			cmd = exec.Command(cmdName, "/c", "start", "", url)
		} else {
			cmd = exec.Command(cmdName, url)
		}
		if err := cmd.Run(); err != nil {
			return mutationFailedMsg{Action: "openBrowser", Err: err}
		}
		return nil
	}
}

func copyActionForFocus(m Model) tea.Cmd {
	switch m.CurrentFocus() {
	case FocusFiles, FocusMain:
		if m.PRDetail != nil && m.MainFileIndex < len(m.PRDetail.Files) {
			return copyCmd(m.PRDetail.Files[m.MainFileIndex].Path, ToastCopiedFilePath)
		}
	case FocusThread:
		if url := currentThreadURL(m); url != "" {
			return copyCmd(url, ToastCopiedThreadPermalink)
		}
	case FocusStatus:
		if m.PRDetail != nil && m.PRDetail.HeadRefName != "" {
			return copyCmd(m.PRDetail.HeadRefName, ToastCopiedBranchName)
		}
	case FocusChecks:
		if m.PRDetail != nil && m.PRDetail.HeadRefOID != "" {
			return copyCmd(m.PRDetail.HeadRefOID, ToastCopiedHeadSHA)
		}
	}
	if m.PRDetail != nil && m.PRDetail.URL != "" {
		return copyCmd(m.PRDetail.URL, ToastCopiedPRURL)
	}
	return nil
}

func browserActionForFocus(m Model) tea.Cmd {
	url := ""
	switch m.CurrentFocus() {
	case FocusPRs:
		prs := VisiblePRs(m)
		if m.PRPanel.Cursor >= 0 && m.PRPanel.Cursor < len(prs) {
			url = prs[m.PRPanel.Cursor].URL
		}
	case FocusThread:
		url = currentThreadURL(m)
	case FocusChecks:
		if m.PRDetail != nil {
			rows := m.PRDetail.Checks
			idx := m.ChecksPanel.Cursor
			if idx >= 0 && idx < len(rows) {
				url = rows[idx].DetailsURL
				if url == "" {
					url = rows[idx].TargetURL
				}
			}
		}
	case FocusFiles:
		url = currentSelectedFileURL(m)
	case FocusMain:
		url = currentFileURL(m, true)
	default:
		if m.PRDetail != nil {
			url = m.PRDetail.URL
		}
	}
	if url == "" {
		return nil
	}
	return openURLCmd(url, m.Config.OS.OpenLink)
}

func currentFileURL(m Model, lineScoped bool) string {
	if m.PRDetail == nil || m.MainFileIndex < 0 || m.MainFileIndex >= len(m.DiffFiles) {
		if m.PRDetail != nil {
			return m.PRDetail.URL
		}
		return ""
	}
	file := m.DiffFiles[m.MainFileIndex]
	sum := sha256.Sum256([]byte(file.Path))
	base := fmt.Sprintf("%s/files#diff-%s", m.PRDetail.URL, hex.EncodeToString(sum[:]))
	if m.Config.GUI.DiffPager != "" {
		// Pager output lines don't map to Rendered indices; only the
		// file-level anchor is trustworthy.
		lineScoped = false
	}
	if !lineScoped || m.MainCursor <= 0 || m.MainCursor-1 >= len(file.Rendered) {
		return base
	}
	line := file.Rendered[m.MainCursor-1]
	if line.NewNo != nil {
		return fmt.Sprintf("%sR%d", base, *line.NewNo)
	}
	if line.OldNo != nil {
		return fmt.Sprintf("%sL%d", base, *line.OldNo)
	}
	return base
}

func currentSelectedFileURL(m Model) string {
	if m.PRDetail == nil || m.FilesPanel.Cursor < 0 || m.FilesPanel.Cursor >= len(m.PRDetail.Files) {
		if m.PRDetail != nil {
			return m.PRDetail.URL
		}
		return ""
	}
	path := m.PRDetail.Files[m.FilesPanel.Cursor].Path
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%s/files#diff-%s", m.PRDetail.URL, hex.EncodeToString(sum[:]))
}

func currentThreadURL(m Model) string {
	if m.CurrentFocus() == FocusThread {
		if at, ok := focusedThread(m); ok && len(at.Thread.Comments) > 0 {
			return at.Thread.Comments[0].URL
		}
		return ""
	}
	rows := visibleThreads(m)
	if m.ThreadsPanel.Cursor >= 0 && m.ThreadsPanel.Cursor < len(rows) && len(rows[m.ThreadsPanel.Cursor].Thread.Comments) > 0 {
		return rows[m.ThreadsPanel.Cursor].Thread.Comments[0].URL
	}
	return ""
}

func currentThreadBody(m Model) string {
	if m.CurrentFocus() == FocusThread {
		if c, ok := focusedComment(m); ok {
			return c.Body
		}
		return ""
	}
	rows := visibleThreads(m)
	if m.ThreadsPanel.Cursor >= 0 && m.ThreadsPanel.Cursor < len(rows) && len(rows[m.ThreadsPanel.Cursor].Thread.Comments) > 0 {
		return rows[m.ThreadsPanel.Cursor].Thread.Comments[0].Body
	}
	return ""
}

func copyFailureToast(err error) string {
	return fmt.Sprintf("clipboard failed: %v", err)
}
