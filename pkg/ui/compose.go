package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	"github.com/charmbracelet/x/ansi"
)

type composeKind string

const (
	composeComment composeKind = "comment"
	composeReview  composeKind = "review"
	composeEdit    composeKind = "edit"
	composeReply   composeKind = "reply"
)

// composeState carries what an active textarea popup will produce on submit.
type composeState struct {
	Kind     composeKind
	Title    string
	Required bool // body required (client-side validation)
	Err      string
	// comment target
	Path        string
	Line        int
	Side        string
	StartLine   *int
	StartSide   string
	SubjectType string            // "LINE" or "FILE"
	ClientID    string            // optimistic draft id this submit owns
	Event       forge.ReviewEvent // composeReview: the submit event
	CommentID   string            // composeEdit: the comment being edited
	ThreadID    string            // composeReply: the thread being replied to
	Preview     []string          // diff rows the comment targets, shown in the modal
}

// optimisticDraft is a locally-synthesized PENDING draft shown immediately
// after submit. Confirmed marks that the server accepted it, so the next
// reconcile drops it in favor of server truth; unconfirmed drafts (still
// in flight) survive every reconcile until their own success/error resolves.
type optimisticDraft struct {
	Thread    domain.Thread
	Confirmed bool
}

// commentResultMsg reports the outcome of a draft-comment submit. On failure it
// carries the body + compose state so the popup can reopen with text intact.
type commentResultMsg struct {
	OK       bool
	Err      error
	ClientID string
	Body     string
	Compose  composeState
	ReviewID string
}

// openComposer arms a textarea popup. It is sized as a centered modal: wide
// enough to write in but capped so the UI behind it stays visible.
func openComposer(m Model, cs composeState) Model {
	layout := ComputeLayout(m)
	w := layout.MainWidth - 4
	if w > 72 {
		w = 72
	}
	if w < 20 {
		w = 20
	}
	h := layout.MainHeight - 6
	switch {
	case h < 3:
		h = 3
	case h > 10:
		h = 10
	}
	ta := textarea.New()
	ta.SetVirtualCursor(true)
	ta.SetWidth(w)
	ta.SetHeight(h)
	ta.Focus()
	m.Composer = ta
	m.Compose = cs
	return m.PushFocus(FocusCompose)
}

func closeComposer(m Model) Model {
	m.Composer.Blur()
	m.Compose = composeState{}
	if m.CurrentFocus() == FocusCompose {
		m = m.PopFocus()
	}
	return m
}

// updateCompose routes keys while the popup is open: esc cancels, ctrl+s
// submits, everything else edits the textarea.
func updateCompose(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.Keystroke() {
	case "esc":
		return closeComposer(m), nil
	case "ctrl+s":
		return submitCompose(m)
	}
	var cmd tea.Cmd
	m.Composer, cmd = m.Composer.Update(msg)
	return m, cmd
}

func submitCompose(m Model) (Model, tea.Cmd) {
	body := strings.TrimSpace(m.Composer.Value())
	if m.Compose.Required && body == "" {
		m.Compose.Err = "Body required — type something or press esc"
		return m, nil
	}
	switch m.Compose.Kind {
	case composeComment:
		return submitComment(m, body)
	case composeReview:
		return submitReview(m, body)
	case composeEdit:
		return submitEdit(m, body)
	case composeReply:
		return submitThreadReply(m, body)
	default:
		return closeComposer(m), nil
	}
}

func submitComment(m Model, body string) (Model, tea.Cmd) {
	if m.PRDetail == nil {
		return closeComposer(m), nil
	}
	// Resolve the review to attach to: a locally-cached id (from an earlier
	// ensure this session), else the server's adopted pending review.
	reviewID := m.PendingReviewID
	if reviewID == "" && m.PRDetail.PendingReview != nil {
		reviewID = m.PRDetail.PendingReview.ID
	}
	// If none exists yet and one is already being created by an in-flight
	// comment, don't race a second addPullRequestReview — keep the popup open.
	if reviewID == "" && m.EnsureInFlight {
		m.Compose.Err = "Finishing previous comment — press ctrl+s again in a moment"
		return m, nil
	}
	cs := m.Compose
	m.ComposeSeq++
	cs.ClientID = fmt.Sprintf("optimistic-%d", m.ComposeSeq)
	in := forge.AddThreadInput{
		Repo:        m.Repo,
		PRID:        m.PRDetail.ID,
		Path:        cs.Path,
		Line:        cs.Line,
		Side:        cs.Side,
		StartLine:   cs.StartLine,
		StartSide:   cs.StartSide,
		Body:        body,
		SubjectType: cs.SubjectType,
	}
	prID, headOID := m.PRDetail.ID, m.PRDetail.HeadRefOID
	if reviewID == "" {
		m.EnsureInFlight = true
	}
	m = insertOptimisticDraft(m, cs, body)
	m = closeComposer(m)
	m.Activity = "Adding comment…"
	return m, commentSubmitCmd(m.Forge, prID, headOID, reviewID, in, cs, body)
}

