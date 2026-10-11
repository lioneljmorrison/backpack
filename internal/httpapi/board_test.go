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

// newAPI serves a real, empty board. Every request is made as "tester".
func newAPI(t *testing.T) http.Handler {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	return New(map[string]health.Check{}, tester, b)
}

func call(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer bp_good")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func want(t *testing.T, w *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d; body %s", w.Code, status, w.Body)
	}
	if status == http.StatusNoContent {
		return nil
	}
	return decode(t, w)
}

// withDemo returns an API whose board has workspace "demo".
func withDemo(t *testing.T) http.Handler {
	t.Helper()
	h := newAPI(t)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo"}`), http.StatusCreated)
	return h
}

func TestCreatingAWorkspaceRecordsTheTokensActor(t *testing.T) {
	h := newAPI(t)
	ws := want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo"}`), http.StatusCreated)
	if ws["slug"] != "demo" || ws["created_by"] != "tester" {
		t.Fatalf("created %v", ws)
	}
	list := want(t, call(t, h, "GET", "/v1/workspaces", ""), http.StatusOK)
	if got := list["workspaces"].([]any); len(got) != 1 {
		t.Fatalf("listed %v", got)
	}
}

func TestAWorkspaceCanBeCreatedWithAReleaseStage(t *testing.T) {
	h := newAPI(t)
	ws := want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo", "has_release_stage": true}`), http.StatusCreated)
	if ws["has_release_stage"] != true {
		t.Fatalf("created %v", ws)
	}
}

func TestMalformedBodiesAreBadRequests(t *testing.T) {
	h := withDemo(t)
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/v1/workspaces", `{"slug": `},
		{"POST", "/v1/workspaces", `{"slug": "x", "name": "x", "colour": "red"}`},
		{"POST", "/v1/workspaces/demo/stories", `[]`},
		{"PATCH", "/v1/workspaces/demo/phases/one", `{}`},
	} {
		if w := call(t, h, c.method, c.path, c.body); w.Code != http.StatusBadRequest {
			t.Errorf("%s %s %s: status %d, want 400", c.method, c.path, c.body, w.Code)
		}
	}
}

func TestRefusalsMapToStatusCodes(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "Bad Slug", "name": "x"}`), http.StatusBadRequest)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "again"}`), http.StatusConflict)
	want(t, call(t, h, "GET", "/v1/workspaces/nope/stories", ""), http.StatusNotFound)
	want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-1", ""), http.StatusNotFound)
}

func TestPhasesAndGoalsOverHTTP(t *testing.T) {
	h := withDemo(t)
	p := want(t, call(t, h, "POST", "/v1/workspaces/demo/phases", `{"title": "One", "description": "first"}`), http.StatusCreated)
	if p["number"] != float64(1) {
		t.Fatalf("added %v", p)
	}
	p = want(t, call(t, h, "PATCH", "/v1/workspaces/demo/phases/1", `{"title": "Uno"}`), http.StatusOK)
	if p["title"] != "Uno" || p["description"] != "first" {
		t.Fatalf("patched %v", p)
	}
	g := want(t, call(t, h, "POST", "/v1/workspaces/demo/phases/1/goals", `{"text": "it works"}`), http.StatusCreated)
	if g["id"] != "G-1" {
		t.Fatalf("goal %v", g)
	}
	list := want(t, call(t, h, "GET", "/v1/workspaces/demo/phases", ""), http.StatusOK)
	phases := list["phases"].([]any)
	if len(phases) != 1 || len(phases[0].(map[string]any)["goals"].([]any)) != 1 {
		t.Fatalf("listed %v", phases)
	}
}

func TestStoriesOverHTTP(t *testing.T) {
	h := withDemo(t)
	s := want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A", "goal": "g"}`), http.StatusCreated)
	if s["id"] != "S-1" || s["status"] != "backlog" || s["created_by"] != "tester" {
		t.Fatalf("added %v", s)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "B", "before": "S-1"}`), http.StatusCreated)
	s = want(t, call(t, h, "PATCH", "/v1/workspaces/demo/stories/S-1", `{"plan": "do it"}`), http.StatusOK)
	if s["plan"] != "do it" || s["goal"] != "g" {
		t.Fatalf("patched %v", s)
	}
	s = want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/move", `{"before": "S-2"}`), http.StatusOK)
	if s["moved_by"] != "tester" {
		t.Fatalf("moved %v", s)
	}
	list := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories", ""), http.StatusOK)
	stories := list["stories"].([]any)
	if len(stories) != 2 || stories[0].(map[string]any)["id"] != "S-1" {
		t.Fatalf("listed %v", stories)
	}
	got := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-2", ""), http.StatusOK)
	if got["title"] != "B" {
		t.Fatalf("read %v", got)
	}
}

func TestDependenciesOverHTTP(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "B"}`), http.StatusCreated)

	s := want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-2/dependencies", `{"on": "S-1"}`), http.StatusOK)
	if by := s["blocked_by"].([]any); len(by) != 1 || by[0] != "S-1" {
		t.Fatalf("S-2 %v", s)
	}
	e := want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-2/move", `{"before": "S-1"}`), http.StatusConflict)
	if !strings.Contains(e["error"].(string), "S-1") {
		t.Fatalf("the refusal does not name the blocking story: %v", e)
	}
	want(t, call(t, h, "DELETE", "/v1/workspaces/demo/stories/S-2/dependencies/S-1", ""), http.StatusNoContent)
	want(t, call(t, h, "DELETE", "/v1/workspaces/demo/stories/S-2/dependencies/S-1", ""), http.StatusNotFound)
}

func TestStatusChangesOverHTTP(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "B"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-2/dependencies", `{"on": "S-1"}`), http.StatusOK)

	s := want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/status", `{"to": "in_progress", "branch": "feature/a"}`), http.StatusOK)
	if s["status"] != "in_progress" || s["branch"] != "feature/a" || s["activity"] != "planning" {
		t.Fatalf("started %v", s)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-2/status", `{"to": "in_progress"}`), http.StatusConflict)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/status", `{"to": "closed"}`), http.StatusBadRequest)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-9/status", `{"to": "in_progress"}`), http.StatusNotFound)
	s = want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-2", ""), http.StatusOK)
	if s["blocked"] != true {
		t.Fatalf("S-2 %v", s)
	}
}
