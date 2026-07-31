package ui

import (
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	"github.com/DreadPirateRob/LazyPrReview/pkg/diff"
	"github.com/DreadPirateRob/LazyPrReview/pkg/domain"
	"github.com/DreadPirateRob/LazyPrReview/pkg/forge"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ui/keymap"
)

type FocusContext string

const (
	FocusStatus  FocusContext = "status"
	FocusPRs     FocusContext = "prs"
	FocusFiles   FocusContext = "files"
	FocusThreads FocusContext = "threads"
	FocusChecks  FocusContext = "checks"
	FocusMain    FocusContext = "main"
	FocusThread  FocusContext = "thread"
	FocusFatal   FocusContext = "fatal"
	FocusHelp    FocusContext = "help"
	FocusMenu    FocusContext = "menu"
	FocusFilter  FocusContext = "filter"
	FocusCompose FocusContext = "compose"
)

type FatalKind string

const (
	FatalNone      FatalKind = ""
	FatalMissingGH FatalKind = "missing-gh"
	FatalUnauthed  FatalKind = "unauthed-gh"
	FatalNoRepo    FatalKind = "no-repo"
)

type ScreenMode string

const (
	ScreenNormal     ScreenMode = "normal"
	ScreenHalf       ScreenMode = "half"
	ScreenFullscreen ScreenMode = "fullscreen"
)

type MainMode string

const (
	MainOverview MainMode = "overview"
	MainDiff     MainMode = "diff"
)

type Panel struct {
	Title    string
	Tab      string
	Cursor   int
	Filter   string
	Filtered []int
}

type Toast struct {
	Message string
	Until   time.Time
}

type HelpEntry struct {
	Context     string
	Action      string
	Keys        []string
	Description string
}

type MenuItem struct {
	Label string
	Key   string
	Kind  string
	Value string
	Toast string
}

type Layout struct {
	Width            int
	Height           int
	Portrait         bool
	SidePanelWidth   int
	MainWidth        int
	CommandLogHeight int
	HintHeight       int
	MainHeight       int
	PanelHeights     [5]int
}

type Model struct {
	Config                config.Config
	Forge                 forge.Forge
	Repo                  forge.Repo
	PRFilter              forge.PRFilter
	PRs                   []domain.PRSummary
	PRCache               map[forge.PRFilter]prCacheEntry
	SearchCache           map[string]prCacheEntry
	LoadingPRs            bool
	LoadingDetail         bool
	PRDetail              *domain.PRDetail
	Commits               []domain.CommitSummary
	DiffFiles             []diff.File
	AnchoredThreads       []diff.AnchoredThread
	UnresolvedThreadIndex []diff.AnchoredThread
	FocusStack            []FocusContext
	ScreenMode            ScreenMode
	Fatal                 FatalKind
	FatalMessage          string
	Status                Panel
	PRPanel               Panel
	FilesPanel            Panel
	ThreadsPanel          Panel
	ChecksPanel           Panel
	MainLines             []string
	// MainRows is the per-line metadata for MainLines, populated ONLY for the
	// built-in single-file diff — the one mode where a line anchor means something.
	// Cleared by setMainLines, so overview / commit / directory / pager content
	// leaves it nil and every line-scoped action is inert there by construction.
	MainRows []mainRow
	// Folded overrides default expansion for anything foldable in Main: inline diff
	// threads (keyed by thread ID) and overview timeline rows (keyed by the
	// synthetic keys in overview.go, since timeline items have no server ID).
	// Copy-on-write, since a Model is passed by value everywhere.
	Folded map[string]bool
	// AuthorRoles holds the user's persisted bot/human overrides, keyed by
	// lowercased login. Loaded at startup and rewritten whenever the user flags an
	// author, so the built-in bot list is only a default. Copy-on-write, since a
	// Model is passed by value everywhere.
	AuthorRoles   map[string]config.AuthorRole
	MainMode      MainMode
	MainFileIndex int
	// MainDirPath names the directory whose aggregate diff Main is showing, or ""
	// for any other content; MainDirFilter records the Files filter it was built
	// under, since the subtree is restricted to the visible set. Both pair with
	// MainFileIndex = -1, since a directory view is not a single-PR-file view, and
	// both are cleared by setMainLines so every other Main-content path drops them
	// without having to remember to.
	MainDirPath       string
	MainDirFilter     string
	MainCursor        int
	MainScroll        int
	MainPendingZ      bool
	MainRangeActive   bool
	MainRangeStart    int
	MainRangeGen      int
	MainGen           int
	ThreadCursor      int
	ThreadTab         string
	FilesFlat         bool
	FilesCollapsed    map[string]bool
	FilesRangeActive  bool
	FilesRangeStart   int
	MenuTitle         string
	MenuItems         []MenuItem
	MenuCursor        int
	CommandLog        []ghcli.CommandLogEntry
	CommandLogOn      bool
	CommandLogFocused bool
	CommandLogOffset  int
	Toast             Toast
	HelpVisible       bool
	HelpCursor        int
	HelpQuery         string
	HelpEntries       []HelpEntry
	Loading           bool
	Activity          string
	Width             int
	Height            int
	ViewerLogin       string
	FilterActive      bool
	OpenedPRNumber    int
	StartupCmd        tea.Cmd
	Composer          textarea.Model
	Compose           composeState
	ComposeSeq        int
	OptimisticDrafts  map[string]optimisticDraft
	PendingReviewID   string
	EnsureInFlight    bool
	DeleteTarget      forge.CommentRef
	FocusedThreadID   string
	SpinnerFrame      int
	// Keys is the effective binding table: shipped defaults with config.yml
	// overrides applied. Built once in New; read by dispatch, `?` help and the
	// hint bar so all three agree on which keys actually work.
	Keys *keymap.Bindings
}

type toastExpiredMsg struct{}

type probesLoadedMsg struct {
	Repo        forge.Repo
	PRs         []domain.PRSummary
	Fatal       FatalKind
	Detail      string
	ViewerLogin string
}

type StartupLoadedMsg = probesLoadedMsg
type prDetailLoadedMsg struct {
	Detail  domain.PRDetail
	Commits []domain.CommitSummary
	Diff    []diff.File
}

type prsLoadedMsg struct {
	Filter forge.PRFilter
	Query  string
	PRs    []domain.PRSummary
	Err    error
}

type mutationFailedMsg struct {
	Action string
	Err    error
}

type viewedToggleResultMsg struct {
	Previous map[int]string
	Failed   []int
}

type commandLogEntryMsg struct {
	Entry ghcli.CommandLogEntry
}

type CommandLogEntryMsg = commandLogEntryMsg

type refreshMsg struct{}

type autoRefreshTickMsg struct{}

type commitDiffLoadedMsg struct {
	SHA      string
	Headline string
	Files    []diff.File
	Err      error
}

type resolveResultMsg struct {
	ThreadID string
	Resolved bool
	Err      error
}

// spinnerTickMsg advances the loading spinner. It is only rescheduled while the
// full-screen loading view is active, so it never churns renders in normal UI.
type spinnerTickMsg struct{}

// authorRolesSavedMsg reports the result of persisting author bot/human flags.
// Success is silent — the UI already applied the flag optimistically.
type authorRolesSavedMsg struct {
	Err error
}
