package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/health"
)

func TestStatusTakesARepositoryAndNarrowsOnlyNext(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	h := New(map[string]health.Check{}, tester, b, nil, nil)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo", "repo": "api"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/repos", `{"name": "web"}`), http.StatusCreated)
	for _, repo := range []string{"api", "web"} {
		want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "In `+repo+`", "repo": "`+repo+`"}`), http.StatusCreated)
	}
	text := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer bp_good")
		r.Header.Set("Accept", "text/plain")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	const status = "/v1/workspaces/demo/status"

	if r := text(status); r.Code != http.StatusOK || r.Body.String() != "Next:\n  S-1 [api] In api\n  S-2 [web] In web\n" {
		t.Fatalf("whole workspace %d %q", r.Code, r.Body)
	}
	if r := text(status + "?repo=web"); r.Code != http.StatusOK || r.Body.String() != "Next:\n  S-2 [web] In web\n" {
		t.Fatalf("web %d %q", r.Code, r.Body)
	}
	if r := text(status + "?repo=ios"); r.Code != http.StatusNotFound || !strings.Contains(r.Body.String(), `demo has no repo \"ios\"`) {
		t.Fatalf("unknown repo %d %q", r.Code, r.Body)
	}
	// As JSON, the same narrowing.
	got := want(t, call(t, h, "GET", status+"?repo=api", ""), http.StatusOK)
	if next := got["next"].([]any); len(next) != 1 || next[0].(map[string]any)["id"] != "S-1" {
		t.Fatalf("json next %v", got["next"])
	}
}
