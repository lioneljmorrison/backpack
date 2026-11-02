package mcpserver_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// releasing serves workspace "demo" with repositories api and web, each with
// dev merging into main. api holds S-1 and web holds W-1, both merged into
// main. The story is added before web exists, because adding to a chosen
// repository is S-70; web is written straight into the graph.
func releasing(t *testing.T) *mcp.ClientSession {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo", RepoName: "api", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	s, err := b.AddStory("demo", "tester", board.NewStory{Title: "Api"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []board.StatusChange{{To: "in_progress"}, {To: "in_review"}, {To: "merged", Commit: "c1"}, {To: "merged", Into: "main", Commit: "c2"}} {
		if _, err := b.SetStatus("demo", "tester", s.ID, c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.SelectGraph(prefix+"ws:demo").Query(`CREATE (r:Repo {name: 'web', created_by: 'test', created_at: 'x'}),
		(r)-[:HAS_BRANCH]->(:Branch {name: 'main', created_by: 'test', created_at: 'x'}),
		(r)-[:HAS_BRANCH]->(:Branch {name: 'dev', into: 'main', created_by: 'test', created_at: 'x'}),
		(r)-[:HOLDS]->(:Story {id: 'W-1', title: 'Web', status: 'merged', into: 'main', rank: 'n'})`, nil, nil); err != nil {
		t.Fatal(err)
	}
	return session(t, serve(t, b, nil))
}

func TestReleaseToolsTakeARepositoryAndNeedOneWhenThereAreSeveral(t *testing.T) {
	cs := releasing(t)
	const several = "demo has several repos; name the repo: api or web"

	// cut_release, through batch: the repository is named, and echoed.
	if got := refused(t, cs, "cut_release", map[string]any{"workspace": "demo", "version": "v1.0.0", "tag": "v1.0.0"}); got != several {
		t.Fatalf("cut without repo: %s", got)
	}
	if got := refused(t, cs, "cut_release", map[string]any{"workspace": "demo", "repo": "x", "version": "v1.0.0", "tag": "v1.0.0"}); got != `demo has no repo "x"` {
		t.Fatalf("cut in an unknown repo: %s", got)
	}
	web := use(t, cs, "cut_release", map[string]any{"workspace": "demo", "repo": "web", "version": "v1.0.0", "tag": "v1.0.0"})
	if web["repo"] != "web" || web["id"] != "R-1" || web["stories"] != float64(1) {
		t.Fatalf("web cut %v", web)
	}
	api := use(t, cs, "cut_release", map[string]any{"workspace": "demo", "repo": "api", "version": "v1.0.0", "tag": "v1.0.0"})
	if api["repo"] != "api" || api["id"] != "R-2" || api["stories"] != float64(1) {
		t.Fatalf("api cut %v", api)
	}
	if got := refused(t, cs, "cut_release", map[string]any{"workspace": "demo", "repo": "api", "version": "v1.0.0", "tag": "t"}); got != "api already has a release v1.0.0" {
		t.Fatalf("duplicate: %s", got)
	}

	// list_releases, through actions: every repository's, labelled, or one's.
	all := use(t, cs, "list_releases", map[string]any{"workspace": "demo"})["releases"].([]any)
	if len(all) != 2 || all[0].(map[string]any)["repo"] != "api" || all[1].(map[string]any)["repo"] != "web" {
		t.Fatalf("all %v", all)
	}
	one := use(t, cs, "list_releases", map[string]any{"workspace": "demo", "repo": "web"})["releases"].([]any)
	if len(one) != 1 || one[0].(map[string]any)["id"] != "R-1" || one[0].(map[string]any)["repo"] != "web" {
		t.Fatalf("web %v", one)
	}
	if got := refused(t, cs, "list_releases", map[string]any{"workspace": "demo", "repo": "x"}); got != `demo has no repo "x"` {
		t.Fatalf("unknown repo: %s", got)
	}

	// get_release shows the repository.
	if got := use(t, cs, "get_release", map[string]any{"workspace": "demo", "id": "R-1"}); got["repo"] != "web" {
		t.Fatalf("get_release %v", got)
	}
}

func TestReleasesOfASingleRepositoryAnswerWithoutRepoLabels(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	if _, err := b.MergeBranch("staged", "tester", "", "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	absent(t, use(t, cs, "cut_release", map[string]any{"workspace": "staged", "version": "v1.0.0", "tag": "v1.0.0"}), "repo")
	for _, x := range use(t, cs, "list_releases", map[string]any{"workspace": "staged"})["releases"].([]any) {
		absent(t, x.(map[string]any), "repo")
	}
	absent(t, use(t, cs, "get_release", map[string]any{"workspace": "staged", "id": "R-1"}), "repo")
}
