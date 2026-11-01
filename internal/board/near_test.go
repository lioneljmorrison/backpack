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
