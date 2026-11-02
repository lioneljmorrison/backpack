package web

import (
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/live"
)

// The release pages name a release's repository only when the workspace has
// several. web is written straight into the graph, as no action adds a
// repository yet.
func TestReleasePagesShowTheRepositoryOnlyWhenThereAreSeveral(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	hub := live.New()
	b.OnChange(hub.Publish)
	h := New(known, b, nil, hub)
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo", RepoName: "apirepo"}))
	shipped(t, b, "Shipped")
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1.0.0", Tag: "v1.0.0"}))
	for _, path := range []string{"/w/demo/releases", "/w/demo/releases/R-1"} {
		if body := get(h, path, "bp_good").Body.String(); strings.Contains(body, "apirepo") {
			t.Errorf("%s names the repository of a one-repository workspace", path)
		}
	}

	if _, err := db.SelectGraph(prefix+"ws:demo").Query(`CREATE (r:Repo {name: 'webrepo', created_by: 'test', created_at: 'x'}),
		(r)-[:HAS_BRANCH]->(:Branch {name: 'main', created_by: 'test', created_at: 'x'}),
		(r)-[:HOLDS]->(:Story {id: 'W-1', title: 'Web', status: 'merged', into: 'main', rank: 'n'})`, nil, nil); err != nil {
		t.Fatal(err)
	}
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Repo: "webrepo", Version: "v1.0.0", Tag: "v1.0.0"}))
	list := get(h, "/w/demo/releases", "bp_good").Body.String()
	if !strings.Contains(list, "apirepo") || !strings.Contains(list, "webrepo") {
		t.Errorf("the list lacks the repositories: %s", list)
	}
	if one := get(h, "/w/demo/releases/R-2", "bp_good").Body.String(); !strings.Contains(one, "webrepo") {
		t.Error("the release page lacks its repository")
	}
}
