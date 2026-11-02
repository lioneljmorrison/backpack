package httpapi

import (
	"net/http"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/health"
)

func TestNextTakesARepositoryAndAnswersItsTopReadyStory(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	h := New(map[string]health.Check{}, tester, b, nil, nil)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo", "repo": "api"}`), http.StatusCreated)
	if _, err := db.SelectGraph(prefix+"ws:demo").Query(`CREATE (r:Repo {name: 'web', created_by: 'test', created_at: 'x'}),
		(r)-[:HAS_BRANCH]->(:Branch {name: 'main', created_by: 'test', created_at: 'x'})`, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{"api", "web"} {
		want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "In `+repo+`", "repo": "`+repo+`"}`), http.StatusCreated)
	}
	const next = "/v1/workspaces/demo/next"

	if s := want(t, call(t, h, "GET", next, ""), http.StatusOK)["story"].(map[string]any); s["id"] != "S-1" {
		t.Fatalf("whole workspace %v", s)
	}
	if s := want(t, call(t, h, "GET", next+"?repo=web", ""), http.StatusOK)["story"].(map[string]any); s["id"] != "S-2" || s["repo"] != "web" {
		t.Fatalf("web %v", s)
	}
	r := want(t, call(t, h, "GET", next+"?repo=ios", ""), http.StatusNotFound)
	if r["error"] != `demo has no repo "ios"` {
		t.Fatalf("refusal %v", r)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-2/status", `{"to": "in_progress"}`), http.StatusOK)
	if got := want(t, call(t, h, "GET", next+"?repo=web", ""), http.StatusOK); got["story"] != nil {
		t.Fatalf("nothing ready in web: %v", got)
	}
}
