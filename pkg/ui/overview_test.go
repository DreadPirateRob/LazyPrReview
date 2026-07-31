package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

// overviewModel builds a PR whose conversation mirrors the real shape that
// motivated this work: a run of bot verdicts plus a couple of human comments.
func overviewModel(t *testing.T) Model {
	t.Helper()
	base := time.Date(2026, 4, 7, 11, 37, 0, 0, time.UTC)
	at := func(d int) time.Time { return base.AddDate(0, 0, d) }

	d := domain.PRDetail{
		Number: 6574, Title: "notification on cade insufficient bal", State: "OPEN",
		Author: "fazil56", HeadRefName: "CJ-10354", BaseRefName: "master",
		Additions: 2873, Deletions: 89, ChangedFiles: 34,
		ReviewDecision: "APPROVED",
		Body:           "JIRA https://example.test/browse/CJ-10354",
		Timeline: []domain.TimelineItem{
			{Kind: "IssueComment", Author: "github-actions", SortAt: at(0), Body: "**Verdict:** ▲ Needs Changes"},
			{Kind: "IssueComment", Author: "github-actions", SortAt: at(1), Body: "**Verdict:** ✅ Approved"},
			{Kind: "IssueComment", Author: "github-actions", SortAt: at(2), Body: "**Verdict:** ✅ Approved"},
			{Kind: "IssueComment", Author: "DreadPirateRob", SortAt: at(3), Body: "@codex take a look"},
			{Kind: "IssueComment", Author: "fazil56", SortAt: at(4), Body: "done"},
		},
	}
	m := New(config.Default(), fakeforge.New())
	m.Loading = false
	m.Width, m.Height = 120, 40
	m.PRDetail = &d
	m.FocusStack = []FocusContext{FocusMain}
	return setMainOverview(m, d)
}

func plain(m Model) string { return ansi.Strip(strings.Join(m.MainLines, "\n")) }

func rowOfKind(m Model, kind mainRowKind) int {
	for i, r := range m.MainRows {
		if r.Kind == kind {
			return i
		}
	}
	return -1
}

// Bot runs collapse into ONE row by default and report the latest verdict; the
// human comments stay readable. This is the density win the redesign is for.
func TestOverviewGroupsBotRunAndExpandsHumans(t *testing.T) {
	m := overviewModel(t)
	out := plain(m)

	if !strings.Contains(out, "github-actions") {
		t.Fatal("expected the bot run to be listed")
	}
	if !strings.Contains(out, "3 comments") {
		t.Fatalf("consecutive bot comments should collapse into one group row:\n%s", out)
	}
	if !strings.Contains(out, "latest") || !strings.Contains(out, "Approved") {
		t.Fatalf("a collapsed run should report its latest verdict:\n%s", out)
	}
	// Collapsed by default, so no bot body text.
	if strings.Contains(out, "Needs Changes") {
		t.Fatalf("bot bodies should be collapsed by default:\n%s", out)
	}
	// Humans expanded by default.
	if !strings.Contains(out, "take a look") {
		t.Fatalf("human comments should be expanded by default:\n%s", out)
	}
	if rowOfKind(m, rowGroupHeader) < 0 {
		t.Fatal("expected a group header row in the model")
	}
}

// Markdown is rendered, not dumped: the reported bug was literal ** asterisks.
func TestOverviewRendersMarkdownNotRaw(t *testing.T) {
	m := overviewModel(t)
	// Expanding a bot GROUP reveals its members, each still collapsed (bots default
	// shut), so reach the body by expanding the group and then one member.
	m.MainCursor = rowOfKind(m, rowGroupHeader)
	m = toggleOverviewFold(m, foldTargetKey(m))

	member := -1
	for i, r := range m.MainRows {
		if r.Kind == rowEventHeader && strings.Contains(ansi.Strip(m.MainLines[i]), "github-actions") {
			member = i
			break
		}
	}
	if member < 0 {
		t.Fatal("expanding the group should reveal its member rows")
	}
	m.MainCursor = member
	m = toggleOverviewFold(m, foldTargetKey(m))
	out := plain(m)

	if !strings.Contains(out, "Verdict:") {
		t.Fatalf("expected the verdict text:\n%s", out)
	}
	if strings.Contains(out, "**Verdict:**") {
		t.Fatalf("markdown emphasis must be rendered, not literal:\n%s", out)
	}
}

