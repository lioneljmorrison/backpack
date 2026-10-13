package mcpserver_test

import (
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestSetStatusStartsAStoryOnItsBranch(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	s := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "in_progress",
		"branch": "feature/20261010-first"})
	if s["status"] != "in_progress" || s["branch"] != "feature/20261010-first" || s["activity"] != "planning" {
		t.Fatalf("story %v", s)
	}
}

func TestSetStatusRefusesToStartABlockedStoryWithoutAWorkaround(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	if _, err := b.AddDependency("demo", "tester", "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	refused(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-2", "to": "in_progress"})
	s := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-2", "to": "in_progress",
		"workaround": "build against mock data"})
	if s["workaround"] != "build against mock data" {
		t.Fatalf("story %v", s)
	}
}

func TestSetStatusNeedsAReasonToHold(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	if why := refused(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "on_hold"}); !strings.Contains(why, "reason") {
		t.Fatalf("reason %q", why)
	}
	s := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "on_hold",
		"reason": "needs more info"})
	if s["hold_reason"] != "needs more info" {
		t.Fatalf("story %v", s)
	}
}

func TestLogAddsAnEntryByTheTokensActor(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	e := use(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "decision",
		"text": "use the standard library", "reason": "fewer dependencies"})
	if e["kind"] != "decision" || e["actor"] != "tester" || e["reason"] != "fewer dependencies" {
		t.Fatalf("entry %v", e)
	}
}

func TestLogRefusesAStatusEntry(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	refused(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "status", "text": "done"})
}

func TestReadLogListsStatusChangesAndCorrections(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "in_progress"})
	note := use(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "note", "text": "built on Tuesday"})
	use(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "correction",
		"text": "it was Wednesday", "corrects": note["id"]})
	l := use(t, cs, "read_log", map[string]any{"workspace": "demo", "id": "S-1"})
	entries := l["entries"].([]any)
	var kinds []string
	for _, e := range entries {
		kinds = append(kinds, e.(map[string]any)["kind"].(string))
	}
	if strings.Join(kinds, ",") != "status,note,correction" {
		t.Fatalf("entries %v", entries)
	}
	if by := entries[1].(map[string]any)["corrected_by"]; by == nil {
		t.Fatalf("note %v is not marked corrected", entries[1])
	}
}

func TestSetStatusHoldsAProposalAndResumesItToProposed(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.AddStory("demo", "tester", board.NewStory{Title: "Maybe", Proposed: true, Reasoning: "it might help"}); err != nil {
		t.Fatal(err)
	}
	refused(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "on_hold"})
	s := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "on_hold", "reason": "wait for the design"})
	if s["status"] != "on_hold" || s["held_from"] != "proposed" {
		t.Fatalf("held %v", s)
	}
	s = use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "proposed"})
	if s["status"] != "proposed" {
		t.Fatalf("resumed %v", s)
	}
}
