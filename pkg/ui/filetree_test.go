package ui

import (
	"testing"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// makeFile is a test helper that constructs a domain.ChangedFile with the
// minimum fields exercised by the filetree builders.
func makeFile(path string, adds, dels int, viewed bool) domain.ChangedFile {
	vs := "UNVIEWED"
	if viewed {
		vs = "VIEWED"
	}
	return domain.ChangedFile{
		Path:              path,
		Additions:         adds,
		Deletions:         dels,
		ViewerViewedState: vs,
	}
}

// ── buildFlatRows ──────────────────────────────────────────────────────────────

func TestBuildFlatRows_Basic(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("a/x.go", 1, 2, false),
		makeFile("b/y.go", 3, 4, true),
		makeFile("a/z.go", 5, 6, false),
	}
	rows := buildFlatRows(files, []int{0, 1, 2})

	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	cases := []struct {
		label  string
		path   string
		fidx   int
		adds   int
		dels   int
		viewed bool
	}{
		{"a/x.go", "a/x.go", 0, 1, 2, false},
		{"b/y.go", "b/y.go", 1, 3, 4, true},
		{"a/z.go", "a/z.go", 2, 5, 6, false},
	}
	for i, c := range cases {
		r := rows[i]
		if r.Label != c.label {
			t.Errorf("rows[%d].Label = %q; want %q", i, r.Label, c.label)
		}
		if r.Path != c.path {
			t.Errorf("rows[%d].Path = %q; want %q", i, r.Path, c.path)
		}
		if r.Depth != 0 {
			t.Errorf("rows[%d].Depth = %d; want 0", i, r.Depth)
		}
		if r.IsDir {
			t.Errorf("rows[%d].IsDir = true; want false", i)
		}
		if r.FileIndex != c.fidx {
			t.Errorf("rows[%d].FileIndex = %d; want %d", i, r.FileIndex, c.fidx)
		}
		if r.Adds != c.adds {
			t.Errorf("rows[%d].Adds = %d; want %d", i, r.Adds, c.adds)
		}
		if r.Dels != c.dels {
			t.Errorf("rows[%d].Dels = %d; want %d", i, r.Dels, c.dels)
		}
		if r.Viewed != c.viewed {
			t.Errorf("rows[%d].Viewed = %v; want %v", i, r.Viewed, c.viewed)
		}
	}
}

func TestBuildFlatRows_StableOrder(t *testing.T) {
	// include selects a subset in reverse order — output must respect include order
	files := []domain.ChangedFile{
		makeFile("a/x.go", 1, 0, false), // 0
		makeFile("b/y.go", 2, 0, false), // 1
		makeFile("a/z.go", 3, 0, false), // 2
	}
	rows := buildFlatRows(files, []int{2, 0})
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].FileIndex != 2 || rows[1].FileIndex != 0 {
		t.Errorf("include order not preserved: got FileIndex [%d, %d]; want [2, 0]",
			rows[0].FileIndex, rows[1].FileIndex)
	}
}

func TestBuildFlatRows_Empty(t *testing.T) {
	files := []domain.ChangedFile{makeFile("x.go", 1, 0, false)}
	if rows := buildFlatRows(files, nil); len(rows) != 0 {
		t.Errorf("nil include: want 0 rows, got %d", len(rows))
	}
	if rows := buildFlatRows(files, []int{}); len(rows) != 0 {
		t.Errorf("empty include slice: want 0 rows, got %d", len(rows))
	}
}

// ── buildTreeRows ──────────────────────────────────────────────────────────────

