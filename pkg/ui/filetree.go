package ui

import (
	"strings"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

// fileRow is one line in the Files panel. In flat mode every row is a file; in
// tree mode directory rows are interleaved with the files beneath them, so a row
// is NOT necessarily a file — FileIndex < 0 marks a directory. The Files cursor
// therefore indexes ROWS, not files, and every consumer maps through FileIndex.
type fileRow struct {
	Label     string // display text: basename for files, compressed segment ("ui/keymap/") for dirs
	Path      string // full file path, or full directory path with no trailing slash
	Depth     int    // indent level, 0 at the panel root
	IsDir     bool
	Collapsed bool // dirs only: subtree hidden
	FileIndex int  // index into PRDetail.Files; -1 for directories
	Adds      int  // the file's own additions, or the subtree sum for a dir
	Dels      int  // the file's own deletions, or the subtree sum for a dir
	Viewed    bool // the file is VIEWED, or every file in the dir's subtree is
}

// buildFlatRows returns one row per index in include, in include order.
// Every row is a file: full path as Label, Depth 0, no directory rows.
func buildFlatRows(files []domain.ChangedFile, include []int) []fileRow {
	if len(include) == 0 {
		return nil
	}
	rows := make([]fileRow, 0, len(include))
	for _, idx := range include {
		if idx < 0 || idx >= len(files) {
			continue
		}
		f := files[idx]
		rows = append(rows, fileRow{
			Label:     f.Path,
			Path:      f.Path,
			Depth:     0,
			IsDir:     false,
			FileIndex: idx,
			Adds:      f.Additions,
			Dels:      f.Deletions,
			Viewed:    f.ViewerViewedState == "VIEWED",
		})
	}
	return rows
}

// treeNode is an ephemeral internal node used only while building the directory
// tree. It is never exposed outside this file.
type treeNode struct {
	name          string
	path          string // full path, no trailing slash; "" for the virtual root
	isDir         bool
	fileIdx       int // -1 for dir nodes
	file          *domain.ChangedFile
	childrenOrder []string // child names in first-appearance order
	children      map[string]*treeNode
}

// buildTree constructs an internal tree from the included files, preserving
// the first-appearance order of each directory (sibling order = the order in
// which the first file from each subtree appears in include).
func buildTree(files []domain.ChangedFile, include []int) *treeNode {
	root := &treeNode{isDir: true, path: "", children: make(map[string]*treeNode)}
	for _, idx := range include {
		if idx < 0 || idx >= len(files) {
			continue
		}
		f := &files[idx]
		parts := strings.Split(f.Path, "/")
		cur := root
		for i, part := range parts {
			if i == len(parts)-1 {
				// leaf: file node
				if _, exists := cur.children[part]; !exists {
					cur.children[part] = &treeNode{
						name:    part,
						path:    f.Path,
						isDir:   false,
						fileIdx: idx,
						file:    f,
					}
					cur.childrenOrder = append(cur.childrenOrder, part)
				}
			} else {
				// intermediate: directory node
				dirPath := strings.Join(parts[:i+1], "/")
				if _, exists := cur.children[part]; !exists {
					cur.children[part] = &treeNode{
						name:     part,
						path:     dirPath,
						isDir:    true,
						fileIdx:  -1,
						children: make(map[string]*treeNode),
					}
					cur.childrenOrder = append(cur.childrenOrder, part)
				}
				cur = cur.children[part]
			}
		}
	}
	return root
}

// compressDir follows single-child-directory chains, collapsing them into one
// display row. It returns the label (all compressed segment names joined with
// "/" plus a trailing "/") and the deepest node whose children will be emitted.
//
// A chain stops compressing when the current node has more than one child, or
// its sole child is a file rather than a directory.
func compressDir(n *treeNode) (label string, bottom *treeNode) {
	label = n.name
	bottom = n
	for {
		if len(bottom.childrenOrder) != 1 {
			break
		}
		only := bottom.children[bottom.childrenOrder[0]]
		if !only.isDir {
			break
		}
		label = label + "/" + only.name
		bottom = only
	}
	return label + "/", bottom
}

// subtreeStats recursively computes the aggregate Adds, Dels, and Viewed for n
// and all of its descendants. For a file node it returns the file's own values.
// A dir's Viewed is true only when every descendant file is VIEWED and the
// subtree is non-empty.
func subtreeStats(n *treeNode) (adds, dels int, allViewed bool) {
	if !n.isDir {
		return n.file.Additions, n.file.Deletions, n.file.ViewerViewedState == "VIEWED"
	}
	allViewed = len(n.childrenOrder) > 0
	for _, key := range n.childrenOrder {
		a, d, v := subtreeStats(n.children[key])
		adds += a
		dels += d
		if !v {
			allViewed = false
		}
	}
	return
}

// emitRows recursively emits fileRows in depth-first display order.
// n == root (path == "") is the virtual root, which has no row of its own;
// its children start at depth 0.
func emitRows(n *treeNode, depth int, collapsed map[string]bool) []fileRow {
	if !n.isDir {
		return []fileRow{{
			Label:     n.name,
			Path:      n.path,
			Depth:     depth,
			IsDir:     false,
			FileIndex: n.fileIdx,
			Adds:      n.file.Additions,
			Dels:      n.file.Deletions,
			Viewed:    n.file.ViewerViewedState == "VIEWED",
		}}
	}
	if n.path == "" {
		// virtual root: no row of its own, emit children at depth 0
		var rows []fileRow
		for _, key := range n.childrenOrder {
			rows = append(rows, emitRows(n.children[key], 0, collapsed)...)
		}
		return rows
	}

	// real dir node: compress single-child-dir chains into one row
	label, bottom := compressDir(n)
	// subtreeStats starts at bottom; intermediate compressed nodes have no
	// sibling files (by the compression invariant), so bottom's totals equal n's.
	adds, dels, allViewed := subtreeStats(bottom)
	isCollapsed := collapsed[bottom.path] // nil map read is safe in Go

	row := fileRow{
		Label:     label,
		Path:      bottom.path,
		Depth:     depth,
		IsDir:     true,
		Collapsed: isCollapsed,
		FileIndex: -1,
		Adds:      adds,
		Dels:      dels,
		Viewed:    allViewed,
	}
	result := []fileRow{row}
	if !isCollapsed {
		for _, key := range bottom.childrenOrder {
			result = append(result, emitRows(bottom.children[key], depth+1, collapsed)...)
		}
	}
	return result
}

// buildTreeRows arranges the included files into a directory tree, emitting rows
// depth-first (a dir row immediately followed by its subtree). Sibling order is
// stable first-appearance: the order in include, never alphabetically re-sorted.
// Single-child-dir chains are compressed into one row. Collapsed dirs emit their
// row (including subtree totals) but suppress their children.
func buildTreeRows(files []domain.ChangedFile, include []int, collapsed map[string]bool) []fileRow {
	if len(include) == 0 {
		return nil
	}
	root := buildTree(files, include)
	return emitRows(root, 0, collapsed)
}

// allDirPaths returns every directory Path that buildTreeRows would emit for the
// same files/include, regardless of collapse state. Order is unspecified; no
// duplicates. Compressed chains contribute the deepest path in the chain (e.g.
// "pkg/ui/keymap", not "pkg" and "pkg/ui" and "pkg/ui/keymap" separately).
// Used by collapse-all to build the collapsed map.
func allDirPaths(files []domain.ChangedFile, include []int) []string {
	if len(include) == 0 {
		return nil
	}
	root := buildTree(files, include)
	var result []string
	collectDirPaths(root, &result)
	return result
}

// collectDirPaths traverses the tree and appends compressed directory paths,
// mirroring the compression logic in emitRows so the returned paths agree
// exactly with what buildTreeRows emits.
func collectDirPaths(n *treeNode, result *[]string) {
	if n.path == "" {
		// virtual root: no dir path of its own, traverse children
		for _, key := range n.childrenOrder {
			collectDirPaths(n.children[key], result)
		}
		return
	}
	if !n.isDir {
		return // file nodes contribute no directory path
	}
	_, bottom := compressDir(n)
	*result = append(*result, bottom.path)
	// continue into bottom's children (compression consumed only single-child
	// intermediate dirs, so bottom may still have dir children of its own)
	for _, key := range bottom.childrenOrder {
		collectDirPaths(bottom.children[key], result)
	}
}
