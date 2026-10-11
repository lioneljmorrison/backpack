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
	p, err := b.UpdatePhase("demo", "editor", 1, board.PhasePatch{Title: ptr("New")})
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "New" || p.Description != "about Old" || p.UpdatedBy != "editor" || p.UpdatedAt == "" {
		t.Fatalf("updated %+v", p)
	}
}

func TestUpdatePhaseRefusesAnEmptyTitleAndAnUnknownPhase(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	_, err := b.UpdatePhase("demo", actor, 1, board.PhasePatch{Title: ptr("")})
	wantKind(t, err, board.Invalid)
	_, err = b.UpdatePhase("demo", actor, 9, board.PhasePatch{Title: ptr("x")})
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

func addGoal(t *testing.T, b *board.Board, phase int, text string) board.Goal {
	t.Helper()
	g, err := b.AddGoal("demo", actor, phase, text)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func phase(t *testing.T, b *board.Board, n int) board.Phase {
	t.Helper()
	ps, err := b.Phases("demo")
	if err != nil {
		t.Fatal(err)
	}
	return ps[n-1]
}

func TestAGoalCanBeMarkedMetAndUnmarked(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addGoal(t, b, 1, "it works")
	g, err := b.SetGoalMet("demo", actor, "G-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Met || g.MetBy != actor || g.MetAt == "" {
		t.Fatalf("met %+v", g)
	}
	if got := phase(t, b, 1).Goals[0]; !got.Met || got.MetBy != actor {
		t.Fatalf("listed %+v", got)
	}
	g, err = b.SetGoalMet("demo", actor, "G-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if g.Met || g.MetBy != "" || g.MetAt != "" {
		t.Fatalf("unmarked %+v", g)
	}
	_, err = b.SetGoalMet("demo", actor, "G-9", true)
	wantKind(t, err, board.NotFound)
}

func TestAPhaseIsCompleteWhenEveryStoryStillWantedIsComplete(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addStory(t, b, board.NewStory{Title: "A", Phase: 1})
	addStory(t, b, board.NewStory{Title: "B", Phase: 1})
	addStory(t, b, board.NewStory{Title: "C", Phase: 1})
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	if p := phase(t, b, 1); p.Complete || p.StoriesTotal != 3 || p.StoriesComplete != 1 {
		t.Fatalf("one of three merged %+v", p)
	}
	walk(t, b, "S-2", "in_progress", "in_review", "merged", "deployed")
	setStatus(t, b, "S-3", board.StatusChange{To: "closed", Reason: "not needed"})
	if p := phase(t, b, 1); !p.Complete || p.StoriesTotal != 2 || p.StoriesComplete != 2 {
		t.Fatalf("the rest merged or closed %+v", p)
	}
}

func TestAPhaseWithNoStoriesIsNotComplete(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	if p := phase(t, b, 1); p.Complete {
		t.Fatalf("empty phase %+v", p)
	}
}

func TestAPhaseIsFlaggedWhenEveryGoalIsMetButStoriesRemainOpen(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addGoal(t, b, 1, "fast")
	addGoal(t, b, 1, "small")
	addStory(t, b, board.NewStory{Title: "A", Phase: 1})
	if _, err := b.SetGoalMet("demo", actor, "G-1", true); err != nil {
		t.Fatal(err)
	}
	if p := phase(t, b, 1); p.GoalsMetEarly {
		t.Fatalf("flagged with a goal unmet %+v", p)
	}
	if _, err := b.SetGoalMet("demo", actor, "G-2", true); err != nil {
		t.Fatal(err)
	}
	if p := phase(t, b, 1); !p.GoalsMetEarly {
		t.Fatalf("not flagged %+v", p)
	}
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	if p := phase(t, b, 1); p.GoalsMetEarly {
		t.Fatalf("flagged once complete %+v", p)
	}
}

func TestAPhaseWithNoGoalsIsNeverFlagged(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addStory(t, b, board.NewStory{Title: "A", Phase: 1})
	if p := phase(t, b, 1); p.GoalsMetEarly {
		t.Fatalf("flagged %+v", p)
	}
}

func TestAPhaseWithNoStoriesIsNotFlagged(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addGoal(t, b, 1, "fast")
	if _, err := b.SetGoalMet("demo", actor, "G-1", true); err != nil {
		t.Fatal(err)
	}
	if p := phase(t, b, 1); p.GoalsMetEarly {
		t.Fatalf("flagged with no open stories %+v", p)
	}
}
