package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func depend(t *testing.T, b *board.Board, id, on string) {
	t.Helper()
	if _, err := b.AddDependency("demo", actor, id, on); err != nil {
		t.Fatalf("%s depends on %s: %v", id, on, err)
	}
}

func TestADependencyReadsFromBothEnds(t *testing.T) {
	b := threeStories(t)
	depend(t, b, "S-3", "S-1")
	depend(t, b, "S-3", "S-2")
	s3, _ := b.Story("demo", "S-3")
	s1, _ := b.Story("demo", "S-1")
	if !slices.Equal(s3.BlockedBy, []string{"S-1", "S-2"}) || len(s3.Blocks) != 0 {
		t.Fatalf("S-3 %+v", s3)
	}
	if !slices.Equal(s1.Blocks, []string{"S-3"}) || len(s1.BlockedBy) != 0 {
		t.Fatalf("S-1 %+v", s1)
	}
}

func TestAddingADependencyTwiceIsHarmless(t *testing.T) {
	b := threeStories(t)
	depend(t, b, "S-2", "S-1")
	depend(t, b, "S-2", "S-1")
	if s, _ := b.Story("demo", "S-2"); len(s.BlockedBy) != 1 {
		t.Fatalf("S-2 blocked by %v", s.BlockedBy)
	}
}

func TestAStoryCannotDependOnOneRankedBelowIt(t *testing.T) {
	b := threeStories(t)
	_, err := b.AddDependency("demo", actor, "S-1", "S-2")
	wantKind(t, err, board.Conflict)
	if s, _ := b.Story("demo", "S-1"); len(s.BlockedBy) != 0 {
		t.Fatal("a refused dependency was recorded")
	}
}

func TestAStoryCannotDependOnItself(t *testing.T) {
	b := threeStories(t)
	_, err := b.AddDependency("demo", actor, "S-1", "S-1")
	wantKind(t, err, board.Invalid)
}

func TestADependencyNeedsBothStories(t *testing.T) {
	b := threeStories(t)
	_, err := b.AddDependency("demo", actor, "S-2", "S-9")
	wantKind(t, err, board.NotFound)
	_, err = b.AddDependency("demo", actor, "S-9", "S-1")
	wantKind(t, err, board.NotFound)
}

func TestADependencyCannotReachIntoAnotherWorkspace(t *testing.T) {
	b := newWorkspace(t)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "other", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := b.AddStory("other", actor, board.NewStory{Title: "theirs"}); err != nil {
			t.Fatal(err)
		}
	}
	addStory(t, b, board.NewStory{Title: "ours"}) // demo's S-1; "other" has S-1 and S-2
	_, err := b.AddDependency("demo", actor, "S-1", "S-2")
	wantKind(t, err, board.NotFound)
}

// The phase rule applies only when both stories have a phase.
func TestDependenciesPointToTheSameOrAnEarlierPhase(t *testing.T) {
	cases := []struct {
		name                 string
		dependent, dependsOn int // phase numbers; 0 is no phase
		allowed              bool
	}{
		{"same phase", 1, 1, true},
		{"earlier phase", 2, 1, true},
		{"later phase", 1, 2, false},
		{"from a story with no phase", 0, 2, true},
		{"to a story with no phase", 1, 0, true},
		{"neither has a phase", 0, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newWorkspace(t)
			addPhase(t, b, "One")
			addPhase(t, b, "Two")
			on := addStory(t, b, board.NewStory{Title: "target", Phase: c.dependsOn})
			dep := addStory(t, b, board.NewStory{Title: "dependent", Phase: c.dependent})
			_, err := b.AddDependency("demo", actor, dep.ID, on.ID)
			if c.allowed && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.allowed {
				wantKind(t, err, board.Conflict)
			}
		})
	}
}

// A later phase's story may rank above an earlier phase's stories when it
// does not depend on them.
func TestRankIsNotConstrainedByPhaseAlone(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addPhase(t, b, "Two")
	addStory(t, b, board.NewStory{Title: "early", Phase: 1})
	addStory(t, b, board.NewStory{Title: "quick fix", Phase: 2, Position: board.Position{Before: "S-1"}})
	wantOrder(t, b, "S-2", "S-1")
}

func TestADependencyCannotCloseACycle(t *testing.T) {
	b := threeStories(t)
	depend(t, b, "S-2", "S-1")
	depend(t, b, "S-3", "S-2")
	_, err := b.AddDependency("demo", actor, "S-1", "S-3")
	wantKind(t, err, board.Conflict)
}

func TestAStoryCannotMoveAboveWhatItDependsOn(t *testing.T) {
	b := threeStories(t)
	depend(t, b, "S-3", "S-2")
	_, err := b.MoveStory("demo", actor, "S-3", board.Position{Before: "S-1"})
	wantKind(t, err, board.Conflict)
	wantOrder(t, b, "S-1", "S-2", "S-3")
}

func TestAStoryCannotMoveBelowWhatDependsOnIt(t *testing.T) {
	b := threeStories(t)
	depend(t, b, "S-2", "S-1")
	_, err := b.MoveStory("demo", actor, "S-1", board.Position{After: "S-3"})
	wantKind(t, err, board.Conflict)
	wantOrder(t, b, "S-1", "S-2", "S-3")
}

func TestAStoryCanMoveFreelyBetweenItsDependencies(t *testing.T) {
	b := threeStories(t)
	addStory(t, b, board.NewStory{Title: "D"}) // S-4
	depend(t, b, "S-4", "S-1")
	if _, err := b.MoveStory("demo", actor, "S-4", board.Position{After: "S-1"}); err != nil {
		t.Fatalf("a legal move was refused: %v", err)
	}
	wantOrder(t, b, "S-1", "S-4", "S-2", "S-3")
}

func TestARemovedDependencyNoLongerConstrainsOrder(t *testing.T) {
	b := threeStories(t)
	depend(t, b, "S-2", "S-1")
	if err := b.RemoveDependency("demo", "remover", "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	if s, _ := b.Story("demo", "S-1"); len(s.Blocks) != 0 {
		t.Fatalf("S-1 still blocks %v", s.Blocks)
	}
	if s, _ := b.Story("demo", "S-2"); s.UpdatedBy != "remover" || s.UpdatedAt == "" {
		t.Fatalf("removal not attributed on the dependent story: %+v", s)
	}
	if _, err := b.MoveStory("demo", actor, "S-1", board.Position{After: "S-3"}); err != nil {
		t.Fatalf("move after removal: %v", err)
	}
}

func TestRemovingADependencyThatDoesNotExistIsNotFound(t *testing.T) {
	b := threeStories(t)
	wantKind(t, b.RemoveDependency("demo", actor, "S-2", "S-1"), board.NotFound)
}
