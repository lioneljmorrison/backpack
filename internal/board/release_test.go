package board_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// mergedIntoMain adds a story and merges it into dev, then into main.
func mergedIntoMain(t *testing.T, b *board.Board, title string) board.Story {
	t.Helper()
	s := addStory(t, b, board.NewStory{Title: title})
	walk(t, b, s.ID, board.InProgress, board.InReview)
	setStatus(t, b, s.ID, board.StatusChange{To: board.Merged, Commit: "d-" + s.ID})
	return setStatus(t, b, s.ID, board.StatusChange{To: board.Merged, Into: "main", Commit: "m-" + s.ID})
}

// threeInMainOneInDev is the board the acceptance describes: S-1 to S-3
// merged into main, S-4 merged into dev.
func threeInMainOneInDev(t *testing.T) *board.Board {
	t.Helper()
	b := newReleasingWorkspace(t)
	for i := range 3 {
		mergedIntoMain(t, b, fmt.Sprintf("In main %d", i+1))
	}
	mergedIntoDev(t, b, "In dev", "d4")
	return b
}

var v1 = board.NewRelease{Version: "v1.0.0", Tag: "v1.0.0", Commit: "abc123", Notes: "First cut"}

func TestCuttingAReleaseTakesEveryStoryMergedIntoTheReleaseBranchAndLeavesTheRest(t *testing.T) {
	b := threeInMainOneInDev(t)
	r, err := b.CutRelease("demo", actor, v1)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "R-1" || r.Version != "v1.0.0" || r.Tag != "v1.0.0" || r.Commit != "abc123" || r.Notes != "First cut" ||
		r.CreatedBy != actor || r.CreatedAt == "" || !slices.Equal(r.Stories, []string{"S-1", "S-2", "S-3"}) {
		t.Fatalf("release %+v", r)
	}
	for _, id := range r.Stories {
		s := wantStatus(t, b, id, board.Released)
		if s.Into != "main" {
			t.Fatalf("%s keeps the branch it reached: into %q", id, s.Into)
		}
		log := readLog(t, b, id)
		last := log[len(log)-1]
		if last.Kind != "status" || last.From != "merged" || last.To != "released" || last.Into != "main" ||
			last.Commit != "abc123" || last.Actor != actor || last.At == "" ||
			!strings.Contains(last.Text, "R-1") || !strings.Contains(last.Text, "v1.0.0") {
			t.Fatalf("%s last entry %+v", id, last)
		}
		if len(log) != 5 {
			t.Fatalf("%s log has %d entries, want the four moves then the release", id, len(log))
		}
	}
	if s := wantStatus(t, b, "S-4", board.Merged); s.Into != "dev" || len(readLog(t, b, "S-4")) != 3 {
		t.Fatalf("the story merged into dev changed: %+v", s)
	}
}

