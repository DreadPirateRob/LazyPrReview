package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

const longTitle = "Cj 10354 notification on cade insufficient bal2 with a considerably longer trailing description"

func titleModel(t *testing.T, term int) Model {
	t.Helper()
	m := New(config.Default(), fakeforge.New())
	m.Loading = false
	m.Width, m.Height = term, 40
	d := domain.PRDetail{
		Number: 6574, Title: longTitle, State: "OPEN", Author: "fazil56",
		HeadRefName: strings.Repeat("very-long-branch-name/", 4) + "end",
		BaseRefName: "master.jul23old",
		Labels: []domain.Label{
			{Name: "enhancement"}, {Name: "Frontend"}, {Name: "needs-qa"},
			{Name: "backend"}, {Name: "release-blocker"}, {Name: "discussion"},
		},
	}
	m.PRDetail = &d
	return setMainOverview(m, d)
}

// A long PR title must WRAP, never truncate: it is the most identifying text on
// screen, and the pane can simply use another row.
func TestOverviewTitleWrapsNeverTruncates(t *testing.T) {
	for _, term := range []int{80, 100, 130} {
		m := titleModel(t, term)
		budget := mainContentWidth(m)
		joined := ansi.Strip(strings.Join(m.MainLines, "\n"))

		if strings.Contains(joined, "…") {
			t.Fatalf("term=%d: header must not be ellipsised:\n%s", term, joined)
		}
		// Every word of the title survives somewhere in the output.
		for _, word := range strings.Fields(longTitle) {
			if !strings.Contains(joined, word) {
				t.Fatalf("term=%d: title word %q was lost:\n%s", term, word, joined)
			}
		}
		for i, l := range m.MainLines {
			if w := ansi.StringWidth(l); w > budget {
				t.Fatalf("term=%d line %d is %d cols (budget %d): %q",
					term, i, w, budget, ansi.Strip(l))
			}
		}
	}
}

// The wrapped remainder is indented under the title text, not under the number.
func TestOverviewTitleContinuationIsIndented(t *testing.T) {
	m := titleModel(t, 100)
	if len(m.MainLines) < 2 {
		t.Fatal("expected the long title to occupy more than one line")
	}
	first := ansi.Strip(m.MainLines[0])
	second := ansi.Strip(m.MainLines[1])
	if !strings.HasPrefix(first, "#6574  ") {
		t.Fatalf("first line should lead with the PR number, got %q", first)
	}
	hang := len("#6574  ")
	if !strings.HasPrefix(second, strings.Repeat(" ", hang)) {
		t.Fatalf("continuation should align under the title text, got %q", second)
	}
	if strings.TrimSpace(second) == "" {
		t.Fatal("continuation line should carry the rest of the title")
	}
}

// The other header rows (branch, labels) are prose too and must not clip either.
func TestOverviewHeaderRowsWrap(t *testing.T) {
	m := titleModel(t, 90)
	budget := mainContentWidth(m)
	joined := ansi.Strip(strings.Join(m.MainLines, "\n"))

	for _, want := range []string{"master.jul23old", "release-blocker", "discussion"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("header content %q was lost:\n%s", want, joined)
		}
	}
	for i, l := range m.MainLines {
		if w := ansi.StringWidth(l); w > budget {
			t.Fatalf("line %d is %d cols (budget %d): %q", i, w, budget, ansi.Strip(l))
		}
	}
}

// A hyphenated label must stay one token. Generic word wrapping split it into
// "release" / "-" / "blocker" — three fragments reading as different labels —
// because ansi.Wordwrap breaks at hyphens and overshoots the limit doing it.
func TestOverviewLabelListNeverSplitsHyphenatedItem(t *testing.T) {
	m := titleModel(t, 90)
	joined := ansi.Strip(strings.Join(m.MainLines, "\n"))

	if !strings.Contains(joined, "release-blocker") {
		t.Fatalf("hyphenated label must stay intact:\n%s", joined)
	}
	// A bare "-" line is the fragmentation signature.
	for i, l := range m.MainLines {
		if strings.TrimSpace(ansi.Strip(l)) == "-" {
			t.Fatalf("line %d is a stray hyphen from mid-word wrapping:\n%s", i, joined)
		}
	}
}

// Wrapped header continuations stay rowMeta, so overview navigation and folding
// keep treating them as plain header text.
func TestOverviewWrappedHeaderRowsAreMeta(t *testing.T) {
	m := titleModel(t, 90)
	// Everything before the first section rule is header.
	for i, r := range m.MainRows {
		if r.Kind == rowSection {
			break
		}
		if r.Kind != rowMeta {
			t.Fatalf("header row %d should be rowMeta, got kind %d (%q)",
				i, r.Kind, ansi.Strip(m.MainLines[i]))
		}
	}
}

// Packing must preserve list order exactly — no sorting, no regrouping.
func TestOverviewLabelOrderPreserved(t *testing.T) {
	m := New(config.Default(), fakeforge.New())
	m.Loading = false
	m.Width, m.Height = 70, 40
	d := domain.PRDetail{
		Number: 1, Title: "t", State: "OPEN",
		Labels: []domain.Label{{Name: "bug"}, {Name: "release-blocker"}, {Name: "p1"}},
	}
	m.PRDetail = &d
	m = setMainOverview(m, d)

	joined := ansi.Strip(strings.Join(m.MainLines, "\n"))
	iBug := strings.Index(joined, "bug")
	iRel := strings.Index(joined, "release-blocker")
	iP1 := strings.Index(joined, "p1")
	if iBug < 0 || iRel < 0 || iP1 < 0 {
		t.Fatalf("all labels should render:\n%s", joined)
	}
	if !(iBug < iRel && iRel < iP1) {
		t.Fatalf("label order must be preserved (bug, release-blocker, p1):\n%s", joined)
	}
}
