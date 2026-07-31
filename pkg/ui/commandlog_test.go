package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/DreadPirateRob/LazyPrReview/pkg/ghcli"
)

func entry(cmd string) ghcli.CommandLogEntry {
	return ghcli.CommandLogEntry{
		Command:  []string{"gh", cmd},
		Duration: time.Millisecond,
		ExitCode: 0,
	}
}

func TestCommandLogRingEmpty(t *testing.T) {
	var r CommandLogRing
	if r.Len() != 0 {
		t.Fatalf("expected len 0, got %d", r.Len())
	}
	if got := r.Entries(); got != nil {
		t.Fatalf("expected nil entries on empty ring, got %v", got)
	}
}

func TestCommandLogRingAppend(t *testing.T) {
	var r CommandLogRing
	r.Append(entry("api"))
	r.Append(entry("pr"))
	if r.Len() != 2 {
		t.Fatalf("expected len 2, got %d", r.Len())
	}
	got := r.Entries()
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Command[1] != "api" || got[1].Command[1] != "pr" {
		t.Fatalf("wrong order: %v", got)
	}
}

func TestCommandLogRingOrder(t *testing.T) {
	var r CommandLogRing
	for i := range 10 {
		r.Append(entry(fmt.Sprintf("cmd%d", i)))
	}
	got := r.Entries()
	for i, e := range got {
		want := fmt.Sprintf("cmd%d", i)
		if e.Command[1] != want {
			t.Fatalf("entry[%d]: got %q want %q", i, e.Command[1], want)
		}
	}
}

func TestCommandLogRingTruncation(t *testing.T) {
	var r CommandLogRing

	// Fill exactly to capacity.
	for i := range CommandLogCapacity {
		r.Append(entry(fmt.Sprintf("cmd%d", i)))
	}
	if r.Len() != CommandLogCapacity {
		t.Fatalf("expected len %d, got %d", CommandLogCapacity, r.Len())
	}

	// One more entry must drop the oldest.
	r.Append(entry("NEW"))
	if r.Len() != CommandLogCapacity {
		t.Fatalf("len should stay at %d after overflow, got %d", CommandLogCapacity, r.Len())
	}

	entries := r.Entries()
	// Oldest surviving entry is now cmd1 (cmd0 was evicted).
	if entries[0].Command[1] != "cmd1" {
		t.Fatalf("expected oldest surviving to be cmd1, got %q", entries[0].Command[1])
	}
	// Newest entry is NEW.
	if entries[CommandLogCapacity-1].Command[1] != "NEW" {
		t.Fatalf("expected newest to be NEW, got %q", entries[CommandLogCapacity-1].Command[1])
	}
}

func TestCommandLogRingDoubleFill(t *testing.T) {
	var r CommandLogRing
	// Write 3× capacity to ensure wrap-around is stable.
	total := CommandLogCapacity * 3
	for i := range total {
		r.Append(entry(fmt.Sprintf("x%d", i)))
	}
	if r.Len() != CommandLogCapacity {
		t.Fatalf("expected len %d, got %d", CommandLogCapacity, r.Len())
	}
	got := r.Entries()
	// The last CommandLogCapacity entries should be present, oldest first.
	oldest := total - CommandLogCapacity
	for i, e := range got {
		want := fmt.Sprintf("x%d", oldest+i)
		if e.Command[1] != want {
			t.Fatalf("entry[%d]: got %q want %q", i, e.Command[1], want)
		}
	}
}

func TestCommandLogRingClear(t *testing.T) {
	var r CommandLogRing
	for range 50 {
		r.Append(entry("x"))
	}
	r.Clear()
	if r.Len() != 0 {
		t.Fatalf("expected len 0 after Clear, got %d", r.Len())
	}
	if r.Entries() != nil {
		t.Fatalf("expected nil entries after Clear")
	}
	// Append after clear should work cleanly.
	r.Append(entry("y"))
	if r.Len() != 1 {
		t.Fatalf("expected len 1 after post-clear append, got %d", r.Len())
	}
}

func TestPhase1CommandLogMenuOptions(t *testing.T) {
	if len(Phase1CommandLogMenuOptions) != 2 {
		t.Fatalf("expected exactly 2 Phase 1 options, got %d", len(Phase1CommandLogMenuOptions))
	}
	for _, opt := range Phase1CommandLogMenuOptions {
		if opt == "" {
			t.Fatal("empty menu option found")
		}
	}
	// "clear visible errors" must not appear in Phase 1.
	for _, opt := range Phase1CommandLogMenuOptions {
		if opt == "clear visible errors" {
			t.Fatal("\"clear visible errors\" must not appear in Phase 1 menu")
		}
	}
}
