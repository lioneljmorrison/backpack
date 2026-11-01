package board_test

import (
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// straightBoard is a workspace with one story, S-1, to find bugs in.
func straightBoard(t *testing.T) *board.Board {
	t.Helper()
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "Build A"})
	return b
}

// straightBug is a request that meets every check.
func straightBug() board.NewStory {
	return board.NewStory{
		Title: "Crash on save", Kind: "bug", FoundIn: "S-1", Reasoning: "next to where it was found",
		Position: board.Position{After: "S-1"},
		StraightIn: &board.StraightIn{Fix: "Check the length before copying", NoDesignChoice: true,
			NoContractChange: true, NothingElse: true},
	}
}

func straightRefused(t *testing.T, b *board.Board, n board.NewStory, want board.Kind) {
	t.Helper()
	before := order(t, b)
	_, err := b.AddStory("demo", actor, n)
	wantKind(t, err, want)
	wantOrder(t, b, before...)
}

func TestAStraightInBugLandsInTheBacklogWithOneDecisionEntry(t *testing.T) {
	b := straightBoard(t)
	s, err := b.AddStory("demo", actor, straightBug())
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != board.Backlog || !s.StraightIn || s.Kind != "bug" || s.FoundIn != "S-1" {
		t.Fatalf("added %+v", s)
	}
	wantOrder(t, b, "S-1", s.ID)
	log, err := b.Log("demo", s.ID)
	if err != nil || len(log) != 1 {
		t.Fatalf("log %+v, %v", log, err)
	}
	e := log[0]
	if e.Kind != "decision" || e.Actor != actor || e.Reason != "next to where it was found" {
		t.Fatalf("entry %+v", e)
	}
	for _, want := range []string{"Check the length before copying", "needs no design choice",
		"changes no API or tool contract", "changes nothing else"} {
		if !strings.Contains(e.Text, want) {
			t.Errorf("entry text %q lacks %q", e.Text, want)
		}
	}
	got, err := b.Story("demo", "S-1")
	if err != nil || len(got.Found) != 1 || got.Found[0] != s.ID || got.StraightIn {
		t.Fatalf("found-in story reads %+v, %v", got, err)
	}
}

func TestAStraightInBugMayTakeTheFoundInStorysPhaseOrNone(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addStory(t, b, board.NewStory{Title: "Build A", Phase: 1})
	n := straightBug()
	n.Phase = 1
	if s, err := b.AddStory("demo", actor, n); err != nil || s.Phase != 1 {
		t.Fatalf("same phase: %+v, %v", s, err)
	}
	n.Phase = 0
	if s, err := b.AddStory("demo", actor, n); err != nil || s.Phase != 0 {
		t.Fatalf("no phase: %+v, %v", s, err)
	}
}

func TestAStraightInBugIsRefusedWhenAnyCheckFails(t *testing.T) {
	cases := []struct {
		name string
		edit func(*board.NewStory)
		want board.Kind
	}{
		{"not a bug", func(n *board.NewStory) { n.Kind = "" }, board.Invalid},
		{"no found-in story", func(n *board.NewStory) { n.FoundIn = "" }, board.Invalid},
		{"unknown found-in story", func(n *board.NewStory) { n.FoundIn = "S-99" }, board.NotFound},
		{"no fix", func(n *board.NewStory) { n.StraightIn.Fix = "  " }, board.Invalid},
		{"a design choice is needed", func(n *board.NewStory) { n.StraightIn.NoDesignChoice = false }, board.Invalid},
		{"a contract changes", func(n *board.NewStory) { n.StraightIn.NoContractChange = false }, board.Invalid},
		{"something else changes", func(n *board.NewStory) { n.StraightIn.NothingElse = false }, board.Invalid},
		{"no place given", func(n *board.NewStory) { n.Position = board.Position{} }, board.Invalid},
		{"no reasoning", func(n *board.NewStory) { n.Reasoning = "" }, board.Invalid},
		{"also proposed", func(n *board.NewStory) { n.Proposed = true }, board.Invalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := straightBoard(t)
			n := straightBug()
			c.edit(&n)
			straightRefused(t, b, n, c.want)
		})
	}
}

func TestAStraightInBugIsRefusedWhenItsFoundInStoryIsClosed(t *testing.T) {
	b := straightBoard(t)
	setStatus(t, b, "S-1", board.StatusChange{To: "closed", Reason: "dropped"})
	straightRefused(t, b, straightBug(), board.Conflict)
}

func TestAStraightInBugIsRefusedInAPhaseThatIsNotTheFoundInStorys(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addPhase(t, b, "Two")
	addStory(t, b, board.NewStory{Title: "Build A", Phase: 1})
	addStory(t, b, board.NewStory{Title: "Build B"})
	n := straightBug()
	n.Phase = 2
	straightRefused(t, b, n, board.Conflict)
	// Found in a story with no phase, it cannot take one.
	n.FoundIn, n.Position, n.Phase = "S-2", board.Position{After: "S-2"}, 1
	straightRefused(t, b, n, board.Conflict)
	n.Phase = 99
	straightRefused(t, b, n, board.NotFound)
}