// submitThreadReply adds a batched draft reply to a thread, ensuring a pending
// review exists first (replies attach to it, verified live).
func submitThreadReply(m Model, body string) (Model, tea.Cmd) {
	if m.PRDetail == nil {
		return closeComposer(m), nil
	}
	reviewID := m.PendingReviewID
	if reviewID == "" && m.PRDetail.PendingReview != nil {
		reviewID = m.PRDetail.PendingReview.ID
	}
	if reviewID == "" && m.EnsureInFlight {
		m.Compose.Err = "Finishing previous action — press ctrl+s again in a moment"
		return m, nil
	}
	cs := m.Compose
	prID, headOID := m.PRDetail.ID, m.PRDetail.HeadRefOID
	if reviewID == "" {
		m.EnsureInFlight = true
	}
	m = closeComposer(m)
	m.Activity = "Replying…"
	return m, threadReplyCmd(m.Forge, prID, headOID, reviewID, cs.ThreadID, body, cs)
}

func threadReplyCmd(f forge.Forge, prID, headOID, reviewID, threadID, body string, cs composeState) tea.Cmd {
	return func() tea.Msg {
		rid := reviewID
		if rid == "" {
			id, err := f.EnsurePendingReview(context.Background(), prID, headOID)
			if err != nil {
				return replyResultMsg{Err: err, Body: body, Compose: cs}
			}
			rid = id
		}
		if err := f.AddThreadReply(context.Background(), rid, threadID, body); err != nil {
			return replyResultMsg{Err: err, Body: body, Compose: cs, ReviewID: rid}
		}
		return replyResultMsg{ReviewID: rid}
	}
}

// replyResultMsg reports a batched reply outcome; success reconciles, failure
// reopens the composer with the body intact.
type replyResultMsg struct {
	Err      error
	Body     string
	Compose  composeState
	ReviewID string
}

// submitEdit updates an existing comment's body.
func submitEdit(m Model, body string) (Model, tea.Cmd) {
	cs := m.Compose
	m = closeComposer(m)
	m.Activity = "Updating comment…"
	return m, editCmd(m.Forge, cs.CommentID, body, cs)
}

func editCmd(f forge.Forge, commentID, body string, cs composeState) tea.Cmd {
	return func() tea.Msg {
		if err := f.UpdateComment(context.Background(), commentID, body); err != nil {
			return editResultMsg{Err: err, Body: body, Compose: cs}
		}
		return editResultMsg{}
	}
}

// editResultMsg reports an edit outcome; on failure the popup reopens with the
// body intact, on success a reconcile brings the updated comment.
type editResultMsg struct {
	Err     error
	Body    string
	Compose composeState
}

// ownComment returns the first viewer-authored comment in a thread.
func ownComment(th domain.Thread) (domain.Comment, bool) {
	for _, c := range th.Comments {
		if c.ViewerDidAuthor {
			return c, true
		}
	}
	return domain.Comment{}, false
}

// deleteCommentCmd deletes a review comment; the forge routes GraphQL (pending)
// vs REST (published) on ref.Pending.
func deleteCommentCmd(f forge.Forge, repo forge.Repo, ref forge.CommentRef) tea.Cmd {
	return func() tea.Msg {
		return deleteResultMsg{Err: f.DeleteComment(context.Background(), repo, ref)}
	}
}

// deleteResultMsg reports a delete outcome; success triggers a reconcile.
type deleteResultMsg struct {
	Err error
}

// commentSubmitCmd ensures a pending review exists (once), then adds the draft
// thread to it. Both round-trips run off the UI thread.
func commentSubmitCmd(f forge.Forge, prID, headOID, reviewID string, in forge.AddThreadInput, cs composeState, body string) tea.Cmd {
	return func() tea.Msg {
		rid := reviewID
		if rid == "" {
			id, err := f.EnsurePendingReview(context.Background(), prID, headOID)
			if err != nil {
				return commentResultMsg{Err: err, ClientID: cs.ClientID, Body: body, Compose: cs}
			}
			rid = id
		}
		in.ReviewID = rid
		if err := f.AddReviewThread(context.Background(), in); err != nil {
			return commentResultMsg{Err: err, ClientID: cs.ClientID, Body: body, Compose: cs, ReviewID: rid}
		}
		return commentResultMsg{OK: true, ClientID: cs.ClientID, ReviewID: rid}
	}
}

