package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/health"
)

// Fixtures are invented: demo-api is joined into demo-web, demo is being joined.
func withJoined(t *testing.T) http.Handler {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	h := New(map[string]health.Check{}, tester, b, nil, nil)
	for _, s := range []string{"demo", "demo-api", "demo-web"} {
		want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "`+s+`", "name": "`+s+`"}`), http.StatusCreated)
	}
	falkortest.MarkEntry(t, db, prefix+"system", "demo-api", map[string]any{"joined_into": "demo-web", "joined_repos": []string{"api"}})
	falkortest.MarkEntry(t, db, prefix+"system", "demo", map[string]any{"joining": "demo-web"})
	return h
}

func TestAJoinedWorkspaceIs404WithAPointerOverHTTP(t *testing.T) {
	h := withJoined(t)
	const pointer = "demo-api was joined into demo-web; use workspace demo-web with repo api"
	for _, c := range [][2]string{
		{"GET", "/v1/workspaces/demo-api/stories/S-1"},
		{"GET", "/v1/workspaces/demo-api/status"},
		{"POST", "/v1/workspaces/demo-api/stories"},
	} {
		w := call(t, h, c[0], c[1], `{"title": "A"}`)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), pointer) {
			t.Fatalf("%s %s: %d %s", c[0], c[1], w.Code, w.Body.String())
		}
	}
	w := call(t, h, "GET", "/v1/workspaces/demo/status", "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "demo is being joined into demo-web; try again shortly") {
		t.Fatalf("being joined: %d %s", w.Code, w.Body.String())
	}
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo-api", "name": "Again"}`), http.StatusConflict)
	list := want(t, call(t, h, "GET", "/v1/workspaces", ""), http.StatusOK)["workspaces"].([]any)
	if len(list) != 2 {
		t.Fatalf("listed %v", list)
	}
}
