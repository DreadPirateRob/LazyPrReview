package ui

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// diffPagerTimeout bounds how long an external diff pager may run; the pager
// executes synchronously from a key handler, so a misbehaving command must
// not be able to hang the UI.
const diffPagerTimeout = 3 * time.Second

// renderDiffFileExternal pipes the file's raw unified-diff section through
// the user-configured gui.diffPager command (run via `sh -c`) and returns its
// output lines for verbatim display. COLUMNS is set to the Main pane's
// interior width so width-aware tools (delta, diff-so-fancy) format
// correctly. It returns nil when the pager is unusable — missing raw text,
// spawn failure, non-zero exit, or empty output — so the caller can fall back
// to the built-in renderer.
func renderDiffFileExternal(m Model, idx int) []string {
	if idx < 0 || idx >= len(m.DiffFiles) {
		return nil
	}
	file := m.DiffFiles[idx]
	if strings.TrimSpace(file.Raw) == "" {
		return nil
	}

	width := ComputeLayout(m).MainWidth - 4
	if width < 40 {
		width = 40
	}

	ctx, cancel := context.WithTimeout(context.Background(), diffPagerTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", m.Config.GUI.DiffPager)
	cmd.Stdin = strings.NewReader(file.Raw)
	cmd.Env = append(os.Environ(), "COLUMNS="+strconv.Itoa(width))
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
}
