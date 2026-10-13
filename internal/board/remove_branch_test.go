package board_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func branchNames(t *testing.T, b *board.Board) [][2]string {
	t.Helper()
	bs, err := b.Branches("demo")
	if err != nil {
		t.Fatal(err)
	}
	return names(bs)
}

func TestRemovingAnEmptyBranchTakesItOffTheBoard(t *testing.T) {
	b := newReleasingWorkspace(t)
	mustAddBranch(t, b, "staging", "main") // dev and staging both start the line
	if err := b.RemoveBranch("demo", actor, "staging"); err != nil {
		t.Fatal(err)
	}
	if got, want := branchNames(t, b), [][2]string{{"dev", "main"}, {"main", ""}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("branches %v, want %v", got, want)
	}
	// It is gone, so it is unknown now.
	wantKind(t, b.RemoveBranch("demo", actor, "staging"), board.NotFound)
	// The name can be used again.
	mustAddBranch(t, b, "staging", "main")
}

func TestRemovingABranchRefusesWhileAStoryIsMergedIntoItAndNamesTheStories(t *testing.T) {
	b := newReleasingWorkspace(t)
	mustAddBranch(t, b, "staging", "main")
	s := addStory(t, b, board.NewStory{Title: "On staging"})
	walk(t, b, s.ID, board.InProgress, board.InReview)
	setStatus(t, b, s.ID, board.StatusChange{To: board.Merged, Into: "staging", Commit: "c1"})
	err := b.RemoveBranch("demo", actor, "staging")
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), s.ID) {
		t.Fatalf("refusal %q does not name %s", err, s.ID)
	}
	if got := len(branchNames(t, b)); got != 3 {
		t.Fatalf("%d branches, want 3", got)
	}
	// Moving the stories on (the branch merged) is what empties it: a story
	// that reached main is no longer on staging.
	if _, err := b.MergeBranch("demo", actor, "staging", "m1"); err != nil {
		t.Fatal(err)
	}
	if err := b.RemoveBranch("demo", actor, "staging"); err != nil {
		t.Fatal(err)
	}
	// Past log entries keep the removed name.
	if l := readLog(t, b, s.ID); l[len(l)-2].Into != "staging" {
		t.Fatalf("log %+v", l)
	}
}

func TestRemovingABranchRefusesWhileABranchMergesIntoItAndNamesIt(t *testing.T) {
	b := newReleasingWorkspace(t)
	mustAddBranch(t, b, "staging", "main")
	mustAddBranch(t, b, "feature", "staging")
	err := b.RemoveBranch("demo", actor, "staging")
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), "feature") {
		t.Fatalf("refusal %q does not name feature", err)
	}
	// Removing the one that merges in first leaves staging empty.
	if err := b.RemoveBranch("demo", actor, "feature"); err != nil {
		t.Fatal(err)
	}
	if err := b.RemoveBranch("demo", actor, "staging"); err != nil {
		t.Fatal(err)
	}
}

func TestTheReleaseBranchAndTheOnlyStartingBranchAreNeverRemoved(t *testing.T) {
	b := newReleasingWorkspace(t)
	for _, name := range []string{"main", "dev"} {
		err := b.RemoveBranch("demo", actor, name)
		wantKind(t, err, board.Conflict)
		if name == "main" && !strings.Contains(err.Error(), "release") {
			t.Fatalf("refusal %q", err)
		}
		if name == "dev" && !strings.Contains(err.Error(), "first") {
			t.Fatalf("refusal %q", err)
		}
	}
	// With a second starting branch, either may go while empty.
	mustAddBranch(t, b, "staging", "main")
	if err := b.RemoveBranch("demo", actor, "dev"); err != nil {
		t.Fatal(err)
	}
	// staging is now the only starting branch again.
	wantKind(t, b.RemoveBranch("demo", actor, "staging"), board.Conflict)
	if got := len(branchNames(t, b)); got != 2 {
		t.Fatalf("%d branches, want 2", got)
	}
}

func TestRemovingAnUnknownBranchOrFromAnUnknownWorkspaceIsNotFound(t *testing.T) {
	b := newReleasingWorkspace(t)
	wantKind(t, b.RemoveBranch("demo", actor, "nope"), board.NotFound)
	wantKind(t, b.RemoveBranch("nope", actor, "dev"), board.NotFound)
	wantKind(t, b.RemoveBranch("demo", actor, ""), board.Invalid)
}
