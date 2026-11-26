package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func setClosed(t *testing.T, b *board.Board, n int, closed bool) board.Phase {
	t.Helper()
	p, err := b.UpdatePhase("demo", actor, n, board.PhasePatch{Closed: &closed})
	if err != nil {
		t.Fatalf("closed=%v: %v", closed, err)
	}
	return p
}

func TestAPhaseCanBeClosedAndReopened(t *testing.T) {
	b := twoPhases(t)
	p := setClosed(t, b, 1, true)
	if !p.Closed || p.ClosedBy != actor || p.ClosedAt == "" {
		t.Fatalf("closed %+v", p)
	}
	ps, _ := b.Phases("demo")
	if !ps[0].Closed || ps[1].Closed {
		t.Fatalf("phases %+v", ps)
	}
	if p = setClosed(t, b, 1, false); p.Closed || p.ClosedBy != "" || p.ClosedAt != "" {
		t.Fatalf("reopened %+v", p)
	}
}

func TestClosingAndReopeningAPhaseIsLoggedOnThePhase(t *testing.T) {
	b := twoPhases(t)
	setClosed(t, b, 1, true)
	setClosed(t, b, 1, true) // already closed: nothing to log
	setClosed(t, b, 1, false)
	log, err := b.PhaseLog("demo", 1)
	if err != nil || len(log) != 2 {
		t.Fatalf("log %+v err %v", log, err)
	}
	if log[0].Field != "closed" || log[0].From != "open" || log[0].To != "closed" ||
		log[1].From != "closed" || log[1].To != "open" {
		t.Fatalf("log %+v", log)
	}
}

func TestACloseDoesNotTouchTheTitleOrDescription(t *testing.T) {
	b := twoPhases(t)
	p := setClosed(t, b, 1, true)
	if p.Title != "One" || p.Description != "about One" {
		t.Fatalf("phase %+v", p)
	}
}

func TestAClosedPhaseIsSkippedWhenChoosingTheCurrentPhase(t *testing.T) {
	b := twoPhases(t) // phase 1 has no stories, so it is not complete
	w, _ := b.Where("demo")
	if w.Phase == nil || w.Phase.Number != 1 {
		t.Fatalf("phase %+v", w.Phase)
	}
	setClosed(t, b, 1, true)
	w, _ = b.Where("demo")
	if w.Phase == nil || w.Phase.Number != 2 {
		t.Fatalf("phase %+v", w.Phase)
	}
	setClosed(t, b, 2, true)
	if w, _ = b.Where("demo"); w.Phase != nil {
		t.Fatalf("phase %+v", w.Phase)
	}
	setClosed(t, b, 1, false)
	if w, _ = b.Where("demo"); w.Phase == nil || w.Phase.Number != 1 {
		t.Fatalf("phase %+v", w.Phase)
	}
}

func TestClosingAPhaseDoesNotMakeItComplete(t *testing.T) {
	b := twoPhases(t)
	if p := setClosed(t, b, 1, true); p.Complete {
		t.Fatalf("phase %+v", p)
	}
}

func TestAStoryCannotBeAddedToAClosedPhase(t *testing.T) {
	b := twoPhases(t)
	addStory(t, b, board.NewStory{Title: "found in", Phase: 1}) // S-1
	setClosed(t, b, 1, true)
	_, err := b.AddStory("demo", actor, board.NewStory{Title: "late", Phase: 1})
	wantKind(t, err, board.Conflict)
	if ss, _ := b.Stories("demo"); len(ss) != 1 {
		t.Fatalf("stories %+v", ss)
	}
	n := straightBug()
	n.Phase = 1
	straightRefused(t, b, n, board.Conflict)
}

func TestAStoryCannotMoveIntoAClosedPhaseButCanMoveOut(t *testing.T) {
	b := twoPhases(t)
	s := addStory(t, b, board.NewStory{Title: "in one", Phase: 1})
	o := addStory(t, b, board.NewStory{Title: "loose"})
	setClosed(t, b, 1, true)
	_, err := toPhase(t, b, o.ID, 1)
	wantKind(t, err, board.Conflict)
	// the same phase it is already in is not a move
	if _, err := toPhase(t, b, s.ID, 1); err != nil {
		t.Fatalf("staying put: %v", err)
	}
	u, err := toPhase(t, b, s.ID, 2)
	if err != nil || !slices.Equal(u.Changed, []string{"phase"}) {
		t.Fatalf("out of a closed phase: %v %v", u.Changed, err)
	}
	if _, err := toPhase(t, b, s.ID, 0); err != nil {
		t.Fatal(err)
	}
}

func TestAReopenedPhaseTakesStoriesAgain(t *testing.T) {
	b := twoPhases(t)
	setClosed(t, b, 1, true)
	setClosed(t, b, 1, false)
	if _, err := b.AddStory("demo", actor, board.NewStory{Title: "on time", Phase: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestClosingAPhaseReportsIt(t *testing.T) {
	b := twoPhases(t)
	got := recorder(b)
	setClosed(t, b, 2, true)
	want := board.Change{Workspace: "demo", Kind: board.KindPhase, ID: "2"}
	if len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("changes %+v", *got)
	}
}
