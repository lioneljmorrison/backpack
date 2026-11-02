package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/health"
)

func TestAWorkspaceIsCreatedWithAnOptionalRepoAndListsItsRepositories(t *testing.T) {
	h := withDemo(t)
	made := want(t, call(t, h, "POST", "/v1/workspaces",
		`{"slug": "shop", "name": "Shop", "repo": "storefront", "repo_url": "https://example.com/shop"}`), http.StatusCreated)
	repos := made["repos"].([]any)
	if r := repos[0].(map[string]any); len(repos) != 1 || r["name"] != "storefront" || r["url"] != "https://example.com/shop" {
		t.Fatalf("created %v", made)
	}
	if _, ok := made["repo_url"]; ok {
		t.Fatalf("the single URL is gone from the answer: %v", made)
	}
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "bad", "name": "Bad", "repo": "Not A Slug"}`), http.StatusBadRequest)

	list := want(t, call(t, h, "GET", "/v1/workspaces", ""), http.StatusOK)["workspaces"].([]any)
	demo := list[0].(map[string]any)["repos"].([]any)
	if r := demo[0].(map[string]any); len(demo) != 1 || r["name"] != "demo" {
		t.Fatalf("demo's repositories %v", demo)
	}

	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusCreated)
	s := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-1", ""), http.StatusOK)
	if s["repo"] != "demo" {
		t.Fatalf("story %v", s)
	}
}

// twoRepoAPI serves workspace "demo" with repositories api (dev into main)
// and web (dev into main, a story merged into dev). The invented story W-1 is
// written straight into the graph, because no route puts a story in a status
// and branch in one step.
func twoRepoAPI(t *testing.T) http.Handler {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	h := New(map[string]health.Check{}, tester, b, nil, nil)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo", "repo": "api", "has_release_stage": true}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/repos", `{"name": "web"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/branches", `{"repo": "web", "name": "dev", "into": "main"}`), http.StatusCreated)
	if _, err := db.SelectGraph(prefix+"ws:demo").Query(`MATCH (r:Repo {name: 'web'})
		CREATE (r)-[:HOLDS]->(:Story {id: 'W-1', title: 'Web', status: 'merged', into: 'dev', rank: 'n'})`, nil, nil); err != nil {
		t.Fatal(err)
	}
	return h
}

func branchLayout(t *testing.T, h http.Handler, query string) string {
	t.Helper()
	var out []string
	for _, x := range want(t, call(t, h, "GET", "/v1/workspaces/demo/branches"+query, ""), http.StatusOK)["branches"].([]any) {
		b := x.(map[string]any)
		out = append(out, b["repo"].(string)+"/"+b["name"].(string))
	}
	return strings.Join(out, " ")
}

func TestBranchRoutesTakeARepositoryAndNeedOneWhenThereAreSeveral(t *testing.T) {
	h := twoRepoAPI(t)
	const base = "/v1/workspaces/demo/branches"

	// GET lists every repository's branches, or one's.
	if got := branchLayout(t, h, ""); got != "api/dev api/main web/dev web/main" {
		t.Fatalf("all: %s", got)
	}
	if got := branchLayout(t, h, "?repo=web"); got != "web/dev web/main" {
		t.Fatalf("web: %s", got)
	}
	want(t, call(t, h, "GET", base+"?repo=x", ""), http.StatusNotFound)

	// POST: repo in the body.
	want(t, call(t, h, "POST", base, `{"name": "hotfix", "into": "main"}`), http.StatusBadRequest)
	added := want(t, call(t, h, "POST", base, `{"name": "hotfix", "into": "main", "repo": "web"}`), http.StatusCreated)
	if added["repo"] != "web" {
		t.Fatalf("added %v", added)
	}
	want(t, call(t, h, "POST", base, `{"name": "hotfix", "into": "main", "repo": "x"}`), http.StatusNotFound)

	// PATCH: ?repo=.
	r := want(t, call(t, h, "PATCH", base+"/hotfix", `{"alias": "Fix"}`), http.StatusBadRequest)
	if msg := r["error"]; msg != "demo has several repos; name the repo: api or web" {
		t.Fatalf("refusal %v", r)
	}
	if got := want(t, call(t, h, "PATCH", base+"/hotfix?repo=web", `{"alias": "Fix"}`), http.StatusOK); got["alias"] != "Fix" || got["repo"] != "web" {
		t.Fatalf("patched %v", got)
	}
	want(t, call(t, h, "PATCH", base+"/hotfix?repo=api", `{"alias": "Fix"}`), http.StatusNotFound)

	// POST .../merged: ?repo=.
	want(t, call(t, h, "POST", base+"/dev/merged", `{"commit": "x9"}`), http.StatusBadRequest)
	moved := want(t, call(t, h, "POST", base+"/dev/merged?repo=web", `{"commit": "x9"}`), http.StatusOK)["stories"].([]any)
	if len(moved) != 1 || moved[0].(map[string]any)["id"] != "W-1" {
		t.Fatalf("moved %v", moved)
	}
	if moved = want(t, call(t, h, "POST", base+"/dev/merged?repo=api", `{"commit": "x9"}`), http.StatusOK)["stories"].([]any); len(moved) != 0 {
		t.Fatalf("api moved %v", moved)
	}
	want(t, call(t, h, "POST", base+"/dev/merged?repo=x", `{"commit": "x9"}`), http.StatusNotFound)

	// DELETE: ?repo=.
	want(t, call(t, h, "DELETE", base+"/hotfix", ""), http.StatusBadRequest)
	want(t, call(t, h, "DELETE", base+"/hotfix?repo=x", ""), http.StatusNotFound)
	want(t, call(t, h, "DELETE", base+"/main?repo=web", ""), http.StatusConflict)
	want(t, call(t, h, "DELETE", base+"/hotfix?repo=web", ""), http.StatusNoContent)
	if got := branchLayout(t, h, ""); got != "api/dev api/main web/dev web/main" {
		t.Fatalf("after: %s", got)
	}
}

func TestAddingAStoryTakesARepositoryAndNeedsOneWhenThereAreSeveral(t *testing.T) {
	h := twoRepoAPI(t)
	const p = "/v1/workspaces/demo/stories"
	r := want(t, call(t, h, "POST", p, `{"title": "Which"}`), http.StatusBadRequest)
	if r["error"] != "demo has several repos; name the repo: api or web" {
		t.Fatalf("refusal %v", r)
	}
	want(t, call(t, h, "POST", p, `{"title": "Nowhere", "repo": "ios"}`), http.StatusNotFound)
	s := want(t, call(t, h, "POST", p, `{"title": "Web one", "repo": "web"}`), http.StatusCreated)
	if s["repo"] != "web" {
		t.Fatalf("story %v", s)
	}
	// The repository is fixed at creation: an update cannot carry one.
	want(t, call(t, h, "PATCH", p+"/"+s["id"].(string), `{"repo": "api"}`), http.StatusBadRequest)
	if got := want(t, call(t, h, "GET", p+"/"+s["id"].(string), ""), http.StatusOK); got["repo"] != "web" {
		t.Fatalf("read back %v", got)
	}
}
