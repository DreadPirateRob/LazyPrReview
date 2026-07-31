package ui

import (
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

// screenModeOrder defines the +/_ cycle direction.
var screenModeOrder = []ScreenMode{ScreenNormal, ScreenHalf, ScreenFullscreen}

// New creates a root Model in the initial loading state.
// The caller (cmd/lazypr) is responsible for injecting a probes Cmd into the
// program before starting the event loop.
func New(cfg config.Config, f forge.Forge) Model {
	sm := ScreenMode(cfg.GUI.ScreenMode)
	switch sm {
	case ScreenNormal, ScreenHalf, ScreenFullscreen:
	default:
		sm = ScreenNormal
	}
	return Model{
		Config:      cfg,
		Forge:       f,
		FocusStack:  []FocusContext{FocusPRs},
		ScreenMode:  sm,
		Loading:     true,
		PRCache:     map[forge.PRFilter]prCacheEntry{},
		SearchCache: map[string]prCacheEntry{},
	}
}

// ── Focus stack ──────────────────────────────────────────────────────────────

// CurrentFocus returns the active focus context (top of stack).
// Falls back to FocusPRs on an empty stack.
func (m Model) CurrentFocus() FocusContext {
	if len(m.FocusStack) == 0 {
		return FocusPRs
	}
	return m.FocusStack[len(m.FocusStack)-1]
}

// PushFocus appends ctx onto the focus stack. The receiver is copied, so the
// original model is unmodified.
func (m Model) PushFocus(ctx FocusContext) Model {
	next := make([]FocusContext, len(m.FocusStack)+1)
	copy(next, m.FocusStack)
	next[len(m.FocusStack)] = ctx
	m.FocusStack = next
	return m
}

// PopFocus removes the top focus context. A stack of one entry is left intact
// so navigation always has a home position.
func (m Model) PopFocus() Model {
	if len(m.FocusStack) <= 1 {
		return m
	}
	next := make([]FocusContext, len(m.FocusStack)-1)
	copy(next, m.FocusStack[:len(m.FocusStack)-1])
	m.FocusStack = next
	return m
}

// showPROverview switches the Main pane back to the PR overview.
func showPROverview(m Model) Model {
	if m.PRDetail == nil {
		return m
	}
	m.MainMode = MainOverview
	m = setMainLines(m, composePROverview(*m.PRDetail))
	m.MainCursor = 0
	m.MainScroll = 0
	m.MainPendingZ = false
	return m
}

// followPRSelection puts Main back on the opened PR's overview when the PR list
// takes focus, mirroring how focusing Files shows the selected file's diff.
// Already showing the overview is a no-op, so Main keeps its scroll position.
//
// It never fetches: unlike a file diff (already local), another PR's detail needs
// network round-trips, so navigating the list never previews — enter still opens.
func followPRSelection(m Model) Model {
	if m.PRDetail == nil || m.MainMode == MainOverview {
		return m
	}
	return showPROverview(m)
}

// ── Screen mode ──────────────────────────────────────────────────────────────

// NextScreenMode advances one step through the ScreenMode cycle.
func (m Model) NextScreenMode() Model {
	m.ScreenMode = cycleScreenMode(m.ScreenMode, +1)
	return m
}

// PrevScreenMode retreats one step through the ScreenMode cycle.
func (m Model) PrevScreenMode() Model {
	m.ScreenMode = cycleScreenMode(m.ScreenMode, -1)
	return m
}

func cycleScreenMode(cur ScreenMode, delta int) ScreenMode {
	n := len(screenModeOrder)
	for i, sm := range screenModeOrder {
		if sm == cur {
			return screenModeOrder[(i+delta+n)%n]
		}
	}
	return ScreenNormal
}

// ── tea.Model implementation ─────────────────────────────────────────────────

// Init satisfies tea.Model. If StartupCmd is set, Bubble Tea runs it on the
// first tick so startup probes remain asynchronous.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.StartupCmd, autoRefreshTick(m.Config.GitHub.AutoRefreshInterval), spinnerTick())
}

// autoRefreshTick schedules the next background refresh tick, or nil when
// auto-refresh is disabled (interval <= 0).
func autoRefreshTick(seconds int) tea.Cmd {
	if seconds <= 0 {
		return nil
	}
	return tea.Tick(time.Duration(seconds)*time.Second, func(time.Time) tea.Msg {
		return autoRefreshTickMsg{}
	})
}

