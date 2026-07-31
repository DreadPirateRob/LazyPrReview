package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	fakeforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/fake"
)

// Two small diffs used across all multifile tests.
const (
	multiDiffAlpha = "diff --git a/alpha.go b/alpha.go\n--- a/alpha.go\n+++ b/alpha.go\n@@ -1,2 +1,2 @@\n-old_a\n+new_a\n ctx_a\n"
	multiDiffBeta  = "diff --git a/beta.go b/beta.go\n--- a/beta.go\n+++ b/beta.go\n@@ -1,2 +1,2 @@\n-old_b\n+new_b\n ctx_b\n"
)

// multiFiles parses both test diffs and returns them as a two-element slice.
func multiFiles(t *testing.T) []diff.File {
	t.Helper()
	a, err := diff.Parse(multiDiffAlpha)
	if err != nil {
		t.Fatal(err)
	}
	b, err := diff.Parse(multiDiffBeta)
	if err != nil {
		t.Fatal(err)
	}
	return append(a, b...)
}

// multiDirModel returns a Model wired up for renderDirDiff, with two files in both
// DiffFiles and PRDetail.Files, and a terminal wide enough to host the split view.
func multiDirModel(t *testing.T) (Model, []diff.File, []int) {
	t.Helper()
	files := multiFiles(t)
	m := New(config.Default(), fakeforge.New())
	m.Width, m.Height = 200, 44
	m.DiffFiles = files
	m.PRDetail = &domain.PRDetail{
		Title: "multi-file test",
		Files: []domain.ChangedFile{
			{Path: files[0].Path},
			{Path: files[1].Path},
		},
	}
	return m, files, []int{0, 1}
}

// ----- renderDirDiff tests -----

func TestMultiFileDir_LenParallel(t *testing.T) {
	m, _, indices := multiDirModel(t)
	lines, rows := renderDirDiff(m, "pkg", indices)
	if len(lines) == 0 {
		t.Fatal("renderDirDiff returned no lines")
	}
	if len(lines) != len(rows) {
		t.Fatalf("lines/rows length mismatch: %d lines, %d rows", len(lines), len(rows))
	}
}

func TestMultiFileDir_FoldCollapsesSingleFile(t *testing.T) {
	m, files, indices := multiDirModel(t)

	// Collapse alpha.go; beta.go stays expanded.
	m.Folded = map[string]bool{
		fileFoldKey(files[0].Path): true,
	}
	lines, rows := renderDirDiff(m, "pkg", indices)
	if len(lines) != len(rows) {
		t.Fatalf("lines/rows mismatch: %d vs %d", len(lines), len(rows))
	}

	// Count rowFileHeader rows — must have exactly two (one per file).
	headers := 0
	for _, r := range rows {
		if r.Kind == rowFileHeader {
			headers++
		}
	}
	if headers != 2 {
		t.Fatalf("expected 2 file header rows, got %d", headers)
	}

	// No body row should carry text from alpha.go (collapsed).
	alphaPath := files[0].Path
	betaPath := files[1].Path
	alphaBodyRows := 0
	betaBodyRows := 0
	for i, r := range rows {
		if r.Kind != rowCode {
			continue
		}
		stripped := ansi.Strip(lines[i])
		if strings.Contains(stripped, "old_a") || strings.Contains(stripped, "new_a") || strings.Contains(stripped, "ctx_a") {
			alphaBodyRows++
		}
		if strings.Contains(stripped, "old_b") || strings.Contains(stripped, "new_b") || strings.Contains(stripped, "ctx_b") {
			betaBodyRows++
		}
		_ = alphaPath
		_ = betaPath
	}
	if alphaBodyRows != 0 {
		t.Fatalf("collapsed alpha.go should have 0 body rows, got %d", alphaBodyRows)
	}
	if betaBodyRows == 0 {
		t.Fatal("expanded beta.go must have at least one body row")
	}
}

func TestMultiFileDir_AllBodyRowsHaveNoRenderIndex(t *testing.T) {
	m, _, indices := multiDirModel(t)
	_, rows := renderDirDiff(m, "pkg", indices)
	for i, r := range rows {
		if r.Kind == rowCode && r.RenderIndex != -1 {
			t.Fatalf("body row %d has RenderIndex %d (want -1)", i, r.RenderIndex)
		}
	}
}