func TestEachReleaseTakesOnlyWhatWasMergedSinceTheLast(t *testing.T) {
	b := threeInMainOneInDev(t)
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	// The dev story reaches main, and one more story is merged there.
	setStatus(t, b, "S-4", board.StatusChange{To: board.Merged, Into: "main", Commit: "m4"})
	mergedIntoMain(t, b, "Later")
	r, err := b.CutRelease("demo", actor, board.NewRelease{Version: "v1.1.0", Tag: "v1.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "R-2" || !slices.Equal(r.Stories, []string{"S-4", "S-5"}) || r.Commit != "" || r.Notes != "" {
		t.Fatalf("release %+v", r)
	}
	// S-1 is not in the second release: its log has one release entry.
	if n := len(readLog(t, b, "S-1")); n != 5 {
		t.Fatalf("S-1 log has %d entries", n)
	}
}

func TestCuttingAReleaseWorksWithASingleBranch(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	walk(t, b, "S-1", board.InProgress, board.InReview, board.Merged)
	r, err := b.CutRelease("demo", actor, v1)
	if err != nil || !slices.Equal(r.Stories, []string{"S-1"}) {
		t.Fatalf("release %+v, %v", r, err)
	}
	wantStatus(t, b, "S-1", board.Released)
}

func TestACutHandsOutSequentialUniqueLogIdsInDeliveryOrder(t *testing.T) {
	b := threeInMainOneInDev(t)
	// Fifteen entries so far: four each for three stories, three for S-4.
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, id := range []string{"S-1", "S-2", "S-3"} {
		log := readLog(t, b, id)
		ids = append(ids, log[len(log)-1].ID)
	}
	if !slices.Equal(ids, []string{"L-16", "L-17", "L-18"}) {
		t.Fatalf("ids %v, want L-16, L-17, L-18", ids)
	}
	if _, err := b.AddLogEntry("demo", actor, "S-4", board.NewLogEntry{Kind: "note", Text: "after"}); err != nil {
		t.Fatal(err)
	}
	log := readLog(t, b, "S-4")
	if got := log[len(log)-1].ID; got != "L-19" {
		t.Fatalf("next id %s, want L-19", got)
	}
}

func TestACutWithNothingToReleaseIsRefusedAndTakesNoIds(t *testing.T) {
	b := newReleasingWorkspace(t)
	mergedIntoDev(t, b, "Only in dev", "d1")
	got := recorder(b)
	_, err := b.CutRelease("demo", actor, v1)
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), "nothing") || len(*got) != 0 {
		t.Fatalf("refusal %q, reported %+v", err, *got)
	}
	// The refusal used no release id and no log ids.
	mergedIntoMain(t, b, "Now in main")
	r, err := b.CutRelease("demo", actor, v1)
	if err != nil || r.ID != "R-1" {
		t.Fatalf("release %+v, %v; want R-1", r, err)
	}
}

func TestADuplicateVersionIsRefusedAndChangesNothing(t *testing.T) {
	b := threeInMainOneInDev(t)
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	mergedIntoMain(t, b, "Later")
	got := recorder(b)
	_, err := b.CutRelease("demo", actor, board.NewRelease{Version: "v1.0.0", Tag: "other"})
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), "v1.0.0") || len(*got) != 0 {
		t.Fatalf("refusal %q, reported %+v", err, *got)
	}
	if s := wantStatus(t, b, "S-5", board.Merged); s.Into != "main" {
		t.Fatalf("%+v", s)
	}
}

func TestACutNeedsAVersionAndATagInPlainText(t *testing.T) {
	b := threeInMainOneInDev(t)
	for _, c := range []struct {
		name, ws string
		r        board.NewRelease
		kind     board.Kind
	}{
		{"no version", "demo", board.NewRelease{Tag: "v1"}, board.Invalid},
		{"no tag", "demo", board.NewRelease{Version: "v1"}, board.Invalid},
		{"odd version", "demo", board.NewRelease{Version: "v​1", Tag: "v1"}, board.Invalid},
		{"odd notes", "demo", board.NewRelease{Version: "v1", Tag: "v1", Notes: "a​b"}, board.Invalid},
		{"odd commit", "demo", board.NewRelease{Version: "v1", Tag: "v1", Commit: "a​b"}, board.Invalid},
		{"unknown workspace", "nope", v1, board.NotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := b.CutRelease(c.ws, actor, c.r)
			wantKind(t, err, c.kind)
		})
	}
	wantStatus(t, b, "S-1", board.Merged)
}

func TestACutReportsEveryStoryItReleased(t *testing.T) {
	b := threeInMainOneInDev(t)
	got := recorder(b)
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
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

func TestAReleasedStoryIsCompleteSoItsDependentsAreNotBlocked(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	depend(t, b, "S-2", "S-1")
	walk(t, b, "S-1", board.InProgress, board.InReview, board.Merged)
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	if s, err := b.Story("demo", "S-2"); err != nil || s.Blocked {
		t.Fatalf("S-2 blocked=%v, %v", s.Blocked, err)
	}
	// A released story is final.
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: board.Backlog})
	wantKind(t, err, board.Conflict)
}

