package httpapi

import (
	"net/http"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/health"
)

// releasingAPI serves workspace "demo" with repositories api and web. api
// holds S-1 and web holds W-1, both merged into main. The story is added
// before web exists, because adding to a chosen repository is S-70; web is
// written straight into the graph.
func releasingAPI(t *testing.T) http.Handler {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	h := New(map[string]health.Check{}, tester, b, nil, nil)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo", "repo": "api", "has_release_stage": true}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "Api"}`), http.StatusCreated)
	for _, c := range []string{`{"to": "in_progress"}`, `{"to": "in_review"}`, `{"to": "merged", "commit": "c1"}`, `{"to": "merged", "into": "main", "commit": "c2"}`} {
		want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/status", c), http.StatusOK)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/repos", `{"name": "web"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/branches", `{"repo": "web", "name": "dev", "into": "main"}`), http.StatusCreated)
	if _, err := db.SelectGraph(prefix+"ws:demo").Query(`MATCH (r:Repo {name: 'web'})
		CREATE (r)-[:HOLDS]->(:Story {id: 'W-1', title: 'Web', status: 'merged', into: 'main', rank: 'n'})`, nil, nil); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestReleaseRoutesTakeARepositoryAndNeedOneWhenThereAreSeveral(t *testing.T) {
	h := releasingAPI(t)
	const rp = "/v1/workspaces/demo/releases"

	// POST: repo in the body.
	r := want(t, call(t, h, "POST", rp, `{"version": "v1.0.0", "tag": "v1.0.0"}`), http.StatusBadRequest)
	if r["error"] != "demo has several repos; name the repo: api or web" {
		t.Fatalf("refusal %v", r)
	}
	want(t, call(t, h, "POST", rp, `{"repo": "x", "version": "v1.0.0", "tag": "v1.0.0"}`), http.StatusNotFound)
	web := want(t, call(t, h, "POST", rp, `{"repo": "web", "version": "v1.0.0", "tag": "v1.0.0"}`), http.StatusCreated)
	if web["repo"] != "web" || web["id"] != "R-1" {
		t.Fatalf("web release %v", web)
	}
	api := want(t, call(t, h, "POST", rp, `{"repo": "api", "version": "v1.0.0", "tag": "v1.0.0"}`), http.StatusCreated)
	if api["repo"] != "api" || api["id"] != "R-2" {
		t.Fatalf("api release %v", api)
	}
	want(t, call(t, h, "POST", rp, `{"repo": "api", "version": "v1.0.0", "tag": "v1.0.0"}`), http.StatusConflict)

	// GET lists every repository's releases, or one's.
	if all := want(t, call(t, h, "GET", rp, ""), http.StatusOK)["releases"].([]any); len(all) != 2 {
		t.Fatalf("all %v", all)
	}
	one := want(t, call(t, h, "GET", rp+"?repo=web", ""), http.StatusOK)["releases"].([]any)
	if len(one) != 1 || one[0].(map[string]any)["id"] != "R-1" || one[0].(map[string]any)["repo"] != "web" {
		t.Fatalf("web %v", one)
	}
	want(t, call(t, h, "GET", rp+"?repo=x", ""), http.StatusNotFound)

	if got := want(t, call(t, h, "GET", rp+"/R-1", ""), http.StatusOK); got["repo"] != "web" {
		t.Fatalf("R-1 %v", got)
	}
}
