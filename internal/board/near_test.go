package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestNearIsTheStoriesAroundOneStory(t *testing.T) {
	b := neighbourhood(t)
	addPhase(t, b, "Elsewhere")
	addStory(t, b, board.NewStory{Title: "Sibling", Phase: 1})          // S-9
	addStory(t, b, board.NewStory{Title: "In another phase", Phase: 2}) // S-10
	n, err := b.Near("demo", "S-3")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(n.Stories)
	// itself, dependencies, dependent, found, nearby bug, phase sibling; not S-4 or S-10.
	want := []string{"S-1", "S-2", "S-3", "S-5", "S-6", "S-7", "S-8", "S-9"}
	if !slices.Equal(n.Stories, want) {
		t.Fatalf("stories %v, want %v", n.Stories, want)
	}
	if !slices.Equal(n.Ideas, []string{"I-1"}) || n.Phase != 1 {
		t.Fatalf("ideas %v phase %d", n.Ideas, n.Phase)
	}
}

func TestNearIncludesWhereAStoryWasFound(t *testing.T) {
	b := neighbourhood(t)
	n, err := b.Near("demo", "S-5")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(n.Stories, "S-3") || n.Phase != 0 || len(n.Ideas) != 0 {
		t.Fatalf("near %+v", n)
	}
}

func TestNearAMissingStoryIsNotFound(t *testing.T) {
	b := neighbourhood(t)
	_, err := b.Near("demo", "S-99")
	if be, ok := err.(*board.Error); !ok || be.Kind != board.NotFound {
		t.Fatalf("error %v", err)
	}
}

// TestNearHoldsEverythingContextReturns keeps the two neighbourhood queries in
// step: a new kind of link added to Context and missed in Near fails here. The
// phase's other stories are the one thing Near adds on purpose, so they are
// not compared.
func TestNearHoldsEverythingContextReturns(t *testing.T) {
	b := neighbourhood(t)
	for _, id := range []string{"S-1", "S-2", "S-3", "S-5", "S-6", "S-7", "S-8"} {
		c, err := b.Context("demo", id)
		if err != nil {
			t.Fatal(err)
		}
		n, err := b.Near("demo", id)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{c.ID}
		for _, l := range c.DependsOn {
			ids = append(ids, l.ID)
		}
		for _, l := range c.DependedOnBy {
			ids = append(ids, l.ID)
		}
		if c.FoundIn != nil {
			ids = append(ids, c.FoundIn.ID)
		}
		for _, s := range c.Found {
			ids = append(ids, s.ID)
		}
		for _, bug := range c.Bugs {
			ids = append(ids, bug.ID)
		}
		for _, want := range ids {
			if !slices.Contains(n.Stories, want) {
				t.Errorf("%s: Near stories %v leave out %s, which Context returns", id, n.Stories, want)
			}
		}
		if c.Idea != nil && !slices.Contains(n.Ideas, c.Idea.ID) {
			t.Errorf("%s: Near ideas %v leave out %s, which Context returns", id, n.Ideas, c.Idea.ID)
		}
		phase := 0
		if c.Phase != nil {
			phase = c.Phase.Number
		}
		if n.Phase != phase {
			t.Errorf("%s: Near phase %d, Context phase %d", id, n.Phase, phase)
		}
	}
	// The fixture must use every link kind, or the loop proves nothing.
	c, _ := b.Context("demo", "S-3")
	if c.Phase == nil || c.Idea == nil || len(c.DependsOn) != 2 || len(c.DependedOnBy) != 1 || len(c.Found) != 2 || len(c.Bugs) == 0 {
		t.Fatalf("fixture too thin: %+v", c)
	}
	if f, _ := b.Context("demo", "S-5"); f.FoundIn == nil {
		t.Fatalf("fixture has no found-in: %+v", f)
	}
}