// A write that fails part way, here on the second log entry of the block, must
// leave no story released, no release made and no id taken: a cut is one query.
func TestACutThatFailsPartWayChangesNothing(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		mergedIntoMain(t, b, fmt.Sprintf("In main %d", i+1))
	}
	g := db.SelectGraph(prefix + "ws:demo")
	// Twelve entries exist, so the block is L-13 to L-15. Take the second.
	if _, err := g.Query("CREATE (:LogEntry {id: 'L-14'})", nil, nil); err != nil {
		t.Fatal(err)
	}
	uniqueConstraint(t, g, "LogEntry", "id")

	if _, err := b.CutRelease("demo", actor, v1); err == nil {
		t.Fatal("want the clash on L-14 to fail the cut")
	}
	for _, id := range []string{"S-1", "S-2", "S-3"} {
		wantStatus(t, b, id, board.Merged)
		if n := len(readLog(t, b, id)); n != 4 {
			t.Fatalf("%s log has %d entries", id, n)
		}
	}
	res, err := g.ROQuery("MATCH (w:Workspace) OPTIONAL MATCH (r:Release) RETURN w.next_log, w.next_release, count(r)", nil, nil)
	if err != nil || !res.Next() {
		t.Fatalf("read counters: %v", err)
	}
	rec := res.Record()
	log, _ := rec.GetByIndex(0)
	rel, _ := rec.GetByIndex(1)
	n, _ := rec.GetByIndex(2)
	if log != int64(13) || rel != nil || n != int64(0) {
		t.Fatalf("after the failed cut next_log=%v next_release=%v releases=%v, want 13, none, 0", log, rel, n)
	}
}

func TestSettingReleasedOrDeployedIsRefusedAndNamesCutRelease(t *testing.T) {
	b := newReleasingWorkspace(t)
	mergedIntoMain(t, b, "A")
	for _, to := range []string{"released", "deployed"} {
		_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: to})
		wantKind(t, err, board.Invalid)
		if !strings.Contains(err.Error(), "cut_release") {
			t.Fatalf("%s: refusal %q", to, err)
		}
	}
	wantStatus(t, b, "S-1", board.Merged)
}

func TestReleasesListNewestFirstByNumberNotByText(t *testing.T) {
	b := newReleasingWorkspace(t)
	if rs, err := b.ListReleases("demo"); err != nil || rs == nil || len(rs) != 0 {
		t.Fatalf("no releases yet: %v, %v", rs, err)
	}
	for i := 1; i <= 10; i++ {
		s := mergedIntoMain(t, b, fmt.Sprintf("Story %d", i))
		v := fmt.Sprintf("v%d", i)
		if _, err := b.CutRelease("demo", actor, board.NewRelease{Version: v, Tag: v, Notes: "n " + v, Commit: "c" + v}); err != nil {
			t.Fatal(err, s.ID)
		}
	}
	rs, err := b.ListReleases("demo")
	if err != nil || len(rs) != 10 {
		t.Fatalf("%d releases, %v", len(rs), err)
	}
	if rs[0].ID != "R-10" || rs[1].ID != "R-9" || rs[9].ID != "R-1" {
		t.Fatalf("order %s %s ... %s", rs[0].ID, rs[1].ID, rs[9].ID)
	}
	top := rs[0]
	if top.Version != "v10" || top.Tag != "v10" || top.Commit != "cv10" || top.Notes != "n v10" ||
		top.CreatedBy != actor || top.CreatedAt == "" || !slices.Equal(top.Stories, []string{"S-10"}) {
		t.Fatalf("top %+v", top)
	}
	_, err = b.ListReleases("nope")
	wantKind(t, err, board.NotFound)
}

