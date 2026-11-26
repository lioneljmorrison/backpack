package board_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func toPhase(t *testing.T, b *board.Board, id string, phase int) (board.StoryUpdate, error) {
	t.Helper()
	return b.UpdateStoryDetail("demo", actor, id, board.StoryPatch{Phase: &phase})
}

func twoPhases(t *testing.T) *board.Board {
	t.Helper()
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addPhase(t, b, "Two")
	return b
}

func TestAStoryMovesIntoBetweenAndOutOfPhases(t *testing.T) {
	b := twoPhases(t)
	s := addStory(t, b, board.NewStory{Title: "loose"})
	for _, to := range []int{1, 2, 0} {
		u, err := toPhase(t, b, s.ID, to)
		if err != nil {
			t.Fatalf("to phase %d: %v", to, err)
		}
		if u.Story.Phase != to || !slices.Equal(u.Changed, []string{"phase"}) {
			t.Fatalf("to phase %d: story %+v changed %v", to, u.Story, u.Changed)
		}
		got, _ := b.Story("demo", s.ID)
		if got.Phase != to {
			t.Fatalf("read back phase %d, want %d", got.Phase, to)
		}
	}
	// the phases' counts follow the story
	ps, _ := b.Phases("demo")
	if ps[0].StoriesTotal != 0 || ps[1].StoriesTotal != 0 {
		t.Fatalf("phases %+v", ps)
	}
}

func TestMovingToThePhaseItIsInChangesNothing(t *testing.T) {
	b := twoPhases(t)
	s := addStory(t, b, board.NewStory{Title: "in one", Phase: 1})
	before, _ := b.Log("demo", s.ID)
	u, err := toPhase(t, b, s.ID, 1)
	if err != nil || len(u.Changed) != 0 {
		t.Fatalf("changed %v err %v", u.Changed, err)
	}
	after, _ := b.Log("demo", s.ID)
	if len(after) != len(before) {
		t.Fatal("a move to the same phase was logged")
	}
}

func TestAStoryCannotMoveToAPhaseThatDoesNotExist(t *testing.T) {
	b := twoPhases(t)
	s := addStory(t, b, board.NewStory{Title: "in one", Phase: 1})
	_, err := toPhase(t, b, s.ID, 9)
	wantKind(t, err, board.NotFound)
	if got, _ := b.Story("demo", s.ID); got.Phase != 1 {
		t.Fatalf("phase %d after a refused move", got.Phase)
	}
	_, err = toPhase(t, b, s.ID, -1)
	wantKind(t, err, board.Invalid)
}

func TestAStoryCannotMoveLaterThanItsDependency(t *testing.T) {
	b := twoPhases(t)
	on := addStory(t, b, board.NewStory{Title: "target", Phase: 2})
	dep := addStory(t, b, board.NewStory{Title: "dependent", Phase: 2})
	depend(t, b, dep.ID, on.ID)
	// dependent depends on target (phase 2): dependent may not go to phase 1
	_, err := toPhase(t, b, dep.ID, 1)
	wantKind(t, err, board.Conflict)
	if err == nil || !strings.Contains(err.Error(), on.ID) || !strings.Contains(err.Error(), "phase 2") {
		t.Fatalf("reason should name %s and its phase: %v", on.ID, err)
	}
	if got, _ := b.Story("demo", dep.ID); got.Phase != 2 {
		t.Fatalf("phase %d after a refused move", got.Phase)
	}
}

func TestAStoryCannotMoveEarlierThanAStoryThatDependsOnIt(t *testing.T) {
	b := twoPhases(t)
	on := addStory(t, b, board.NewStory{Title: "target", Phase: 1})
	dep := addStory(t, b, board.NewStory{Title: "dependent", Phase: 1})
	depend(t, b, dep.ID, on.ID)
	// target moves to phase 2 while dependent (depends on it) is in phase 1
	_, err := toPhase(t, b, on.ID, 2)
	wantKind(t, err, board.Conflict)
	if err == nil || !strings.Contains(err.Error(), dep.ID) || !strings.Contains(err.Error(), "phase 1") {
		t.Fatalf("reason should name %s and its phase: %v", dep.ID, err)
	}
	if got, _ := b.Story("demo", on.ID); got.Phase != 1 {
		t.Fatalf("phase %d after a refused move", got.Phase)
	}
}

func TestThePhaseRuleSkipsStoriesWithNoPhase(t *testing.T) {
	b := twoPhases(t)
	on := addStory(t, b, board.NewStory{Title: "target"})
	dep := addStory(t, b, board.NewStory{Title: "dependent"})
	depend(t, b, dep.ID, on.ID)
	if _, err := toPhase(t, b, on.ID, 2); err != nil {
		t.Fatalf("dependent has no phase: %v", err)
	}
	// removing a phase is never refused by the rule
	if _, err := toPhase(t, b, on.ID, 0); err != nil {
		t.Fatalf("out of a phase: %v", err)
	}
}

func TestMovingAStoryToAPhaseIsLoggedAndReported(t *testing.T) {
	b := twoPhases(t)
	s := addStory(t, b, board.NewStory{Title: "loose"})
	got := recorder(b)
	if _, err := toPhase(t, b, s.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := toPhase(t, b, s.ID, 2); err != nil {
		t.Fatal(err)
	}
	log, _ := b.Log("demo", s.ID)
	if len(log) < 2 || !strings.Contains(log[len(log)-2].Text, "phase 1") || !strings.Contains(log[len(log)-1].Text, "phase 1") ||
		!strings.Contains(log[len(log)-1].Text, "phase 2") {
		t.Fatalf("log %+v", log)
	}
	for _, c := range []board.Change{
		{Workspace: "demo", Kind: board.KindStory, ID: s.ID},
		{Workspace: "demo", Kind: board.KindPhase, ID: "1"},
		{Workspace: "demo", Kind: board.KindPhase, ID: "2"},
	} {
		if !slices.Contains(*got, c) {
			t.Fatalf("no change %+v in %+v", c, *got)
		}
	}
}