// TestBuildTreeRows_FirstAppearanceOrder verifies the interleaved a/x, b/y, a/z
// case: dir "a" appears first in the output because its first member (a/x.go)
// precedes b/y.go in include order, even though a/z.go follows b/y.go.
func TestBuildTreeRows_FirstAppearanceOrder(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("a/x.go", 1, 0, false), // 0
		makeFile("b/y.go", 2, 0, false), // 1
		makeFile("a/z.go", 3, 0, false), // 2
	}
	rows := buildTreeRows(files, []int{0, 1, 2}, nil)

	// Expected depth-first layout: a/ → x.go → z.go → b/ → y.go
	if len(rows) != 5 {
		t.Fatalf("want 5 rows, got %d: %v", len(rows), labelsOf(rows))
	}
	wantLabels := []string{"a/", "x.go", "z.go", "b/", "y.go"}
	for i, wl := range wantLabels {
		if rows[i].Label != wl {
			t.Errorf("rows[%d].Label = %q; want %q", i, rows[i].Label, wl)
		}
	}
	// dir rows
	if !rows[0].IsDir || !rows[3].IsDir {
		t.Error("rows at index 0 and 3 should be dirs")
	}
	// file rows FileIndex
	if rows[1].FileIndex != 0 {
		t.Errorf("x.go FileIndex want 0, got %d", rows[1].FileIndex)
	}
	if rows[2].FileIndex != 2 {
		t.Errorf("z.go FileIndex want 2, got %d", rows[2].FileIndex)
	}
	if rows[4].FileIndex != 1 {
		t.Errorf("y.go FileIndex want 1, got %d", rows[4].FileIndex)
	}
}

// TestBuildTreeRows_TreeStableOrder verifies that buildTreeRows also respects
// first-appearance when include is a subset (indices [1, 0]).
func TestBuildTreeRows_TreeStableOrder_Subset(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("z/a.go", 1, 0, false), // 0
		makeFile("a/b.go", 2, 0, false), // 1
	}
	// include order [1,0]: "a" should appear before "z"
	rows := buildTreeRows(files, []int{1, 0}, nil)
	if len(rows) != 4 {
		t.Fatalf("want 4 rows, got %d", len(rows))
	}
	if rows[0].Path != "a" {
		t.Errorf("first dir should be a (first in include), got %q", rows[0].Path)
	}
	if rows[2].Path != "z" {
		t.Errorf("second dir should be z, got %q", rows[2].Path)
	}
}

// TestBuildTreeRows_SingleChildCompression3Deep verifies that a 3-deep
// single-child directory chain collapses into one row.
func TestBuildTreeRows_SingleChildCompression3Deep(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("pkg/ui/keymap/keymap.go", 10, 5, false), // 0
		makeFile("pkg/ui/keymap/help.go", 3, 1, true),     // 1
	}
	rows := buildTreeRows(files, []int{0, 1}, nil)

	// pkg → ui → keymap: single-child chain of depth 3 → one dir row
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d: %v", len(rows), labelsOf(rows))
	}
	dir := rows[0]
	if !dir.IsDir {
		t.Fatal("rows[0] should be a dir")
	}
	if dir.Label != "pkg/ui/keymap/" {
		t.Errorf("compressed label = %q; want %q", dir.Label, "pkg/ui/keymap/")
	}
	if dir.Path != "pkg/ui/keymap" {
		t.Errorf("compressed path = %q; want %q", dir.Path, "pkg/ui/keymap")
	}
	if dir.Depth != 0 {
		t.Errorf("compressed dir Depth = %d; want 0", dir.Depth)
	}
	if dir.Adds != 13 || dir.Dels != 6 {
		t.Errorf("compressed dir Adds/Dels = %d/%d; want 13/6", dir.Adds, dir.Dels)
	}
	// children sit one level deeper
	if rows[1].Depth != 1 || rows[2].Depth != 1 {
		t.Errorf("children of compressed dir should be Depth 1, got %d and %d",
			rows[1].Depth, rows[2].Depth)
	}
	if rows[1].Label != "keymap.go" {
		t.Errorf("rows[1].Label = %q; want keymap.go", rows[1].Label)
	}
	if rows[2].Label != "help.go" {
		t.Errorf("rows[2].Label = %q; want help.go", rows[2].Label)
	}
}

// TestBuildTreeRows_TwoChildrenNoCompression verifies that a dir with two
// children is never compressed, preserving its local segment name as Label.
func TestBuildTreeRows_TwoChildrenNoCompression(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("pkg/a.go", 1, 0, false), // 0
		makeFile("pkg/b.go", 2, 0, false), // 1
	}
	rows := buildTreeRows(files, []int{0, 1}, nil)

	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	if rows[0].Label != "pkg/" {
		t.Errorf("dir label = %q; want %q", rows[0].Label, "pkg/")
	}
	if rows[0].Path != "pkg" {
		t.Errorf("dir path = %q; want %q", rows[0].Path, "pkg")
	}
	if rows[1].Depth != 1 || rows[2].Depth != 1 {
		t.Error("children of non-compressed dir should be Depth 1")
	}
}

