package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/DreadPirateRob/LazyPrReview/pkg/config"
	ghforge "github.com/DreadPirateRob/LazyPrReview/pkg/forge/github"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
	"github.com/DreadPirateRob/LazyPrReview/pkg/ui"
)

func TestE2EReadOnlySmoke(t *testing.T) {
	if os.Getenv("LAZYPR_E2E") != "1" {
		t.Skip("set LAZYPR_E2E=1")
	}

	opts, err := parseArgs([]string{"https://github.com/jesseduffield/lazygit/pull/5731"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}

	var entries []ghcli.CommandLogEntry
	runner := ghcli.New(30*time.Second, func(entry ghcli.CommandLogEntry) { entries = append(entries, entry) })
	client := ghforge.New(runner)
	m := ui.New(config.Default(), client)
	m.OpenedPRNumber = opts.PRNumber
	m.StartupCmd = startupCmd(client, opts)

	msg := m.StartupCmd()
	updated, cmd := m.Update(msg)
	m = updated.(ui.Model)
	for _, entry := range entries {
		updated, _ = m.Update(ui.CommandLogEntryMsg{Entry: entry})
		m = updated.(ui.Model)
	}
	if cmd == nil {
		t.Fatal("expected detail-fetch command after startup")
	}
	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated.(ui.Model)

	view := fmt.Sprint(m.View())
	for _, section := range []string{"jesseduffield/lazygit", "Files", "[Unresolved]", "Checks", "Main"} {
		if !strings.Contains(view, section) {
			t.Fatalf("expected %q in initial view", section)
		}
	}
	if m.CurrentFocus() != ui.FocusMain {
		t.Fatalf("expected main overview focus after open, got %s", m.CurrentFocus())
	}

	m = applyKey(t, m, tea.KeyPressMsg{Text: "3", Code: '3'})
	if m.CurrentFocus() != ui.FocusFiles {
		t.Fatalf("expected files focus after key 3, got %s", m.CurrentFocus())
	}
	m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.CurrentFocus() != ui.FocusMain {
		t.Fatalf("expected main focus after file enter, got %s", m.CurrentFocus())
	}
	m = applyKey(t, m, tea.KeyPressMsg{Text: "t", Code: 't'})
	m = applyKey(t, m, tea.KeyPressMsg{Text: "T", Code: 'T'})
	m = applyKey(t, m, tea.KeyPressMsg{Text: "4", Code: '4'})
	if m.CurrentFocus() != ui.FocusThreads {
		t.Fatalf("expected threads focus after key 4, got %s", m.CurrentFocus())
	}
	m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.CurrentFocus() != ui.FocusThread {
		t.Fatalf("expected thread focus after enter, got %s", m.CurrentFocus())
	}
	m = applyKey(t, m, tea.KeyPressMsg{Text: "?", Code: '?'})
	if !m.HelpVisible || !strings.Contains(fmt.Sprint(m.View()), "Help") {
		t.Fatalf("expected help overlay visible")
	}
	m.CommandLog = entries
	m = applyKey(t, m, tea.KeyPressMsg{Text: "@", Code: '@'})
	if m.CurrentFocus() != ui.FocusMenu || !strings.Contains(fmt.Sprint(m.View()), "Command log menu") {
		t.Fatalf("expected command log menu visible")
	}
	m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.CommandLogOn || !strings.Contains(fmt.Sprint(m.View()), "Command log") {
		t.Fatalf("expected command log visible after menu selection")
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
	m = updated.(ui.Model)
	if cmd == nil {
		t.Fatalf("expected browser open command")
	}
	if next := cmd(); next != nil {
		updated, _ = m.Update(next)
		m = updated.(ui.Model)
	}
	m = applyKey(t, m, tea.KeyPressMsg{Text: "y", Code: 'y'})
	if m.CurrentFocus() != ui.FocusMenu || !strings.Contains(fmt.Sprint(m.View()), "Copy menu") {
		t.Fatalf("expected copy menu visible")
	}
	m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Toast.Message == "" {
		t.Fatalf("expected copy action to surface a toast")
	}
}

func applyKey(t *testing.T, m ui.Model, msg tea.KeyPressMsg) ui.Model {
	t.Helper()
	updated, cmd := m.Update(msg)
	m = updated.(ui.Model)
	if cmd != nil {
		next := cmd()
		updated, _ = m.Update(next)
		m = updated.(ui.Model)
	}
	return m
}