// z folds the row under the cursor in the overview, same key as the diff.
func TestOverviewZFoldsUnderCursor(t *testing.T) {
	m := overviewModel(t)
	// Park on a human comment (expanded by default) and collapse it.
	target := -1
	for i, r := range m.MainRows {
		if r.Kind == rowEventHeader && strings.Contains(ansi.Strip(m.MainLines[i]), "DreadPirateRob") {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("expected a human comment header row")
	}
	m.MainCursor = target

	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "z", Code: 'z'})
	if got.MainPendingZ {
		t.Fatal("z on a foldable overview row should fold, not arm zz")
	}
	if strings.Contains(plain(got), "take a look") {
		t.Fatal("folding a human comment should hide its body")
	}
	// And the cursor stays on that same row rather than drifting.
	if key := foldTargetKey(got); key == "" {
		t.Fatal("cursor should remain on the folded row")
	}
}

// z on plain prose (title/description/section) must still arm zz centering.
func TestOverviewZOnProseArmsCenter(t *testing.T) {
	m := overviewModel(t)
	m.MainCursor = rowOfKind(m, rowMeta) // the title row
	got, _ := UpdateMain(m, tea.KeyPressMsg{Text: "z", Code: 'z'})
	if !got.MainPendingZ {
		t.Fatal("z on a non-foldable overview row should arm the zz prefix")
	}
}

// Fold state must survive a rebuild. Timeline items have no server ID, so this is
// the guard on the synthetic key: rebuilding after a draft lands must not reset or
// transfer folds.
func TestOverviewFoldSurvivesRebuild(t *testing.T) {
	m := overviewModel(t)
	m.MainCursor = rowOfKind(m, rowGroupHeader)
	key := foldTargetKey(m)
	m = toggleOverviewFold(m, key) // expand the bot run
	if !strings.Contains(plain(m), "Needs Changes") {
		t.Fatal("precondition: the run should be expanded")
	}

	// Rebuild the way an optimistic draft or refetch does.
	rebuilt := rebuildWithOptimistic(m)
	if !strings.Contains(plain(rebuilt), "Needs Changes") {
		t.Fatal("fold state must survive a rebuild — the synthetic key is unstable")
	}
	if rebuilt.Folded[key] {
		t.Fatalf("expanded run should stay expanded after rebuild, Folded[%q]=true", key)
	}
}

// - collapses everything and = expands everything, from EITHER starting state.
//
// Guards a fixed bug: fold-all used to seed keys from the VISIBLE rows, where a
// collapsed bot run exposes only its group key — so "expand all" opened the run and
// left every member shut, and the reader saw no bodies. Keys now come from the data
// (allOverviewFoldKeys), which is why the collapsed-run case below must pass.
func TestOverviewCollapseAndExpandAll(t *testing.T) {
	m := overviewModel(t)

	// From the default state (bot collapsed, humans expanded): collapse all.
	collapsed, _ := UpdateMain(m, tea.KeyPressMsg{Text: "-", Code: '-'})
	if strings.Contains(plain(collapsed), "take a look") {
		t.Fatal("collapse all should hide human comment bodies too")
	}

	// Expand all from the fully collapsed state.
	expanded, _ := UpdateMain(collapsed, tea.KeyPressMsg{Text: "=", Code: '='})
	out := plain(expanded)
	if !strings.Contains(out, "take a look") {
		t.Fatalf("expand all should reveal human bodies:\n%s", out)
	}
	if !strings.Contains(out, "Needs Changes") {
		t.Fatalf("expand all must reopen a run that was already collapsed:\n%s", out)
	}

	// And collapse all again from the fully expanded state.
	recollapsed, _ := UpdateMain(expanded, tea.KeyPressMsg{Text: "-", Code: '-'})
	if strings.Contains(plain(recollapsed), "Needs Changes") {
		t.Fatal("collapse all should re-hide an expanded run")
	}
}

// Every overview line must fit the content budget — the original bug was body text
// clipped at the right edge.
func TestOverviewLinesFitBudget(t *testing.T) {
	m := overviewModel(t)
	// A long unbroken body is the hard case.
	m.PRDetail.Timeline = append(m.PRDetail.Timeline, domain.TimelineItem{
		Kind: "IssueComment", Author: "someone", SortAt: time.Now(),
		Body: strings.Repeat("verylongtokenwithoutspaces", 12) + " " + strings.Repeat("word ", 80),
	})
	m = setMainOverview(m, *m.PRDetail)

	budget := mainContentWidth(m)
	for i, l := range m.MainLines {
		if w := ansi.StringWidth(l); w > budget {
			t.Fatalf("overview line %d is %d cols (budget %d): %q", i, w, budget, ansi.Strip(l))
		}
	}
}
