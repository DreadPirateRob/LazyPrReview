package diff

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
)

type LineKind string

const (
	LineKindFileHeader LineKind = "fileHeader"
	LineKindHunkHeader LineKind = "hunkHeader"
	LineKindContext    LineKind = "context"
	LineKindAdd        LineKind = "add"
	LineKindDel        LineKind = "del"
)

type File struct {
	Path     string
	OldPath  string
	NewPath  string
	Hunks    []Hunk
	Rendered []RenderedLine
	// Raw is the file's verbatim unified-diff section (from its `diff --git`
	// line up to the next file or EOF), for handing to external diff pagers.
	Raw string
}

type Hunk struct {
	Header    string
	OldStart  int
	OldLines  int
	NewStart  int
	NewLines  int
	Lines     []RenderedLine
}

type RenderedLine struct {
	Kind        LineKind
	OldNo       *int
	NewNo       *int
	Text        string
	FilePath    string
	HunkIndex   int
	RenderIndex int
}

type AnchoredThread struct {
	Thread      domain.Thread
	FilePath    string
	RenderIndex int
	AnchorSide  string
	Anchored    bool
	Badge       string
}

var hunkRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func Parse(text string) ([]File, error) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	files := []File{}
	var current *File
	var currentHunk *Hunk
	var raw strings.Builder
	oldLine := 0
	newLine := 0
	flush := func() {
		if current != nil {
			current.Raw = raw.String()
			files = append(files, *current)
		}
		raw.Reset()
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			oldPath, newPath := parseDiffHeader(line)
			path := newPath
			if path == "" || path == "/dev/null" {
				path = oldPath
			}
			current = &File{Path: path, OldPath: oldPath, NewPath: newPath}
			currentHunk = nil
			raw.WriteString(line)
			raw.WriteByte('\n')
			appendRendered(current, RenderedLine{Kind: LineKindFileHeader, Text: line, FilePath: path, HunkIndex: -1})
			continue
		}
		if current == nil {
			continue
		}
		raw.WriteString(line)
		raw.WriteByte('\n')
		switch {
		case strings.HasPrefix(line, "--- "):
			current.OldPath = trimPatchPath(strings.TrimPrefix(line, "--- "))
			if current.Path == "" || current.Path == "/dev/null" {
				current.Path = current.OldPath
			}
		case strings.HasPrefix(line, "+++ "):
			current.NewPath = trimPatchPath(strings.TrimPrefix(line, "+++ "))
			if current.NewPath != "" && current.NewPath != "/dev/null" {
				current.Path = current.NewPath
			}
		case strings.HasPrefix(line, "@@ "):
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			current.Hunks = append(current.Hunks, hunk)
			currentHunk = &current.Hunks[len(current.Hunks)-1]
			oldLine = currentHunk.OldStart
			newLine = currentHunk.NewStart
			rl := RenderedLine{Kind: LineKindHunkHeader, Text: line, FilePath: current.Path, HunkIndex: len(current.Hunks) - 1}
			appendToHunk(current, currentHunk, rl)
		case currentHunk != nil && strings.HasPrefix(line, "\\ No newline at end of file"):
			continue
		case currentHunk != nil:
			rendered, nextOld, nextNew, ok := parseContentLine(line, current.Path, len(current.Hunks)-1, oldLine, newLine)
			if !ok {
				continue
			}
			oldLine, newLine = nextOld, nextNew
			appendToHunk(current, currentHunk, rendered)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return files, nil
}

func CursorSide(kind LineKind) string {
	if kind == LineKindDel {
		return "LEFT"
	}
	return "RIGHT"
}