func TestAReleaseReadsWithItsStoriesInDeliveryOrder(t *testing.T) {
	b := threeInMainOneInDev(t)
	cut, err := b.CutRelease("demo", actor, v1)
	if err != nil {
		t.Fatal(err)
	}
	// A story that moves down the board after the cut still reads in rank order.
	r, err := b.GetRelease("demo", "R-1")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != cut.ID || r.Version != "v1.0.0" || r.Tag != "v1.0.0" || r.Commit != "abc123" || r.Notes != "First cut" ||
		r.CreatedBy != actor || r.CreatedAt != cut.CreatedAt {
		t.Fatalf("release %+v", r)
	}
	got := ids(r.Entries)
	if !slices.Equal(got, []string{"S-1", "S-2", "S-3"}) {
		t.Fatalf("stories %v", got)
	}
	if r.Entries[0].Title != "In main 1" || r.Entries[0].Status != board.Released || r.Entries[0].Into != "main" {
		t.Fatalf("story %+v", r.Entries[0])
	}
	_, err = b.GetRelease("demo", "R-9")
	wantKind(t, err, board.NotFound)
	_, err = b.GetRelease("nope", "R-1")
	wantKind(t, err, board.NotFound)
}

func TestWhereAreWeNamesTheStoriesMergedIntoTheReleaseBranchWaitingForARelease(t *testing.T) {
	b := threeInMainOneInDev(t)
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(w.Unreleased), []string{"S-1", "S-2", "S-3"}) || w.Unreleased[0].Title != "In main 1" {
		t.Fatalf("unreleased %+v", w.Unreleased)
	}
	if text := w.Text(); !strings.Contains(text, "Merged, waiting for a release: 3 (latest: S-1 In main 1, S-2 In main 2, S-3 In main 3)\n") || strings.Contains(text, "S-4") {
		t.Fatalf("text:\n%s", text)
	}
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	w, err = b.Where("demo")
	if err != nil || w.Unreleased == nil || len(w.Unreleased) != 0 {
		t.Fatalf("after the cut: %+v, %v", w.Unreleased, err)
	}
	if strings.Contains(w.Text(), "waiting for a release") {
		t.Fatalf("text:\n%s", w.Text())
	}
}

func TestAReleasedStoryNamesItsReleaseAndOthersNameNone(t *testing.T) {
	b := threeInMainOneInDev(t)
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	s := wantStatus(t, b, "S-1", board.Released)
	if s.Release == nil || s.Release.ID != "R-1" || s.Release.Version != "v1.0.0" {
		t.Fatalf("S-1 release %+v", s.Release)
	}
	if s := wantStatus(t, b, "S-4", board.Merged); s.Release != nil {
		t.Fatalf("an unreleased story names release %+v", s.Release)
	}
	// The list and a release's own stories carry it too, from the same read.
	all, err := b.Stories("demo")
	if err != nil || all[0].Release == nil || all[0].Release.ID != "R-1" || all[3].Release != nil {
		t.Fatalf("stories %+v, %v", all, err)
	}
}

func TestAStoryReleasedBeforeReleasesWereRecordedNamesNone(t *testing.T) {
	b, g := newWorkspaceGraph(t)
	if _, err := g.Query("CREATE (:Story {id: 'S-1', title: 'old', status: 'released', into: 'main', rank: 'a'})", nil, nil); err != nil {
		t.Fatal(err)
	}
	s, err := b.Story("demo", "S-1")
	if err != nil || s.Status != board.Released || s.Release != nil {
		t.Fatalf("story %+v, %v", s, err)
	}
}

