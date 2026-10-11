package board_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func addIdea(t *testing.T, b *board.Board, i board.NewIdea) board.Idea {
	t.Helper()
	got, err := b.AddIdea("demo", actor, i)
	if err != nil {
		t.Fatalf("add idea %q: %v", i.Title, err)
	}
	return got
}

// ideaOrder is the ideas list as IDs, top first.
func ideaOrder(t *testing.T, b *board.Board) []string {
	t.Helper()
	is, err := b.Ideas("demo")
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, i := range is {
		out = append(out, i.ID)
	}
	return out
}

func wantIdeaOrder(t *testing.T, b *board.Board, want ...string) {
	t.Helper()
	if got := ideaOrder(t, b); !slices.Equal(got, want) {
		t.Fatalf("ideas %v, want %v", got, want)
	}
}

func idea(t *testing.T, b *board.Board, id string) board.Idea {
	t.Helper()
	i, err := b.Idea("demo", id)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func TestIdeasAreNumberedAndPlacedInTheirOwnOrder(t *testing.T) {
	b := newWorkspace(t)
	first := addIdea(t, b, board.NewIdea{Title: "A", Context: "rough"})
	addIdea(t, b, board.NewIdea{Title: "B"})
	addIdea(t, b, board.NewIdea{Title: "C", Position: board.Position{Before: "I-1"}})
	addIdea(t, b, board.NewIdea{Title: "D", Position: board.Position{After: "I-1"}})
	if first.ID != "I-1" || first.State != "open" || first.Context != "rough" || first.CreatedBy != actor {
		t.Fatalf("added %+v", first)
	}
	wantIdeaOrder(t, b, "I-3", "I-1", "I-4", "I-2")
}

func TestIdeasAndStoriesNeverShareAnOrder(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "Story"})
	addIdea(t, b, board.NewIdea{Title: "Idea"})
	wantOrder(t, b, "S-1")
	wantIdeaOrder(t, b, "I-1")
	if _, err := b.AddIdea("demo", actor, board.NewIdea{Title: "X", Position: board.Position{After: "S-1"}}); err == nil {
		t.Fatal("an idea was placed next to a story")
	}
	if next, _ := b.Next("demo"); next == nil || next.ID != "S-1" {
		t.Fatalf("next %+v", next)
	}
}

func TestAddIdeaRefusesBadInput(t *testing.T) {
	b := newWorkspace(t)
	for _, i := range []board.NewIdea{
		{},
		{Title: "A", Position: board.Position{Before: "I-1", After: "I-2"}},
		{Title: "bad\x00"},
	} {
		_, err := b.AddIdea("demo", actor, i)
		wantKind(t, err, board.Invalid)
	}
	_, err := b.AddIdea("demo", actor, board.NewIdea{Title: "A", Position: board.Position{After: "I-9"}})
	wantKind(t, err, board.NotFound)
	wantIdeaOrder(t, b)
}

func TestAnIdeaIsOpenFromTheStartAndNeedsNoContext(t *testing.T) {
	b := newWorkspace(t)
	i := addIdea(t, b, board.NewIdea{Title: "Bare"})
	if i.State != "open" || i.Context != "" {
		t.Fatalf("idea %+v", i)
	}
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Text(), "Ideas") || strings.Contains(w.Text(), "I-1") {
		t.Fatalf("where mentions ideas:\n%s", w.Text())
	}
}

func TestAnIdeaCanBeMovedAndEditedWhileItIsOpen(t *testing.T) {
	b := newWorkspace(t)
	addIdea(t, b, board.NewIdea{Title: "A"})
	addIdea(t, b, board.NewIdea{Title: "B"})
	i, err := b.MoveIdea("demo", actor, "I-2", board.Position{Before: "I-1"})
	if err != nil {
		t.Fatal(err)
	}
	if i.MovedBy != actor {
		t.Fatalf("moved %+v", i)
	}
	wantIdeaOrder(t, b, "I-2", "I-1")
	i, err = b.UpdateIdea("demo", actor, "I-1", board.IdeaPatch{Context: ptr("grown")})
	if err != nil {
		t.Fatal(err)
	}
	if i.Title != "A" || i.Context != "grown" || i.UpdatedBy != actor {
		t.Fatalf("updated %+v", i)
	}
	_, err = b.UpdateIdea("demo", actor, "I-1", board.IdeaPatch{Title: ptr("")})
	wantKind(t, err, board.Invalid)
	_, err = b.MoveIdea("demo", actor, "I-1", board.Position{Before: "I-1"})
	wantKind(t, err, board.Invalid)
}

