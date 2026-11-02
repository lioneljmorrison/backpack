package board_test

import (
	"strings"
	"testing"

	"github.com/FalkorDB/falkordb-go/v2"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// count runs a query that returns one integer.
func count(t *testing.T, g *falkordb.Graph, q string) int {
	t.Helper()
	res, err := g.ROQuery(q, nil, nil)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	if !res.Next() {
		t.Fatalf("%s: no row", q)
	}
	v, _ := res.Record().GetByIndex(0)
	return int(v.(int64))
}

// unlinked counts the branches, stories and releases that no repository holds.
func unlinked(t *testing.T, g *falkordb.Graph) int {
	t.Helper()
	return count(t, g, `
		OPTIONAL MATCH (b:Branch) WHERE NOT ()-[:HAS_BRANCH]->(b)
		WITH count(b) AS bs
		OPTIONAL MATCH (s:Story) WHERE NOT ()-[:HOLDS]->(s)
		WITH bs, count(s) AS ss
		OPTIONAL MATCH (r:Release) WHERE NOT ()-[:HAS_RELEASE]->(r)
		WITH bs, ss, count(r) AS rs
		RETURN bs + ss + rs`)
}

func reposOf(t *testing.T, b *board.Board, ws string) []board.Repo {
	t.Helper()
	all, err := b.Workspaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range all {
		if w.Slug == ws {
			return w.Repos
		}
	}
	t.Fatalf("no workspace %q", ws)
	return nil
}

// A graph at the version before repositories holds branches, stories and R-0
// with no repository. Starting up gives it one, named after the workspace and
// with the registry's URL, and links everything to it.
func TestAWorkspaceFromBeforeRepositoriesIsMigratedAtStartUp(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", RepoURL: "https://example.com/demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	addStory(t, b, board.NewStory{Title: "One"})
	addStory(t, b, board.NewStory{Title: "Old"})
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	g := db.SelectGraph(prefix + "ws:demo")
	// S-2 stands for a story released before releases were recorded, in R-0.
	for _, q := range []string{
		"MATCH (s:Story {id: 'S-2'}) SET s.status = 'released'",
		"MATCH (s:Story {id: 'S-2'}) CREATE (s)-[:IN_RELEASE]->(:Release {id: 'R-0', version: 'Before releases', created_by: 'migration', created_at: 'x'})",
		// The graph as the previous version left it: no repository, version 10.
		"MATCH (r:Repo) DETACH DELETE r",
		"MATCH (s:SchemaVersion) SET s.v = 10",
	} {
		if _, err := g.Query(q, nil, nil); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if unlinked(t, g) != 5 { // dev, main, S-1, S-2, R-0
		t.Fatalf("setup: %d unlinked, want 5", unlinked(t, g))
	}

	b, err = board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if v, _ := falkor.Version(g); v != 11 {
		t.Fatalf("version %d, want 11", v)
	}
	if got := reposOf(t, b, "demo"); len(got) != 1 || got[0].Name != "demo" || got[0].URL != "https://example.com/demo" {
		t.Fatalf("repositories %+v", got)
	}
	if n := unlinked(t, g); n != 0 {
		t.Fatalf("%d branches, stories or releases left unlinked", n)
	}
	s, err := b.Story("demo", "S-2")
	if err != nil || s.Repo != "demo" {
		t.Fatalf("story %+v, %v", s, err)
	}
	bs, _ := b.Branches("demo", "")
	if len(bs) != 2 || bs[0].Repo != "demo" || bs[1].Repo != "demo" {
		t.Fatalf("branches %+v", bs)
	}
	r0, err := b.GetRelease("demo", "R-0")
	if err != nil || r0.Repo != "demo" {
		t.Fatalf("R-0 %+v, %v", r0, err)
	}

	// Running again links nothing and makes no second repository.
	var before = count(t, g, "MATCH ()-[e]->() RETURN count(e)")
	if _, err := g.Query("MATCH (s:SchemaVersion) SET s.v = 10", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := board.New(db, prefix+"system", prefix+"ws:"); err != nil {
		t.Fatalf("second restart: %v", err)
	}
	if after := count(t, g, "MATCH ()-[e]->() RETURN count(e)"); after != before {
		t.Fatalf("a repeat changed the edges: %d, then %d", before, after)
	}
	if n := count(t, g, "MATCH (r:Repo) RETURN count(r)"); n != 1 {
		t.Fatalf("%d repositories after a repeat, want 1", n)
	}
}

func TestANewWorkspaceGetsOneRepositoryNamedAfterItsSlug(t *testing.T) {
	b := newBoard(t)
	w, err := b.CreateWorkspace(actor, board.Workspace{Slug: "my-app", Name: "My app", RepoURL: "https://example.com/my-app", HasReleaseStage: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []board.Repo{{Name: "my-app", URL: "https://example.com/my-app"}}
	if len(w.Repos) != 1 || w.Repos[0] != want[0] {
		t.Fatalf("created %+v", w.Repos)
	}
	if got := reposOf(t, b, "my-app"); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("listed %+v", got)
	}
	bs, _ := b.Branches("my-app", "")
	if len(bs) != 2 || bs[0].Repo != "my-app" || bs[1].Repo != "my-app" {
		t.Fatalf("branches %+v", bs)
	}
}

func TestAWorkspaceCanBeCreatedWithANamedRepository(t *testing.T) {
	b := newBoard(t)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "shop", Name: "Shop", RepoName: "storefront"}); err != nil {
		t.Fatal(err)
	}
	got := reposOf(t, b, "shop")
	if len(got) != 1 || got[0].Name != "storefront" || got[0].URL != "" {
		t.Fatalf("repositories %+v", got)
	}
}

func TestARepositoryNameFollowsTheSlugRule(t *testing.T) {
	b := newBoard(t)
	for _, name := range []string{"Has-Caps", "has space", "-lead", strings.Repeat("a", 65)} {
		_, err := b.CreateWorkspace(actor, board.Workspace{Slug: "ok", Name: "x", RepoName: name})
		wantKind(t, err, board.Invalid)
	}
	if got, _ := b.Workspaces(); len(got) != 0 {
		t.Fatalf("a refused create left %+v", got)
	}
}

// Repeating an interrupted create over a graph that already holds a
// repository keeps one.
func TestRepeatingACreateKeepsOneRepository(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	g := db.SelectGraph(prefix + "ws:demo")
	if _, err := g.Query("CREATE (:Workspace {slug: 'demo', next_story: 1, next_goal: 1})", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, g, "MATCH (r:Repo) RETURN count(r)"); n != 1 {
		t.Fatalf("%d repositories, want 1", n)
	}
	if n := unlinked(t, g); n != 0 {
		t.Fatalf("%d unlinked", n)
	}
}

func TestStoriesBranchesAndReleasesAddedLaterJoinTheRepository(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, _ := board.New(db, prefix+"system", prefix+"ws:")
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	g := db.SelectGraph(prefix + "ws:demo")
	s := addStory(t, b, board.NewStory{Title: "A"})
	if s.Repo != "demo" {
		t.Fatalf("new story in repo %q", s.Repo)
	}
	prop := addStory(t, b, board.NewStory{Title: "Proposed", Proposed: true, Reasoning: "why"})
	if prop.Repo != "demo" {
		t.Fatalf("proposed story in repo %q", prop.Repo)
	}
	br := mustAddBranch(t, b, "hotfix", "main")
	if br.Repo != "demo" {
		t.Fatalf("new branch in repo %q", br.Repo)
	}
	walk(t, b, "S-1", "in_progress", "in_review")
	setStatus(t, b, "S-1", board.StatusChange{To: "merged", Into: "dev"})
	setStatus(t, b, "S-1", board.StatusChange{To: "merged", Into: "main"})
	rel, err := b.CutRelease("demo", actor, board.NewRelease{Version: "1.0.0", Tag: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if rel.Repo != "demo" {
		t.Fatalf("new release in repo %q", rel.Repo)
	}
	if n := unlinked(t, g); n != 0 {
		t.Fatalf("%d unlinked", n)
	}
	if n := count(t, g, "MATCH (:Repo)-[e]->() RETURN count(e)"); n != 6 { // dev, main, hotfix, 2 stories, R-1
		t.Fatalf("%d edges from the repository, want 6", n)
	}
}

// A workspace with one repository reads exactly as it did before there were
// repositories: the same where-are-we text and the same board columns.
func TestAOneRepositoryWorkspaceReadsAsItDidBeforeRepositories(t *testing.T) {
	b := whereBoard(t)
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	const wantText = `Phase 2: Features — 0 of 8 stories complete
In progress:
  S-2 Building — waiting on the test agent
  S-3 Mocked — planning; blocked by S-2, working around it: mock the S-2 API
In review:
  S-4 Reviewing — https://example.com/pr/4
On hold:
  S-5 Unclear — needs more info (was in backlog)
Merged, waiting for a release: 1 (latest: S-1 Done)
Next:
  S-6 Next one
  S-7 Then — blocked by S-6
  S-8 After
`
	if got := w.Text(); got != wantText {
		t.Fatalf("where are we changed:\n%s\nwant:\n%s", got, wantText)
	}
	cols, err := b.Board("demo")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"proposed||", "on_hold||S-5", "backlog||S-6 S-7 S-8 S-9", "in_progress||S-2 S-3",
		"in_review||S-4", "merged|main|S-1", "released||", "closed||"}
	var got []string
	for _, c := range cols {
		got = append(got, c.Status+"|"+c.Branch+"|"+strings.Join(ids(c.Stories), " "))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("columns changed:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