func TestMultiFileDir_FileHeaderRowsHaveKey(t *testing.T) {
	m, files, indices := multiDirModel(t)
	_, rows := renderDirDiff(m, "pkg", indices)
	wantKeys := map[string]bool{
		fileFoldKey(files[0].Path): false,
		fileFoldKey(files[1].Path): false,
	}
	for _, r := range rows {
		if r.Kind != rowFileHeader {
			continue
		}
		if _, ok := wantKeys[r.Key]; !ok {
			t.Fatalf("unexpected file header key %q", r.Key)
		}
		wantKeys[r.Key] = true
	}
	for k, seen := range wantKeys {
		if !seen {
			t.Fatalf("file header key %q not found in rows", k)
		}
	}
}

// In split mode the del/add pair (old_a → new_a) are placed on the SAME visual row.
// In unified mode they are always separate rows. This is a reliable discriminator
// because the unified gutter also contains │ (between line numbers and the +/- marker).
func TestMultiFileDir_SplitPairsDelAddOnOneRow(t *testing.T) {
	m, _, indices := multiDirModel(t)
	if !splitFits(mainContentWidth(m)) {
		t.Fatal("fixture must be wide enough for split")
	}

	// With split on, a code row should contain BOTH old_a and new_a (the paired del/add).
	m.DiffSplit = true
	lines, rows := renderDirDiff(m, "pkg", indices)
	foundPaired := false
	for i, r := range rows {
		if r.Kind != rowCode {
			continue
		}
		s := ansi.Strip(lines[i])
		if strings.Contains(s, "old_a") && strings.Contains(s, "new_a") {
			foundPaired = true
			break
		}
	}
	if !foundPaired {
		t.Fatal("split mode: expected del/add pair on one row (old_a and new_a same line)")
	}

	// With split off, no single code row contains both old_a and new_a.
	m.DiffSplit = false
	lines, rows = renderDirDiff(m, "pkg", indices)
	for i, r := range rows {
		if r.Kind != rowCode {
			continue
		}
		s := ansi.Strip(lines[i])
		if strings.Contains(s, "old_a") && strings.Contains(s, "new_a") {
			t.Fatalf("unified mode: del and add must not share a row (row %d): %q", i, s)
		}
	}
}

// ----- renderCommitDiff tests -----

func TestMultiFileCommit_LenParallel(t *testing.T) {
	m, files, _ := multiDirModel(t)
	lines, rows := renderCommitDiff(m, "abc1234567890", "fix: something", files, mainContentWidth(m))
	if len(lines) == 0 {
		t.Fatal("renderCommitDiff returned no lines")
	}
	if len(lines) != len(rows) {
		t.Fatalf("lines/rows length mismatch: %d lines, %d rows", len(lines), len(rows))
	}
}

func TestMultiFileCommit_EmptyFilesNoChanges(t *testing.T) {
	m, _, _ := multiDirModel(t)
	lines, rows := renderCommitDiff(m, "abc1234", "empty commit", nil, mainContentWidth(m))
	if len(lines) != len(rows) {
		t.Fatalf("lines/rows mismatch: %d vs %d", len(lines), len(rows))
	}
	if len(lines) != 2 {
		t.Fatalf("empty commit: expected 2 lines (header + no-changes), got %d", len(lines))
	}
	if !strings.Contains(ansi.Strip(lines[1]), "(no changes)") {
		t.Fatalf("empty commit second line should say '(no changes)', got %q", ansi.Strip(lines[1]))
	}
}

