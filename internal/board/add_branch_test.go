package board_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func addBranch(b *board.Board, name, into string) (board.Branch, error) {
	return b.AddBranch("demo", actor, board.Branch{Name: name, Into: into})
}

func repoint(b *board.Board, branch, into string) (board.Branch, error) {
	return b.UpdateBranch("demo", actor, branch, board.BranchPatch{Into: &into})
}

func mustAddBranch(t *testing.T, b *board.Board, name, into string) board.Branch {
	t.Helper()
	br, err := addBranch(b, name, into)
	if err != nil {
		t.Fatalf("add %s into %s: %v", name, into, err)
	}
	return br
}

func intoOf(t *testing.T, b *board.Board, name string) string {
	t.Helper()
	bs, err := b.Branches("demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, br := range bs {
		if br.Name == name {
			return br.Into
		}
	}
	t.Fatalf("no branch %q", name)
	return ""
}

// The plan's acceptance: staging between dev and main, every story where it
// was, and merges from dev now go to staging.
func TestAddingStagingAndRepointingDevLeavesEveryStoryInPlace(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Into: "main", Commit: "c9"})
	type snap struct {
		s   board.Story
		log []board.LogEntry
	}
	ids := []string{"S-1", "S-2", "S-3", "S-4"}
	snapshot := func() map[string]snap {
		out := map[string]snap{}
		for _, id := range ids {
			st, err := b.Story("demo", id)
			if err != nil {
				t.Fatal(err)
			}
			out[id] = snap{st, readLog(t, b, id)}
		}
		return out
	}
	before := snapshot()

	got := mustAddBranch(t, b, "staging", "main")
	if got.Name != "staging" || got.Into != "main" || got.Release || got.CreatedBy != actor || got.CreatedAt == "" {
		t.Fatalf("answered %+v", got)
	}
	re, err := repoint(b, "dev", "staging")
	if err != nil || re.Into != "staging" || re.Name != "dev" {
		t.Fatalf("re-pointing: %+v, %v", re, err)
	}
	bs, err := b.Branches("demo")
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]string{{"dev", "staging"}, {"staging", "main"}, {"main", ""}}
	if g := names(bs); !reflect.DeepEqual(g, want) {
		t.Fatalf("branches %v, want %v", g, want)
	}
	if after := snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("stories changed:\n%+v\n%+v", before, after)
	}

	// Merging dev now goes to staging, then staging to main.
	moved, err := b.MergeBranch("demo", actor, "dev", "x1")
	if err != nil || len(moved) != 2 {
		t.Fatalf("merging dev: %+v, %v", moved, err)
	}
	for _, s := range moved {
		if s.Into != "staging" {
			t.Fatalf("%s went to %q, want staging", s.ID, s.Into)
		}
	}
	moved, err = b.MergeBranch("demo", actor, "staging", "x2")
	if err != nil || len(moved) != 2 || moved[0].Into != "main" {
		t.Fatalf("merging staging: %+v, %v", moved, err)
	}
}

func TestAMergedStoryFollowsTheCurrentIntoAfterARepoint(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	mustAddBranch(t, b, "staging", "main")
	if _, err := repoint(b, "dev", "staging"); err != nil {
		t.Fatal(err)
	}
	// Per-story merge: from dev, the next is staging; main is refused.
	if _, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: board.Merged, Into: "main", Commit: "c9"}); err == nil ||
		!strings.Contains(err.Error(), "staging") {
		t.Fatalf("skipping staging: %v", err)
	}
	s := setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Into: "staging", Commit: "c9"})
	if s.Into != "staging" {
		t.Fatalf("S-1 into %q, want staging", s.Into)
	}
	// With dev re-pointed, review still reaches dev, the only starting branch.
	if s := setStatus(t, b, "S-4", board.StatusChange{To: board.Merged, Commit: "c9"}); s.Into != "dev" {
		t.Fatalf("S-4 into %q, want dev", s.Into)
	}
}

