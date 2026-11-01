package board_test

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FalkorDB/falkordb-go/v2"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// eventsBoard is a releasing workspace "demo" with direct access to its graph,
// so a test can set the times events happened.
func eventsBoard(t *testing.T) (*board.Board, *falkordb.Graph) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	return b, db.SelectGraph(prefix + "ws:demo")
}

// stamp gives every log entry, story, idea and release the time at(id) says.
// Items that happened together are given the same time, so a cut release's
// entries still carry the release's own time.
func stamp(t *testing.T, g *falkordb.Graph, at func(id string) string) {
	t.Helper()
	for label, prop := range map[string]string{"LogEntry": "at", "Story": "created_at", "Idea": "created_at", "Release": "created_at"} {
		res, err := g.ROQuery(fmt.Sprintf("MATCH (n:%s) RETURN n.id", label), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for res.Next() {
			v, _ := res.Record().GetByIndex(0)
			ids = append(ids, v.(string))
		}
		for _, id := range ids {
			q := fmt.Sprintf("MATCH (n:%s {id: $id}) SET n.%s = $at", label, prop)
			if _, err := g.Query(q, map[string]any{"id": id, "at": at(id)}, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// stampAll gives everything one time.
func stampAll(t *testing.T, g *falkordb.Graph, at string) {
	t.Helper()
	stamp(t, g, func(string) string { return at })
}

func latest(t *testing.T, b *board.Board, q board.EventsQuery) []board.Event {
	t.Helper()
	evs, err := b.Events("demo", q)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func eventIDs(evs []board.Event) []string {
	ids := []string{}
	for _, e := range evs {
		ids = append(ids, e.ID)
	}
	return ids
}

func eventLines(evs []board.Event) string {
	var lines []string
	for _, e := range evs {
		lines = append(lines, e.Line)
	}
	return strings.Join(lines, "\n")
}

// at builds the time of the n-th second of a fixed day.
func at(n int) string {
	return time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Second).Format(time.RFC3339)
}

func number(id string) int {
	n, _ := strconv.Atoi(id[2:])
	return n
}

func TestAnEmptyBoardHasNoEvents(t *testing.T) {
	b, _ := eventsBoard(t)
	if evs := latest(t, b, board.EventsQuery{}); evs == nil || len(evs) != 0 {
		t.Fatalf("events %v, want an empty list", evs)
	}
}

func TestEventsAreTheLatestTwentyHoweverOldOldestFirst(t *testing.T) {
	b, g := eventsBoard(t)
	for i := range 25 {
		addStory(t, b, board.NewStory{Title: fmt.Sprintf("Story %d", i+1)})
	}
	// Weeks old, one second apart.
	stamp(t, g, func(id string) string { return at(number(id)) })
	evs := latest(t, b, board.EventsQuery{})
	var want []string
	for i := 6; i <= 25; i++ {
		want = append(want, fmt.Sprintf("S-%d", i))
	}
	if got := eventIDs(evs); !slices.Equal(got, want) {
		t.Fatalf("ids %v, want %v", got, want)
	}
	if !strings.HasPrefix(evs[0].At, "2026-08-01") {
		t.Fatalf("first event at %s", evs[0].At)
	}
}

func TestEventsAreWhatHappenedAndNothingElse(t *testing.T) {
	b, g := eventsBoard(t)
	addPhase(t, b, "Phase one")
	if _, err := b.AddGoal("demo", actor, 1, "a goal"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.UpdatePhase("demo", actor, 1, board.PhasePatch{Title: ptr("Renamed")}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddIdea("demo", actor, board.NewIdea{Title: "An idea"}); err != nil {
		t.Fatal(err)
	}
	addStory(t, b, board.NewStory{Title: "Merged one"})
	addStory(t, b, board.NewStory{Title: "Held one"})
	addStory(t, b, board.NewStory{Title: "Closed one"})
	addStory(t, b, board.NewStory{Title: "Found one", FoundIn: "S-1"})
	log := func(id string, e board.NewLogEntry) board.LogEntry {
		t.Helper()
		got, err := b.AddLogEntry("demo", actor, id, e)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	log("S-1", board.NewLogEntry{Kind: "note", Text: "a note about it"})
	log("S-1", board.NewLogEntry{Kind: "commit", Commit: "feedface"})
	log("S-1", board.NewLogEntry{Kind: "pr", PRURL: "https://example.invalid/pr/1"})
	d := log("S-1", board.NewLogEntry{Kind: "decision", Text: "Keep ranks as text", Reason: "the store orders them as Go does"})
	log("S-1", board.NewLogEntry{Kind: "correction", Text: "It was ordering, not ranking", Corrects: d.ID})
	walk(t, b, "S-1", board.InProgress, board.InReview)
	setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Commit: "abc1234567"})
	if _, err := b.MergeBranch("demo", actor, "dev", "ffff0000"); err != nil {
		t.Fatal(err)
	}
	walk(t, b, "S-2", board.InProgress)
	setStatus(t, b, "S-2", board.StatusChange{To: board.OnHold, Reason: "waiting on the vendor"})
	setStatus(t, b, "S-3", board.StatusChange{To: board.Closed, Reason: "no longer needed"})
	if _, err := b.CutRelease("demo", actor, board.NewRelease{Version: "v1.0.0", Tag: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	// R-0 stands for what shipped before releases: it is not an event.
	if _, err := g.Query("CREATE (:Release {id: 'R-0', version: 'Before releases', created_by: 'migration', created_at: $at})", map[string]any{"at": at(0)}, nil); err != nil {
		t.Fatal(err)
	}
	stampAll(t, g, "2026-10-01T10:00:00Z") // one second: kind then number orders them

	evs := latest(t, b, board.EventsQuery{})
	lines := eventLines(evs)
	for _, want := range []string{
		"story added: Merged one",
		"story added: Found one (found in S-1)",
		"idea added: An idea",
		"release v1.0.0 (tag v1.0.0): 1 story",
		"decision: Keep ranks as text — the store orders them as Go does",
		"correction of " + d.ID + ": It was ordering, not ranking",
		"merged into dev (abc1234)",
		"merged into main (ffff000)",
		"on hold: waiting on the vendor",
		"closed: no longer needed",
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("no line %q in:\n%s", want, lines)
		}
	}
	for _, not := range []string{"a note about it", "feedface", "example.invalid", "in progress", "in_progress", "review", "Renamed", "a goal", "R-0", "Before releases", "Released in", "Phase one"} {
		if strings.Contains(lines, not) {
			t.Errorf("line %q should not be there:\n%s", not, lines)
		}
	}
	if len(evs) != 4+1+1+6 { // four stories, an idea, a release, six log lines
		t.Errorf("%d events:\n%s", len(evs), lines)
	}
	// Each line starts with its own position and a short time.
	for _, e := range evs {
		if !strings.HasPrefix(e.Line, e.ID+" 2026-10-01 10:00 ") {
			t.Errorf("line %q does not start with %s and its time", e.Line, e.ID)
		}
	}
}

func TestAReleaseCutShowsOnceNotOncePerStory(t *testing.T) {
	b := threeInMainOneInDev(t)
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	evs := latest(t, b, board.EventsQuery{})
	releases := 0
	for _, e := range evs {
		if e.Kind == "release" {
			releases++
			if !strings.Contains(e.Line, "release v1.0.0 (tag v1.0.0): 3 stories") || e.ID != "R-1" {
				t.Errorf("release event %+v", e)
			}
		}
		if e.To == "released" || strings.Contains(e.Line, "Released in") {
			t.Errorf("a release's own entry repeated: %+v", e)
		}
	}
	if releases != 1 {
		t.Fatalf("%d release events, want 1:\n%s", releases, eventLines(evs))
	}
}

func TestAProposalsPlacementFoldsIntoItsAddedLine(t *testing.T) {
	b, g := eventsBoard(t)
	propose(t, b, board.NewStory{Title: "Proposed one"})
	if _, err := b.AddLogEntry("demo", actor, "S-1", board.NewLogEntry{Kind: "decision", Text: "Proposed again later", Reason: "a real decision"}); err != nil {
		t.Fatal(err)
	}
	// The placement entry shares the story's own instant; the decision is later.
	if _, err := g.Query("MATCH (e:LogEntry {id: 'L-2'}) SET e.at = '2999-01-01T00:00:00Z'", nil, nil); err != nil {
		t.Fatal(err)
	}
	evs := latest(t, b, board.EventsQuery{})
	if len(evs) != 2 || evs[0].ID != "S-1" || evs[1].ID != "L-2" {
		t.Fatalf("events:\n%s", eventLines(evs))
	}
	if strings.Contains(eventLines(evs), "Proposed at the bottom") {
		t.Fatalf("the placement shows on its own:\n%s", eventLines(evs))
	}
}

func TestOldStatusEntriesAreLabelledAsRecordedBeforeReleases(t *testing.T) {
	b, g := eventsBoard(t)
	addStory(t, b, board.NewStory{Title: "Old"})
	for id, to := range map[string]string{"L-50": "released", "L-51": "deployed"} {
		if _, err := g.Query(`MATCH (s:Story {id: 'S-1'}) CREATE (s)-[:LOGGED]->(:LogEntry {id: $id, at: '2026-01-01T00:00:00Z',
			actor: 'migration', kind: 'status', from: 'merged', to: $to})`, map[string]any{"id": id, "to": to}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// A legacy released entry on a story of R-0 reads the same: no cut wrote it.
	evs := latest(t, b, board.EventsQuery{})
	var released, deployed string
	for _, e := range evs {
		switch e.ID {
		case "L-50":
			released = e.Line
		case "L-51":
			deployed = e.Line
		}
	}
	if !strings.Contains(released, "reached the release branch") || !strings.Contains(released, "recorded before releases") {
		t.Errorf("old released: %q", released)
	}
	if !strings.Contains(deployed, "released") || !strings.Contains(deployed, "recorded before releases") ||
		strings.Contains(deployed, "reached") {
		t.Errorf("old deployed: %q", deployed)
	}
}

func TestEventsOfOneSecondAreOrderedByKindThenNumberAsANumber(t *testing.T) {
	b, g := eventsBoard(t)
	mergedIntoMain(t, b, "Shipped")
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	addStory(t, b, board.NewStory{Title: "Another"})
	if _, err := b.AddIdea("demo", actor, board.NewIdea{Title: "An idea"}); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		if _, err := b.AddLogEntry("demo", actor, "S-2", board.NewLogEntry{Kind: "decision", Text: fmt.Sprintf("d%d", i), Reason: "r"}); err != nil {
			t.Fatal(err)
		}
	}
	stampAll(t, g, at(0))
	evs := latest(t, b, board.EventsQuery{})
	rank := map[byte]int{'S': 1, 'I': 2, 'R': 3, 'L': 4}
	var logs int
	for i := 1; i < len(evs); i++ {
		a, c := evs[i-1].ID, evs[i].ID
		ka, kc := rank[a[0]], rank[c[0]]
		if ka > kc || (ka == kc && number(a) >= number(c)) {
			t.Fatalf("%s before %s:\n%s", a, c, eventLines(evs))
		}
		if c[0] == 'L' {
			logs++
		}
	}
	if logs < 10 {
		t.Fatalf("only %d log lines in the shared second", logs)
	}
	ids := eventIDs(evs)
	if slices.Index(ids, "S-1") > slices.Index(ids, "I-1") || slices.Index(ids, "I-1") > slices.Index(ids, "R-1") || slices.Index(ids, "R-1") > slices.Index(ids, "L-9") {
		t.Fatalf("kinds out of order: %v", ids)
	}
}

func TestStoredTimesAreUTCToTheSecond(t *testing.T) {
	b, _ := eventsBoard(t)
	mergedIntoMain(t, b, "Shipped")
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddIdea("demo", actor, board.NewIdea{Title: "An idea"}); err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`)
	evs := latest(t, b, board.EventsQuery{})
	if len(evs) == 0 {
		t.Fatal("no events")
	}
	for _, e := range evs {
		if !re.MatchString(e.At) {
			t.Errorf("%s at %q", e.ID, e.At)
		}
	}
}

func TestLongTextIsCutAtOneHundredCharacters(t *testing.T) {
	b, _ := eventsBoard(t)
	addStory(t, b, board.NewStory{Title: "A"})
	long := strings.Repeat("x", 300)
	if _, err := b.AddLogEntry("demo", actor, "S-1", board.NewLogEntry{Kind: "decision", Text: long, Reason: long}); err != nil {
		t.Fatal(err)
	}
	evs := latest(t, b, board.EventsQuery{})
	line := evs[len(evs)-1].Line
	want := strings.Repeat("x", 99) + "…"
	if strings.Count(line, want) != 2 || strings.Contains(line, strings.Repeat("x", 100)) {
		t.Fatalf("line %q", line)
	}
	if n := len([]rune(line)); n > 260 {
		t.Fatalf("line is %d characters", n)
	}
}

// busyBoard has fifty events packed seven to a second, so any page of twenty
// ends inside a second.
func busyBoard(t *testing.T) (*board.Board, []board.Event) {
	t.Helper()
	b, g := eventsBoard(t)
	for i := range 20 {
		addStory(t, b, board.NewStory{Title: fmt.Sprintf("Story %d", i+1)})
	}
	for i := range 30 {
		if _, err := b.AddLogEntry("demo", actor, "S-1", board.NewLogEntry{Kind: "decision", Text: fmt.Sprintf("d%d", i), Reason: "r"}); err != nil {
			t.Fatal(err)
		}
	}
	stamp(t, g, func(id string) string {
		i := number(id) - 1
		if id[0] == 'L' {
			i += 20
		}
		return at(i / 7)
	})
	all := latest(t, b, board.EventsQuery{Since: "2000-01-01"})
	if len(all) != 50 {
		t.Fatalf("%d events, want 50", len(all))
	}
	return b, all
}

func TestBeforeWalksTheWholeBoardWithNoGapAndNoOverlap(t *testing.T) {
	b, all := busyBoard(t)
	got := latest(t, b, board.EventsQuery{})
	pages := 1
	for len(got) < len(all) {
		page := latest(t, b, board.EventsQuery{Before: got[0].ID})
		if len(page) == 0 || len(page) > 20 {
			t.Fatalf("page of %d before %s", len(page), got[0].ID)
		}
		got = append(page, got...)
		pages++
	}
	if !slices.Equal(eventIDs(got), eventIDs(all)) {
		t.Fatalf("walked %v\nwant %v", eventIDs(got), eventIDs(all))
	}
	if pages != 3 {
		t.Fatalf("%d pages", pages)
	}
	if evs := latest(t, b, board.EventsQuery{Before: all[0].ID}); len(evs) != 0 {
		t.Fatalf("before the first: %v", eventIDs(evs))
	}
}

func TestBeforeAnythingOnTheBoardWorksEvenWhatIsNotAnEvent(t *testing.T) {
	b, _ := eventsBoard(t)
	addStory(t, b, board.NewStory{Title: "A"})
	n, err := b.AddLogEntry("demo", actor, "S-1", board.NewLogEntry{Kind: "note", Text: "just a note"})
	if err != nil {
		t.Fatal(err)
	}
	// The note is no event, but it is a place: the story, added first, is before it.
	if evs := latest(t, b, board.EventsQuery{Before: n.ID}); !slices.Equal(eventIDs(evs), []string{"S-1"}) {
		t.Fatalf("events before the note: %v", eventIDs(evs))
	}
}

func TestAfterGivesEverythingLaterAndTheRestOfItsOwnSecond(t *testing.T) {
	b, all := busyBoard(t)
	for _, from := range []int{0, 10, 24, 30, 49} {
		x := all[from]
		var want []string
		for _, e := range all {
			if e.At >= x.At && e.ID != x.ID {
				want = append(want, e.ID)
			}
		}
		got := latest(t, b, board.EventsQuery{After: x.ID})
		if !slices.Equal(eventIDs(got), want) {
			t.Fatalf("after %s: %v, want %v", x.ID, eventIDs(got), want)
		}
	}
	// The events of its own second that come before it are given again.
	x := all[10] // the fourth of its second
	got := eventIDs(latest(t, b, board.EventsQuery{After: x.ID}))
	if !slices.Contains(got, all[8].ID) || slices.Contains(got, x.ID) {
		t.Fatalf("after %s: %v", x.ID, got)
	}
}

func TestSinceIsInclusiveAndReadsOffsetsAndBareDates(t *testing.T) {
	b, g := eventsBoard(t)
	for range 3 {
		addStory(t, b, board.NewStory{Title: "A story"})
	}
	times := map[string]string{"S-1": "2026-10-01T23:30:00Z", "S-2": "2026-10-02T00:00:00Z", "S-3": "2026-10-02T00:00:01Z"}
	stamp(t, g, func(id string) string { return times[id] })
	for since, want := range map[string][]string{
		"2026-10-01":                {"S-1", "S-2", "S-3"},
		"2026-10-02":                {"S-2", "S-3"},
		"2026-10-02T00:00:00Z":      {"S-2", "S-3"},
		"2026-10-02T00:00:01Z":      {"S-3"},
		"2026-10-02T00:00:02Z":      {},
		"2026-10-01T19:00:00-05:00": {"S-2", "S-3"},
		"2026-10-01T18:30:00-05:00": {"S-1", "S-2", "S-3"},
		"2026-10-02T02:00:00+02:00": {"S-2", "S-3"},
		"2026-10-02T02:00:01+02:00": {"S-3"},
	} {
		got := eventIDs(latest(t, b, board.EventsQuery{Since: since}))
		if !slices.Equal(got, want) {
			t.Errorf("since %s: %v, want %v", since, got, want)
		}
	}
}

func TestOptionsAreRefusedWhenTheyCannotBeMet(t *testing.T) {
	b, _ := eventsBoard(t)
	addStory(t, b, board.NewStory{Title: "A"})
	for _, q := range []board.EventsQuery{
		{Before: "S-1", After: "S-1"},
		{Before: "S-1", Since: "2026-01-01"},
		{After: "S-1", Since: "2026-01-01"},
		{Since: "yesterday"},
		{Since: "2026-10"},
		{Before: "banana"},
	} {
		_, err := b.Events("demo", q)
		wantKind(t, err, board.Invalid)
	}
	for _, q := range []board.EventsQuery{{Before: "S-99"}, {After: "L-999"}, {Before: "I-1"}, {After: "R-7"}} {
		_, err := b.Events("demo", q)
		wantKind(t, err, board.NotFound)
	}
	_, err := b.Events("nowhere", board.EventsQuery{})
	wantKind(t, err, board.NotFound)
}
