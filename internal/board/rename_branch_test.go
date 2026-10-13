package board_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func rename(b *board.Board, branch, to string) (board.Branch, error) {
	return b.UpdateBranch("demo", actor, branch, board.BranchPatch{Name: &to})
}

func TestRenamingABranchKeepsEveryStoryWhereItWasAndRenamesWhatReferToIt(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	// S-1 has gone on to main; S-2 and S-3 are still in dev.
	setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Into: "main", Commit: "c9"})
	logBefore := readLog(t, b, "S-2")

	got, err := rename(b, "dev", "develop")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "develop" || got.Into != "main" || got.Release {
		t.Fatalf("answered %+v", got)
	}
	bs, err := b.Branches("demo")
	if err != nil {
		t.Fatal(err)
	}
	if g := names(bs); len(g) != 2 || g[0] != [2]string{"develop", "main"} || g[1] != [2]string{"main", ""} {
		t.Fatalf("branches %v", g)
	}
	for id, into := range map[string]string{"S-1": "main", "S-2": "develop", "S-3": "develop"} {
		if s := wantStatus(t, b, id, board.Merged); s.Into != into {
			t.Fatalf("%s into %q, want %q", id, s.Into, into)
		}
	}
	if s := wantStatus(t, b, "S-4", board.InReview); s.Into != "" {
		t.Fatalf("the story in review changed: %+v", s)
	}
	// Logs are never edited: S-2 keeps "dev" and gains no entry.
	logAfter := readLog(t, b, "S-2")
	if !reflect.DeepEqual(logBefore, logAfter) || logAfter[len(logAfter)-1].Into != "dev" {
		t.Fatalf("log changed: %+v", logAfter)
	}
	// The renamed branch is what a merge now names.
	moved, err := b.MergeBranch("demo", actor, "develop", "x9")
	if err != nil || len(moved) != 2 {
		t.Fatalf("merging develop: %+v, %v", moved, err)
	}
	if _, err := b.MergeBranch("demo", actor, "dev", "x9"); err == nil {
		t.Fatal("the old name still answers")
	}
}

func TestRenamingTheReleaseBranchFollowsToTheBranchesMergingIntoIt(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Into: "main", Commit: "c9"})
	got, err := rename(b, "main", "master")
	if err != nil || !got.Release {
		t.Fatalf("%+v, %v", got, err)
	}
	bs, _ := b.Branches("demo")
	if g := names(bs); g[0] != [2]string{"dev", "master"} || g[1] != [2]string{"master", ""} {
		t.Fatalf("branches %v", g)
	}
	if s := wantStatus(t, b, "S-1", board.Merged); s.Into != "master" {
		t.Fatalf("S-1 into %q", s.Into)
	}
	if s := wantStatus(t, b, "S-2", board.Merged); s.Into != "dev" {
		t.Fatalf("S-2 into %q", s.Into)
	}
}

func TestRenamingABranchIsRefusedForABadOrTakenNameOrAnUnknownBranch(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	for _, c := range []struct {
		branch, to string
		kind       board.Kind
	}{
		{"dev", "main", board.Conflict},
		{"nope", "x", board.NotFound},
		{"dev", "", board.Invalid},
		{"dev", "two words", board.Invalid},
		{"dev", "tab\tname", board.Invalid},
		{"dev", "bad​name", board.Invalid},
		{"dev", strings.Repeat("a", 101), board.Invalid},
	} {
		_, err := rename(b, c.branch, c.to)
		var e *board.Error
		if !errors.As(err, &e) || e.Kind != c.kind {
			t.Fatalf("rename %q to %q: %v, want kind %v", c.branch, c.to, err, c.kind)
		}
	}
	if _, err := rename(b, "dev", strings.Repeat("a", 100)); err != nil {
		t.Fatalf("100 characters is allowed: %v", err)
	}
	bs, _ := b.Branches("demo")
	if g := names(bs); len(g) != 2 || g[1] != [2]string{"main", ""} {
		t.Fatalf("a refusal changed the branches: %v", g)
	}
	if _, err := b.UpdateBranch("nope", actor, "dev", board.BranchPatch{}); err == nil {
		t.Fatal("an unknown workspace is refused")
	}
}

func TestRenamingABranchToItsOwnNameChangesNothingAndNotifiesNoOne(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	got := recorder(b)
	if br, err := rename(b, "dev", "dev"); err != nil || br.Name != "dev" {
		t.Fatalf("%+v, %v", br, err)
	}
	if br, err := b.UpdateBranch("demo", actor, "dev", board.BranchPatch{}); err != nil || br.Name != "dev" {
		t.Fatalf("an empty patch: %+v, %v", br, err)
	}
	if len(*got) != 0 {
		t.Fatalf("notices %v", *got)
	}
	if s := wantStatus(t, b, "S-1", board.Merged); s.Into != "dev" {
		t.Fatalf("S-1 into %q", s.Into)
	}
}

func TestRenamingABranchNoticesEveryStoryThatReachedIt(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Into: "main", Commit: "c9"})
	got := recorder(b)
	if _, err := rename(b, "dev", "develop"); err != nil {
		t.Fatal(err)
	}
	want := []board.Change{
		{Workspace: "demo", Kind: board.KindStory, ID: "S-2"},
		{Workspace: "demo", Kind: board.KindStory, ID: "S-3"},
	}
	if !slices.Equal(*got, want) {
		t.Fatalf("notices %v, want %v", *got, want)
	}
	*got = nil
	if _, err := rename(b, "dev", "x"); err == nil || len(*got) != 0 {
		t.Fatalf("a refused rename sent %v", *got)
	}
}
