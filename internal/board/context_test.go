package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func decide(t *testing.T, b *board.Board, id, text string) board.LogEntry {
	t.Helper()
	e, err := b.AddLogEntry("demo", actor, id, board.NewLogEntry{Kind: "decision", Text: text, Reason: "because " + text})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// neighbourhood builds an invented neighbourhood around S-3:
//
//	phase 1 (goals G-1 met, G-2 not) contains S-3; idea I-1 became S-3
//	S-1 (merged) and S-2 (backlog) are S-3's dependencies; S-6 depends on S-3
//	S-3 turned up S-5 (an open bug) and S-8 (a merged bug); S-4 is unrelated
//	S-7 is an open bug found in S-1
func neighbourhood(t *testing.T) *board.Board {
	t.Helper()
	b := newWorkspace(t)
	phase := addPhase(t, b, "Foundations")
	for _, g := range []string{"it runs", "it ships"} {
		if _, err := b.AddGoal("demo", actor, phase.Number, g); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.SetGoalMet("demo", actor, "G-1", true); err != nil {
		t.Fatal(err)
	}
	addStory(t, b, board.NewStory{Title: "First"})
	addStory(t, b, board.NewStory{Title: "Second"})
	addStory(t, b, board.NewStory{Title: "Target", Phase: 1, Goal: "the goal"})
	addStory(t, b, board.NewStory{Title: "Finder"})
	addStory(t, b, board.NewStory{Title: "Found bug", Kind: "bug", FoundIn: "S-3"})
	addStory(t, b, board.NewStory{Title: "After"})
	addStory(t, b, board.NewStory{Title: "Dependency bug", Kind: "bug", FoundIn: "S-1"})
	addStory(t, b, board.NewStory{Title: "Old bug", Kind: "bug", FoundIn: "S-3"})
	depend(t, b, "S-3", "S-1")
	depend(t, b, "S-3", "S-2")
	depend(t, b, "S-6", "S-3")
	if _, err := b.AddIdea("demo", actor, board.NewIdea{Title: "Rough"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.RefineIdea("demo", actor, "I-1", board.Refinement{Stories: []string{"S-3"}}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one", "two", "three", "four"} {
		decide(t, b, "S-1", text)
	}
	wrong := decide(t, b, "S-2", "wrong")
	decide(t, b, "S-2", "right")
	if _, err := b.AddLogEntry("demo", actor, "S-2", board.NewLogEntry{Kind: "correction", Text: "not so", Corrects: wrong.ID}); err != nil {
		t.Fatal(err)
	}
	walk(t, b, "S-1", board.InProgress, board.InReview)
	setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Commit: "c1"})
	walk(t, b, "S-8", board.InProgress, board.InReview)
	setStatus(t, b, "S-8", board.StatusChange{To: board.Merged, Commit: "c8"})
	return b
}

func TestContextSummarisesTheStoryNeighbourhood(t *testing.T) {
	b := neighbourhood(t)
	c, err := b.Context("demo", "S-3")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "S-3" || c.Title != "Target" || c.Status != board.Backlog {
		t.Fatalf("story %+v", c)
	}
	if c.Phase == nil || c.Phase.Number != 1 || c.Phase.Title != "Foundations" || len(c.Phase.Goals) != 2 ||
		c.Phase.Goals[0] != (board.ContextGoal{ID: "G-1", Text: "it runs", Met: true}) || c.Phase.Goals[1].Met {
		t.Fatalf("phase %+v", c.Phase)
	}
	if c.Idea == nil || c.Idea.ID != "I-1" || c.Idea.Title != "Rough" {
		t.Fatalf("idea %+v", c.Idea)
	}
	if len(c.DependsOn) != 2 || c.DependsOn[0].ID != "S-1" || c.DependsOn[1].ID != "S-2" {
		t.Fatalf("depends on %+v", c.DependsOn)
	}
	// The merged dependency is here although the story read no longer lists it.
	first := c.DependsOn[0]
	if first.Status != board.Merged || first.Title != "First" {
		t.Fatalf("merged dependency %+v", first)
	}
	if got := decisionTexts(first.Decisions); !slices.Equal(got, []string{"two", "three", "four"}) {
		t.Fatalf("last three decisions of S-1: %v", got)
	}
	second := c.DependsOn[1]
	if len(second.Decisions) != 2 || second.Decisions[0].Text != "wrong" || !second.Decisions[0].Corrected ||
		second.Decisions[1].Text != "right" || second.Decisions[1].Corrected {
		t.Fatalf("decisions of S-2 %+v", second.Decisions)
	}
	if second.Decisions[1].Reason != "because right" {
		t.Fatalf("reason %+v", second.Decisions[1])
	}
	if len(c.DependedOnBy) != 1 || c.DependedOnBy[0].ID != "S-6" || c.DependedOnBy[0].Status != board.Backlog {
		t.Fatalf("depended on by %+v", c.DependedOnBy)
	}
	if c.FoundIn != nil {
		t.Fatalf("found in %+v", c.FoundIn)
	}
	if got := storyIDs(c.Found); !slices.Equal(got, []string{"S-5", "S-8"}) {
		t.Fatalf("found %v", got)
	}
	// Open bugs found in the story or a direct dependency: S-5 and S-7
	// (S-8 is merged).
	var bugs []string
	for _, bug := range c.Bugs {
		bugs = append(bugs, bug.ID+" in "+bug.FoundIn)
	}
	if !slices.Equal(bugs, []string{"S-5 in S-3", "S-7 in S-1"}) {
		t.Fatalf("bugs %v", bugs)
	}
}

func decisionTexts(ds []board.ContextDecision) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Text)
	}
	return out
}

func storyIDs(ss []board.ContextStory) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return out
}

func TestContextOfAFoundStoryNamesWhereItWasFound(t *testing.T) {
	b := neighbourhood(t)
	c, err := b.Context("demo", "S-5")
	if err != nil {
		t.Fatal(err)
	}
	if c.FoundIn == nil || c.FoundIn.ID != "S-3" || c.FoundIn.Title != "Target" || c.FoundIn.Status != board.Backlog {
		t.Fatalf("found in %+v", c.FoundIn)
	}
	if c.Phase != nil || c.Idea != nil || len(c.DependsOn) != 0 || len(c.DependedOnBy) != 0 || len(c.Found) != 0 || len(c.Bugs) != 0 {
		t.Fatalf("expected an otherwise bare context, got %+v", c)
	}
}

func TestContextListsAtMostFiveOpenBugs(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "Host"})
	for range 7 {
		addStory(t, b, board.NewStory{Title: "Bug", Kind: "bug", FoundIn: "S-1"})
	}
	c, err := b.Context("demo", "S-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Bugs) != 5 || c.Bugs[0].ID != "S-2" || c.Bugs[4].ID != "S-6" {
		t.Fatalf("bugs %+v", c.Bugs)
	}
	if len(c.Found) != 7 {
		t.Fatalf("found %v", storyIDs(c.Found))
	}
}

func TestContextOfAMissingStoryIsNotFound(t *testing.T) {
	b := newWorkspace(t)
	_, err := b.Context("demo", "S-9")
	if be, ok := err.(*board.Error); !ok || be.Kind != board.NotFound {
		t.Fatalf("err %v, want not found", err)
	}
}