// TestBuildTreeRows_Collapsed verifies that a collapsed dir emits exactly one
// row (its own), suppresses its children, but still reports the full subtree
// Adds and Dels.
func TestBuildTreeRows_Collapsed(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("pkg/a.go", 5, 3, false), // 0
		makeFile("pkg/b.go", 2, 1, true),  // 1
	}
	collapsed := map[string]bool{"pkg": true}
	rows := buildTreeRows(files, []int{0, 1}, collapsed)

	if len(rows) != 1 {
		t.Fatalf("collapsed dir: want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if !r.IsDir {
		t.Error("collapsed row should have IsDir=true")
	}
	if !r.Collapsed {
		t.Error("collapsed row should have Collapsed=true")
	}
	// subtree totals are still reported even though children are hidden
	if r.Adds != 7 || r.Dels != 4 {
		t.Errorf("collapsed dir Adds/Dels = %d/%d; want 7/4", r.Adds, r.Dels)
	}
}

// TestBuildTreeRows_DirViewed_Mixed verifies that a dir is not Viewed when the
// subtree contains at least one non-VIEWED file.
func TestBuildTreeRows_DirViewed_Mixed(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("d/a.go", 1, 0, true),  // 0 viewed
		makeFile("d/b.go", 2, 0, false), // 1 not viewed
	}
	rows := buildTreeRows(files, []int{0, 1}, nil)
	if rows[0].Viewed {
		t.Error("mixed subtree: dir Viewed should be false")
	}
}

// TestBuildTreeRows_DirViewed_AllViewed verifies that a dir is Viewed when
// every file in its subtree is VIEWED.
func TestBuildTreeRows_DirViewed_AllViewed(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("d/a.go", 1, 0, true), // 0
		makeFile("d/b.go", 2, 0, true), // 1
	}
	rows := buildTreeRows(files, []int{0, 1}, nil)
	if !rows[0].Viewed {
		t.Error("all-viewed subtree: dir Viewed should be true")
	}
}

// TestBuildTreeRows_DirAggregatesNested verifies that a parent dir's Adds/Dels
// sum across all nested subdirectories, not just direct children.
func TestBuildTreeRows_DirAggregatesNested(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("root/sub1/a.go", 5, 1, false), // 0
		makeFile("root/sub2/b.go", 3, 2, false), // 1
	}
	rows := buildTreeRows(files, []int{0, 1}, nil)

	// Expected layout:
	//   root/    depth 0  Adds=8  Dels=3
	//     sub1/  depth 1  Adds=5  Dels=1
	//       a.go depth 2
	//     sub2/  depth 1  Adds=3  Dels=2
	//       b.go depth 2
	if len(rows) != 5 {
		t.Fatalf("want 5 rows, got %d: %v", len(rows), labelsOf(rows))
	}

	depthCases := []struct{ idx, want int }{
		{0, 0}, {1, 1}, {2, 2}, {3, 1}, {4, 2},
	}
	for _, c := range depthCases {
		if rows[c.idx].Depth != c.want {
			t.Errorf("rows[%d].Depth = %d; want %d", c.idx, rows[c.idx].Depth, c.want)
		}
	}

	if rows[0].Adds != 8 || rows[0].Dels != 3 {
		t.Errorf("root Adds/Dels = %d/%d; want 8/3", rows[0].Adds, rows[0].Dels)
	}
	if rows[1].Adds != 5 || rows[1].Dels != 1 {
		t.Errorf("sub1 Adds/Dels = %d/%d; want 5/1", rows[1].Adds, rows[1].Dels)
	}
	if rows[3].Adds != 3 || rows[3].Dels != 2 {
		t.Errorf("sub2 Adds/Dels = %d/%d; want 3/2", rows[3].Adds, rows[3].Dels)
	}
}

func TestBuildTreeRows_EmptyInclude(t *testing.T) {
	files := []domain.ChangedFile{makeFile("x.go", 1, 0, false)}
	if rows := buildTreeRows(files, nil, nil); len(rows) != 0 {
		t.Errorf("nil include: want 0 rows, got %d", len(rows))
	}
	if rows := buildTreeRows(files, []int{}, nil); len(rows) != 0 {
		t.Errorf("empty include: want 0 rows, got %d", len(rows))
	}
}

