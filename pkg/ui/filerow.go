package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// renderFileRow formats one Files-panel row into exactly `width` visible
// columns. `width` is the row body budget — the caller has already subtracted
// the pane border (2) and the 2-column cursor prefix.
//
// Layout left-to-right: indent + marker + " " + label + filler + counters.
//
// The counters (+A / -D) are always flush-right and are never sacrificed: when
// the full row does not fit, the label is truncated from the LEFT so the
// identifying tail (basename) survives. At least one space always separates
// the label from the counters.
//
// When width <= 0 the row is returned unpadded with the full label intact —
// existing substring-based tests render at zero budget and must still match.
func renderFileRow(r fileRow, width int) string {
	indent := strings.Repeat("  ", r.Depth)

	var marker string
	if r.IsDir {
		if r.Collapsed {
			marker = "▸"
		} else {
			marker = "▾"
		}
	} else {
		if r.Viewed {
			marker = "✓"
		} else {
			marker = " "
		}
	}

	adds := diffAddStyle.Render(fmt.Sprintf("+%d", r.Adds))
	dels := diffDelStyle.Render(fmt.Sprintf("-%d", r.Dels))
	counters := adds + " " + dels

	// Headless path: caller has no width budget; return full label unpadded.
	if width <= 0 {
		return indent + marker + " " + r.Label + " " + counters
	}

	// prefixW = indent + marker + one mandatory space after marker.
	prefixW := ansi.StringWidth(indent) + ansi.StringWidth(marker) + 1
	counterW := ansi.StringWidth(counters)

	// labelBudget is the columns available for the label text, leaving a
	// minimum one-space gap between label and the flush-right counters.
	labelBudget := width - prefixW - 1 - counterW

	if labelBudget <= 0 {
		// No room for even a single label character. Counters win.
		if counterW <= width {
			return strings.Repeat(" ", width-counterW) + counters
		}
		// Even the counters overflow; clip ANSI-stripped text from the left so
		// the rightmost (most significant) digits survive.
		return rightWidthClip(counters, width)
	}

	label := r.Label
	labelW := ansi.StringWidth(label)

	var displayLabel string
	if labelW <= labelBudget {
		displayLabel = label
	} else {
		// Left-truncate: keep the rightmost characters so the basename survives.
		// "…" occupies exactly 1 display column.
		tailBudget := labelBudget - 1
		if tailBudget <= 0 {
			// Budget fits only the ellipsis itself.
			displayLabel = "…"
		} else {
			displayLabel = "…" + longestSuffixFitting(label, tailBudget)
		}
	}

	// fillerW >= 1 is guaranteed: displayLabelW <= labelBudget = width-prefixW-1-counterW.
	displayLabelW := ansi.StringWidth(displayLabel)
	fillerW := width - prefixW - displayLabelW - counterW
	return indent + marker + " " + displayLabel + strings.Repeat(" ", fillerW) + counters
}

// longestSuffixFitting returns the longest rune-wise suffix of s that fits
// within budget display columns, measured with ansi.StringWidth.
func longestSuffixFitting(s string, budget int) string {
	runes := []rune(s)
	total := 0
	start := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		w := ansi.StringWidth(string(runes[i]))
		if total+w > budget {
			break
		}
		total += w
		start = i
	}
	return string(runes[start:])
}

// rightWidthClip returns the rightmost maxW display columns of s
// (ANSI-stripped), left-padded with spaces to exactly maxW. Used only in the
// extreme degenerate case where even the counter string overflows the budget.
func rightWidthClip(s string, maxW int) string {
	plain := ansi.Strip(s)
	runes := []rune(plain)
	total := 0
	start := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		cw := ansi.StringWidth(string(runes[i]))
		if total+cw > maxW {
			break
		}
		total += cw
		start = i
	}
	result := string(runes[start:])
	if total < maxW {
		result = strings.Repeat(" ", maxW-total) + result
	}
	return result
}
