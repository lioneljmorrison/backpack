package httpapi

import (
	"net/http"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/health"
	"github.com/lioneljmorrison/backpack/internal/search"
	"github.com/lioneljmorrison/backpack/internal/search/typesensetest"
)

// withSearch returns an API whose board, with workspace "demo", is indexed
// on every change.
func withSearch(t *testing.T) http.Handler {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	url, key, cprefix := typesensetest.Prefix(t)
	x := search.New(url, key, cprefix, b)
	b.OnChange(x.Report)
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	h := New(map[string]health.Check{}, tester, b, x, nil)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo"}`), http.StatusCreated)
	return h
}

func TestSearchAnswersWithTheMatches(t *testing.T) {
	h := withSearch(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "Export invoices as PDF"}`), http.StatusCreated)
	got := want(t, call(t, h, "GET", "/v1/workspaces/demo/search?q=invoices", ""), http.StatusOK)
	rs, _ := got["results"].([]any)
	if len(rs) != 1 {
		t.Fatalf("results %v", got)
	}
	if r := rs[0].(map[string]any); r["kind"] != "story" || r["id"] != "S-1" || r["state"] != "backlog" {
		t.Fatalf("result %v", r)
	}
}

func TestSearchWithNoMatchesAnswersWithAnEmptyList(t *testing.T) {
	h := withSearch(t)
	got := want(t, call(t, h, "GET", "/v1/workspaces/demo/search?q=invoices", ""), http.StatusOK)
	if rs, ok := got["results"].([]any); !ok || len(rs) != 0 {
		t.Fatalf("results %v", got)
	}
}

func TestSearchNeedsAQuery(t *testing.T) {
	h := withSearch(t)
	want(t, call(t, h, "GET", "/v1/workspaces/demo/search?q=", ""), http.StatusBadRequest)
}

func TestSearchingAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := withSearch(t)
	want(t, call(t, h, "GET", "/v1/workspaces/nope/search?q=invoices", ""), http.StatusNotFound)
}

func TestSearchWithTypesenseUnreachableIsUnavailable(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	h := New(map[string]health.Check{}, tester, b, search.New("http://127.0.0.1:1", "none", "nothing_", b), nil)
	got := want(t, call(t, h, "GET", "/v1/workspaces/demo/search?q=invoices", ""), http.StatusServiceUnavailable)
	if got["error"] != "search unavailable" {
		t.Fatalf("answer %v", got)
	}
}

func TestSearchGivenAStoryLooksNearItFirstAndSaysSo(t *testing.T) {
	h := withSearch(t)
	for _, title := range []string{"Invoices in the work", "Invoices dependency", "Invoices far away", "Gardening notes about tomatoes"} {
		want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "`+title+`"}`), http.StatusCreated)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-2/dependencies", `{"on": "S-1"}`), http.StatusOK)
	got := want(t, call(t, h, "GET", "/v1/workspaces/demo/search?q=invoices&story=S-1", ""), http.StatusOK)
	if rs, _ := got["results"].([]any); len(rs) != 2 || got["scope"] != "story" {
		t.Fatalf("answer %v", got)
	}
	got = want(t, call(t, h, "GET", "/v1/workspaces/demo/search?q=tomatoes&story=S-1", ""), http.StatusOK)
	if rs, _ := got["results"].([]any); len(rs) != 1 || got["scope"] != "workspace" {
		t.Fatalf("answer %v", got)
	}
}

func TestSearchWithoutAStoryNamesNoScope(t *testing.T) {
	h := withSearch(t)
	got := want(t, call(t, h, "GET", "/v1/workspaces/demo/search?q=invoices", ""), http.StatusOK)
	if _, ok := got["scope"]; ok {
		t.Fatalf("answer %v", got)
	}
}

func TestSearchNearAnUnknownStoryIsNotFound(t *testing.T) {
	h := withSearch(t)
	want(t, call(t, h, "GET", "/v1/workspaces/demo/search?q=invoices&story=S-99", ""), http.StatusNotFound)
}
