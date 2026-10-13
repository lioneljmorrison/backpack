package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func branchNotice() board.Change {
	return board.Change{Workspace: "demo", Kind: board.KindBranch}
}

func TestEveryBranchChangeSendsABranchNotice(t *testing.T) {
	b := newReleasingWorkspace(t)
	got := recorder(b)
	alias, empty, other := "Development", "", "develop"
	steps := []struct {
		name string
		do   func() error
	}{
		{"add", func() error { _, err := addBranch(b, "staging", "main"); return err }},
		{"alias", func() error {
			_, err := b.UpdateBranch("demo", actor, "dev", board.BranchPatch{Alias: &alias})
			return err
		}},
		{"clear alias", func() error {
			_, err := b.UpdateBranch("demo", actor, "dev", board.BranchPatch{Alias: &empty})
			return err
		}},
		{"rename", func() error { _, err := rename(b, "staging", other); return err }},
		{"re-point", func() error { _, err := repoint(b, "dev", other); return err }},
		{"re-point back", func() error { _, err := repoint(b, "dev", "main"); return err }},
		{"remove", func() error { return b.RemoveBranch("demo", actor, other) }},
	}
	for _, s := range steps {
		*got = nil
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if want := []board.Change{branchNotice()}; !slices.Equal(*got, want) {
			t.Fatalf("%s: notices %v, want %v", s.name, *got, want)
		}
	}
}

func TestARefusedOrUnchangedBranchChangeSendsNoBranchNotice(t *testing.T) {
	b := newReleasingWorkspace(t)
	alias := "Dev"
	if _, err := b.UpdateBranch("demo", actor, "dev", board.BranchPatch{Alias: &alias}); err != nil {
		t.Fatal(err)
	}
	got := recorder(b)
	same, into := "dev", "main"
	_ = []error{
		first(addBranch(b, "dev", "main")),                                            // name taken
		first(addBranch(b, "x", "nowhere")),                                           // unknown target
		first(rename(b, "nope", "y")),                                                 // unknown branch
		first(repoint(b, "main", "dev")),                                              // release branch
		b.RemoveBranch("demo", actor, "main"),                                         // release branch
		b.RemoveBranch("demo", actor, "dev"),                                          // first branch
		first(b.UpdateBranch("demo", actor, "dev", board.BranchPatch{Alias: &alias})), // same alias: no change
		first(b.UpdateBranch("demo", actor, "dev", board.BranchPatch{Name: &same, Into: &into})),
		first(b.UpdateBranch("demo", actor, "dev", board.BranchPatch{})),
	}
	if len(*got) != 0 {
		t.Fatalf("notices %v", *got)
	}
}

func first(_ board.Branch, err error) error { return err }

func TestRenamingABranchWithStoriesNoticesTheStoriesAndThenTheBranch(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	got := recorder(b)
	if _, err := rename(b, "dev", "develop"); err != nil {
		t.Fatal(err)
	}
	want := []board.Change{
		{Workspace: "demo", Kind: board.KindStory, ID: "S-1"},
		{Workspace: "demo", Kind: board.KindStory, ID: "S-2"},
		{Workspace: "demo", Kind: board.KindStory, ID: "S-3"},
		branchNotice(),
	}
	if !slices.Equal(*got, want) {
		t.Fatalf("notices %v, want %v", *got, want)
	}
}