func TestAnIdeaBecomesStoriesAndAPhaseAndIsClosed(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	addIdea(t, b, board.NewIdea{Title: "Idea"})
	i, err := b.RefineIdea("demo", actor, "I-1", board.Refinement{Stories: []string{"S-2", "S-1", "S-2"}, Phase: 1})
	if err != nil {
		t.Fatal(err)
	}
	if i.State != "closed" || !slices.Equal(i.BecameStories, []string{"S-1", "S-2"}) || i.BecamePhase != 1 ||
		i.ClosedBy != actor || i.ClosedAt == "" {
		t.Fatalf("refined %+v", i)
	}
	wantIdeaOrder(t, b)
	if got := idea(t, b, "I-1"); got.State != "closed" {
		t.Fatalf("read back %+v", got)
	}
}

func TestAnIdeaCanBecomeAPhaseAlone(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addIdea(t, b, board.NewIdea{Title: "Idea"})
	i, err := b.RefineIdea("demo", actor, "I-1", board.Refinement{Phase: 1})
	if err != nil {
		t.Fatal(err)
	}
	if i.State != "closed" || i.BecamePhase != 1 || len(i.BecameStories) != 0 {
		t.Fatalf("refined %+v", i)
	}
}

func TestRefiningIsRefusedAndWritesNothingWhenATargetIsWrong(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "Gone"})
	setStatus(t, b, "S-2", board.StatusChange{To: "closed", Reason: "not needed"})
	addIdea(t, b, board.NewIdea{Title: "Idea"})
	for _, c := range []struct {
		id   string
		to   board.Refinement
		want board.Kind
	}{
		{"I-1", board.Refinement{}, board.Invalid},
		{"I-1", board.Refinement{Stories: []string{"S-1", "S-9"}}, board.NotFound},
		{"I-1", board.Refinement{Stories: []string{"S-1"}, Phase: 4}, board.NotFound},
		{"I-1", board.Refinement{Stories: []string{"S-1", "S-2"}}, board.Conflict},
		{"I-9", board.Refinement{Stories: []string{"S-1"}}, board.NotFound},
	} {
		_, err := b.RefineIdea("demo", actor, c.id, c.to)
		wantKind(t, err, c.want)
	}
	if i := idea(t, b, "I-1"); i.State != "open" || len(i.BecameStories) != 0 {
		t.Fatalf("idea %+v", i)
	}
}

func TestAnIdeaCanBeDroppedWithAReasonAndIsThenFinal(t *testing.T) {
	b := newWorkspace(t)
	addIdea(t, b, board.NewIdea{Title: "A"})
	addIdea(t, b, board.NewIdea{Title: "B"})
	_, err := b.DropIdea("demo", actor, "I-1", "")
	wantKind(t, err, board.Invalid)
	i, err := b.DropIdea("demo", actor, "I-1", "out of scope")
	if err != nil {
		t.Fatal(err)
	}
	if i.State != "closed" || i.DropReason != "out of scope" || len(i.BecameStories) != 0 {
		t.Fatalf("dropped %+v", i)
	}
	if _, err := b.DropIdea("demo", actor, "I-2", "no"); err != nil {
		t.Fatalf("a second open idea can be dropped: %v", err)
	}
	wantIdeaOrder(t, b)
	_, err = b.DropIdea("demo", actor, "I-1", "again")
	wantKind(t, err, board.Conflict)
	_, err = b.UpdateIdea("demo", actor, "I-1", board.IdeaPatch{Title: ptr("X")})
	wantKind(t, err, board.Conflict)
	_, err = b.MoveIdea("demo", actor, "I-1", board.Position{})
	wantKind(t, err, board.Conflict)
}
