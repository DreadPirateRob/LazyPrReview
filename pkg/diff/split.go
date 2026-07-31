package diff

// SplitRow is one visual row of a side-by-side view: the old revision on the left,
// the new on the right. Either side may be nil where the revisions have unequal
// numbers of changed lines, which the renderer draws as filler.
//
// FullWidth rows (hunk and file headers) carry their line in Left and span both
// columns; they have no right-hand counterpart.
type SplitRow struct {
	Left      *RenderedLine
	Right     *RenderedLine
	FullWidth bool
}

// SplitRows aligns a file's rendered lines into side-by-side rows.
//
// Context lines appear on both sides — it is the same line of code, and repeating it
// is what lets the eye track across the gutter. Within a hunk, a run of deletions is
// zipped against the run of additions that follows it, so a modified line sits
// opposite the line it replaced instead of being pushed down by every preceding
// insertion. Whichever run is longer leaves the other side nil.
//
// Pairing never crosses a hunk boundary: unrelated regions of a file must not be
// presented as replacements for each other.
func SplitRows(file File) []SplitRow {
	lines := file.Rendered
	rows := make([]SplitRow, 0, len(lines))

	for i := 0; i < len(lines); {
		switch lines[i].Kind {
		case LineKindFileHeader, LineKindHunkHeader:
			rows = append(rows, SplitRow{Left: &lines[i], FullWidth: true})
			i++
		case LineKindContext:
			rows = append(rows, SplitRow{Left: &lines[i], Right: &lines[i]})
			i++
		case LineKindDel, LineKindAdd:
			i = appendChangeRun(lines, i, &rows)
		default:
			// A kind this switch does not model: emit it full width rather than
			// dropping the line or failing to advance.
			rows = append(rows, SplitRow{Left: &lines[i], FullWidth: true})
			i++
		}
	}
	return rows
}

// appendChangeRun zips one deletion run against the addition run that follows it,
// both scoped to the same hunk, and returns the index just past the pair of runs.
func appendChangeRun(lines []RenderedLine, i int, rows *[]SplitRow) int {
	hunk := lines[i].HunkIndex

	delStart := i
	for i < len(lines) && lines[i].Kind == LineKindDel && lines[i].HunkIndex == hunk {
		i++
	}
	delEnd := i

	addStart := i
	for i < len(lines) && lines[i].Kind == LineKindAdd && lines[i].HunkIndex == hunk {
		i++
	}
	addEnd := i

	dels, adds := delEnd-delStart, addEnd-addStart
	for n := range max(dels, adds) {
		var row SplitRow
		if n < dels {
			row.Left = &lines[delStart+n]
		}
		if n < adds {
			row.Right = &lines[addStart+n]
		}
		*rows = append(*rows, row)
	}
	return i
}