// submitReview submits the pending review (creating one first if needed, e.g.
// a direct approve with no drafts) with the chosen event + body.
func submitReview(m Model, body string) (Model, tea.Cmd) {
	if m.PRDetail == nil {
		return closeComposer(m), nil
	}
	reviewID := m.PendingReviewID
	if reviewID == "" && m.PRDetail.PendingReview != nil {
		reviewID = m.PRDetail.PendingReview.ID
	}
	event := m.Compose.Event
	prID, headOID := m.PRDetail.ID, m.PRDetail.HeadRefOID
	m = closeComposer(m)
	m.Activity = "Submitting review…"
	return m, submitReviewCmd(m.Forge, prID, headOID, reviewID, event, body)
}

func submitReviewCmd(f forge.Forge, prID, headOID, reviewID string, event forge.ReviewEvent, body string) tea.Cmd {
	return func() tea.Msg {
		rid := reviewID
		if rid == "" {
			id, err := f.EnsurePendingReview(context.Background(), prID, headOID)
			if err != nil {
				return reviewDoneMsg{Err: err}
			}
			rid = id
		}
		if err := f.SubmitReview(context.Background(), rid, event, body); err != nil {
			return reviewDoneMsg{Err: err, ReviewID: rid}
		}
		return reviewDoneMsg{ReviewID: rid}
	}
}

// discardReviewCmd deletes the pending review.
func discardReviewCmd(f forge.Forge, reviewID string) tea.Cmd {
	return func() tea.Msg {
		if reviewID != "" {
			if err := f.DiscardReview(context.Background(), reviewID); err != nil {
				return reviewDoneMsg{Discarded: true, Err: err, ReviewID: reviewID}
			}
		}
		return reviewDoneMsg{Discarded: true}
	}
}

// reviewDoneMsg reports a submit/discard outcome; the handler returns to the PR
// list and refetches so reviewDecision + pending state update. ReviewID carries
// the resolved review so a failed submit is retried without re-ensuring.
type reviewDoneMsg struct {
	Discarded bool
	ReviewID  string
	Err       error
}

func insertOptimisticDraft(m Model, cs composeState, body string) Model {
	if m.OptimisticDrafts == nil {
		m.OptimisticDrafts = map[string]optimisticDraft{}
	}
	c := domain.Comment{Body: body, State: "PENDING", Author: m.ViewerLogin, ViewerDidAuthor: true, Path: cs.Path}
	th := domain.Thread{ID: cs.ClientID, Path: cs.Path, DiffSide: cs.Side, ViewerCanReply: true, Comments: []domain.Comment{c}}
	if cs.SubjectType != "FILE" {
		line := cs.Line
		th.Line = &line
		th.Comments[0].Line = &line
	}
	m.OptimisticDrafts[cs.ClientID] = optimisticDraft{Thread: th}
	return rebuildWithOptimistic(m)
}

// effectiveThreads overlays the in-flight optimistic drafts on the server
// threads for display; server data itself is never mutated.
func effectiveThreads(m Model) []domain.Thread {
	if m.PRDetail == nil {
		return nil
	}
	if len(m.OptimisticDrafts) == 0 {
		return m.PRDetail.Threads
	}
	out := make([]domain.Thread, 0, len(m.PRDetail.Threads)+len(m.OptimisticDrafts))
	out = append(out, m.PRDetail.Threads...)
	for _, d := range m.OptimisticDrafts {
		out = append(out, d.Thread)
	}
	return out
}

func optimisticUnconfirmed(m Model) int {
	n := 0
	for _, d := range m.OptimisticDrafts {
		if !d.Confirmed {
			n++
		}
	}
	return n
}

// rebuildWithOptimistic recomputes anchors and the overview from server threads
// plus the current optimistic overlay.
func rebuildWithOptimistic(m Model) Model {
	if m.PRDetail == nil {
		return m
	}
	threads := effectiveThreads(m)
	m.AnchoredThreads, m.UnresolvedThreadIndex = anchorsFromThreads(threads, m.DiffFiles)
	if m.MainMode == MainOverview {
		d := *m.PRDetail
		d.Threads = threads
		return setMainOverview(m, d)
	}
	// A file diff renders threads inline, so it has to be rebuilt too or a new
	// draft stays invisible until the reader leaves and re-enters the file.
	return refreshMainDiff(m)
}

