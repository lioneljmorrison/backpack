package board_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// mergedIntoDev adds a story and merges it into dev, leaving it there.
func mergedIntoDev(t *testing.T, b *board.Board, title, commit string) {
	t.Helper()
	s := addStory(t, b, board.NewStory{Title: title})
	walk(t, b, s.ID, board.InProgress, board.InReview)
	setStatus(t, b, s.ID, board.StatusChange{To: board.Merged, Commit: commit})
}

// threeMergedAndOneInReview is the board the plan's acceptance describes: S-1
// to S-3 merged into dev, S-4 in review.
func threeMergedAndOneInReview(t *testing.T) *board.Board {
	t.Helper()
	b := newReleasingWorkspace(t)
	for i, c := range []string{"c1", "c2", "c3"} {
		mergedIntoDev(t, b, fmt.Sprintf("Merged %d", i+1), c)
	}
	s := addStory(t, b, board.NewStory{Title: "Reviewing"})
	walk(t, b, s.ID, board.InProgress, board.InReview)
	return b
}

func TestMergingABranchMovesEveryStoryMergedIntoItAndLogsTheCommit(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	moved, err := b.MergeBranch("demo", actor, "dev", "x9")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range moved {
		got = append(got, s.ID)
		if s.Status != board.Merged || s.Into != "main" {
			t.Fatalf("%s is %s into %q, want merged into main", s.ID, s.Status, s.Into)
		}
	}
	if !slices.Equal(got, []string{"S-1", "S-2", "S-3"}) {
		t.Fatalf("moved %v, want S-1, S-2, S-3", got)
	}
	for _, id := range got {
		if s := wantStatus(t, b, id, board.Merged); s.Into != "main" {
			t.Fatalf("%s into %q", id, s.Into)
		}
		log := readLog(t, b, id)
		last := log[len(log)-1]
		// The same entry a merge by set_status writes.
		if last.Kind != "status" || last.From != "merged" || last.To != "merged" || last.Into != "main" ||
			last.Commit != "x9" || last.Actor != actor || last.At == "" {
			t.Fatalf("%s last entry %+v", id, last)
		}
		if len(log) != 4 || log[2].Into != "dev" {
			t.Fatalf("%s log %+v: want the merge into dev kept, then the one into main", id, log)
		}
	}
	if s := wantStatus(t, b, "S-4", board.InReview); s.Into != "" || len(readLog(t, b, "S-4")) != 2 {
		t.Fatalf("the story in review changed: %+v", s)
	}
}

func TestMergingABranchWritesTheSameEntryAsMergingOneStory(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Into: "main", Commit: "x9"})
	if _, err := b.MergeBranch("demo", actor, "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	one, all := readLog(t, b, "S-1"), readLog(t, b, "S-2")
	a, c := one[len(one)-1], all[len(all)-1]
	a.ID, a.At, c.ID, c.At = "", "", "", ""
	if !reflect.DeepEqual(a, c) {
		t.Fatalf("one story's merge %+v differs from the branch's %+v", a, c)
	}
}

func TestMergingABranchHandsOutSequentialUniqueLogIds(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	// Eleven entries so far: three stories with three each, the fourth with two.
	if _, err := b.MergeBranch("demo", actor, "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, id := range []string{"S-1", "S-2", "S-3"} {
		log := readLog(t, b, id)
		ids = append(ids, log[len(log)-1].ID)
	}
	if !slices.Equal(ids, []string{"L-12", "L-13", "L-14"}) {
		t.Fatalf("ids %v, want L-12, L-13, L-14 in delivery order", ids)
	}
	// The counter moved past the block: the next entry is the next id.
	if _, err := b.AddLogEntry("demo", actor, "S-4", board.NewLogEntry{Kind: "note", Text: "after"}); err != nil {
		t.Fatal(err)
	}
	log := readLog(t, b, "S-4")
	if got := log[len(log)-1].ID; got != "L-15" {
		t.Fatalf("next id %s, want L-15", got)
	}
}