func TestSeveralStartingBranchesNeedTheMergeToNameOne(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	mustAddBranch(t, b, "staging", "main") // dev still merges into main: two starting branches

	_, err := b.SetStatus("demo", actor, "S-4", board.StatusChange{To: board.Merged, Commit: "c1"})
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), "name the branch: dev or staging") {
		t.Fatalf("refusal %q does not list the starting branches", err)
	}
	wantStatus(t, b, "S-4", board.InReview)

	// main is not a starting branch: dev and staging lead to it.
	_, err = b.SetStatus("demo", actor, "S-4", board.StatusChange{To: board.Merged, Into: "main"})
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), "dev or staging") {
		t.Fatalf("refusal %q", err)
	}

	// Either starting branch is accepted.
	if s := setStatus(t, b, "S-4", board.StatusChange{To: board.Merged, Into: "staging", Commit: "c1"}); s.Into != "staging" {
		t.Fatalf("into %q", s.Into)
	}
	s := addStory(t, b, board.NewStory{Title: "Other"})
	walk(t, b, s.ID, board.InProgress, board.InReview)
	if got := setStatus(t, b, s.ID, board.StatusChange{To: board.Merged, Into: "dev", Commit: "c2"}); got.Into != "dev" {
		t.Fatalf("into %q", got.Into)
	}
}

func TestMovesOffersOneMergePerStartingBranch(t *testing.T) {
	bs := []board.Branch{{Name: "dev", Into: "main"}, {Name: "staging", Into: "main"}, {Name: "main", Release: true}}
	var got []string
	for _, m := range board.Moves(board.Story{Status: board.InReview}, bs) {
		got = append(got, m.String())
	}
	want := []string{"on_hold", "in_progress", "merged:dev", "merged:staging", "closed"}
	if !slices.Equal(got, want) {
		t.Fatalf("moves %v, want %v", got, want)
	}
	// One starting branch: unchanged.
	got = nil
	chain := []board.Branch{{Name: "dev", Into: "staging"}, {Name: "staging", Into: "main"}, {Name: "main", Release: true}}
	for _, m := range board.Moves(board.Story{Status: board.InReview}, chain) {
		got = append(got, m.String())
	}
	if want := []string{"on_hold", "in_progress", "merged:dev", "closed"}; !slices.Equal(got, want) {
		t.Fatalf("moves %v, want %v", got, want)
	}
}

func TestAddBranchRefusesWhatCannotBeAdded(t *testing.T) {
	b := newReleasingWorkspace(t)
	long := strings.Repeat("a", 101)
	for _, c := range []struct {
		name string
		br   board.Branch
		kind board.Kind
	}{
		{"merges nowhere", board.Branch{Name: "other"}, board.Invalid},
		{"empty name", board.Branch{Into: "main"}, board.Invalid},
		{"whitespace", board.Branch{Name: "a b", Into: "main"}, board.Invalid},
		{"too long", board.Branch{Name: long, Into: "main"}, board.Invalid},
		{"alias too long", board.Branch{Name: "x", Into: "main", Alias: long}, board.Invalid},
		{"taken", board.Branch{Name: "dev", Into: "main"}, board.Conflict},
		{"taken by the release branch", board.Branch{Name: "main", Into: "dev"}, board.Conflict},
		{"unknown into", board.Branch{Name: "x", Into: "nope"}, board.NotFound},
	} {
		before, _ := b.Branches("demo")
		_, err := b.AddBranch("demo", actor, c.br)
		wantKind(t, err, c.kind)
		if after, _ := b.Branches("demo"); !reflect.DeepEqual(before, after) {
			t.Fatalf("%s: a refused add changed the branches: %v", c.name, after)
		}
	}
	if _, err := b.AddBranch("nowhere", actor, board.Branch{Name: "x", Into: "main"}); err == nil {
		t.Fatal("no workspace was not refused")
	} else {
		wantKind(t, err, board.NotFound)
	}
}

func TestAddBranchTakesAnAlias(t *testing.T) {
	b := newReleasingWorkspace(t)
	got, err := b.AddBranch("demo", actor, board.Branch{Name: "staging", Into: "main", Alias: "Pre-prod"})
	if err != nil || got.Alias != "Pre-prod" {
		t.Fatalf("%+v, %v", got, err)
	}
	if a := aliasOf(t, b, "staging"); a != "Pre-prod" {
		t.Fatalf("alias %q", a)
	}
}

