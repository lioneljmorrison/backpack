package search_test

import (
	"bytes"
	"context"
	"errors"
	"log"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/search"
	"github.com/lioneljmorrison/backpack/internal/search/typesensetest"
)

const actor = "tester"

var ctx = context.Background()

// target is the Typesense a test's index writes to.
type target struct{ url, key, prefix string }

// newTarget returns an empty board, an index of it not yet connected to
// the board's changes, and where that index lives.
func newTarget(t *testing.T) (*board.Board, *search.Index, target) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	url, key, cprefix := typesensetest.Prefix(t)
	return b, search.New(url, key, cprefix, b), target{url, key, cprefix}
}

func newBoard(t *testing.T) (*board.Board, *search.Index) {
	t.Helper()
	b, x, _ := newTarget(t)
	return b, x
}

// quiet sends the log to buf until the test ends.
func quiet(t *testing.T, buf *bytes.Buffer) {
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
}

// newIndexed returns a board whose every change is synced to its index,
// with one workspace, "demo".
func newIndexed(t *testing.T) (*board.Board, *search.Index) {
	t.Helper()
	b, x := newBoard(t)
	b.OnChange(func(c board.Change) {
		if err := x.Sync(ctx, c); err != nil {
			t.Errorf("sync %+v: %v", c, err)
		}
	})
	workspace(t, b, "demo")
	return b, x
}

func workspace(t *testing.T, b *board.Board, slug string) {
	t.Helper()
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: slug, Name: slug}); err != nil {
		t.Fatal(err)
	}
}

func addStory(t *testing.T, b *board.Board, ws string, s board.NewStory) board.Story {
	t.Helper()
	got, err := b.AddStory(ws, actor, s)
	if err != nil {
		t.Fatalf("add story %q: %v", s.Title, err)
	}
	return got
}

// find returns what a search answers with, best first.
func find(t *testing.T, x *search.Index, ws, q string) []search.Result {
	t.Helper()
	got, err := x.Search(ctx, ws, q)
	if err != nil {
		t.Fatalf("search %q: %v", q, err)
	}
	return got
}

func ids(rs []search.Result) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func wantIDs(t *testing.T, x *search.Index, ws, q string, want ...string) {
	t.Helper()
	if got := ids(find(t, x, ws, q)); !slices.Equal(got, want) {
		t.Fatalf("search %q found %v, want %v", q, got, want)
	}
}

func TestAWordOnlyInALogEntryFindsItsStory(t *testing.T) {
	b, x := newIndexed(t)
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices", Description: "Monthly statements"})
	addStory(t, b, "demo", board.NewStory{Title: "Team calendar"})
	if _, err := b.AddLogEntry("demo", actor, "S-1", board.NewLogEntry{Kind: "decision",
		Text: "Render with wkhtmltopdf", Reason: "the other library mangled tables"}); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, x, "demo", "wkhtmltopdf", "S-1")
}

func TestAStoryIsFoundByMeaning(t *testing.T) {
	b, x := newIndexed(t)
	addStory(t, b, "demo", board.NewStory{Title: "Sign-in with passkeys",
		Description: "People authenticate with a passkey instead of a password"})
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices as PDF"})
	wantIDs(t, x, "demo", "login", "S-1")
}

func TestUnrelatedItemsAreLeftOut(t *testing.T) {
	b, x := newIndexed(t)
	addStory(t, b, "demo", board.NewStory{Title: "Sign-in with passkeys"})
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices as PDF"})
	wantIDs(t, x, "demo", "kubernetes cluster")
}

func TestAResultCarriesKindTitleAndState(t *testing.T) {
	b, x := newIndexed(t)
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices as PDF"})
	got := find(t, x, "demo", "invoices")
	want := []search.Result{{Kind: "story", ID: "S-1", Title: "Export invoices as PDF", State: "backlog"}}
	if !slices.Equal(got, want) {
		t.Fatalf("found %+v, want %+v", got, want)
	}
}

func TestClosedStoriesAndDroppedIdeasAreFoundWithTheirState(t *testing.T) {
	b, x := newIndexed(t)
	addStory(t, b, "demo", board.NewStory{Title: "Dark theme for the dashboard"})
	if _, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: board.Closed, Reason: "not wanted"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddIdea("demo", actor, board.NewIdea{Title: "Dark theme for emails"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.DropIdea("demo", actor, "I-1", "no one reads them"); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range find(t, x, "demo", "dark theme") {
		got[r.ID] = r.State
	}
	if got["S-1"] != board.Closed || got["I-1"] != "closed" {
		t.Fatalf("found %v, want S-1 closed and I-1 closed", got)
	}
}

func TestIdeasAndPhasesAreFound(t *testing.T) {
	b, x := newIndexed(t)
	if _, err := b.AddIdea("demo", actor, board.NewIdea{Title: "Recurring reminders",
		Context: "Nudge people before deadlines"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddPhase("demo", actor, "Billing", "Charging customers"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddGoal("demo", actor, 1, "Receipts reach every customer"); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, x, "demo", "deadlines", "I-1")
	got := find(t, x, "demo", "receipts")
	if len(got) != 1 || got[0].Kind != "phase" || got[0].ID != "1" || got[0].Title != "Billing" {
		t.Fatalf("found %+v, want phase 1, Billing", got)
	}
}

func TestAnotherWorkspacesItemsNeverAppear(t *testing.T) {
	b, x := newIndexed(t)
	workspace(t, b, "other")
	addStory(t, b, "other", board.NewStory{Title: "Export invoices as PDF"})
	wantIDs(t, x, "demo", "invoices")
}

func TestAnEditIsSearchableStraightAway(t *testing.T) {
	b, x := newIndexed(t)
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices"})
	title := "Export receipts"
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Title: &title}); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, x, "demo", "receipts", "S-1")
}

func TestAStatusChangeUpdatesTheState(t *testing.T) {
	b, x := newIndexed(t)
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices"})
	if _, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: board.InProgress}); err != nil {
		t.Fatal(err)
	}
	if got := find(t, x, "demo", "invoices"); len(got) != 1 || got[0].State != board.InProgress {
		t.Fatalf("found %+v, want S-1 in progress", got)
	}
}