func TestMergingABranchWithNothingOnItSucceedsAndMovesNothing(t *testing.T) {
	b := newReleasingWorkspace(t)
	addStory(t, b, board.NewStory{Title: "Waiting"})
	got := recorder(b)
	moved, err := b.MergeBranch("demo", actor, "dev", "x9")
	if err != nil {
		t.Fatal(err)
	}
	if moved == nil || len(moved) != 0 || len(*got) != 0 {
		t.Fatalf("moved %#v, reported %+v; want an empty list and no notices", moved, *got)
	}
	// And the counter did not move: the next entry is the next id.
	setStatus(t, b, "S-1", board.StatusChange{To: board.InProgress})
	if log := readLog(t, b, "S-1"); log[0].ID != "L-1" {
		t.Fatalf("first id %s, want L-1", log[0].ID)
	}
}

func TestMergingABranchTwiceMovesNothingTheSecondTime(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	if _, err := b.MergeBranch("demo", actor, "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	moved, err := b.MergeBranch("demo", actor, "dev", "y1")
	if err != nil || len(moved) != 0 {
		t.Fatalf("moved %+v, %v; stories already in main must stay", moved, err)
	}
}

func TestMergingABranchReportsEveryStoryItMoved(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	got := recorder(b)
	if _, err := b.MergeBranch("demo", actor, "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	var want []board.Change
	for _, id := range []string{"S-1", "S-2", "S-3"} {
		want = append(want, board.Change{Workspace: "demo", Kind: board.KindStory, ID: id})
	}
	if !slices.Equal(*got, want) {
		t.Fatalf("reported %+v, want %+v", *got, want)
	}
}

func TestMergingTheReleaseBranchAnUnknownBranchOrWithoutACommitIsRefused(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	got := recorder(b)
	cases := []struct {
		name, ws, branch, commit string
		kind                     board.Kind
	}{
		{"release branch", "demo", "main", "x9", board.Conflict},
		{"unknown branch", "demo", "nope", "x9", board.NotFound},
		{"no commit", "demo", "dev", "", board.Invalid},
		{"odd commit", "demo", "dev", "x​9", board.Invalid},
		{"unknown workspace", "nope", "dev", "x9", board.NotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := b.MergeBranch(c.ws, actor, c.branch, c.commit)
			wantKind(t, err, c.kind)
		})
	}
	if len(*got) != 0 {
		t.Fatalf("reported %+v", *got)
	}
	for _, id := range []string{"S-1", "S-2", "S-3"} {
		if s := wantStatus(t, b, id, board.Merged); s.Into != "dev" {
			t.Fatalf("%s moved to %q", id, s.Into)
		}
	}
}

// A write that fails part way, here on the second log entry of the block,
// must leave no story moved and no id taken: the merge is one query.
func TestMergingABranchThatFailsPartWayMovesNothing(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"c1", "c2", "c3"} {
		mergedIntoDev(t, b, c, c)
	}
	g := db.SelectGraph(prefix + "ws:demo")
	// Nine entries exist, so the block is L-10 to L-12. Take the second.
	if _, err := g.Query("CREATE (:LogEntry {id: 'L-11'})", nil, nil); err != nil {
		t.Fatal(err)
	}
	uniqueConstraint(t, g, "LogEntry", "id")

	if _, err := b.MergeBranch("demo", actor, "dev", "x9"); err == nil {
		t.Fatal("want the clash on L-11 to fail the merge")
	}
	for _, id := range []string{"S-1", "S-2", "S-3"} {
		s := wantStatus(t, b, id, board.Merged)
		if s.Into != "dev" || len(readLog(t, b, id)) != 3 {
			t.Fatalf("%s moved: into %q, log %+v", id, s.Into, readLog(t, b, id))
		}
	}
	res, err := g.ROQuery("MATCH (w:Workspace) RETURN w.next_log", nil, nil)
	if err != nil || !res.Next() {
		t.Fatalf("read counter: %v", err)
	}
	if n, _ := res.Record().GetByIndex(0); n != int64(10) {
		t.Fatalf("counter is %v after the failed merge, want 10", n)
	}
}
