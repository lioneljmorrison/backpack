package search_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/search"
)

// nearBoard has an invented phase with two stories and an idea, a dependency
// outside the phase, and unrelated stories. All of them mention "invoices".
func nearBoard(t *testing.T) (*board.Board, *search.Index) {
	t.Helper()
	b, x := newIndexed(t)
	if _, err := b.AddPhase("demo", actor, "Plumbing", "pipes and valves"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddPhase("demo", actor, "Other", "elsewhere"); err != nil {
		t.Fatal(err)
	}
	addStory(t, b, "demo", board.NewStory{Title: "Invoices in the work", Phase: 1}) // S-1
	addStory(t, b, "demo", board.NewStory{Title: "Invoices sibling", Phase: 1})     // S-2
	addStory(t, b, "demo", board.NewStory{Title: "Invoices dependent"})             // S-3
	addStory(t, b, "demo", board.NewStory{Title: "Invoices far away", Phase: 2})    // S-4
	addStory(t, b, "demo", board.NewStory{Title: "Gardening notes about tomatoes"}) // S-5
	if _, err := b.AddDependency("demo", actor, "S-3", "S-1"); err != nil {
		t.Fatal(err)
	}
	return b, x
}

func near(t *testing.T, x *search.Index, q, story string) ([]string, string) {
	t.Helper()
	rs, scope, err := x.SearchNear(ctx, "demo", q, story)
	if err != nil {
		t.Fatalf("search %q near %s: %v", q, story, err)
	}
	got := ids(rs)
	slices.Sort(got)
	return got, scope
}

func TestSearchNearAStoryLeavesOutMatchesOutsideItsNeighbourhood(t *testing.T) {
	_, x := nearBoard(t)
	got, scope := near(t, x, "invoices", "S-1")
	if want := []string{"S-1", "S-2", "S-3"}; !slices.Equal(got, want) || scope != "story" {
		t.Fatalf("found %v in scope %q, want %v in story", got, scope, want)
	}
}

func TestSearchNearAStoryFindsItsIdeaAndPhase(t *testing.T) {
	b, x := nearBoard(t)
	if _, err := b.AddIdea("demo", actor, board.NewIdea{Title: "Wording ideas", Context: "billing phase wording"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.RefineIdea("demo", actor, "I-1", board.Refinement{Stories: []string{"S-1"}}); err != nil {
		t.Fatal(err)
	}
	got, scope := near(t, x, "pipes", "S-1")
	if !slices.Contains(got, "1") || scope != "story" {
		t.Fatalf("found %v in %q", got, scope)
	}
	got, _ = near(t, x, "billing phase wording", "S-1")
	if !slices.Contains(got, "I-1") {
		t.Fatalf("idea not found: %v", got)
	}
}

func TestSearchNearAStoryWidensWhenNothingIsInside(t *testing.T) {
	_, x := nearBoard(t)
	got, scope := near(t, x, "tomatoes", "S-1")
	if want := []string{"S-5"}; !slices.Equal(got, want) || scope != "workspace" {
		t.Fatalf("found %v in %q, want %v in workspace", got, scope, want)
	}
}

func TestSearchNearAStoryWithNothingAnywhereReportsWorkspace(t *testing.T) {
	_, x := nearBoard(t)
	got, scope := near(t, x, "zeppelin", "S-1")
	if len(got) != 0 || scope != "workspace" {
		t.Fatalf("found %v in %q", got, scope)
	}
}

func TestSearchNearAnUnknownStoryIsNotFound(t *testing.T) {
	_, x := nearBoard(t)
	_, _, err := x.SearchNear(ctx, "demo", "invoices", "S-99")
	var be *board.Error
	if !errors.As(err, &be) || be.Kind != board.NotFound {
		t.Fatalf("error %v", err)
	}
}

func TestSearchNearNeedsAQuery(t *testing.T) {
	_, x := nearBoard(t)
	_, _, err := x.SearchNear(ctx, "demo", " ", "S-1")
	var be *board.Error
	if !errors.As(err, &be) || be.Kind != board.Invalid {
		t.Fatalf("error %v", err)
	}
}

func TestSearchWithoutAStoryStaysWorkspaceWide(t *testing.T) {
	_, x := nearBoard(t)
	got := ids(find(t, x, "demo", "invoices"))
	slices.Sort(got)
	if want := []string{"S-1", "S-2", "S-3", "S-4"}; !slices.Equal(got, want) {
		t.Fatalf("found %v, want %v", got, want)
	}
}