func TestMultiFileCommit_FoldCollapsesSingleFile(t *testing.T) {
	m, files, _ := multiDirModel(t)

	// Collapse alpha.go.
	m.Folded = map[string]bool{
		fileFoldKey(files[0].Path): true,
	}
	lines, rows := renderCommitDiff(m, "deadbeef", "folded", files, mainContentWidth(m))
	if len(lines) != len(rows) {
		t.Fatalf("lines/rows mismatch: %d vs %d", len(lines), len(rows))
	}

	headers := 0
	for _, r := range rows {
		if r.Kind == rowFileHeader {
			headers++
		}
	}
	if headers != 2 {
		t.Fatalf("expected 2 file header rows, got %d", headers)
	}

	alphaBody := 0
	betaBody := 0
	for i, r := range rows {
		if r.Kind != rowCode {
			continue
		}
		s := ansi.Strip(lines[i])
		if strings.Contains(s, "old_a") || strings.Contains(s, "new_a") || strings.Contains(s, "ctx_a") {
			alphaBody++
		}
		if strings.Contains(s, "old_b") || strings.Contains(s, "new_b") || strings.Contains(s, "ctx_b") {
			betaBody++
		}
	}
	if alphaBody != 0 {
		t.Fatalf("collapsed alpha.go: expected 0 body rows, got %d", alphaBody)
	}
	if betaBody == 0 {
		t.Fatal("expanded beta.go must produce at least one body row")
	}
}

func TestMultiFileCommit_AllBodyRowsHaveNoRenderIndex(t *testing.T) {
	m, files, _ := multiDirModel(t)
	_, rows := renderCommitDiff(m, "abc1234", "ri check", files, mainContentWidth(m))
	for i, r := range rows {
		if r.Kind == rowCode && r.RenderIndex != -1 {
			t.Fatalf("body row %d has RenderIndex %d (want -1)", i, r.RenderIndex)
		}
	}
}

func TestMultiFileCommit_FileHeaderRowsHaveKey(t *testing.T) {
	m, files, _ := multiDirModel(t)
	_, rows := renderCommitDiff(m, "abc1234", "keys check", files, mainContentWidth(m))
	wantKeys := map[string]bool{
		fileFoldKey(files[0].Path): false,
		fileFoldKey(files[1].Path): false,
	}
	for _, r := range rows {
		if r.Kind != rowFileHeader {
			continue
		}
		if _, ok := wantKeys[r.Key]; !ok {
			t.Fatalf("unexpected file header key %q", r.Key)
		}
		wantKeys[r.Key] = true
	}
	for k, seen := range wantKeys {
		if !seen {
			t.Fatalf("file header key %q not found", k)
		}
	}
}

func TestMultiFileCommit_SplitPairsDelAddOnOneRow(t *testing.T) {
	m, files, _ := multiDirModel(t)
	if !splitFits(mainContentWidth(m)) {
		t.Fatal("fixture must be wide enough for split")
	}
	width := mainContentWidth(m)

	// Split on: del/add pair on same code row.
	m.DiffSplit = true
	lines, rows := renderCommitDiff(m, "abc1234", "split on", files, width)
	foundPaired := false
	for i, r := range rows {
		if r.Kind != rowCode {
			continue
		}
		s := ansi.Strip(lines[i])
		if strings.Contains(s, "old_a") && strings.Contains(s, "new_a") {
			foundPaired = true
			break
		}
	}
	if !foundPaired {
		t.Fatal("split mode: expected del/add pair on one row (old_a and new_a same line)")
	}

	// Split off: no code row contains both.
	m.DiffSplit = false
	lines, rows = renderCommitDiff(m, "abc1234", "split off", files, width)
	for i, r := range rows {
		if r.Kind != rowCode {
			continue
		}
		s := ansi.Strip(lines[i])
		if strings.Contains(s, "old_a") && strings.Contains(s, "new_a") {
			t.Fatalf("unified mode: del and add must not share a row (row %d): %q", i, s)
		}
	}
}

func TestMultiFileCommit_ShortSHA(t *testing.T) {
	m, files, _ := multiDirModel(t)
	lines, rows := renderCommitDiff(m, "abc1234567890abcdef", "sha truncation", files, mainContentWidth(m))
	if len(lines) == 0 || len(rows) == 0 {
		t.Fatal("expected non-empty output")
	}
	first := ansi.Strip(lines[0])
	if !strings.Contains(first, "abc1234") {
		t.Fatalf("header should contain 7-char short SHA, got %q", first)
	}
	if strings.Contains(first, "abc1234567890") {
		t.Fatalf("header should truncate SHA to 7 chars, got %q", first)
	}
}