// anchorAt resolves the (path, line, side) for a Main-diff row via the row model.
// Returns ok=false when not in a built-in diff (an external pager has no line
// mapping, and non-file modes have no row model) or when the row is not a
// commentable code line — which now includes inline thread rows, since a comment
// on a comment has no diff line to anchor to.
func anchorAt(m Model, row int) (path string, line int, side string, ok bool) {
	if m.MainMode != MainDiff || m.Config.GUI.DiffPager != "" {
		return "", 0, "", false
	}
	if m.MainFileIndex < 0 || m.MainFileIndex >= len(m.DiffFiles) {
		return "", 0, "", false
	}
	i := codeRenderIndex(m, row)
	file := m.DiffFiles[m.MainFileIndex]
	if i < 0 || i >= len(file.Rendered) {
		return "", 0, "", false
	}
	rl := file.Rendered[i]
	switch rl.Kind {
	case diff.LineKindAdd, diff.LineKindDel, diff.LineKindContext:
	default:
		return "", 0, "", false
	}
	side = diff.CursorSide(rl.Kind)
	no := rl.NewNo
	if side == "LEFT" {
		no = rl.OldNo
	}
	if no == nil {
		return "", 0, "", false
	}
	return file.Path, *no, side, true
}

// mainCommentAnchor resolves the anchor under the Main-diff cursor.
func mainCommentAnchor(m Model) (string, int, string, bool) {
	return anchorAt(m, m.MainCursor)
}

// mainRangeActive reports whether a multi-line selection is live. It is bound to
// the render generation at which the range began, so any Main-content rewrite
// (file switch, reopen, commit diff, overview) silently invalidates it.
func mainRangeActive(m Model) bool {
	return m.MainRangeActive && m.MainMode == MainDiff && m.MainRangeGen == m.MainGen
}