func AnchorThreads(files []File, threads []domain.Thread) []AnchoredThread {
	positions := make(map[string]map[string]int, len(files))
	for _, file := range files {
		positions[file.Path] = map[string]int{}
		for _, line := range file.Rendered {
			if line.NewNo != nil {
				positions[file.Path][fmt.Sprintf("RIGHT:%d", *line.NewNo)] = line.RenderIndex
			}
			if line.OldNo != nil {
				positions[file.Path][fmt.Sprintf("LEFT:%d", *line.OldNo)] = line.RenderIndex
			}
		}
	}
	anchored := make([]AnchoredThread, 0, len(threads))
	for _, thread := range threads {
		at := AnchoredThread{Thread: thread, FilePath: thread.Path, RenderIndex: -1, AnchorSide: thread.DiffSide}
		if thread.Line == nil {
			at.Badge = badgeFor(thread)
			anchored = append(anchored, at)
			continue
		}
		if posByKey, ok := positions[thread.Path]; ok {
			if idx, ok := posByKey[fmt.Sprintf("%s:%d", thread.DiffSide, *thread.Line)]; ok {
				at.RenderIndex = idx
				at.Anchored = true
				anchored = append(anchored, at)
				continue
			}
		}
		at.Badge = badgeFor(thread)
		anchored = append(anchored, at)
	}
	return anchored
}

func BuildUnresolvedIndex(files []File, anchored []AnchoredThread) []AnchoredThread {
	fileOrder := make(map[string]int, len(files))
	for i, file := range files {
		fileOrder[file.Path] = i
	}
	index := make([]AnchoredThread, 0, len(anchored))
	for _, item := range anchored {
		if !item.Anchored || item.Thread.IsResolved {
			continue
		}
		index = append(index, item)
	}
	sort.SliceStable(index, func(i, j int) bool {
		left, right := index[i], index[j]
		if fileOrder[left.FilePath] != fileOrder[right.FilePath] {
			return fileOrder[left.FilePath] < fileOrder[right.FilePath]
		}
		return left.RenderIndex < right.RenderIndex
	})
	return index
}

func appendRendered(file *File, line RenderedLine) {
	line.RenderIndex = len(file.Rendered)
	file.Rendered = append(file.Rendered, line)
}

func appendToHunk(file *File, hunk *Hunk, line RenderedLine) {
	appendRendered(file, line)
	line.RenderIndex = len(file.Rendered) - 1
	file.Hunks[len(file.Hunks)-1].Lines = append(file.Hunks[len(file.Hunks)-1].Lines, line)
}

func parseDiffHeader(line string) (string, string) {
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return "", ""
	}
	return trimPatchPath(parts[2]), trimPatchPath(parts[3])
}

func trimPatchPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}

func parseHunkHeader(line string) (Hunk, error) {
	m := hunkRE.FindStringSubmatch(line)
	if m == nil {
		return Hunk{}, fmt.Errorf("invalid hunk header %q", line)
	}
	oldStart, _ := strconv.Atoi(m[1])
	oldLines := 1
	if m[2] != "" {
		oldLines, _ = strconv.Atoi(m[2])
	}
	newStart, _ := strconv.Atoi(m[3])
	newLines := 1
	if m[4] != "" {
		newLines, _ = strconv.Atoi(m[4])
	}
	return Hunk{Header: line, OldStart: oldStart, OldLines: oldLines, NewStart: newStart, NewLines: newLines}, nil
}

func parseContentLine(line, path string, hunkIndex, oldLine, newLine int) (RenderedLine, int, int, bool) {
	if line == "" {
		return RenderedLine{}, oldLine, newLine, false
	}
	switch line[0] {
	case ' ':
		oldNo, newNo := oldLine, newLine
		return RenderedLine{Kind: LineKindContext, OldNo: &oldNo, NewNo: &newNo, Text: line[1:], FilePath: path, HunkIndex: hunkIndex}, oldLine + 1, newLine + 1, true
	case '+':
		newNo := newLine
		return RenderedLine{Kind: LineKindAdd, NewNo: &newNo, Text: line[1:], FilePath: path, HunkIndex: hunkIndex}, oldLine, newLine + 1, true
	case '-':
		oldNo := oldLine
		return RenderedLine{Kind: LineKindDel, OldNo: &oldNo, Text: line[1:], FilePath: path, HunkIndex: hunkIndex}, oldLine + 1, newLine, true
	default:
		return RenderedLine{}, oldLine, newLine, false
	}
}

func badgeFor(thread domain.Thread) string {
	if thread.IsOutdated {
		return "[outdated]"
	}
	return "[unanchored]"
}
