package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func wantNext(t *testing.T, b *board.Board, repo, id string) {
	t.Helper()
	next, err := b.Next("demo", repo)
	if err != nil || next == nil || next.ID != id {
		t.Fatalf("next %+v, %v; want %s", next, err, id)
	}
}

func TestNextPrefersTheCurrentPhasesTopReadyStoryOverAHigherRankedOne(t *testing.T) {
	b := twoPhases(t)
	addStory(t, b, board.NewStory{Title: "outside"})            // S-1
	addStory(t, b, board.NewStory{Title: "inside", Phase: 1})   // S-2
	addStory(t, b, board.NewStory{Title: "inside 2", Phase: 1}) // S-3
	wantNext(t, b, "", "S-2")
}

func TestNextSkipsABlockedStoryInThePhase(t *testing.T) {
	b := twoPhases(t)
	addStory(t, b, board.NewStory{Title: "outside"})           // S-1
	addStory(t, b, board.NewStory{Title: "blocked", Phase: 1}) // S-2
	addStory(t, b, board.NewStory{Title: "ready", Phase: 1})   // S-3
	depend(t, b, "S-2", "S-1")
	wantNext(t, b, "", "S-3")
}

func TestNextFallsBackToRankWhenThePhaseHasNothingReady(t *testing.T) {
	b := twoPhases(t)
	addStory(t, b, board.NewStory{Title: "outside"})           // S-1
	addStory(t, b, board.NewStory{Title: "blocked", Phase: 1}) // S-2
	depend(t, b, "S-2", "S-1")
	wantNext(t, b, "", "S-1")
}

func TestNextFollowsRankWhenThereIsNoCurrentPhase(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "first"})
	wantNext(t, b, "", "S-1")
}

func TestNextNeverUsesAClosedPhaseAndFollowsRankWhenEveryPhaseIsClosed(t *testing.T) {
	b := twoPhases(t)
	addStory(t, b, board.NewStory{Title: "outside"})          // S-1
	addStory(t, b, board.NewStory{Title: "in one", Phase: 1}) // S-2
	addStory(t, b, board.NewStory{Title: "in two", Phase: 2}) // S-3
	setClosed(t, b, 1, true)
	wantNext(t, b, "", "S-3") // phase 2 is current; closed phase 1 is passed over
	setClosed(t, b, 2, true)
	wantNext(t, b, "", "S-1") // nothing current: plain rank
}

func TestNextForARepositoryStaysInThePhaseAndTheRepository(t *testing.T) {
	b, _ := twoRepos(t)
	addWeb(t, b)
	if _, err := b.AddPhase("demo", actor, "One", ""); err != nil {
		t.Fatal(err)
	}
	a := addStory(t, b, board.NewStory{Title: "api in phase", Repo: "api", Phase: 1})
	w0 := addStory(t, b, board.NewStory{Title: "web outside", Repo: "web"})
	w1 := addStory(t, b, board.NewStory{Title: "web in phase", Repo: "web", Phase: 1})
	wantNext(t, b, "", a.ID)
	wantNext(t, b, "web", w1.ID)
	_ = w0
	wantNext(t, b, "api", a.ID)
}

func TestWhereAreWeListsTheCurrentPhasesReadyStoriesFirstThenFillsByRank(t *testing.T) {
	b := twoPhases(t)
	addStory(t, b, board.NewStory{Title: "outside 1"})         // S-1
	addStory(t, b, board.NewStory{Title: "outside 2"})         // S-2
	addStory(t, b, board.NewStory{Title: "blocked", Phase: 1}) // S-3
	addStory(t, b, board.NewStory{Title: "ready", Phase: 1})   // S-4
	depend(t, b, "S-3", "S-1")
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	// the phase's ready story first, then rank order with no repeats
	if got := ids(w.Next); !slices.Equal(got, []string{"S-4", "S-1", "S-2"}) {
		t.Fatalf("next %v", got)
	}
	setClosed(t, b, 1, true)
	w, _ = b.Where("demo")
	if got := ids(w.Next); !slices.Equal(got, []string{"S-1", "S-2", "S-3"}) {
		t.Fatalf("next with the phase closed %v", got)
	}
}