// mainRangeBounds returns the inclusive, ordered [lo, hi] rendered-row span of
// the current selection (range start + cursor).
func mainRangeBounds(m Model) (lo, hi int) {
	lo, hi = m.MainRangeStart, m.MainCursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

// openRangeComment opens a multi-line draft for the active selection, enforcing
// the single-side rule (SPEC §7).
func openRangeComment(m Model) (Model, tea.Cmd) {
	lo, hi := mainRangeBounds(m)
	m.MainRangeActive = false
	loPath, loLine, side, ok := anchorAt(m, lo)
	if !ok {
		return m, nil
	}
	// Every CODE row in the span must be a commentable line on the same file+side
	// (SPEC §7 one-side rule); reject headers or opposite-side rows anywhere in
	// the interior, not just at the endpoints.
	//
	// Inline thread rows are skipped rather than rejected: a block consumes Main
	// rows but no file lines, so a selection spanning one is still a contiguous
	// range of code — rejecting it would make any line under discussion
	// un-range-commentable.
	hiLine := loLine
	for r := lo; r <= hi; r++ {
		if rw, ok := rowAt(m, r); ok && rw.Kind != rowCode {
			continue
		}
		p, ln, s, ok := anchorAt(m, r)
		if !ok || p != loPath || s != side {
			m.Toast = NewToast(ToastMixedSideSelection)
			return m, nil
		}
		hiLine = ln
	}
	cs := composeState{
		Kind: composeComment, Required: true, SubjectType: "LINE",
		Title: fmt.Sprintf("Comment %s:%d-%d (%s) — ctrl+s submit", loPath, loLine, hiLine, side),
		Path:  loPath, Line: hiLine, Side: side,
	}
	cs.Preview = previewRows(m, lo, hi)
	if loLine != hiLine {
		start := loLine
		cs.StartLine = &start
		cs.StartSide = side
	}
	return openComposer(m, cs), nil
}

// previewRows returns the rendered Main-diff rows for the inclusive span [lo,hi]
// so the composer can show exactly which lines the comment targets. Callers pass
// a span already validated as commentable; long selections are elided.
//
// Inline thread rows inside the span are dropped: openRangeComment skips them when
// resolving the anchor, so including them here would preview lines that are not
// part of what gets submitted.
func previewRows(m Model, lo, hi int) []string {
	const maxRows = 8
	if m.MainMode != MainDiff || lo < 0 || hi < lo || hi >= len(m.MainLines) {
		return nil
	}
	rows := make([]string, 0, hi-lo+1)
	for r := lo; r <= hi; r++ {
		if rw, ok := rowAt(m, r); ok && rw.Kind != rowCode {
			continue
		}
		rows = append(rows, m.MainLines[r])
	}
	if len(rows) <= maxRows {
		return rows
	}
	return append(rows[:maxRows:maxRows], fmt.Sprintf("… +%d more lines", len(rows)-maxRows))
}

// ComposeView renders the textarea popup body: the diff line(s) the comment
// targets, then the editor, then its key hints.
func ComposeView(m Model) string {
	title := m.Compose.Title
	if title == "" {
		title = "Comment"
	}
	const hint = "[ctrl+s] submit  ·  [esc] cancel"
	editor := m.Composer.View()

	var sb strings.Builder
	sb.WriteString(title + "\n")
	if m.Compose.Err != "" {
		sb.WriteString(m.Compose.Err + "\n")
	}
	if rows := m.Compose.Preview; len(rows) > 0 {
		w := previewWidth(m, title, m.Compose.Err, editor, hint)
		for _, row := range previewBlock(rows, w) {
			sb.WriteString(row + "\n")
		}
		sb.WriteString(strings.Repeat("─", w) + "\n")
	}
	sb.WriteString(editor)
	sb.WriteString("\n" + hint)
	return sb.String()
}

// codeColumn returns the column where a rendered diff row's code text begins:
// past the line-number gutter's "│ " and past the 2-column change marker that
// renderDiffLine always emits ("+ ", "- ", or "  "), so wrapped continuations tie
// to the code rather than the sign column. 0 when the row has no gutter
// (file/hunk headers).
func codeColumn(row string) int {
	const sep = "│ "
	plain := ansi.Strip(row)
	i := strings.Index(plain, sep)
	if i < 0 {
		return 0
	}
	col := ansi.StringWidth(plain[:i]) + 2 // the separator plus its trailing space
	rest := plain[i+len(sep):]
	for _, marker := range []string{"+ ", "- ", "  "} {
		if strings.HasPrefix(rest, marker) {
			return col + 2
		}
	}
	return col
}

// wrapPreviewRow soft-wraps one target row to w columns instead of cutting it, so
// no part of the selected code is lost. Only the code after the gutter is wrapped;
// continuation lines are padded to the gutter's exact width so rows stay aligned
// and the gutter never visually restarts.
func wrapPreviewRow(row string, w int) []string {
	if w < 1 {
		return []string{row}
	}
	indent := codeColumn(row)
	if indent <= 0 || indent >= w {
		return strings.Split(ansi.Wrap(row, w, ""), "\n")
	}
	gutter := ansi.Truncate(row, indent, "")
	code := ansi.TruncateLeft(row, indent, "")
	segs := strings.Split(ansi.Wrap(code, w-indent, ""), "\n")
	out := make([]string, 0, len(segs))
	pad := strings.Repeat(" ", indent)
	for i, seg := range segs {
		if i == 0 {
			out = append(out, gutter+seg)
			continue
		}
		out = append(out, pad+seg)
	}
	return out
}

// previewBlock renders the target rows for the modal, wrapping rather than cutting
// and capping the total height so a large selection can't grow the modal unbounded.
func previewBlock(rows []string, w int) []string {
	const maxRendered = 12
	out := make([]string, 0, len(rows))
	for i, row := range rows {
		for _, seg := range wrapPreviewRow(row, w) {
			if len(out) >= maxRendered {
				return append(out, fmt.Sprintf("… +%d more line(s)", len(rows)-i))
			}
			out = append(out, seg)
		}
	}
	return out
}

// previewWidth is the column budget for the composer's target-line rows and the
// separator rule. The modal is sized to its widest line, so the budget tracks the
// widest NON-preview line (title — often a long path — plus the editor and hints).
// That way the target lines fill the modal exactly instead of stopping short of
// its right edge, while never widening it themselves. Floored so the diff
// line-number gutter can't squeeze out the code, and capped to what the modal can
// actually show (modalDims caps the box at frame-4, so content at frame-6).
func previewWidth(m Model, parts ...string) int {
	w := 0
	for _, p := range parts {
		if pw := lipgloss.Width(p); pw > w {
			w = pw
		}
	}
	if w < 60 {
		w = 60
	}
	if lim := m.Width - 6; m.Width > 0 && w > lim {
		w = lim
	}
	if w < 1 {
		w = 1
	}
	return w
}
