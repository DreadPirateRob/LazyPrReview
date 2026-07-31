package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

// CommandLogCapacity is the maximum number of entries stored in the ring
// buffer. When it is full, the oldest entry is silently discarded.
const CommandLogCapacity = 200

// CommandLogMenuOption enumerates the Phase 1 command-log menu options.
// "clear visible errors" is intentionally absent; it belongs to a later phase.
type CommandLogMenuOption string

const (
	// CmdLogToggle shows or hides the command-log pane.
	CmdLogToggle CommandLogMenuOption = "show/hide"
	// CmdLogFocus moves focus into the command-log pane so the user can scroll it.
	CmdLogFocus CommandLogMenuOption = "focus"
)

// Phase1CommandLogMenuOptions is the ordered list of options shown by @ in Phase 1.
var Phase1CommandLogMenuOptions = []CommandLogMenuOption{
	CmdLogToggle,
	CmdLogFocus,
}

// CommandLogRing is a fixed-capacity ring buffer for ghcli.CommandLogEntry values.
// It is not safe for concurrent use; callers must serialise access via the Tea
// update loop.
type CommandLogRing struct {
	buf   [CommandLogCapacity]ghcli.CommandLogEntry
	write int // index of the next write slot
	size  int // number of valid entries currently held
}

// Append adds e to the ring. If the buffer is full, the oldest entry is
// overwritten.
func (r *CommandLogRing) Append(e ghcli.CommandLogEntry) {
	r.buf[r.write] = e
	r.write = (r.write + 1) % CommandLogCapacity
	if r.size < CommandLogCapacity {
		r.size++
	}
	// When size == CommandLogCapacity the write cursor has just stepped over
	// the oldest slot — it is now the new oldest, so no additional bookkeeping
	// is needed: the next Entries call computes the correct start.
}

// Entries returns a snapshot of all entries in chronological order (oldest
// first). The returned slice is newly allocated on every call.
func (r *CommandLogRing) Entries() []ghcli.CommandLogEntry {
	if r.size == 0 {
		return nil
	}
	out := make([]ghcli.CommandLogEntry, r.size)
	start := (r.write - r.size + CommandLogCapacity) % CommandLogCapacity
	for i := range r.size {
		out[i] = r.buf[(start+i)%CommandLogCapacity]
	}
	return out
}

// Len returns the number of entries currently held.
func (r *CommandLogRing) Len() int { return r.size }

// Clear removes all entries from the ring without releasing memory.
func (r *CommandLogRing) Clear() {
	r.write = 0
	r.size = 0
}

// UpdateCommandLog scrolls the visible command log when it is focused.
func UpdateCommandLog(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.Keystroke() {
	case "j", "down":
		if m.CommandLogOffset < len(m.CommandLog)-1 {
			m.CommandLogOffset++
		}
	case "k", "up":
		if m.CommandLogOffset > 0 {
			m.CommandLogOffset--
		}
	}
	return m, nil
}