func TestRepointingRefusesWhatWouldBreakTheChain(t *testing.T) {
	b := newReleasingWorkspace(t)
	mustAddBranch(t, b, "staging", "main")
	if _, err := repoint(b, "dev", "staging"); err != nil { // dev -> staging -> main
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, branch, into string
		kind               board.Kind
		says               string
	}{
		{"into itself", "dev", "dev", board.Conflict, "itself"},
		{"back to what leads to it", "staging", "dev", board.Conflict, "leads back"},
		{"two steps back", "main", "dev", board.Conflict, "release"},
		{"a second release branch", "dev", "", board.Conflict, "release"},
		{"the release branch merging on", "main", "staging", board.Conflict, "release"},
		{"unknown branch to merge into", "dev", "nope", board.NotFound, "nope"},
		{"unknown branch to change", "nope", "main", board.NotFound, "nope"},
	} {
		_, err := repoint(b, c.branch, c.into)
		wantKind(t, err, c.kind)
		if !strings.Contains(err.Error(), c.says) {
			t.Fatalf("%s: refusal %q lacks %q", c.name, err, c.says)
		}
	}
	if got := intoOf(t, b, "dev"); got != "staging" {
		t.Fatalf("dev into %q after the refusals", got)
	}
	if got := intoOf(t, b, "staging"); got != "main" {
		t.Fatalf("staging into %q after the refusals", got)
	}
}

func TestARepointFarBackAroundALongChainIsACycle(t *testing.T) {
	b := newReleasingWorkspace(t)
	mustAddBranch(t, b, "a", "main")
	mustAddBranch(t, b, "b", "a")
	mustAddBranch(t, b, "c", "b")
	if _, err := repoint(b, "a", "c"); err == nil || !strings.Contains(err.Error(), "leads back") {
		t.Fatalf("a into c: %v", err)
	}
	// Moving to a sibling chain is not a cycle.
	if _, err := repoint(b, "c", "main"); err != nil {
		t.Fatalf("c into main: %v", err)
	}
	if _, err := repoint(b, "b", "c"); err != nil {
		t.Fatalf("b into c: %v", err)
	}
}

func TestRepointingToWhereItAlreadyMergesAndAnEmptyPatchChangeNothing(t *testing.T) {
	b := newReleasingWorkspace(t)
	if got, err := repoint(b, "dev", "main"); err != nil || got.Into != "main" {
		t.Fatalf("%+v, %v", got, err)
	}
	if got, err := b.UpdateBranch("demo", actor, "dev", board.BranchPatch{}); err != nil || got.Into != "main" {
		t.Fatalf("empty patch: %+v, %v", got, err)
	}
	// Clearing the release branch's into, which it has not got, is nothing.
	if got, err := repoint(b, "main", ""); err != nil || got.Into != "" || !got.Release {
		t.Fatalf("%+v, %v", got, err)
	}
}

func TestACombinedPatchOfNameAliasAndIntoIsOneChangeOrNone(t *testing.T) {
	b := newReleasingWorkspace(t)
	mustAddBranch(t, b, "staging", "main")
	name, al, into := "develop", "Dev", "staging"
	got, err := b.UpdateBranch("demo", actor, "dev", board.BranchPatch{Name: &name, Alias: &al, Into: &into})
	if err != nil || got.Name != "develop" || got.Alias != "Dev" || got.Into != "staging" {
		t.Fatalf("%+v, %v", got, err)
	}
	// A refused part (a cycle) leaves the rename and alias undone too.
	n2, a2, i2 := "again", "Again", "develop"
	_, err = b.UpdateBranch("demo", actor, "staging", board.BranchPatch{Name: &n2, Alias: &a2, Into: &i2})
	wantKind(t, err, board.Conflict)
	bs, _ := b.Branches("demo")
	if g, want := names(bs), [][2]string{{"develop", "staging"}, {"staging", "main"}, {"main", ""}}; !reflect.DeepEqual(g, want) {
		t.Fatalf("branches %v, want %v", g, want)
	}
	if a := aliasOf(t, b, "staging"); a != "" {
		t.Fatalf("staging gained alias %q", a)
	}
}
