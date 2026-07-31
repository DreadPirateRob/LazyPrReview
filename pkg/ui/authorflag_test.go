package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
)

// An author the built-in list has never heard of is still treated as a bot once the
// user flags them. This is the case that motivated the feature.
func TestAuthorRoleOverrideMakesUnknownActorABot(t *testing.T) {
	m := overviewModel(t)
	if isBotAuthor(m, "chatgpt-codex-connector") {
		t.Fatal("precondition: this actor is not a built-in bot")
	}
	m.AuthorRoles = map[string]config.AuthorRole{"chatgpt-codex-connector": config.RoleBot}
	if !isBotAuthor(m, "chatgpt-codex-connector") {
		t.Fatal("a flagged author must be treated as a bot")
	}
}

// The override works in BOTH directions: a built-in default can be demoted.
func TestAuthorRoleOverrideDemotesBuiltInBot(t *testing.T) {
	m := overviewModel(t)
	if !isBotAuthor(m, "github-actions") {
		t.Fatal("precondition: github-actions is a built-in bot")
	}
	m.AuthorRoles = map[string]config.AuthorRole{"github-actions": config.RoleHuman}
	if isBotAuthor(m, "github-actions") {
		t.Fatal("a human override must beat the built-in bot default")
	}
}

// Lookup is case- and whitespace-insensitive, matching how the file stores keys.
func TestAuthorRoleLookupNormalizes(t *testing.T) {
	m := overviewModel(t)
	m.AuthorRoles = map[string]config.AuthorRole{"somebot": config.RoleBot}
	for _, in := range []string{"SomeBot", "  somebot  ", "SOMEBOT"} {
		if !isBotAuthor(m, in) {
			t.Errorf("expected %q to resolve to the stored override", in)
		}
	}
}

// Anything suffixed [bot] is a bot without the user having to flag it.
func TestAuthorRoleBotSuffixStillDetected(t *testing.T) {
	m := overviewModel(t)
	if !isBotAuthor(m, "some-app[bot]") {
		t.Fatal("GitHub App actors carry a [bot] suffix and should be detected")
	}
}

// b on a human comment flags them as a bot: the row regroups (bots collapse by
// default, so the body disappears), a save command is dispatched, and the cursor
// stays with that author rather than drifting to a stale index.
func TestBFlagsHumanAsBot(t *testing.T) {
	m := overviewModel(t)
	target := -1
	for i, r := range m.MainRows {
		if r.Kind == rowEventHeader && r.Author == "DreadPirateRob" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("expected a row for the human comment")
	}
	m.MainCursor = target
	if !strings.Contains(plain(m), "take a look") {
		t.Fatal("precondition: the human comment should be expanded")
	}

	got, cmd := UpdateMain(m, tea.KeyPressMsg{Text: "b", Code: 'b'})
	if cmd == nil {
		t.Fatal("flagging an author should dispatch a save")
	}
	if got.AuthorRoles[normalizeAuthor("DreadPirateRob")] != config.RoleBot {
		t.Fatalf("expected a bot override, got %q", got.AuthorRoles[normalizeAuthor("DreadPirateRob")])
	}
	if strings.Contains(plain(got), "take a look") {
		t.Fatal("newly flagged bot should collapse by default")
	}
	if !strings.Contains(ansi.Strip(got.Toast.Message), "DreadPirateRob") {
		t.Fatalf("expected a toast naming the author, got %q", got.Toast.Message)
	}
	// Cursor still on a row belonging to that author.
	if r, ok := rowAt(got, got.MainCursor); !ok || normalizeAuthor(r.Author) != normalizeAuthor("DreadPirateRob") {
		t.Fatalf("cursor should stay with the flagged author, got %+v", r)
	}
}

// And back the other way: b on a bot row demotes them, expanding their body.
func TestBFlagsBotAsHuman(t *testing.T) {
	m := overviewModel(t)
	target := -1
	for i, r := range m.MainRows {
		if r.Kind == rowGroupHeader && r.Author == "github-actions" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("expected a grouped bot row")
	}
	m.MainCursor = target

	got, cmd := UpdateMain(m, tea.KeyPressMsg{Text: "b", Code: 'b'})
	if cmd == nil {
		t.Fatal("flagging an author should dispatch a save")
	}
	if got.AuthorRoles["github-actions"] != config.RoleHuman {
		t.Fatalf("expected a human override, got %q", got.AuthorRoles["github-actions"])
	}
	// No longer a bot, so the run must not be grouped as one any more.
	for _, r := range got.MainRows {
		if r.Kind == rowGroupHeader && normalizeAuthor(r.Author) == "github-actions" {
			t.Fatal("a demoted author's comments should no longer collapse into a bot run")
		}
	}
	if !strings.Contains(plain(got), "Verdict") {
		t.Fatalf("demoted author's comments should now be expanded:\n%s", plain(got))
	}
}

// b on a row with nobody to flag (section rule, PR header, thread location summary)
// is inert and must not dispatch a write.
func TestBOnRowWithoutAuthorIsInert(t *testing.T) {
	m := overviewModel(t)
	m.MainCursor = rowOfKind(m, rowSection)
	got, cmd := UpdateMain(m, tea.KeyPressMsg{Text: "b", Code: 'b'})
	if cmd != nil {
		t.Fatal("a row with no author must not dispatch a save")
	}
	if len(got.AuthorRoles) != len(m.AuthorRoles) {
		t.Fatal("no override should be recorded")
	}
}

// A failed write must surface. Otherwise the flag appears to work and silently
// fails to survive a restart.
func TestAuthorRoleSaveFailureToasts(t *testing.T) {
	m := overviewModel(t)
	updated, _ := m.Update(authorRolesSavedMsg{Err: errors.New("disk full")})
	if got := updated.(Model).Toast.Message; got != ToastAuthorFlagFailed {
		t.Fatalf("expected a failure toast, got %q", got)
	}
}

// A successful save is silent — the flag was already applied optimistically.
func TestAuthorRoleSaveSuccessIsSilent(t *testing.T) {
	m := overviewModel(t)
	updated, _ := m.Update(authorRolesSavedMsg{})
	if got := updated.(Model).Toast.Message; got != "" {
		t.Fatalf("a successful save should not toast, got %q", got)
	}
}