func TestAStraightInBugIsRefusedInACompletePhase(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addStory(t, b, board.NewStory{Title: "Build A", Phase: 1})
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	n := straightBug()
	n.Phase = 1
	straightRefused(t, b, n, board.Conflict)
	n.Phase = 0
	if _, err := b.AddStory("demo", actor, n); err != nil {
		t.Fatalf("with no phase: %v", err)
	}
}

func TestAStraightInBugIsListedUntilStartedAndFlaggedWhenItGainsADependencyEitherWay(t *testing.T) {
	b := straightBoard(t)
	first, err := b.AddStory("demo", actor, straightBug())
	if err != nil {
		t.Fatal(err)
	}
	second := straightBug()
	second.Title = "Typo in error"
	second.Position = board.Position{After: first.ID}
	other, err := b.AddStory("demo", actor, second)
	if err != nil {
		t.Fatal(err)
	}
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(w.StraightIn); strings.Join(got, ",") != "S-2,S-3" {
		t.Fatalf("listed %v", got)
	}
	if text := w.Text(); !strings.Contains(text, "S-2 Crash on save — found in S-1\n") ||
		strings.Contains(text, "flagged") {
		t.Fatalf("text before a dependency:\n%s", text)
	}
	// Gains a dependency (it waits on another story), and is depended on.
	addStory(t, b, board.NewStory{Title: "Library", Position: board.Position{Before: first.ID}})
	depend(t, b, first.ID, "S-4")
	addStory(t, b, board.NewStory{Title: "Later"})
	depend(t, b, "S-5", other.ID)
	w, err = b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	text := w.Text()
	for _, line := range []string{"S-2 Crash on save — found in S-1; flagged: now has a dependency",
		"S-3 Typo in error — found in S-1; flagged: now has a dependency"} {
		if !strings.Contains(text, line) {
			t.Errorf("missing %q in:\n%s", line, text)
		}
	}
	// Once started it is no longer listed.
	setStatus(t, b, "S-3", board.StatusChange{To: "in_progress"})
	if w, err = b.Where("demo"); err != nil || strings.Join(ids(w.StraightIn), ",") != "S-2" {
		t.Fatalf("after starting: %v, %v", ids(w.StraightIn), err)
	}
}

func TestAStraightInBugIsFlaggedWhenItLosesItsBugMark(t *testing.T) {
	b := straightBoard(t)
	s, err := b.AddStory("demo", actor, straightBug())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.UpdateStory("demo", actor, s.ID, board.StoryPatch{Kind: ptr("")}); err != nil {
		t.Fatal(err)
	}
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if text := w.Text(); !strings.Contains(text, "S-2 Crash on save — found in S-1; flagged: no longer marked as a bug") {
		t.Fatalf("text:\n%s", text)
	}
}

func TestAStoryThatIsNotStraightInIsNeverListedAsOne(t *testing.T) {
	b := straightBoard(t)
	addStory(t, b, board.NewStory{Title: "Plain bug", Kind: "bug", FoundIn: "S-1"})
	w, err := b.Where("demo")
	if err != nil || len(w.StraightIn) != 0 || strings.Contains(w.Text(), "Straight in") {
		t.Fatalf("where %+v, %v", w, err)
	}
}

func straightText(t *testing.T, b *board.Board) string {
	t.Helper()
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	return w.Text()
}

func TestAStraightInBugStaysFlaggedWhenItsDependencyHasFinished(t *testing.T) {
	b := straightBoard(t)
	bug, err := b.AddStory("demo", actor, straightBug())
	if err != nil {
		t.Fatal(err)
	}
	lib, err := b.AddStory("demo", actor, board.NewStory{Title: "Library", Position: board.Position{Before: bug.ID}})
	if err != nil {
		t.Fatal(err)
	}
	walk(t, b, lib.ID, "in_progress", "in_review", "merged")
	depend(t, b, bug.ID, lib.ID)
	if text := straightText(t, b); !strings.Contains(text, "Crash on save — found in S-1; flagged: now has a dependency") {
		t.Fatalf("not flagged:\n%s", text)
	}
}

func TestAStraightInBugStaysFlaggedWhenItsDependentIsClosed(t *testing.T) {
	b := straightBoard(t)
	bug, err := b.AddStory("demo", actor, straightBug())
	if err != nil {
		t.Fatal(err)
	}
	later, err := b.AddStory("demo", actor, board.NewStory{Title: "Later", Position: board.Position{After: bug.ID}})
	if err != nil {
		t.Fatal(err)
	}
	depend(t, b, later.ID, bug.ID)
	setStatus(t, b, later.ID, board.StatusChange{To: "closed", Reason: "dropped"})
	if text := straightText(t, b); !strings.Contains(text, "Crash on save — found in S-1; flagged: now has a dependency") {
		t.Fatalf("not flagged:\n%s", text)
	}
}