// rateLimited reports whether the known GraphQL budget has dropped below the
// pause threshold; auto-refresh holds off (but keeps ticking) until it recovers.
func (m Model) rateLimited() bool {
	return m.PRDetail != nil && m.PRDetail.RateLimitRemaining > 0 && m.PRDetail.RateLimitRemaining < rateLimitWarnThreshold
}

// shouldAutoRefresh decides whether a background tick performs a silent refetch
// of the active PR tab. It holds off when disabled, while a fetch is already in
// flight, while the user is typing a filter, on the user-driven Search tab, or
// while pausing to respect the rate-limit budget.
func (m Model) shouldAutoRefresh() bool {
	if m.Config.GitHub.AutoRefreshInterval <= 0 {
		return false
	}
	return !m.LoadingPRs && !m.FilterActive && m.PRFilter != forge.FilterSearch && !m.rateLimited()
}

// Update is the central dispatcher for all Tea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case spinnerTickMsg:
		// Only animate (and only keep ticking) while the loading view is up.
		if !m.Loading {
			return m, nil
		}
		m.SpinnerFrame++
		return m, spinnerTick()

	case tea.QuitMsg:
		// Program is already quitting; nothing to do.
		return m, nil

	case probesLoadedMsg:
		m.Loading = false
		m.Activity = ""
		if msg.Fatal != FatalNone {
			m.Fatal = msg.Fatal
			m.FatalMessage = msg.Detail
			m.FocusStack = []FocusContext{FocusFatal}
			return m, nil
		}
		m.Repo = msg.Repo
		m.PRs = msg.PRs
		storePRCache(&m, forge.FilterReviewRequested, msg.PRs)
		m.ViewerLogin = msg.ViewerLogin
		if m.OpenedPRNumber > 0 && m.Forge != nil {
			m.Activity = "Loading PR detail…"
			m.LoadingDetail = true
			return m, fetchPRDetailCmd(m.Forge, m.Repo, m.OpenedPRNumber)
		}
		return m, nil

	case commandLogEntryMsg:
		m.CommandLog = append(m.CommandLog, msg.Entry)
		if len(m.CommandLog) > CommandLogCapacity {
			m.CommandLog = append([]ghcli.CommandLogEntry(nil), m.CommandLog[len(m.CommandLog)-CommandLogCapacity:]...)
		}
		return m, nil
	case copyResultMsg:
		if msg.Err != nil {
			m.Toast = NewToast(copyFailureToast(msg.Err))
		} else if msg.Toast != "" {
			m.Toast = NewToast(msg.Toast)
		}
		return m, nil

	case mutationFailedMsg:
		m.Activity = ""
		if msg.Action == "loadPRDetail" {
			m.LoadingDetail = false
		}
		m.Toast = NewToast(msg.Err.Error())
		return m, nil

	case prDetailLoadedMsg:
		// Stale-guard: ignore detail that isn't the PR the user last opened
		// (they may have opened another while this was in flight).
		if m.OpenedPRNumber != 0 && msg.Detail.Number != m.OpenedPRNumber {
			return m, nil
		}
		m.LoadingDetail = false
		m.Activity = ""
		applyPRDetailData(&m, msg.Detail, msg.Commits, msg.Diff)
		m.ThreadTab = ThreadsTabUnresolved
		m.FilesPanel.Filter = ""
		m.FilesPanel.Cursor = 0
		m.ThreadsPanel.Filter = ""
		m.ThreadsPanel.Cursor = 0
		m.ChecksPanel.Filter = ""
		m.ChecksPanel.Cursor = 0
		m = setMainLines(m, composePROverview(msg.Detail))
		m.MainMode = MainOverview
		m.MainFileIndex = 0
		m.MainCursor = 0
		m.MainScroll = 0
		m = m.PushFocus(FocusMain)
		return m, nil

	case detailReconciledMsg:
		if msg.Err != nil {
			if msg.Attempt < 3 {
				return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
					return reconcileRetryMsg{Attempt: msg.Attempt + 1}
				})
			}
			// Give up after retries: confirmed drafts stay visible (they exist
			// on the server) and reconcile on the next successful detail fetch.
			return m, nil
		}
		if m.PRDetail == nil || (m.OpenedPRNumber != 0 && msg.Detail.Number != m.OpenedPRNumber) {
			return m, nil
		}
		for id, d := range m.OptimisticDrafts {
			if d.Confirmed {
				delete(m.OptimisticDrafts, id)
			}
		}
		applyPRDetailData(&m, msg.Detail, msg.Commits, msg.Diff)
		m = rebuildWithOptimistic(m)
		if n := len(visibleFileIndices(m)); n > 0 && m.FilesPanel.Cursor > n-1 {
			m.FilesPanel.Cursor = 0
		}
		return m, nil

	case reconcileRetryMsg:
		return m, reconcileDetailCmd(m.Forge, m.Repo, m.OpenedPRNumber, msg.Attempt)

	case commentResultMsg:
		m.Activity = ""
		m.EnsureInFlight = false
		// Cache the review id even on failure so a retry reuses the same review
		// instead of creating a second one via EnsurePendingReview.
		if msg.ReviewID != "" {
			m.PendingReviewID = msg.ReviewID
		}
		if msg.Err != nil {
			delete(m.OptimisticDrafts, msg.ClientID)
			m = rebuildWithOptimistic(m)
			m.Toast = NewToast(ToastCommentFailed)
			cs := msg.Compose
			cs.Err = "Add failed — retry (ctrl+s) or esc"
			m = openComposer(m, cs)
			m.Composer.SetValue(msg.Body)
			return m, nil
		}
		if d, ok := m.OptimisticDrafts[msg.ClientID]; ok {
			d.Confirmed = true
			m.OptimisticDrafts[msg.ClientID] = d
		}
		return m, reconcileDetailCmd(m.Forge, m.Repo, m.OpenedPRNumber, 0)

	case reviewDoneMsg:
		m.Activity = ""
		if msg.Err != nil {
			if msg.ReviewID != "" {
				m.PendingReviewID = msg.ReviewID
			}
			m.Toast = NewToast(ToastReviewFailed)
			return m, nil
		}
		// Submitted/discarded: the pending review is closed. Clear local draft
		// state now (so the banner/menu don't show stale drafts during the
		// round-trip), return to the PR list, and refetch.
		m.PendingReviewID = ""
		m.EnsureInFlight = false
		m.OptimisticDrafts = nil
		if m.PRDetail != nil {
			m.PRDetail.PendingReview = nil
			m.PRDetail.PendingReviewCount = 0
		}
		if msg.Discarded {
			m.Toast = NewToast(ToastReviewDiscarded)
		} else {
			m.Toast = NewToast(ToastReviewSubmitted)
		}
		m.FocusStack = []FocusContext{FocusPRs}
		m.LoadingPRs = true
		return m, tea.Batch(
			reconcileDetailCmd(m.Forge, m.Repo, m.OpenedPRNumber, 0),
			prListRefreshCmd(m),
		)

	case editResultMsg:
		m.Activity = ""
		if msg.Err != nil {
			m.Toast = NewToast(ToastEditFailed)
			cs := msg.Compose
			cs.Err = "Edit failed — retry (ctrl+s) or esc"
			m = openComposer(m, cs)
			m.Composer.SetValue(msg.Body)
			return m, nil
		}
		return m, reconcileDetailCmd(m.Forge, m.Repo, m.OpenedPRNumber, 0)

	case deleteResultMsg:
		m.Activity = ""
		m.DeleteTarget = forge.CommentRef{}
		if msg.Err != nil {
			m.Toast = NewToast(ToastDeleteFailed)
			return m, nil
		}
		return m, reconcileDetailCmd(m.Forge, m.Repo, m.OpenedPRNumber, 0)

	case replyResultMsg:
		m.Activity = ""
		m.EnsureInFlight = false
		if msg.ReviewID != "" {
			m.PendingReviewID = msg.ReviewID
		}
		if msg.Err != nil {
			m.Toast = NewToast(ToastReplyFailed)
			cs := msg.Compose
			cs.Err = "Reply failed — retry (ctrl+s) or esc"
			m = openComposer(m, cs)
			m.Composer.SetValue(msg.Body)
			return m, nil
		}
		return m, reconcileDetailCmd(m.Forge, m.Repo, m.OpenedPRNumber, 0)

	case viewedToggleResultMsg:
		m.Activity = ""
		m = applyViewedToggleResult(m, msg)
		return m, nil

	case prsLoadedMsg:
		if prsMsgActive(m, msg) {
			m.LoadingPRs = false
		}
		if msg.Err != nil {
			// Surface only the active tab's failure; keep the current list.
			if prsMsgActive(m, msg) {
				m.Activity = ""
				m.Toast = NewToast(msg.Err.Error())
			}
			return m, nil
		}
		// Cache by the result's own identity (even if the user has since
		// switched away) so returning to that tab/query is instant.
		if msg.Filter == forge.FilterSearch {
			storeSearchCache(&m, msg.Query, msg.PRs)
		} else {
			storePRCache(&m, msg.Filter, msg.PRs)
		}
		if !prsMsgActive(m, msg) {
			return m, nil
		}
		m.Activity = ""
		prev := selectedPRNumber(m)
		m.PRs = msg.PRs
		m.PRPanel.Cursor = indexOfPR(m, prev)
		return m, nil

	case commitDiffLoadedMsg:
		m.Activity = ""
		if msg.Err != nil {
			m.Toast = NewToast(msg.Err.Error())
			return m, nil
		}
		m = setMainLines(m, renderCommitDiff(msg.SHA, msg.Headline, msg.Files))
		m.MainMode = MainDiff
		m.MainFileIndex = -1 // commit diff is not a PR-file view; disable line comments
		m.MainCursor = 0
		m.MainScroll = 0
		if m.CurrentFocus() != FocusMain {
			m = m.PushFocus(FocusMain)
		}
		return m, nil

	case resolveResultMsg:
		m.Activity = ""
		if msg.Err != nil {
			// Roll back the optimistic flip and surface the failure.
			m = setThreadResolved(m, msg.ThreadID, !msg.Resolved)
			m.Toast = NewToast(ToastResolveFailed)
		}
		return m, nil

	case autoRefreshTickMsg:
		next := autoRefreshTick(m.Config.GitHub.AutoRefreshInterval)
		if m.shouldAutoRefresh() {
			// Silent background refresh of the active PR tab (no loading dim).
			return m, tea.Batch(fetchPRsCmd(m.Forge, m.Repo, m.PRFilter), next)
		}
		return m, next

	case refreshMsg:
		// Refetch the active PR tab. Search re-runs its committed query.
		if m.PRFilter == forge.FilterSearch {
			if m.PRPanel.Filter == "" {
				m.Activity = ""
				return m, nil
			}
			m.Activity = "Searching…"
			m.LoadingPRs = true
			return m, searchPRsCmd(m.Forge, m.Repo, m.PRPanel.Filter)
		}
		m.Activity = "Loading PRs…"
		m.LoadingPRs = true
		return m, fetchPRsCmd(m.Forge, m.Repo, m.PRFilter)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// dispatchKey returns the keystroke for command dispatch, folding a shift-modified
