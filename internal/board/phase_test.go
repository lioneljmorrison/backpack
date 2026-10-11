package board_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func addPhase(t *testing.T, b *board.Board, title string) board.Phase {
	t.Helper()
	p, err := b.AddPhase("demo", actor, title, "about "+title)
	if err != nil {
		t.Fatalf("add phase %q: %v", title, err)
	}
	return p
}

func ptr(s string) *string { return &s }

func TestPhasesAreNumberedInTheOrderTheyAreAdded(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "Foundations")
	addPhase(t, b, "Features")
	got, err := b.Phases("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Number != 1 || got[0].Title != "Foundations" || got[1].Number != 2 {
		t.Fatalf("phases %+v", got)
	}
	if got[0].Description != "about Foundations" || got[0].CreatedBy != actor || got[0].CreatedAt == "" {
		t.Fatalf("phase 1 %+v", got[0])
	}
}

func TestAPhaseNeedsATitle(t *testing.T) {
	b := newWorkspace(t)
	_, err := b.AddPhase("demo", actor, "", "")
	wantKind(t, err, board.Invalid)
}

func TestPhasesOfAnUnknownWorkspaceAreNotFound(t *testing.T) {
	b := newBoard(t)
	_, err := b.AddPhase("nope", actor, "x", "")
	wantKind(t, err, board.NotFound)
	_, err = b.Phases("nope")
	wantKind(t, err, board.NotFound)
}

func TestWorkspacesDoNotShareTheirPhases(t *testing.T) {
	b := newWorkspace(t)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "other", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	addPhase(t, b, "Only in demo")
	got, _ := b.Phases("other")
	if len(got) != 0 {
		t.Fatalf("other workspace sees %+v", got)
	}
	p, _ := b.AddPhase("other", actor, "First of other", "")
	if p.Number != 1 {
		t.Fatalf("other's first phase is number %d, want 1", p.Number)
	}
}

func TestUpdatePhaseChangesOnlyTheFieldsGiven(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "Old")
	p, err := b.UpdatePhase("demo", 1, board.PhasePatch{Title: ptr("New")})
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "New" || p.Description != "about Old" {
		t.Fatalf("updated %+v", p)
	}
}

func TestUpdatePhaseRefusesAnEmptyTitleAndAnUnknownPhase(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	_, err := b.UpdatePhase("demo", 1, board.PhasePatch{Title: ptr("")})
	wantKind(t, err, board.Invalid)
	_, err = b.UpdatePhase("demo", 9, board.PhasePatch{Title: ptr("x")})
	wantKind(t, err, board.NotFound)
}

func TestGoalsAreNumberedAcrossTheWorkspaceAndListedWithTheirPhase(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addPhase(t, b, "Two")
	for _, g := range []struct {
		phase int
		text  string
	}{{1, "first"}, {2, "second"}, {1, "third"}} {
		if _, err := b.AddGoal("demo", actor, g.phase, g.text); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := b.Phases("demo")
	one, two := got[0].Goals, got[1].Goals
	if len(one) != 2 || one[0].ID != "G-1" || one[0].Text != "first" || one[1].ID != "G-3" || one[0].CreatedBy != actor {
		t.Fatalf("phase 1 goals %+v", one)
	}
	if len(two) != 1 || two[0].ID != "G-2" {
		t.Fatalf("phase 2 goals %+v", two)
	}
}

func TestAGoalNeedsTextAndAnExistingPhase(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	_, err := b.AddGoal("demo", actor, 1, "")
	wantKind(t, err, board.Invalid)
	_, err = b.AddGoal("demo", actor, 2, "x")
	wantKind(t, err, board.NotFound)
}
