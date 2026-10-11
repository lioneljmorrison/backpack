package board_test

import (
	"sync"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// threeStories returns a workspace whose backlog is S-1, S-2, S-3.
func threeStories(t *testing.T) *board.Board {
	t.Helper()
	b := newWorkspace(t)
	for _, title := range []string{"A", "B", "C"} {
		addStory(t, b, board.NewStory{Title: title})
	}
	return b
}

func TestAStoryCanBeMovedBeforeOrAfterAnother(t *testing.T) {
	b := threeStories(t)
	s, err := b.MoveStory("demo", actor, "S-3", board.Position{Before: "S-1"})
	if err != nil {
		t.Fatal(err)
	}
	wantOrder(t, b, "S-3", "S-1", "S-2")
	if s.MovedBy != actor || s.MovedAt == "" {
		t.Fatalf("move not attributed: %+v", s)
	}
	if _, err := b.MoveStory("demo", actor, "S-3", board.Position{After: "S-1"}); err != nil {
		t.Fatal(err)
	}
	wantOrder(t, b, "S-1", "S-3", "S-2")
}

func TestMovingWithNoPositionGoesToTheBottom(t *testing.T) {
	b := threeStories(t)
	if _, err := b.MoveStory("demo", actor, "S-1", board.Position{}); err != nil {
		t.Fatal(err)
	}
	wantOrder(t, b, "S-2", "S-3", "S-1")
}

// Moving a story to where it already is changes nothing.
func TestMovingAStoryNextToItsOwnNeighbourKeepsTheOrder(t *testing.T) {
	b := threeStories(t)
	if _, err := b.MoveStory("demo", actor, "S-2", board.Position{After: "S-1"}); err != nil {
		t.Fatal(err)
	}
	wantOrder(t, b, "S-1", "S-2", "S-3")
}

func TestMoveStoryRefusesBadInput(t *testing.T) {
	b := threeStories(t)
	_, err := b.MoveStory("demo", actor, "S-1", board.Position{After: "S-1"})
	wantKind(t, err, board.Invalid)
	_, err = b.MoveStory("demo", actor, "S-1", board.Position{Before: "S-2", After: "S-3"})
	wantKind(t, err, board.Invalid)
	_, err = b.MoveStory("demo", actor, "S-9", board.Position{})
	wantKind(t, err, board.NotFound)
	_, err = b.MoveStory("demo", actor, "S-1", board.Position{After: "S-9"})
	wantKind(t, err, board.NotFound)
	wantOrder(t, b, "S-1", "S-2", "S-3")
}

// Concurrent writes into the same gap must never share a rank.
func TestConcurrentPlacementsIntoOneGapKeepRanksDistinct(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	addStory(t, b, board.NewStory{Title: "anchor"})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			// A refusal after repeated collisions is allowed; a shared rank is not.
			b.AddStory("demo", actor, board.NewStory{Title: "x", Position: board.Position{After: "S-1"}})
		})
	}
	wg.Wait()
	res, err := db.SelectGraph(prefix+"ws:demo").ROQuery(
		"MATCH (s:Story) WITH s.rank AS r, count(*) AS n WHERE n > 1 RETURN r", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Next() {
		t.Fatalf("two stories share rank %v", res.Record().Values())
	}
}