// single letter to its uppercase form. Enhanced-keyboard terminals (kitty
// protocol, which bubbletea v2 negotiates by default) report Shift+S as the base
// rune plus ModShift, so Keystroke() yields "shift+s"; legacy terminals send the
// bare uppercase rune, yielding "S". The dispatch switches and the keymap are all
// written with the uppercase letter, so fold "shift+<a-z>" back to it here — that
// way both terminal modes dispatch identically. Also feeds handleFilterInput, so
// capital letters can be typed into a filter regardless of terminal mode.
func dispatchKey(msg tea.KeyPressMsg) string {
	ks := msg.Keystroke()
	if rest, ok := strings.CutPrefix(ks, "shift+"); ok && len(rest) == 1 && rest[0] >= 'a' && rest[0] <= 'z' {
		return strings.ToUpper(rest)
	}
	return ks
}

// handleKey routes key events based on the active focus context.
//
// Fatal and loading states are locked: only q / ctrl+c reach the program exit.
// All other contexts receive universal bindings; per-panel overrides are wired
// in step 7.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	ks := dispatchKey(msg)

	if m.CurrentFocus() == FocusFatal || m.Loading {
		if ks == "q" || ks == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}

	// While the filter input is capturing text, universal bindings must not
	// fire — typing "q" would quit and digits would switch panels. Only the
	// safety quit stays global; every other key feeds the filter.
	if m.FilterActive {
		if ks == "ctrl+c" {
			return m, tea.Quit
		}
		return handleFilterInput(m, ks)
	}

	if m.CurrentFocus() == FocusCompose {
		if ks == "ctrl+c" {
			return m, tea.Quit
		}
		return updateCompose(m, msg)
	}

	switch ks {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.CommandLogFocused {
			m.CommandLogFocused = false
			return m, nil
		}
		if m.CurrentFocus() == FocusHelp {
			m.HelpVisible = false
		}
		if m.CurrentFocus() == FocusMain && m.MainMode == MainDiff && m.PRDetail != nil {
			m = showPROverview(m)
			return m, nil
		}
		m = m.PopFocus()
		return m, nil
	case "+":
		m = m.NextScreenMode()
		return m, nil
	case "_":
		m = m.PrevScreenMode()
		return m, nil
	case "1":
		m = m.PushFocus(FocusStatus)
		return m, nil
	case "2":
		m = m.PushFocus(FocusPRs)
		m = followPRSelection(m)
		return m, nil
	case "3":
		m = m.PushFocus(FocusFiles)
		m = followFilesSelection(m)
		return m, nil
	case "4":
		m = m.PushFocus(FocusThreads)
		return m, nil
	case "5":
		m = m.PushFocus(FocusChecks)
		return m, nil
	case "0":
		// Pure focus switch: whatever Main is showing stays put (a file diff is
		// not blown away). Only esc from a diff pops back to the overview. Falls
		// back to the overview only when Main has nothing to show yet.
		if len(m.MainLines) == 0 {
			m = showPROverview(m)
		}
		if m.CurrentFocus() != FocusMain {
			m = m.PushFocus(FocusMain)
		}
		return m, nil
	case "?":
		if m.HelpVisible {
			m.HelpVisible = false
			if m.CurrentFocus() == FocusHelp {
				m = m.PopFocus()
			}
		} else {
			m.HelpVisible = true
			m.HelpEntries = AllHelpEntries()
			m.HelpCursor = 0
			m = m.PushFocus(FocusHelp)
		}
		return m, nil
	case "@":
		m = openCommandLogMenu(m)
		return m, nil
	case "S":
		if m.PRDetail == nil {
			return m, nil
		}
		m = openSubmitMenu(m)
		return m, nil
	case "y":
		if m.sectionLoading(m.CurrentFocus()) {
			return m, nil
		}
		m = openCopyMenu(m)
		return m, nil
	case "o":
		if m.sectionLoading(m.CurrentFocus()) {
			return m, nil
		}
		return m, browserActionForFocus(m)
	case "R":
		m.Activity = "Refreshing…"
		return m, func() tea.Msg { return refreshMsg{} }
	}

	if m.CommandLogFocused {
		return UpdateCommandLog(m, msg)
	}

	// A section fetching its data is interaction-blocked so the user can't act
	// on stale rows a landing fetch is about to replace. Focus (1-5) and tab
	// navigation ([ / ]) stay live — switching just supersedes the load — but
	// every data-consuming key (enter, space, j/k, t, …) is swallowed.
	if m.sectionLoading(m.CurrentFocus()) && ks != "[" && ks != "]" {
		return m, nil
	}

	// Per-context dispatch for panel behaviors.
	switch m.CurrentFocus() {
	case FocusPRs:
		return UpdatePRs(m, msg)
	case FocusStatus:
		return UpdateStatus(m, msg)
	case FocusChecks:
		return UpdateChecks(m, msg)
	case FocusFiles:
		return UpdateFiles(m, msg)
	case FocusThreads:
		return UpdateThreads(m, msg)
	case FocusMain:
		return UpdateMain(m, msg)
	case FocusThread:
		return UpdateThreadFocus(m, msg)
	case FocusHelp:
		return UpdateHelp(m, msg)
	case FocusMenu:
		return UpdateMenu(m, msg)
	}

	return m, nil
}

// View selects the correct full-terminal surface for the current model state.
func (m Model) View() tea.View {
	var v tea.View
	switch {
	case m.Fatal != FatalNone:
		v = tea.NewView(fatalView(m))
	case m.Loading:
		v = tea.NewView(loadingView(m))
	default:
		v = tea.NewView(fullView(m))
	}
	// lazypr is a full-window TUI; the alternate screen owns the whole grid
	// so full-height frames never scroll-fight the primary buffer.
	v.AltScreen = true
	return v
}

// spinnerFrames is deliberately ASCII-only so it renders identically on every
// terminal and font.
var spinnerFrames = []string{"|", "/", "-", "\\"}

var spinnerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))

// spinnerTick schedules the next spinner frame.
func spinnerTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

// loadingView renders the startup spinner centered both ways in the terminal.
// With unknown dimensions (headless/tests) it degrades to a single plain line.
func loadingView(m Model) string {
	label := m.Activity
	if label == "" {
		label = "Loading…"
	}
	line := spinnerStyle.Render(spinnerFrames[m.SpinnerFrame%len(spinnerFrames)]) + "  " + label
	if m.Width <= 0 || m.Height <= 0 {
		return "\n  " + line + "\n"
	}
	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, line)
}