// beforeReleases builds a workspace as it stood before migration 10: three
// released stories with no release (S-1, S-2 and S-4), one merged and one in
// the backlog. The graph is set back one migration so the next open runs it.
func beforeReleases(t *testing.T, orphans bool) (*falkordb.Graph, func() *board.Board) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	g := db.SelectGraph(prefix + "ws:demo")
	qs := []string{
		"CREATE (:Story {id: 'S-3', title: 'merged', status: 'merged', into: 'main', rank: 'c'})",
		"CREATE (:Story {id: 'S-5', title: 'later', status: 'backlog', rank: 'e'})",
		"MATCH (w:Workspace) SET w.next_log = 7, w.next_story = 6",
	}
	if orphans {
		qs = append(qs,
			"CREATE (:Story {id: 'S-1', title: 'old one', status: 'released', into: 'main', rank: 'a'})",
			"CREATE (:Story {id: 'S-2', title: 'old two', status: 'released', into: 'main', rank: 'b'})",
			"CREATE (:Story {id: 'S-4', title: 'old four', status: 'released', into: 'main', rank: 'd'})")
	}
	for _, q := range qs {
		if _, err := g.Query(q, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	return g, func() *board.Board {
		t.Helper()
		if _, err := g.Query("MATCH (s:SchemaVersion) SET s.v = 9", nil, nil); err != nil {
			t.Fatal(err)
		}
		nb, err := board.New(db, prefix+"system", prefix+"ws:")
		if err != nil {
			t.Fatal(err)
		}
		return nb
	}
}

func TestMigrationPutsReleasedStoriesWithNoReleaseInR0(t *testing.T) {
	g, reopen := beforeReleases(t, true)
	for range 2 { // the second run finds nothing left over
		b := reopen()
		rs, err := b.ListReleases("demo")
		if err != nil || len(rs) != 1 {
			t.Fatalf("releases %+v, %v", rs, err)
		}
		r := rs[0]
		if r.ID != "R-0" || r.Version != "Before releases" || r.Tag != "" || r.Commit != "" ||
			!slices.Equal(r.Stories, []string{"S-1", "S-2", "S-4"}) {
			t.Fatalf("R-0 %+v", r)
		}
		d, err := b.GetRelease("demo", "R-0")
		if err != nil || len(d.Entries) != 3 || d.Entries[0].ID != "S-1" {
			t.Fatalf("release page %+v, %v", d, err)
		}
		s := wantStatus(t, b, "S-2", board.Released)
		if s.Release == nil || s.Release.ID != "R-0" || s.Release.Version != "Before releases" {
			t.Fatalf("S-2 release %+v", s.Release)
		}
		if s := wantStatus(t, b, "S-3", board.Merged); s.Release != nil {
			t.Fatalf("a merged story joined %+v", s.Release)
		}
		ids := map[string]bool{}
		for _, id := range []string{"S-1", "S-2", "S-4"} {
			log := readLog(t, b, id)
			if len(log) != 1 || !strings.Contains(log[0].Text, "R-0") || log[0].Actor == "" || log[0].At == "" {
				t.Fatalf("%s log %+v", id, log)
			}
			if ids[log[0].ID] {
				t.Fatalf("log id %s used twice", log[0].ID)
			}
			ids[log[0].ID] = true
		}
		for _, want := range []string{"L-7", "L-8", "L-9"} {
			if !ids[want] {
				t.Fatalf("log ids %v, want a block from L-7", ids)
			}
		}
	}
	res, err := g.ROQuery("MATCH (w:Workspace) RETURN w.next_log, w.next_release", nil, nil)
	if err != nil || !res.Next() {
		t.Fatal(err)
	}
	if nl, _ := res.Record().GetByIndex(0); nl != int64(10) {
		t.Fatalf("next_log %v, want 10", nl)
	}
	if nr, _ := res.Record().GetByIndex(1); nr != nil {
		t.Fatalf("next_release %v, left alone", nr)
	}
}

func TestMigrationMakesNoR0WhenNothingIsOrphaned(t *testing.T) {
	_, reopen := beforeReleases(t, false)
	b := reopen()
	if rs, err := b.ListReleases("demo"); err != nil || len(rs) != 0 {
		t.Fatalf("releases %+v, %v", rs, err)
	}
}

func TestR0ListsLastAndCuttingStillStartsAtR1(t *testing.T) {
	_, reopen := beforeReleases(t, true)
	b := reopen()
	s := mergedIntoMain(t, b, "New")
	r, err := b.CutRelease("demo", actor, v1)
	if err != nil || r.ID != "R-1" || !slices.Equal(r.Stories, []string{"S-3", s.ID}) {
		t.Fatalf("cut %+v, %v", r, err)
	}
	rs, err := b.ListReleases("demo")
	if err != nil || len(rs) != 2 || rs[0].ID != "R-1" || rs[1].ID != "R-0" {
		t.Fatalf("order %+v, %v", rs, err)
	}
}
