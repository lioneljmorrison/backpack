package board_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func propose(t *testing.T, b *board.Board, s board.NewStory) board.Story {
	t.Helper()
	s.Proposed = true
	if s.Reasoning == "" {
		s.Reasoning = "it follows " + s.After
	}
	return addStory(t, b, s)
}

func TestAProposedStoryWaitsInProposedWithItsReasoningLogged(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	s := addStory(t, b, board.NewStory{Title: "B", Proposed: true, Reasoning: "B builds on A",
		Position: board.Position{After: "S-1"}})
	if s.Status != "proposed" {
		t.Fatalf("status %q, want proposed", s.Status)
	}
	wantOrder(t, b, "S-1", "S-2")
	l := readLog(t, b, "S-2")
	if len(l) != 1 || l[0].Kind != "decision" || l[0].Actor != actor ||
		l[0].Text != "Proposed after S-1" || l[0].Reason != "B builds on A" {
		t.Fatalf("log %+v", l)
	}
}

func TestTheReasoningNamesWhereTheStoryWasProposed(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	propose(t, b, board.NewStory{Title: "B", Position: board.Position{Before: "S-1"}})
	propose(t, b, board.NewStory{Title: "C"})
	if got := readLog(t, b, "S-2")[0].Text; got != "Proposed before S-1" {
		t.Errorf("S-2 %q", got)
	}
	if got := readLog(t, b, "S-3")[0].Text; got != "Proposed at the bottom of the backlog" {
		t.Errorf("S-3 %q", got)
	}
}

func TestAProposalNeedsItsReasoningAndOnlyAProposalHasOne(t *testing.T) {
	b := newWorkspace(t)
	_, err := b.AddStory("demo", actor, board.NewStory{Title: "A", Proposed: true})
	wantKind(t, err, board.Invalid)
	_, err = b.AddStory("demo", actor, board.NewStory{Title: "A", Reasoning: "why"})
	wantKind(t, err, board.Invalid)
	_, err = b.AddStory("demo", actor, board.NewStory{Title: "A", Proposed: true, Reasoning: "bad\x00"})
	wantKind(t, err, board.Invalid)
	wantOrder(t, b)
}

func TestAProposalIsApprovedIntoTheBacklogOrDeclinedByClosingIt(t *testing.T) {
	b := newWorkspace(t)
	propose(t, b, board.NewStory{Title: "A"})
	propose(t, b, board.NewStory{Title: "B"})
	setStatus(t, b, "S-1", board.StatusChange{To: "backlog"})
	setStatus(t, b, "S-2", board.StatusChange{To: "closed", Reason: "not needed"})
	wantStatus(t, b, "S-1", "backlog")
	wantStatus(t, b, "S-2", "closed")
	if l := readLog(t, b, "S-1"); len(l) != 2 || l[1].From != "proposed" || l[1].To != "backlog" {
		t.Fatalf("log %+v", l)
	}
}

func TestAProposalCannotBeStartedAndAStoryCannotGoBackToProposed(t *testing.T) {
	b := newWorkspace(t)
	propose(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	for _, c := range []struct {
		id string
		to board.StatusChange
	}{
		{"S-1", board.StatusChange{To: "in_progress"}},
		{"S-2", board.StatusChange{To: "proposed"}},
	} {
		_, err := b.SetStatus("demo", actor, c.id, c.to)
		wantKind(t, err, board.Conflict)
	}
	wantStatus(t, b, "S-1", "proposed")
	wantStatus(t, b, "S-2", "backlog")
}

func TestAProposalCanBeHeldWithAReasonAndResumesToProposed(t *testing.T) {
	b := newWorkspace(t)
	propose(t, b, board.NewStory{Title: "A"})
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "on_hold"})
	wantKind(t, err, board.Invalid)
	wantStatus(t, b, "S-1", "proposed")

	held := setStatus(t, b, "S-1", board.StatusChange{To: "on_hold", Reason: "wait for the design"})
	if held.HeldFrom != "proposed" || held.HoldReason != "wait for the design" {
		t.Fatalf("held %+v", held)
	}
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Proposed) != 0 {
		t.Fatalf("a held proposal is listed as proposed: %v", ids(w.Proposed))
	}
	_, err = b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "backlog"})
	wantKind(t, err, board.Conflict)

	setStatus(t, b, "S-1", board.StatusChange{To: "proposed"})
	setStatus(t, b, "S-1", board.StatusChange{To: "backlog"})
	wantStatus(t, b, "S-1", "backlog")
	l := readLog(t, b, "S-1")
	if len(l) != 4 || l[1].From != "proposed" || l[1].To != "on_hold" ||
		l[2].From != "on_hold" || l[2].To != "proposed" || l[3].To != "backlog" {
		t.Fatalf("log %+v", l)
	}
}

func TestWhatsNextAndTheNextThreeSkipProposals(t *testing.T) {
	b := newWorkspace(t)
	propose(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	next, err := b.Next("demo")
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "S-2" {
		t.Fatalf("next %+v", next)
	}
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(w.Next), []string{"S-2"}) || !slices.Equal(ids(w.Proposed), []string{"S-1"}) {
		t.Fatalf("next %v, proposed %v", ids(w.Next), ids(w.Proposed))
	}
}

func TestWhereAreWeListsWhatIsWaitingForApproval(t *testing.T) {
	b := newWorkspace(t)
	propose(t, b, board.NewStory{Title: "Maybe"})
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if text := w.Text(); !strings.Contains(text, "Waiting for approval:\n  S-1 Maybe\n") {
		t.Fatalf("text:\n%s", text)
	}
}

func TestTheBoardShowsProposalsFirst(t *testing.T) {
	b := newWorkspace(t)
	propose(t, b, board.NewStory{Title: "A"})
	cols, err := b.Board("demo")
	if err != nil {
		t.Fatal(err)
	}
	if cols[0].Status != "proposed" || !slices.Equal(ids(cols[0].Stories), []string{"S-1"}) {
		t.Fatalf("columns %+v", cols)
	}
}

func TestAPhaseIsNotCompleteWhileAProposalInItWaits(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addStory(t, b, board.NewStory{Title: "A", Phase: 1})
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	propose(t, b, board.NewStory{Title: "B", Phase: 1})
	if p := phase(t, b, 1); p.Complete || p.StoriesTotal != 2 {
		t.Fatalf("phase %+v", p)
	}
}