func TestAnEmptyQueryIsRefused(t *testing.T) {
	_, x := newIndexed(t)
	_, err := x.Search(ctx, "demo", " ")
	var be *board.Error
	if !errors.As(err, &be) || be.Kind != board.Invalid {
		t.Fatalf("error = %v, want invalid", err)
	}
}

func TestSearchingAnUnknownWorkspaceIsNotFound(t *testing.T) {
	_, x := newIndexed(t)
	_, err := x.Search(ctx, "nowhere", "invoices")
	var be *board.Error
	if !errors.As(err, &be) || be.Kind != board.NotFound {
		t.Fatalf("error = %v, want not found", err)
	}
}

// unreachable is an index of the board on a Typesense that isn't there.
func unreachable(b *board.Board) *search.Index {
	return search.New("http://127.0.0.1:1", "none", "nothing_", b)
}

func TestSearchWithTypesenseUnreachableIsUnavailable(t *testing.T) {
	b, _ := newIndexed(t)
	if _, err := unreachable(b).Search(ctx, "demo", "invoices"); !errors.Is(err, search.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestABoardWriteSucceedsWithTypesenseUnreachableAndLogsAReindexNeeded(t *testing.T) {
	b, _ := newBoard(t)
	workspace(t, b, "demo")
	b.OnChange(unreachable(b).Report)
	var logged bytes.Buffer
	quiet(t, &logged)

	addStory(t, b, "demo", board.NewStory{Title: "Export invoices"})
	if !strings.Contains(logged.String(), "demo S-1") || !strings.Contains(logged.String(), "reindex needed") {
		t.Fatalf("logged %q", logged.String())
	}
}

func TestRebuildIndexesABoardIntoAnEmptyStore(t *testing.T) {
	b, x := newBoard(t)
	workspace(t, b, "demo")
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices"})
	if err := x.RebuildAll(ctx); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, x, "demo", "invoices", "S-1")
}

func TestRebuildReplacesAnIndexInAnOlderShape(t *testing.T) {
	b, x, ts := newTarget(t)
	workspace(t, b, "demo")
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices"})
	if code := typesensetest.Do(ts.url, ts.key, http.MethodPost, "/collections",
		`{"name":"`+ts.prefix+`demo","fields":[{"name":"words","type":"string"}]}`); code != http.StatusCreated {
		t.Fatalf("create old collection: %d", code)
	}
	if err := x.Rebuild(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, x, "demo", "invoices", "S-1")
}

func TestAChangeMissedWhileUnreachableIsFoundAfterARebuild(t *testing.T) {
	b, x := newIndexed(t)
	b.OnChange(unreachable(b).Report)
	quiet(t, &bytes.Buffer{})
	addStory(t, b, "demo", board.NewStory{Title: "Export invoices"})
	wantIDs(t, x, "demo", "invoices")

	if err := x.Rebuild(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	wantIDs(t, x, "demo", "invoices", "S-1")
}

func TestWarmLoadsTheModelAndLeavesNoCollectionBehind(t *testing.T) {
	_, x, ts := newTarget(t)
	if err := x.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	if code := typesensetest.Do(ts.url, ts.key, http.MethodGet, "/collections/"+ts.prefix+"_warm", ""); code != http.StatusNotFound {
		t.Fatalf("warm collection answers %d, want 404", code)
	}
}

func TestWarmWithTypesenseUnreachableIsUnavailable(t *testing.T) {
	b, _ := newBoard(t)
	if err := unreachable(b).Warm(ctx); !errors.Is(err, search.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

// A branch merge notifies the index of every story it moved (a failed sync
// fails the test), and search still finds each in its state: merged for the
// ones moved, in review for the one left behind.
func TestAfterABranchMergeSearchFindsEveryMovedStoryAndTheOneLeftBehind(t *testing.T) {
	b, x := newBoard(t)
	b.OnChange(func(c board.Change) {
		if err := x.Sync(ctx, c); err != nil {
			t.Errorf("sync %+v: %v", c, err)
		}
	})
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Export invoices", "Export receipts", "Export statements", "Export refunds"} {
		addStory(t, b, "demo", board.NewStory{Title: title})
	}
	for i, id := range []string{"S-1", "S-2", "S-3", "S-4"} {
		steps := []board.StatusChange{{To: board.InProgress}, {To: board.InReview}}
		if i < 3 {
			steps = append(steps, board.StatusChange{To: board.Merged, Commit: "c1"})
		}
		for _, c := range steps {
			if _, err := b.SetStatus("demo", actor, id, c); err != nil {
				t.Fatal(err)
			}
		}
	}
	moved, err := b.MergeBranch("demo", actor, "dev", "x9")
	if err != nil || len(moved) != 3 {
		t.Fatalf("moved %+v, %v", moved, err)
	}
	got := map[string]string{}
	for _, r := range find(t, x, "demo", "export") {
		got[r.ID] = r.State
	}
	want := map[string]string{"S-1": "merged", "S-2": "merged", "S-3": "merged", "S-4": "in_review"}
	if !maps.Equal(got, want) {
		t.Fatalf("search found %v, want %v", got, want)
	}
}
