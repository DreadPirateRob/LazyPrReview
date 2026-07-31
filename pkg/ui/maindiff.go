package ui

import "github.com/DreadPirateRob/LazyPrReview/pkg/diff"

// mainDiffKind names what diff content Main is showing.
type mainDiffKind uint8

const (
	mainDiffNone   mainDiffKind = iota
	mainDiffFile                // one PR file
	mainDiffDir                 // a directory subtree, aggregated
	mainDiffCommit              // one commit's changes
)

// mainDiffSource records the identity of Main's diff content — enough to redraw it
// without refetching. It is the single source of truth, replacing the per-kind fields
// that came before it (MainFileIndex for files, MainDirPath + MainDirFilter for
// subtrees, and nothing at all for commits, whose files never reach DiffFiles).
//
// That patchwork was survivable while only single files could be re-rendered. Once
// side-by-side and per-file folding apply to every kind, "what is on screen and how
// do I redraw it" needs ONE answer, or `|` from the Files panel, a focus bounce and a
// fold each drift apart.
//
// Cleared by setMainLines; every builder re-sets it immediately after, the same
// opt-back-in pattern the old fields used.
type mainDiffSource struct {
	Kind mainDiffKind

	FileIndex int // mainDiffFile: a DiffFiles index

	DirPath string // mainDiffDir
	// DirFilter is the Files filter the subtree was built under. filesUnderDir is
	// restricted to the visible set, so the same directory under a narrower filter is
	// a genuinely different view and must re-render.
	DirFilter string

	CommitOID      string // mainDiffCommit
	CommitHeadline string // mainDiffCommit
	// CommitFiles is parsed once by the fetch and kept, because these files are not
	// in DiffFiles and redrawing would otherwise cost a network round trip per toggle.
	CommitFiles []diff.File

	// Split records whether what is on screen is side-by-side. Distinct from the
	// DiffSplit preference: a pane too narrow to split renders unified while the
	// preference stays on.
	Split bool
}

// fileFoldKey namespaces a file's fold state inside the shared Folded map, which also
// holds thread ids and overview row keys.
func fileFoldKey(path string) string { return "file:" + path }

// fileExpanded reports whether a file's block is open in a multi-file view. Files
// default to expanded: collapsing is for skipping past what you have already read.
func fileExpanded(m Model, path string) bool {
	folded, ok := m.Folded[fileFoldKey(path)]
	return !ok || !folded
}

// toggleFileFold flips one file's block in a multi-file view and redraws.
func toggleFileFold(m Model, path string) Model {
	next := make(map[string]bool, len(m.Folded)+1)
	for k, v := range m.Folded {
		next[k] = v
	}
	next[fileFoldKey(path)] = fileExpanded(m, path)
	m.Folded = next
	return rerenderMainDiff(m)
}

// setAllFileFolds collapses or expands every file in the multi-file view on screen.
func setAllFileFolds(m Model, folded bool) Model {
	paths := multiFilePaths(m)
	if len(paths) == 0 {
		return m
	}
	next := make(map[string]bool, len(m.Folded)+len(paths))
	for k, v := range m.Folded {
		next[k] = v
	}
	for _, p := range paths {
		next[fileFoldKey(p)] = folded
	}
	m.Folded = next
	return rerenderMainDiff(m)
}

// multiFilePaths lists the files a multi-file view is showing, in render order, or nil
// when Main is not showing one.
func multiFilePaths(m Model) []string {
	switch m.MainDiff.Kind {
	case mainDiffDir:
		if m.PRDetail == nil {
			return nil
		}
		indices := filesUnderDir(m, m.MainDiff.DirPath)
		paths := make([]string, 0, len(indices))
		for _, idx := range indices {
			if idx >= 0 && idx < len(m.PRDetail.Files) {
				paths = append(paths, m.PRDetail.Files[idx].Path)
			}
		}
		return paths
	case mainDiffCommit:
		paths := make([]string, 0, len(m.MainDiff.CommitFiles))
		for _, f := range m.MainDiff.CommitFiles {
			paths = append(paths, f.Path)
		}
		return paths
	}
	return nil
}

// rerenderMainDiff redraws whatever diff content Main is showing, honouring the
// current side-by-side preference and fold state, and keeping the cursor and scroll
// where they were. Every mode or fold change routes through here so `|` from either
// panel, a focus bounce and a fold cannot disagree.
func rerenderMainDiff(m Model) Model {
	src := m.MainDiff
	cursor, scroll := m.MainCursor, m.MainScroll

	switch src.Kind {
	case mainDiffFile:
		m = renderDiffInMain(m, src.FileIndex)
	case mainDiffDir:
		m, _ = renderDirInMain(m, src.DirPath)
	case mainDiffCommit:
		m = renderCommitInMain(m, src.CommitOID, src.CommitHeadline, src.CommitFiles)
	default:
		return m
	}

	// Folding and mode changes shift rows, so clamp rather than assume.
	if cursor > len(m.MainLines)-1 {
		cursor = maxInt(0, len(m.MainLines)-1)
	}
	m.MainCursor = cursor
	m.MainScroll = scroll
	return m
}

// stepMainCursor moves the Main cursor one step, skipping the blank separator rows
// multi-file views place between files — landing on one puts the cursor on nothing:
// z has no fold target there and the row carries no content, which reads as keys
// going dead. Other content kinds keep plain single-step movement; the overview owns
// its own blank-row semantics.
func stepMainCursor(m Model, delta int) int {
	i := m.MainCursor + delta
	n := len(m.MainLines)
	if m.MainDiff.Kind == mainDiffDir || m.MainDiff.Kind == mainDiffCommit {
		for i > 0 && i < n-1 && m.MainLines[i] == "" {
			i += delta
		}
	}
	if i < 0 {
		return 0
	}
	if i > n-1 {
		return maxInt(0, n-1)
	}
	return i
}
