package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func addStory(t *testing.T, b *board.Board, s board.NewStory) board.Story {
	t.Helper()
	got, err := b.AddStory("demo", actor, s)
	if err != nil {
		t.Fatalf("add story %q: %v", s.Title, err)
	}
	return got
}

// order is the backlog as story IDs, top first.
func order(t *testing.T, b *board.Board) []string {
	t.Helper()
	ss, err := b.Stories("demo")
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, s := range ss {
		ids = append(ids, s.ID)
	}
	return ids
}

func wantOrder(t *testing.T, b *board.Board, want ...string) {
	t.Helper()
	if got := order(t, b); !slices.Equal(got, want) {
		t.Fatalf("backlog %v, want %v", got, want)
	}
}

func TestNewStoriesAreNumberedAndGoToTheBottomOfTheBacklog(t *testing.T) {
	b := newWorkspace(t)
	first := addStory(t, b, board.NewStory{Title: "First", Description: "d", Goal: "g", Plan: "p"})
	addStory(t, b, board.NewStory{Title: "Second"})
	if first.ID != "S-1" || first.Status != "backlog" || first.CreatedBy != actor || first.CreatedAt == "" {
		t.Fatalf("added %+v", first)
	}
	wantOrder(t, b, "S-1", "S-2")
}

func TestAStoryCanBePlacedBeforeOrAfterAnother(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})                                          // S-1
	addStory(t, b, board.NewStory{Title: "B"})                                          // S-2
	addStory(t, b, board.NewStory{Title: "C", Position: board.Position{After: "S-1"}})  // S-3
	addStory(t, b, board.NewStory{Title: "D", Position: board.Position{Before: "S-1"}}) // S-4
	addStory(t, b, board.NewStory{Title: "E", Position: board.Position{After: "S-2"}})  // S-5
	wantOrder(t, b, "S-4", "S-1", "S-3", "S-2", "S-5")
}

func TestAddStoryRefusesBadInput(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	_, err := b.AddStory("demo", actor, board.NewStory{})
	wantKind(t, err, board.Invalid)
	_, err = b.AddStory("demo", actor, board.NewStory{Title: "x", Position: board.Position{Before: "S-1", After: "S-1"}})
	wantKind(t, err, board.Invalid)
	_, err = b.AddStory("demo", actor, board.NewStory{Title: "x", Position: board.Position{After: "S-9"}})
	wantKind(t, err, board.NotFound)
	_, err = b.AddStory("demo", actor, board.NewStory{Title: "x", Phase: 3})
	wantKind(t, err, board.NotFound)
	wantOrder(t, b, "S-1")
}

func TestAStoryCanBelongToAPhase(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	s := addStory(t, b, board.NewStory{Title: "In phase", Phase: 1})
	got, err := b.Story("demo", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != 1 || got.Title != "In phase" {
		t.Fatalf("read %+v", got)
	}
}

func TestReadingAnUnknownStoryIsNotFound(t *testing.T) {
	b := newWorkspace(t)
	_, err := b.Story("demo", "S-1")
	wantKind(t, err, board.NotFound)
}

func TestWorkspacesNumberTheirStoriesSeparately(t *testing.T) {
	b := newWorkspace(t)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "other", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	addStory(t, b, board.NewStory{Title: "demo's"})
	s, err := b.AddStory("other", actor, board.NewStory{Title: "other's"})
	if err != nil || s.ID != "S-1" {
		t.Fatalf("other's first story = %+v, %v", s, err)
	}
	if got := order(t, b); len(got) != 1 {
		t.Fatalf("demo lists %v", got)
	}
}

func TestUpdateStoryChangesOnlyTheFieldsGiven(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "Old", Description: "d", Goal: "g", Plan: "p"})
	s, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Title: ptr("New"), Plan: ptr("p2")})
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "New" || s.Plan != "p2" || s.Description != "d" || s.Goal != "g" {
		t.Fatalf("updated %+v", s)
	}
}

func TestUpdateStoryRefusesAnEmptyTitleAndAnUnknownStory(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	_, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Title: ptr("")})
	wantKind(t, err, board.Invalid)
	_, err = b.UpdateStory("demo", actor, "S-9", board.StoryPatch{Title: ptr("x")})
	wantKind(t, err, board.NotFound)
}

// Agents send markdown: quotes, backslashes, newlines, tabs and non-ASCII
// text must read back exactly as sent.
func TestStoryTextRoundTripsExactly(t *testing.T) {
	b := newWorkspace(t)
	text := "## Plan\n\n1. Run `make gates` \u2014 it's \"fine\"\n\t- path C:\\work\\new\n- caf\u00e9, \u65e5\u672c, \U0001F680\r\nend"
	s := addStory(t, b, board.NewStory{Title: "Quotes ' \" `", Description: text, Plan: text})
	if s.Title != "Quotes ' \" `" || s.Description != text || s.Plan != text {
		t.Fatalf("added text changed:\n%q", s.Description)
	}
	s, err := b.UpdateStory("demo", actor, s.ID, board.StoryPatch{Goal: ptr(text)})
	if err != nil || s.Goal != text {
		t.Fatalf("patched goal = %q, %v", s.Goal, err)
	}
}

// The FalkorDB client inlines values into the query as Go-quoted literals,
// and FalkorDB does not read Go's \x and \u escapes: such text would be stored
// changed, without an error. It is refused instead.
func TestTextThatCannotBeStoredExactlyIsRefused(t *testing.T) {
	b := newWorkspace(t)
	for _, c := range []string{"\x00", "\a", "\x1b[31m", "\x7f", "\u2028", string(rune(0xfeff)), "\xff"} {
		_, err := b.AddStory("demo", actor, board.NewStory{Title: "t" + c + "x"})
		wantKind(t, err, board.Invalid)
	}
	addStory(t, b, board.NewStory{Title: "A"})
	_, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("\u2028")})
	wantKind(t, err, board.Invalid)
}

func TestEditingAStoryRecordsWhoEditedIt(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	s, err := b.UpdateStory("demo", "editor", "S-1", board.StoryPatch{Title: ptr("B")})
	if err != nil {
		t.Fatal(err)
	}
	if s.UpdatedBy != "editor" || s.UpdatedAt == "" {
		t.Fatalf("edit not attributed: %+v", s)
	}
}
