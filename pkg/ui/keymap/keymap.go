package keymap

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Action struct {
	Name        string
	Context     string
	Description string
	Keys        []string
}

var keyPattern = regexp.MustCompile(`^<([a-z0-9]+(?:-[a-z0-9]+)*)>$`)

var actions = []Action{
	{"quit", "universal", "Quit lazypr", []string{"q", "<c-c>"}},
	{"back", "universal", "Back (pop focus; diff → overview)", []string{"<esc>"}},
	{"showHelp", "universal", "Help", []string{"?"}},
	{"cursorUp", "universal", "Move up", []string{"k", "<up>"}},
	{"cursorDown", "universal", "Move down", []string{"j", "<down>"}},
	{"prevPanel", "universal", "Previous panel", []string{"h", "<left>", "<backtab>"}},
	{"nextPanel", "universal", "Next panel", []string{"l", "<right>", "<tab>"}},
	{"focusStatus", "universal", "Focus Status panel", []string{"1"}},
	{"focusPRs", "universal", "Focus Pull Requests panel", []string{"2"}},
	{"focusFiles", "universal", "Focus Files panel", []string{"3"}},
	{"focusThreads", "universal", "Focus Threads panel", []string{"4"}},
	{"focusChecks", "universal", "Focus Checks panel", []string{"5"}},
	{"focusMain", "universal", "Jump to PR overview", []string{"0"}},
	{"prevTab", "universal", "Previous tab", []string{"["}},
	{"nextTab", "universal", "Next tab", []string{"]"}},
	{"jumpTop", "universal", "Jump to top", []string{"<"}},
	{"jumpBottom", "universal", "Jump to bottom", []string{">"}},
	{"listPageUp", "universal", "Page up", []string{","}},
	{"listPageDown", "universal", "Page down", []string{"."}},
	{"open", "universal", "Open selected item", []string{"<enter>"}},
	{"togglePrimary", "universal", "Primary toggle", []string{"<space>"}},
	{"toggleRangeSelect", "universal", "Toggle range selection", []string{"v"}},
	{"startFilter", "universal", "Filter or search", []string{"/"}},
	{"searchNext", "universal", "Next search match", []string{"n"}},
	{"searchPrev", "universal", "Previous search match", []string{"N"}},
	{"refresh", "universal", "Refresh current view", []string{"R"}},
	{"nextScreenMode", "universal", "Next screen mode", []string{"+"}},
	{"prevScreenMode", "universal", "Previous screen mode", []string{"_"}},
	{"copyMenu", "universal", "Open copy menu", []string{"y"}},
	{"openBrowser", "universal", "Open in browser", []string{"o"}},
	{"commandLogMenu", "universal", "Command log options", []string{"@"}},
	{"scrollDown", "universal", "Half page down (main)", []string{"J", "<c-d>"}},
	{"scrollUp", "universal", "Half page up (main)", []string{"K", "<c-u>"}},
	{"pageDown", "universal", "Page down main", []string{"<pgdn>"}},
	{"pageUp", "universal", "Page up main", []string{"<pgup>"}},
	{"openPR", "prs", "Open PR", []string{"<enter>"}},
	{"toggleViewed", "files", "Viewed", []string{"<space>"}},
	{"openDiff", "files", "Open file", []string{"<enter>"}},
	{"jumpFirstUnresolvedThread", "files", "First thread", []string{"t"}},
	{"toggleTreeFlat", "files", "Tree/flat", []string{"`"}},
	{"collapseAll", "files", "Collapse all", []string{"-"}},
	{"expandAll", "files", "Expand all", []string{"="}},
	{"openThread", "threads", "Open thread", []string{"<enter>"}},
	{"prevHunk", "main", "Prev hunk", []string{"h"}},
	{"nextHunk", "main", "Next hunk", []string{"l"}},
	{"prevFile", "main", "Prev file", []string{"["}},
	{"nextFile", "main", "Next file", []string{"]"}},
	{"nextUnresolvedThread", "main", "Next thread", []string{"t"}},
	{"prevUnresolvedThread", "main", "Prev thread", []string{"T"}},
	{"nextMention", "main", "Mention", []string{"m"}},
	{"prevMention", "main", "Prev mention", []string{"M"}},
	{"centerCursor", "main", "Center", []string{"zz"}},
	{"toggleThreadFold", "main", "Fold", []string{"z"}},
	{"collapseAllOverview", "main", "Collapse all", []string{"-"}},
	{"expandAllOverview", "main", "Expand all", []string{"="}},
	{"toggleAuthorBot", "main", "Bot?", []string{"b"}},
	{"editConfig", "status", "Edit config", []string{"e"}},
	// Authoring (Phase 2). Registered per real context so `?` help and the
	// hint bar surface them where they actually work.
	{"submitReview", "universal", "Submit review", []string{"S"}},
	{"commentLine", "main", "Comment", []string{"c"}},
	{"rangeSelect", "main", "Range", []string{"v"}},
	{"commentFile", "files", "Comment", []string{"c"}},
	{"resolveThread", "threads", "Resolve", []string{"<space>"}},
	{"replyThread", "thread", "Reply", []string{"r"}},
	{"editComment", "thread", "Edit", []string{"e"}},
	{"deleteComment", "thread", "Delete", []string{"d"}},
}

var byContext = buildByContext(actions)
var knownContexts = map[string]struct{}{
	"universal": {}, "status": {}, "prs": {}, "files": {}, "threads": {}, "thread": {}, "checks": {}, "main": {},
}

func Actions() []Action {
	out := make([]Action, len(actions))
	copy(out, actions)
	return out
}

func ActionsByContext() map[string][]Action {
	out := make(map[string][]Action, len(byContext))
	for ctx, list := range byContext {
		dup := make([]Action, len(list))
		copy(dup, list)
		out[ctx] = dup
	}
	return out
}

func Contexts() []string {
	out := make([]string, 0, len(knownContexts))
	for ctx := range knownContexts {
		out = append(out, ctx)
	}
	sort.Strings(out)
	return out
}

func DefaultBindings() map[string]map[string][]string {
	bindings := make(map[string]map[string][]string, len(byContext))
	for ctx, list := range byContext {
		bindings[ctx] = make(map[string][]string, len(list))
		for _, action := range list {
			keys := append([]string(nil), action.Keys...)
			bindings[ctx][action.Name] = keys
		}
	}
	for ctx := range knownContexts {
		if _, ok := bindings[ctx]; !ok {
			bindings[ctx] = map[string][]string{}
		}
	}
	return bindings
}

func KnownContext(context string) bool {
	_, ok := knownContexts[context]
	return ok
}

func KnownAction(context, action string) bool {
	for _, candidate := range byContext[context] {
		if candidate.Name == action {
			return true
		}
	}
	return false
}

func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if key == "<disabled>" {
		return nil
	}
	if utf8Len(key) == 1 {
		return nil
	}
	if !keyPattern.MatchString(strings.ToLower(key)) {
		return fmt.Errorf("invalid key syntax %q", key)
	}
	return nil
}

func buildByContext(actions []Action) map[string][]Action {
	out := make(map[string][]Action)
	for _, action := range actions {
		out[action.Context] = append(out[action.Context], action)
	}
	return out
}

func utf8Len(s string) int {
	return len([]rune(s))
}
