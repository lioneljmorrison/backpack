package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// recorder returns the changes the board reports from now on.
func recorder(b *board.Board) *[]board.Change {
	var got []board.Change
	b.OnChange(func(c board.Change) { got = append(got, c) })
	return &got
}

func TestEverySearchableWriteReportsWhatItChanged(t *testing.T) {
	story := func(id string) board.Change { return board.Change{Workspace: "demo", Kind: board.KindStory, ID: id} }
	idea := func(id string) board.Change { return board.Change{Workspace: "demo", Kind: board.KindIdea, ID: id} }
	phase := board.Change{Workspace: "demo", Kind: board.KindPhase, ID: "1"}
	title := "Renamed"

	cases := []struct {
		name string
		do   func(b *board.Board) error
		want board.Change
	}{
		{"add story", func(b *board.Board) error {
			_, err := b.AddStory("demo", actor, board.NewStory{Title: "New"})
			return err
		}, story("S-2")},
		{"update story", func(b *board.Board) error {
			_, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Title: &title})
			return err
		}, story("S-1")},
		{"set status", func(b *board.Board) error {
			_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: board.InProgress})
			return err
		}, story("S-1")},
		{"add log entry", func(b *board.Board) error {
			_, err := b.AddLogEntry("demo", actor, "S-1", board.NewLogEntry{Kind: "note", Text: "noted"})
			return err
		}, story("S-1")},
		{"add idea", func(b *board.Board) error {
			_, err := b.AddIdea("demo", actor, board.NewIdea{Title: "Another"})
			return err
		}, idea("I-2")},
		{"update idea", func(b *board.Board) error {
			_, err := b.UpdateIdea("demo", actor, "I-1", board.IdeaPatch{Title: &title})
			return err
		}, idea("I-1")},
		{"drop idea", func(b *board.Board) error {
			_, err := b.DropIdea("demo", actor, "I-1", "not needed")
			return err
		}, idea("I-1")},
		{"refine idea", func(b *board.Board) error {
			_, err := b.RefineIdea("demo", actor, "I-1", board.Refinement{Stories: []string{"S-1"}})
			return err
		}, idea("I-1")},
		{"add phase", func(b *board.Board) error {
			_, err := b.AddPhase("demo", actor, "Second", "")
			return err
		}, board.Change{Workspace: "demo", Kind: board.KindPhase, ID: "2"}},
		{"update phase", func(b *board.Board) error {
			_, err := b.UpdatePhase("demo", actor, 1, board.PhasePatch{Title: &title})
			return err
		}, phase},
		{"add goal", func(b *board.Board) error {
			_, err := b.AddGoal("demo", actor, 1, "Ship it")
			return err
		}, phase},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newWorkspace(t)
			addStory(t, b, board.NewStory{Title: "First"})
			addIdea(t, b, board.NewIdea{Title: "Rough"})
			if _, err := b.AddPhase("demo", actor, "First", ""); err != nil {
				t.Fatal(err)
			}
			got := recorder(b)
			if err := c.do(b); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(*got, []board.Change{c.want}) {
				t.Fatalf("reported %+v, want %+v", *got, c.want)
			}
		})
	}
}

func TestApprovingASuggestedIdeaReportsIt(t *testing.T) {
	b := newWorkspace(t)
	suggestIdea(t, b, board.NewIdea{Title: "Rough"})
	got := recorder(b)
	if _, err := b.ApproveIdea("demo", actor, "I-1"); err != nil {
		t.Fatal(err)
	}
	want := board.Change{Workspace: "demo", Kind: board.KindIdea, ID: "I-1"}
	if !slices.Equal(*got, []board.Change{want}) {
		t.Fatalf("reported %+v, want %+v", *got, want)
	}
}

func TestCreatingAWorkspaceReportsIt(t *testing.T) {
	b := newBoard(t)
	got := recorder(b)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "fresh", Name: "Fresh"}); err != nil {
		t.Fatal(err)
	}
	want := board.Change{Workspace: "fresh", Kind: board.KindWorkspace}
	if !slices.Equal(*got, []board.Change{want}) {
		t.Fatalf("reported %+v, want %+v", *got, want)
	}
}

func TestARefusedWriteReportsNothing(t *testing.T) {
	b := newWorkspace(t)
	got := recorder(b)
	if _, err := b.SetStatus("demo", actor, "S-9", board.StatusChange{To: board.InProgress}); err == nil {
		t.Fatal("status change on a missing story succeeded")
	}
	if len(*got) != 0 {
		t.Fatalf("reported %+v", *got)
	}
}

func TestMovesAndDependenciesReportNothing(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "First"})
	addStory(t, b, board.NewStory{Title: "Second"})
	addStory(t, b, board.NewStory{Title: "Third"})
	got := recorder(b)
	if _, err := b.AddDependency("demo", actor, "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.MoveStory("demo", actor, "S-3", board.Position{Before: "S-1"}); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 0 {
		t.Fatalf("reported %+v", *got)
	}
}