// TestBuildTreeRows_RootLevelFile verifies that a file with no directory
// component becomes a file row at Depth 0 with Label == Path == the filename.
func TestBuildTreeRows_RootLevelFile(t *testing.T) {
	files := []domain.ChangedFile{makeFile("main.go", 3, 1, false)}
	rows := buildTreeRows(files, []int{0}, nil)

	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.Label != "main.go" {
		t.Errorf("Label = %q; want %q", r.Label, "main.go")
	}
	if r.Path != "main.go" {
		t.Errorf("Path = %q; want %q", r.Path, "main.go")
	}
	if r.IsDir {
		t.Error("root-level file should not be a dir")
	}
	if r.Depth != 0 {
		t.Errorf("Depth = %d; want 0", r.Depth)
	}
	if r.FileIndex != 0 {
		t.Errorf("FileIndex = %d; want 0", r.FileIndex)
	}
}

// ── allDirPaths ────────────────────────────────────────────────────────────────

// TestAllDirPaths_MatchesBuildTreeRows asserts set equality: allDirPaths must
// return exactly the Path values of the IsDir rows buildTreeRows emits. The
// fixture exercises both compressed and non-compressed dir rows.
func TestAllDirPaths_MatchesBuildTreeRows(t *testing.T) {
	files := []domain.ChangedFile{
		// pkg/ui has two children (keymap dir + files.go) → not compressed as a chain
		// pkg/ui/keymap has two file children → not compressed
		// but pkg → ui forms a single-child chain that stops at ui (two children)
		makeFile("pkg/ui/keymap/keymap.go", 10, 5, false), // 0
		makeFile("pkg/ui/keymap/help.go", 3, 1, true),     // 1
		makeFile("pkg/ui/files.go", 7, 2, false),          // 2
	}
	include := []int{0, 1, 2}

	rows := buildTreeRows(files, include, nil)
	wantSet := make(map[string]bool)
	for _, r := range rows {
		if r.IsDir {
			wantSet[r.Path] = true
		}
	}

	gotPaths := allDirPaths(files, include)
	gotSet := make(map[string]bool)
	for _, p := range gotPaths {
		if gotSet[p] {
			t.Errorf("allDirPaths returned duplicate path %q", p)
		}
		gotSet[p] = true
	}

	for p := range wantSet {
		if !gotSet[p] {
			t.Errorf("allDirPaths missing %q (present in buildTreeRows output)", p)
		}
	}
	for p := range gotSet {
		if !wantSet[p] {
			t.Errorf("allDirPaths returned extra %q (absent from buildTreeRows output)", p)
		}
	}
}

// TestAllDirPaths_CompressedChain verifies that a 3-deep single-child chain
// appears as a single path entry ("pkg/ui/keymap"), not three separate entries.
func TestAllDirPaths_CompressedChain(t *testing.T) {
	files := []domain.ChangedFile{
		makeFile("pkg/ui/keymap/k.go", 1, 0, false),
	}
	paths := allDirPaths(files, []int{0})
	if len(paths) != 1 {
		t.Fatalf("want 1 dir path, got %d: %v", len(paths), paths)
	}
	if paths[0] != "pkg/ui/keymap" {
		t.Errorf("got %q; want %q", paths[0], "pkg/ui/keymap")
	}
}

func TestAllDirPaths_Empty(t *testing.T) {
	files := []domain.ChangedFile{makeFile("x.go", 1, 0, false)}
	if paths := allDirPaths(files, nil); len(paths) != 0 {
		t.Errorf("nil include: want 0 paths, got %d", len(paths))
	}
}

func TestAllDirPaths_RootLevelFileOnly(t *testing.T) {
	files := []domain.ChangedFile{makeFile("main.go", 1, 0, false)}
	if paths := allDirPaths(files, []int{0}); len(paths) != 0 {
		t.Errorf("root-level file only: want 0 dir paths, got %d: %v", len(paths), paths)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────────

// labelsOf extracts labels from a row slice for compact failure messages.
func labelsOf(rows []fileRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Label
	}
	return out
}
