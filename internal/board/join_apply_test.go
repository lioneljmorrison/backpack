package board_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// joinApplyFixture is joinFixture with the cases a join must carry that the
// preview's fixture lacks: an upstream-changed mark in each source, a story
// found while building another, and a second repository holding stories.
func joinApplyFixture(t *testing.T) joinEnv {
	t.Helper()
	e := joinFixture(t)
	b := e.b
	// demo-api: S-2 depends on S-1, so a new plan on S-1 marks S-2 upstream.
	plan := "A new plan"
	if _, err := b.UpdateStory("demo-api", actor, "S-1", board.StoryPatch{Plan: &plan}); err != nil {
		t.Fatal(err)
	}
	// demo-web: S-3 is found in S-1, S-2 depends on S-1, and S-1's plan changes.
	if _, err := b.AddStory("demo-web", actor, board.NewStory{Title: "Footer", FoundIn: "S-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddDependency("demo-web", actor, "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.UpdateStory("demo-web", actor, "S-1", board.StoryPatch{Plan: &plan}); err != nil {
		t.Fatal(err)
	}
	return e
}

// dumpWS lists a workspace graph's nodes and edges in a fixed order.
func dumpWS(t *testing.T, e joinEnv, slug string) string {
	t.Helper()
	g := e.ws(slug)
	var rows []string
	res, err := g.ROQuery("MATCH (n) RETURN n", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for res.Next() {
		v, _ := res.Record().GetByIndex(0)
		rows = append(rows, canon(v))
	}
	res, err = g.ROQuery("MATCH (a)-[e]->(b) RETURN id(a), e, id(b)", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for res.Next() {
		var parts []string
		for _, v := range res.Record().Values() {
			parts = append(parts, canon(v))
		}
		rows = append(rows, strings.Join(parts, " "))
	}
	slices.Sort(rows)
	return strings.Join(rows, "\n")
}

func hasGraph(t *testing.T, e joinEnv, slug string) bool {
	t.Helper()
	names, err := e.db.ListGraphs()
	if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(names, e.prefix+"ws:"+slug)
}

// registry reads one registry entry's property, "" when absent.
func registry(t *testing.T, e joinEnv, slug, prop string) string {
	t.Helper()
	res, err := e.system().ROQuery("MATCH (e:WorkspaceEntry {slug: $slug}) RETURN e."+prop, map[string]any{"slug": slug}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Next() {
		return "<no entry>"
	}
	v, _ := res.Record().GetByIndex(0)
	s, _ := v.(string)
	return s
}

func TestApplyBuildsTheNewWorkspaceAndSwitchesInOneStep(t *testing.T) {
	e := joinApplyFixture(t)
	apiBefore, webBefore := dumpWS(t, e, "demo-api"), dumpWS(t, e, "demo-web")
	plan, err := e.b.ApplyJoin(joinReq, board.JoinHooks{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Sources) != 2 {
		t.Fatalf("plan %+v", plan)
	}

	// The registry: the new workspace exists, the sources are joined.
	if registry(t, e, "demo", "name") != "Demo" || registry(t, e, "demo", "created_by") != "tester" {
		t.Fatal("the new workspace is not registered")
	}
	for src, repo := range map[string]string{"demo-api": "api", "demo-web": "web"} {
		if registry(t, e, src, "joined_into") != "demo" || registry(t, e, src, "joining") != "" ||
			registry(t, e, src, "joined_by") != "tester" || registry(t, e, src, "joined_at") == "" {
			t.Fatalf("%s is not marked joined", src)
		}
		res, err := e.system().ROQuery("MATCH (e:WorkspaceEntry {slug: $s}) RETURN e.joined_repos", map[string]any{"s": src}, nil)
		if err != nil || !res.Next() {
			t.Fatal(err)
		}
		v, _ := res.Record().GetByIndex(0)
		if l, _ := v.([]any); len(l) != 1 || l[0] != repo {
			t.Fatalf("%s joined_repos %v, want [%s]", src, v, repo)
		}
	}
	list, err := e.b.Workspaces()
	if err != nil || len(list) != 1 || list[0].Slug != "demo" || len(list[0].Repos) != 2 || !list[0].HasReleaseStage {
		t.Fatalf("workspaces %+v %v", list, err)
	}
	if _, err := e.b.Story("demo-api", "S-1"); err == nil || !strings.Contains(err.Error(), "joined into demo") {
		t.Fatalf("a joined source answers with a pointer: %v", err)
	}

	// The sources' graphs are kept exactly.
	if dumpWS(t, e, "demo-api") != apiBefore || dumpWS(t, e, "demo-web") != webBefore {
		t.Fatal("a source graph changed")
	}

	// The new workspace's board.
	stories, err := e.b.Stories("demo")
	if err != nil || len(stories) != 9 {
		t.Fatalf("stories %d %v", len(stories), err)
	}
	s8, err := e.b.Story("demo", "S-8")
	if err != nil || s8.Title != "Pricing page" || s8.Status != "in_review" {
		t.Fatalf("S-8 %+v %v", s8, err)
	}
	if s8.UpstreamChanged == nil || s8.UpstreamChanged.Story != "S-7" {
		t.Fatalf("S-8's upstream mark %+v", s8.UpstreamChanged)
	}
	s2, _ := e.b.Story("demo", "S-2")
	if s2.UpstreamChanged == nil || s2.UpstreamChanged.Story != "S-1" {
		t.Fatalf("S-2's upstream mark %+v", s2.UpstreamChanged)
	}
	log, err := e.b.Log("demo", "S-7")
	if err != nil || len(log) < 1 {
		t.Fatalf("log %+v %v", log, err)
	}
	note := log[len(log)-1]
	want := "Joined into demo from demo-web, where this was S-1. Ids in earlier entries are demo-web's."
	if note.Kind != "note" || note.Text != want || note.Actor != "tester" {
		t.Fatalf("join note %+v", note)
	}
	if note.ID != "L-"+strconv.Itoa(plan.Notes.First+6) {
		t.Fatalf("the seventh story's note is %s, want it in the reserved block starting L-%d", note.ID, plan.Notes.First)
	}
	// Counters carry on after everything the join wrote.
	if counter(t, e.ws("demo"), "next_story") != 10 || counter(t, e.ws("demo"), "next_log") != plan.Target.Log {
		t.Fatalf("counters %d %d, plan %+v", counter(t, e.ws("demo"), "next_story"), counter(t, e.ws("demo"), "next_log"), plan.Target)
	}
	// The next story and log entry of the new workspace take fresh ids.
	n, err := e.b.AddStory("demo", actor, board.NewStory{Title: "After the join", Repo: "web"})
	if err != nil || n.ID != "S-10" {
		t.Fatalf("new story %+v %v", n, err)
	}
}

func TestBoardNewAfterAJoinLeavesTheNewWorkspaceUnchanged(t *testing.T) {
	e := joinApplyFixture(t)
	if _, err := e.b.ApplyJoin(joinReq, board.JoinHooks{}); err != nil {
		t.Fatal(err)
	}
	before := dumpWS(t, e, "demo")
	if _, err := board.New(e.db, e.prefix+"system", e.prefix+"ws:"); err != nil {
		t.Fatal(err)
	}
	if dumpWS(t, e, "demo") != before {
		t.Fatal("start-up changed the joined workspace")
	}
}

// rankOrder lists the backlog's story ids by rank.
func rankOrder(t *testing.T, e joinEnv, ws string) []string {
	t.Helper()
	res, err := e.ws(ws).ROQuery("MATCH (s:Story) RETURN s.id ORDER BY s.rank", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for res.Next() {
		v, _ := res.Record().GetByIndex(0)
		out = append(out, v.(string))
	}
	return out
}

func TestPlacingAStoryAtTheTopAndAtTheSourceBoundaryLandsAsAsked(t *testing.T) {
	e := joinApplyFixture(t)
	if _, err := e.b.ApplyJoin(joinReq, board.JoinHooks{}); err != nil {
		t.Fatal(err)
	}
	top, err := e.b.AddStory("demo", actor, board.NewStory{Title: "Top", Repo: "api", Position: board.Position{Before: "S-1"}})
	if err != nil {
		t.Fatal(err)
	}
	// S-6 is the last story of the first source, S-7 the first of the second.
	mid, err := e.b.AddStory("demo", actor, board.NewStory{Title: "Boundary", Repo: "web", Position: board.Position{After: "S-6"}})
	if err != nil {
		t.Fatal(err)
	}
	got := rankOrder(t, e, "demo")
	if got[0] != top.ID {
		t.Fatalf("order %v: %s is not on top", got, top.ID)
	}
	i := slices.Index(got, mid.ID)
	if got[i-1] != "S-6" || got[i+1] != "S-7" {
		t.Fatalf("order %v: %s is not between S-6 and S-7", got, mid.ID)
	}
}

func TestAForcedVerifyFailureLeavesTheSourcesAsTheyWereAndARerunSucceeds(t *testing.T) {
	e := joinApplyFixture(t)
	all := dump(t, e)
	_, err := e.b.ApplyJoin(joinReq, board.JoinHooks{After: func(stage string) error {
		if stage == "written" {
			// Damage the new graph so that the read-back cannot match.
			exec(t, e.ws("demo"), "MATCH (s:Story {id: 'S-3'}) SET s.title = 'tampered'")
		}
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "verif") {
		t.Fatalf("want a verification failure, got %v", err)
	}
	if hasGraph(t, e, "demo") {
		t.Fatal("the failed join left the new graph behind")
	}
	if after := dump(t, e); after != all {
		t.Fatalf("a failed join changed the board\nbefore:\n%s\nafter:\n%s", all, after)
	}
	if registry(t, e, "demo", "name") != "<no entry>" {
		t.Fatal("the failed join registered the new workspace")
	}
	// The sources work as before.
	if _, err := e.b.AddStory("demo-api", actor, board.NewStory{Title: "Still works"}); err != nil {
		t.Fatalf("a source after a failed join: %v", err)
	}
	if _, err := e.b.ApplyJoin(joinReq, board.JoinHooks{}); err != nil {
		t.Fatalf("a re-run: %v", err)
	}
}

func TestAnInterruptedJoinFreezesTheSourcesAndARerunFinishesIt(t *testing.T) {
	for _, stage := range []string{"frozen", "prepared", "written", "verified"} {
		t.Run(stage, func(t *testing.T) {
			e := joinApplyFixture(t)
			apiBefore := dumpWS(t, e, "demo-api")
			_, err := e.b.ApplyJoin(joinReq, board.JoinHooks{After: func(s string) error {
				if s == stage {
					return board.ErrJoinInterrupted
				}
				return nil
			}})
			if !errors.Is(err, board.ErrJoinInterrupted) {
				t.Fatalf("want the interruption, got %v", err)
			}
			// Frozen: the sources refuse, and the new workspace is not there.
			if registry(t, e, "demo-api", "joining") != "demo" || registry(t, e, "demo", "name") != "<no entry>" {
				t.Fatal("the sources are not frozen, or the new workspace is registered")
			}
			if _, err := e.b.AddStory("demo-api", actor, board.NewStory{Title: "x"}); err == nil || !strings.Contains(err.Error(), "being joined") {
				t.Fatalf("a frozen source accepts writes: %v", err)
			}
			if _, err := e.b.Story("demo", "S-1"); err == nil {
				t.Fatal("the half-built workspace is reachable")
			}
			if dumpWS(t, e, "demo-api") != apiBefore {
				t.Fatal("a source changed")
			}
			if _, err := e.b.ApplyJoin(joinReq, board.JoinHooks{}); err != nil {
				t.Fatalf("the re-run: %v", err)
			}
			stories, err := e.b.Stories("demo")
			if err != nil || len(stories) != 9 {
				t.Fatalf("stories %d %v", len(stories), err)
			}
			if dumpWS(t, e, "demo-api") != apiBefore {
				t.Fatal("a source changed")
			}
		})
	}
}

func TestAbortUndoesAnInterruptedJoin(t *testing.T) {
	e := joinApplyFixture(t)
	before := dump(t, e)
	_, err := e.b.ApplyJoin(joinReq, board.JoinHooks{After: func(s string) error {
		if s == "written" {
			return board.ErrJoinInterrupted
		}
		return nil
	}})
	if !errors.Is(err, board.ErrJoinInterrupted) || !hasGraph(t, e, "demo") {
		t.Fatalf("setup: %v", err)
	}
	if err := e.b.AbortJoin("demo"); err != nil {
		t.Fatal(err)
	}
	if hasGraph(t, e, "demo") {
		t.Fatal("abort left the half-built graph")
	}
	if after := dump(t, e); after != before {
		t.Fatalf("abort did not restore the board\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if _, err := e.b.AddStory("demo-api", actor, board.NewStory{Title: "Back to normal"}); err != nil {
		t.Fatal(err)
	}
	// A second abort has nothing to undo.
	if err := e.b.AbortJoin("demo"); err == nil {
		t.Fatal("abort with nothing under way was accepted")
	}
}

func TestAbortRefusesAFinishedJoinAndANameItDoesNotOwn(t *testing.T) {
	e := joinApplyFixture(t)
	// A graph that no join is building is never deleted.
	exec(t, e.ws("demo"), "CREATE (:Leftover)")
	if err := e.b.AbortJoin("demo"); err == nil || !hasGraph(t, e, "demo") {
		t.Fatalf("abort touched a graph no join froze sources for: %v", err)
	}
	if err := e.ws("demo").Delete(); err != nil { // a graph of this test's own prefix
		t.Fatal(err)
	}
	if _, err := e.b.ApplyJoin(joinReq, board.JoinHooks{}); err != nil {
		t.Fatal(err)
	}
	if err := e.b.AbortJoin("demo"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("abort of a finished join: %v", err)
	}
	if !hasGraph(t, e, "demo") {
		t.Fatal("abort deleted the finished workspace")
	}
	if _, err := e.b.Story("demo", "S-1"); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRefusesWhatThePreviewRefusesAndWritesNothing(t *testing.T) {
	e := joinApplyFixture(t)
	before := dump(t, e)
	for _, r := range []board.JoinRequest{
		{Into: "demo", Name: "Demo", Actor: "tester", From: []string{"demo-api", "nowhere"}},
		{Into: "demo-api", Name: "Demo", Actor: "tester", From: []string{"demo-api", "demo-web"}},
		{Into: "demo", Name: "Demo", Actor: "tester", From: []string{"demo-api"}},
	} {
		if _, err := e.b.ApplyJoin(r, board.JoinHooks{}); err == nil {
			t.Fatalf("%+v was accepted", r)
		}
	}
	if after := dump(t, e); after != before {
		t.Fatalf("a refused join changed the board\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestASourceThatChangesDuringTheJoinFailsTheVerification(t *testing.T) {
	e := joinApplyFixture(t)
	_, err := e.b.ApplyJoin(joinReq, board.JoinHooks{After: func(stage string) error {
		if stage == "written" {
			exec(t, e.ws("demo-web"), "CREATE (:Story {id: 'S-99', title: 'slipped in', status: 'backlog', rank: 'z'})")
		}
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "changed while it was being joined") {
		t.Fatalf("want the change noticed, got %v", err)
	}
	if hasGraph(t, e, "demo") || registry(t, e, "demo-web", "joining") != "" || registry(t, e, "demo", "name") != "<no entry>" {
		t.Fatal("the failed join left the new workspace or the freeze behind")
	}
}

func TestAHookFailureBeforeTheSwitchIsCleanedUpLikeAnyOther(t *testing.T) {
	for _, stage := range []string{"frozen", "prepared", "written", "verified"} {
		e := joinApplyFixture(t)
		before := dump(t, e)
		_, err := e.b.ApplyJoin(joinReq, board.JoinHooks{After: func(s string) error {
			if s == stage {
				return errors.New("boom at " + stage)
			}
			return nil
		}})
		if err == nil || !strings.Contains(err.Error(), "boom at "+stage) {
			t.Fatalf("%s: %v", stage, err)
		}
		if after := dump(t, e); after != before {
			t.Fatalf("a join that failed after %s changed the board", stage)
		}
	}
}
