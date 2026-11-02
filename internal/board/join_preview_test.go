package board_test

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// Fixtures are invented. demo-api (repository api, dev merging into main)
// and demo-web (repository web, main alone) are two sources to preview
// joining into demo.

type joinEnv struct {
	b      *board.Board
	db     *falkordb.FalkorDB
	prefix string
}

func (e joinEnv) system() *falkordb.Graph { return e.db.SelectGraph(e.prefix + "system") }
func (e joinEnv) ws(slug string) *falkordb.Graph {
	return e.db.SelectGraph(e.prefix + "ws:" + slug)
}
func (e joinEnv) mark(t *testing.T, slug string, props map[string]any) {
	t.Helper()
	falkortest.MarkEntry(t, e.db, e.prefix+"system", slug, props)
}

// exec runs a write directly on a graph, to build states the board cannot.
func exec(t *testing.T, g *falkordb.Graph, q string) {
	t.Helper()
	if _, err := g.Query(q, nil, nil); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func newJoinEnv(t *testing.T) joinEnv {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	return joinEnv{b, db, prefix}
}

// joinFixture builds demo-api and demo-web with stories, dependencies,
// ideas, phases with goals, a release, log entries and a correction.
func joinFixture(t *testing.T) joinEnv {
	t.Helper()
	e := newJoinEnv(t)
	b := e.b
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo-api", Name: "Demo API", RepoName: "api", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo-web", Name: "Demo Web", RepoName: "web"}); err != nil {
		t.Fatal(err)
	}

	// demo-api
	if _, err := b.AddPhase("demo-api", actor, "Foundations", "Sign-in first"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddGoal("demo-api", actor, 1, "Sign in works"); err != nil {
		t.Fatal(err)
	}
	addS := func(ws string, s board.NewStory) string {
		got, err := b.AddStory(ws, actor, s)
		if err != nil {
			t.Fatal(err)
		}
		return got.ID
	}
	status := func(ws, id string, cs ...board.StatusChange) {
		for _, c := range cs {
			if _, err := b.SetStatus(ws, actor, id, c); err != nil {
				t.Fatalf("%s %s to %s: %v", ws, id, c.To, err)
			}
		}
	}
	addS("demo-api", board.NewStory{Title: "Add login", Phase: 1, Goal: "People sign in"})
	addS("demo-api", board.NewStory{Title: "Add logout", Plan: "Needs S-1 first."})
	addS("demo-api", board.NewStory{Title: "Rate limits"})
	addS("demo-api", board.NewStory{Title: "Audit trail"})
	addS("demo-api", board.NewStory{Title: "Cache"})
	if _, err := b.AddDependency("demo-api", actor, "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	status("demo-api", "S-3", board.StatusChange{To: "in_progress", Branch: "feature/rate"})
	status("demo-api", "S-4", board.StatusChange{To: "in_progress"}, board.StatusChange{To: "in_review"},
		board.StatusChange{To: "merged", Commit: "c1"}, board.StatusChange{To: "merged", Into: "main", Commit: "c2"})
	if _, err := b.CutRelease("demo-api", actor, board.NewRelease{Version: "v1.0.0", Tag: "v1.0.0", Notes: "Includes S-4"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddIdea("demo-api", actor, board.NewIdea{Title: "Search", Context: "Maybe after S-5"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddIdea("demo-api", actor, board.NewIdea{Title: "Webhooks"}); err != nil {
		t.Fatal(err)
	}
	addS("demo-api", board.NewStory{Title: "Search box", Proposed: true, Reasoning: "from the idea"})
	if _, err := b.RefineIdea("demo-api", actor, "I-1", board.Refinement{Stories: []string{"S-6"}}); err != nil {
		t.Fatal(err)
	}
	first, err := b.AddLogEntry("demo-api", actor, "S-1", board.NewLogEntry{Kind: "note", Text: "Started"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddLogEntry("demo-api", actor, "S-1", board.NewLogEntry{Kind: "correction", Text: "Fixed", Corrects: first.ID}); err != nil {
		t.Fatal(err)
	}

	// demo-web
	if _, err := b.AddPhase("demo-web", actor, "Launch", "Public site"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddGoal("demo-web", actor, 1, "Site is live"); err != nil {
		t.Fatal(err)
	}
	addS("demo-web", board.NewStory{Title: "Landing page", Phase: 1})
	addS("demo-web", board.NewStory{Title: "Pricing page"})
	status("demo-web", "S-2", board.StatusChange{To: "in_progress"}, board.StatusChange{To: "in_review", PRURL: "https://example.com/pr/1"})
	if _, err := b.AddIdea("demo-web", actor, board.NewIdea{Title: "Dark mode"}); err != nil {
		t.Fatal(err)
	}
	return e
}

var joinReq = board.JoinRequest{Into: "demo", Name: "Demo", Actor: "tester", From: []string{"demo-api", "demo-web"}}

// counter reads a counter off a workspace's Workspace node; absent reads as 1.
func counter(t *testing.T, g *falkordb.Graph, name string) int {
	t.Helper()
	res, err := g.Query("MATCH (w:Workspace) RETURN coalesce(w."+name+", 1)", nil, nil)
	if err != nil || !res.Next() {
		t.Fatalf("counter %s: %v", name, err)
	}
	v, _ := res.Record().GetByIndex(0)
	return int(v.(int64))
}

func TestThePreviewGivesEachSourceItsOffsets(t *testing.T) {
	e := joinFixture(t)
	plan, err := e.b.PreviewJoin(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Sources) != 2 || plan.Sources[0].Slug != "demo-api" || plan.Sources[1].Slug != "demo-web" {
		t.Fatalf("sources %+v", plan.Sources)
	}
	api, web := plan.Sources[0], plan.Sources[1]
	if api.Offsets != (board.JoinOffsets{}) {
		t.Fatalf("the first source keeps its ids: %+v", api.Offsets)
	}
	logs := counter(t, e.ws("demo-api"), "next_log") - 1
	want := board.JoinOffsets{Story: 6, Log: logs, Idea: 2, Goal: 1, Release: 1, Phase: 1}
	if web.Offsets != want {
		t.Fatalf("offsets %+v, want %+v", web.Offsets, want)
	}
	// The target's counters carry on after every source, and next_log also
	// reserves one id per story for the join notes.
	webLogs := counter(t, e.ws("demo-web"), "next_log") - 1
	stories := 6 + 2
	wantTarget := board.JoinCounters{Story: 9, Log: 1 + logs + webLogs + stories, Idea: 1 + 2 + 1, Goal: 1 + 1 + 1, Release: 2}
	if plan.Target != wantTarget {
		t.Fatalf("target counters %+v, want %+v", plan.Target, wantTarget)
	}
	if plan.Notes.First != 1+logs+webLogs || plan.Notes.Count != stories {
		t.Fatalf("join-note block %+v", plan.Notes)
	}
	if api.Counts["Story"] != 6 || web.Counts["Story"] != 2 || api.Counts["Release"] != 1 || api.Counts["Idea"] != 2 ||
		web.Counts["Idea"] != 1 || api.Counts["Goal"] != 1 || api.Counts["Phase"] != 1 {
		t.Fatalf("counts %+v / %+v", api.Counts, web.Counts)
	}
	if len(api.Repos) != 1 || api.Repos[0].Name != "api" || !slices.Equal(api.Branches, []string{"api/dev", "api/main"}) ||
		!slices.Equal(web.Branches, []string{"web/main"}) {
		t.Fatalf("repos and branches: %+v %v / %v", api.Repos, api.Branches, web.Branches)
	}
}

// remapped returns every story of the plan after remapping, by new id.
func remapped(plan board.JoinPlan) []board.SnapNode {
	var out []board.SnapNode
	for _, s := range plan.Sources {
		for _, n := range s.Nodes {
			if slices.Contains(n.Labels, "Story") {
				out = append(out, s.Remap(n))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Props["rank"].(string) < out[j].Props["rank"].(string) })
	return out
}

func TestTheRemappedStoriesKeepEachSourcesOrderAndStackSourceBySource(t *testing.T) {
	e := joinFixture(t)
	plan, err := e.b.PreviewJoin(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range remapped(plan) {
		got = append(got, n.Props["id"].(string)+"="+n.Props["old_workspace"].(string)+"/"+n.Props["old_id"].(string))
		if strings.HasSuffix(n.Props["rank"].(string), "0") {
			t.Fatalf("rank %q ends in 0", n.Props["rank"])
		}
	}
	want := "S-1=demo-api/S-1 S-2=demo-api/S-2 S-3=demo-api/S-3 S-4=demo-api/S-4 S-5=demo-api/S-5 S-6=demo-api/S-6 " +
		"S-7=demo-web/S-1 S-8=demo-web/S-2"
	if strings.Join(got, " ") != want {
		t.Fatalf("merged order\n got %s\nwant %s", strings.Join(got, " "), want)
	}
}

func TestRemappingMovesEveryIDAndLeavesTheRest(t *testing.T) {
	e := joinFixture(t)
	plan, err := e.b.PreviewJoin(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	web := plan.Sources[1]
	find := func(label, key, val string) board.SnapNode {
		for _, n := range web.Nodes {
			if slices.Contains(n.Labels, label) && n.Props[key] == val {
				return n
			}
		}
		t.Fatalf("no %s with %s=%s", label, key, val)
		return board.SnapNode{}
	}
	s := web.Remap(find("Story", "id", "S-2"))
	if s.Props["id"] != "S-8" || s.Props["old_id"] != "S-2" || s.Props["old_workspace"] != "demo-web" ||
		s.Props["title"] != "Pricing page" || s.Props["status"] != "in_review" {
		t.Fatalf("story %+v", s.Props)
	}
	if !strings.HasPrefix(s.Props["rank"].(string), "2") {
		t.Fatalf("rank %v is not prefixed with the second source's digit", s.Props["rank"])
	}
	// The original is not changed.
	if find("Story", "id", "S-2").Props["id"] != "S-2" {
		t.Fatal("Remap changed its input")
	}
	if g := web.Remap(find("Goal", "id", "G-1")); g.Props["id"] != "G-2" || g.Props["old_id"] != "G-1" {
		t.Fatalf("goal %+v", g.Props)
	}
	if i := web.Remap(find("Idea", "id", "I-1")); i.Props["id"] != "I-3" || i.Props["old_id"] != "I-1" {
		t.Fatalf("idea %+v", i.Props)
	}
	var phase board.SnapNode
	for _, n := range web.Nodes {
		if slices.Contains(n.Labels, "Phase") {
			phase = web.Remap(n)
		}
	}
	if phase.Props["number"] != int64(2) || phase.Props["old_id"] != "1" || phase.Props["old_workspace"] != "demo-web" {
		t.Fatalf("phase %+v", phase.Props)
	}
	for _, n := range web.Nodes {
		if slices.Contains(n.Labels, "LogEntry") {
			l := web.Remap(n)
			if l.Props["id"] == n.Props["id"] || l.Props["text"] != n.Props["text"] {
				t.Fatalf("log entry %+v became %+v", n.Props, l.Props)
			}
			if _, has := l.Props["old_id"]; has {
				t.Fatal("log entries carry no old_id")
			}
		}
	}
	// A repository and a branch are copied as they are; R-0 is not renumbered.
	repo := web.Remap(find("Repo", "name", "web"))
	if repo.Props["name"] != "web" {
		t.Fatalf("repo %+v", repo.Props)
	}
	r0 := web.Remap(board.SnapNode{Labels: []string{"Release"}, Props: map[string]any{"id": "R-0"}})
	r2 := web.Remap(board.SnapNode{Labels: []string{"Release"}, Props: map[string]any{"id": "R-2"}})
	if r0.Props["id"] != "R-0" || r2.Props["id"] != "R-3" {
		t.Fatalf("releases %v %v", r0.Props["id"], r2.Props["id"])
	}
	up := web.Remap(board.SnapNode{Labels: []string{"Story"}, Props: map[string]any{"id": "S-1", "upstream_id": "S-2", "rank": "5"}})
	if up.Props["upstream_id"] != "S-8" {
		t.Fatalf("upstream %v", up.Props["upstream_id"])
	}
}

func TestTheSnapshotHoldsEveryNodeAndEdgeOfASource(t *testing.T) {
	e := joinFixture(t)
	plan, err := e.b.PreviewJoin(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	api := plan.Sources[0]
	edges := map[string]int{}
	for _, ed := range api.Edges {
		edges[ed.Type]++
	}
	for _, typ := range []string{"HAS_GOAL", "CONTAINS", "DEPENDS_ON", "FOUND_IN", "LOGGED", "CORRECTS", "BECAME", "IN_RELEASE", "HAS_BRANCH", "HOLDS", "HAS_RELEASE"} {
		if typ == "FOUND_IN" {
			continue // none in this fixture; covered by the next test
		}
		if edges[typ] == 0 {
			t.Errorf("no %s edge in the snapshot: %v", typ, edges)
		}
	}
	if edges["BECAME"] != 1 || edges["DEPENDS_ON"] != 1 || edges["IN_RELEASE"] != 1 || edges["CORRECTS"] != 1 {
		t.Fatalf("edges %v", edges)
	}
	ids := map[uint64]bool{}
	for _, n := range api.Nodes {
		ids[n.ID] = true
	}
	for _, ed := range api.Edges {
		if !ids[ed.From] || !ids[ed.To] {
			t.Fatalf("edge %+v points outside the snapshot", ed)
		}
	}
}

func TestAFoundInLinkIsReadToo(t *testing.T) {
	e := joinFixture(t)
	if _, err := e.b.AddStory("demo-web", actor, board.NewStory{Title: "Fix typo", FoundIn: "S-1"}); err != nil {
		t.Fatal(err)
	}
	plan, err := e.b.PreviewJoin(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ed := range plan.Sources[1].Edges {
		if ed.Type == "FOUND_IN" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("FOUND_IN edges %d", n)
	}
}

func TestTheReportShowsWhatAJoinWouldDo(t *testing.T) {
	e := joinFixture(t)
	plan, err := e.b.PreviewJoin(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	rep := plan.Report()
	t.Log("\n" + rep)
	for _, want := range []string{
		"join demo-api, demo-web into demo (Demo) as tester",
		"demo-api", "repositories: api", "branches: api/dev, api/main",
		"stories S-1..S-6 -> S-1..S-6",
		"stories S-1..S-2 -> S-7..S-8",
		"ideas I-1..I-2 -> I-1..I-2",
		"ideas I-1 -> I-3",
		"goals G-1 -> G-2",
		"releases R-1 -> R-1",
		"phases 1 -> 2",
		"next_story 9",
		"top of the merged backlog: S-1 (was demo-api S-1) Add login",
		"S-8 (was demo-web S-2) is in_review and gets a new id",
	} {
		if !strings.Contains(rep, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	// The first source keeps its ids, so nothing of it is warned about.
	for _, not := range []string{"demo-api S-3", "text field"} {
		if strings.Contains(rep, not) {
			t.Errorf("report has %q:\n%s", not, rep)
		}
	}
}

func TestMentionsOfIDsInTextAreCountedWhereIDsChange(t *testing.T) {
	e := joinFixture(t)
	if _, err := e.b.UpdateStory("demo-web", actor, "S-1", board.StoryPatch{Plan: ptr("After S-2 and G-1")}); err != nil {
		t.Fatal(err)
	}
	plan, err := e.b.PreviewJoin(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Report(), "demo-web: 1 text field(s) name ids that change") {
		t.Fatalf("report\n%s", plan.Report())
	}
}

// canon prints a node or edge with its properties in a fixed order.
func canon(v any) string {
	switch x := v.(type) {
	case *falkordb.Node:
		return fmt.Sprintf("node %d %v %v", x.ID, x.Labels, x.Properties)
	case *falkordb.Edge:
		return fmt.Sprintf("edge %d %s %v", x.ID, x.Relation, x.Properties)
	}
	return fmt.Sprintf("%v", v)
}

// dump lists every node and edge of the system graph and of every test
// graph of the env, with the graph names, to compare before and after.
func dump(t *testing.T, e joinEnv) string {
	t.Helper()
	names, err := e.db.ListGraphs()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	var out []string
	for _, name := range names {
		if !strings.HasPrefix(name, e.prefix) {
			continue
		}
		g := e.db.SelectGraph(name)
		out = append(out, "graph "+name)
		res, err := g.ROQuery("MATCH (n) RETURN n", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		var rows []string
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
		out = append(out, rows...)
	}
	return strings.Join(out, "\n")
}

func TestThePreviewWritesNothing(t *testing.T) {
	e := joinFixture(t)
	e.mark(t, "demo-api", map[string]any{"marked": "x"}) // the registry has properties too
	before := dump(t, e)
	if !strings.Contains(before, "demo-api") || len(before) < 500 {
		t.Fatal("the dump is empty; the comparison would prove nothing")
	}
	for range 2 {
		if _, err := e.b.PreviewJoin(joinReq); err != nil {
			t.Fatal(err)
		}
	}
	// Refused previews write nothing either.
	_, _ = e.b.PreviewJoin(board.JoinRequest{Into: "demo", Name: "Demo", Actor: "tester", From: []string{"demo-api", "nope"}})
	if after := dump(t, e); after != before {
		t.Fatalf("the preview changed the board\nbefore:\n%s\nafter:\n%s", before, after)
	}
	names, _ := e.db.ListGraphs()
	if slices.Contains(names, e.prefix+"ws:demo") {
		t.Fatal("the preview created the target graph")
	}
}

func wantJoinRefusal(t *testing.T, err error, kind board.Kind, contains string) {
	t.Helper()
	var be *board.Error
	if err == nil {
		t.Fatalf("no refusal, want %q", contains)
	}
	if !errorsAs(err, &be) || be.Kind != kind || !strings.Contains(be.Msg, contains) {
		t.Fatalf("error = %v, want kind %d containing %q", err, kind, contains)
	}
}

func errorsAs(err error, be **board.Error) bool {
	e, ok := err.(*board.Error)
	*be = e
	return ok
}

func req(from ...string) board.JoinRequest {
	return board.JoinRequest{Into: "demo", Name: "Demo", Actor: "tester", From: from}
}

func TestAJoinNeedsTwoDifferentSources(t *testing.T) {
	e := joinFixture(t)
	_, err := e.b.PreviewJoin(req("demo-api"))
	wantJoinRefusal(t, err, board.Invalid, "at least two")
	_, err = e.b.PreviewJoin(req())
	wantJoinRefusal(t, err, board.Invalid, "at least two")
	_, err = e.b.PreviewJoin(req("demo-api", "demo-api"))
	wantJoinRefusal(t, err, board.Invalid, "named twice")
}

func TestAJoinTakesAtMostSixtyOneSources(t *testing.T) {
	e := joinFixture(t)
	var from []string
	for i := range 62 {
		from = append(from, fmt.Sprintf("demo-%d", i))
	}
	_, err := e.b.PreviewJoin(req(from...))
	wantJoinRefusal(t, err, board.Invalid, "at most 61")
}

func TestTheTargetNeedsAFreeSlugAndAPlainName(t *testing.T) {
	e := joinFixture(t)
	for _, slug := range []string{"", "Demo", "demo_x", "-demo", strings.Repeat("a", 65)} {
		r := req("demo-api", "demo-web")
		r.Into = slug
		_, err := e.b.PreviewJoin(r)
		wantJoinRefusal(t, err, board.Invalid, "slug")
	}
	for _, name := range []string{"", "bad\x00name"} {
		r := req("demo-api", "demo-web")
		r.Name = name
		_, err := e.b.PreviewJoin(r)
		wantJoinRefusal(t, err, board.Invalid, "name")
	}
	r := req("demo-api", "demo-web")
	r.Actor = ""
	_, err := e.b.PreviewJoin(r)
	wantJoinRefusal(t, err, board.Invalid, "actor")

	// A slug that is registered, joined or not, is taken, and so is a source's own.
	if _, err := e.b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	_, err = e.b.PreviewJoin(req("demo-api", "demo-web"))
	wantJoinRefusal(t, err, board.Conflict, `workspace "demo" already exists`)
	e.mark(t, "demo", map[string]any{"joined_into": "elsewhere"})
	_, err = e.b.PreviewJoin(req("demo-api", "demo-web"))
	wantJoinRefusal(t, err, board.Conflict, `workspace "demo" already exists`)
	r = req("demo-api", "demo-web")
	r.Into = "demo-web"
	_, err = e.b.PreviewJoin(r)
	wantJoinRefusal(t, err, board.Conflict, `workspace "demo-web" already exists`)
}

func TestAnExistingTargetGraphRefusesUnlessThisJoinWasInterrupted(t *testing.T) {
	e := joinFixture(t)
	exec(t, e.ws("demo"), "CREATE (:Leftover)")
	_, err := e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "graph")
	// Every source frozen to this target and no entry for it: a resume.
	e.mark(t, "demo-api", map[string]any{"joining": "demo"})
	_, err = e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "graph")
	e.mark(t, "demo-web", map[string]any{"joining": "demo"})
	if _, err := e.b.PreviewJoin(joinReq); err != nil {
		t.Fatalf("a resumed join is previewed: %v", err)
	}
}

func TestAFrozenSourceWithoutAHalfBuiltTargetIsPreviewedToo(t *testing.T) {
	e := joinFixture(t)
	e.mark(t, "demo-api", map[string]any{"joining": "demo"})
	if _, err := e.b.PreviewJoin(joinReq); err != nil {
		t.Fatalf("a source frozen by an interrupted join: %v", err)
	}
}

func TestSourcesMustExistAndNotBeJoinedOrJoiningElsewhere(t *testing.T) {
	e := joinFixture(t)
	_, err := e.b.PreviewJoin(req("demo-api", "nope"))
	wantJoinRefusal(t, err, board.NotFound, `no workspace "nope"`)

	e.mark(t, "demo-web", map[string]any{"joining": "other"})
	_, err = e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "demo-web is being joined into other")

	e.mark(t, "demo-web", map[string]any{"joining": nil, "joined_into": "other", "joined_repos": []string{"web"}})
	_, err = e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "demo-web was joined into other")
}

func TestAnotherJoinBuildingTheTargetRefuses(t *testing.T) {
	e := joinFixture(t)
	if _, err := e.b.CreateWorkspace(actor, board.Workspace{Slug: "demo-x", Name: "X"}); err != nil {
		t.Fatal(err)
	}
	e.mark(t, "demo-x", map[string]any{"joining": "demo"})
	_, err := e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "demo-x")
}

func TestASourceMustBeOnTheCurrentSchema(t *testing.T) {
	e := joinFixture(t)
	exec(t, e.ws("demo-web"), "MATCH (s:SchemaVersion) SET s.v = 3")
	_, err := e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "demo-web is not on the current schema")
}

func TestARegisteredSourceWithNoGraphIsRefusedWithoutCreatingOne(t *testing.T) {
	e := joinFixture(t)
	exec(t, e.system(), "CREATE (:WorkspaceEntry {slug: 'demo-ghost', name: 'Ghost'})")
	_, err := e.b.PreviewJoin(req("demo-api", "demo-ghost"))
	wantJoinRefusal(t, err, board.Conflict, "demo-ghost has no graph")
	names, _ := e.db.ListGraphs()
	if slices.Contains(names, e.prefix+"ws:demo-ghost") {
		t.Fatal("the preview created a graph")
	}
}

func TestSourcesCannotShareARepositoryName(t *testing.T) {
	e := newJoinEnv(t)
	for _, s := range []string{"demo-api", "demo-web"} {
		if _, err := e.b.CreateWorkspace(actor, board.Workspace{Slug: s, Name: s, RepoName: "app"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, `repository "app" is in demo-api and demo-web`)
}

func TestOnlyOneSourceMayHoldTheBeforeReleasesRelease(t *testing.T) {
	e := joinFixture(t)
	exec(t, e.ws("demo-api"), "CREATE (:Release {id: 'R-0', version: 'Before releases'})")
	if _, err := e.b.PreviewJoin(joinReq); err != nil {
		t.Fatalf("one R-0 is fine: %v", err)
	}
	exec(t, e.ws("demo-web"), "CREATE (:Release {id: 'R-0', version: 'Before releases'})")
	_, err := e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "R-0")
}

func TestAnUnknownLabelOrEdgeIsRefusedByName(t *testing.T) {
	e := joinFixture(t)
	exec(t, e.ws("demo-web"), "CREATE (:Mystery {x: 1})")
	_, err := e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "Mystery")

	e = joinFixture(t)
	exec(t, e.ws("demo-api"), "MATCH (s:Story {id: 'S-1'}), (t:Story {id: 'S-2'}) CREATE (s)-[:MYSTERY_LINK]->(t)")
	_, err = e.b.PreviewJoin(joinReq)
	wantJoinRefusal(t, err, board.Conflict, "MYSTERY_LINK")
}
