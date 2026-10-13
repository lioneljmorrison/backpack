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
	h, _ := newAPIWithBoard(t)
	return h
}

// newAPIWithBoard is newAPI, and also the board behind it.
func newAPIWithBoard(t *testing.T) (http.Handler, *board.Board) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	tester := func(string) (string, bool, error) { return "tester", true, nil }
	return New(map[string]health.Check{}, tester, b, nil, nil), b
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

func TestStoryLogsOverHTTP(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusCreated)
	e := want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/log", `{"kind": "note", "text": "hello"}`), http.StatusCreated)
	if e["id"] != "L-1" || e["actor"] != "tester" {
		t.Fatalf("logged %v", e)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/log", `{"kind": "status", "text": "x"}`), http.StatusBadRequest)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/log", `{"kind": "correction", "text": "x", "corrects": "L-9"}`), http.StatusNotFound)
	l := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-1/log", ""), http.StatusOK)
	if entries := l["entries"].([]any); len(entries) != 1 {
		t.Fatalf("log %v", l)
	}
}

func TestMarkingAGoalMetOverHTTP(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/phases", `{"title": "One"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/phases/1/goals", `{"text": "it works"}`), http.StatusCreated)
	g := want(t, call(t, h, "PATCH", "/v1/workspaces/demo/goals/G-1", `{"met": true}`), http.StatusOK)
	if g["met"] != true || g["met_by"] != "tester" {
		t.Fatalf("met %v", g)
	}
	want(t, call(t, h, "PATCH", "/v1/workspaces/demo/goals/G-1", `{}`), http.StatusBadRequest)
	want(t, call(t, h, "PATCH", "/v1/workspaces/demo/goals/G-9", `{"met": true}`), http.StatusNotFound)
	p := want(t, call(t, h, "GET", "/v1/workspaces/demo/phases", ""), http.StatusOK)["phases"].([]any)[0].(map[string]any)
	if p["complete"] != true || p["goals_met_early"] != false { // nothing in it, goal met
		t.Fatalf("phase %v", p)
	}
}

func TestTheQuickReadsOverHTTP(t *testing.T) {
	h := withDemo(t)
	n := want(t, call(t, h, "GET", "/v1/workspaces/demo/next", ""), http.StatusOK)
	if s, ok := n["story"]; !ok || s != nil {
		t.Fatalf("next on an empty board %v", n)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "B"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/status", `{"to": "in_progress"}`), http.StatusOK)

	n = want(t, call(t, h, "GET", "/v1/workspaces/demo/next", ""), http.StatusOK)
	if n["story"].(map[string]any)["id"] != "S-2" {
		t.Fatalf("next %v", n)
	}
	st := want(t, call(t, h, "GET", "/v1/workspaces/demo/status", ""), http.StatusOK)
	if len(st["in_progress"].([]any)) != 1 || len(st["next"].([]any)) != 1 || len(st["proposed"].([]any)) != 0 {
		t.Fatalf("status %v", st)
	}
	b := want(t, call(t, h, "GET", "/v1/workspaces/demo/board", ""), http.StatusOK)
	if cols := b["columns"].([]any); len(cols) != 8 {
		t.Fatalf("board %v", b)
	}
	want(t, call(t, h, "GET", "/v1/workspaces/nope/status", ""), http.StatusNotFound)
}

func TestWhereAreWeAsPlainTextForTheSessionHook(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusCreated)
	r := httptest.NewRequest("GET", "/v1/workspaces/demo/status", nil)
	r.Header.Set("Authorization", "Bearer bp_good")
	r.Header.Set("Accept", "text/plain")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") ||
		w.Body.String() != "Next:\n  S-1 A\n" {
		t.Fatalf("status %d %q: %q", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
}

func TestWhereAreWeIsJSONWhenTheCallerAlsoAcceptsJSON(t *testing.T) {
	h := withDemo(t)
	for _, accept := range []string{"", "*/*", "application/json, text/plain, */*"} {
		r := httptest.NewRequest("GET", "/v1/workspaces/demo/status", nil)
		r.Header.Set("Authorization", "Bearer bp_good")
		r.Header.Set("Accept", accept)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Accept %q got %q", accept, ct)
		}
	}
}

func TestAProposalCanBeHeldAndResumedOverHTTP(t *testing.T) {
	h, b := newAPIWithBoard(t)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo"}`), http.StatusCreated)
	if _, err := b.AddStory("demo", "tester", board.NewStory{Title: "Maybe", Proposed: true, Reasoning: "it might help"}); err != nil {
		t.Fatal(err)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/status", `{"to": "on_hold"}`), http.StatusBadRequest)
	s := want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/status", `{"to": "on_hold", "reason": "wait for the design"}`), http.StatusOK)
	if s["status"] != "on_hold" || s["held_from"] != "proposed" {
		t.Fatalf("held %v", s)
	}
	s = want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/status", `{"to": "proposed"}`), http.StatusOK)
	if s["status"] != "proposed" {
		t.Fatalf("resumed %v", s)
	}
}

func TestListBranches(t *testing.T) {
	h := newAPI(t)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "staged", "name": "Staged", "has_release_stage": true}`), http.StatusCreated)
	got := want(t, call(t, h, "GET", "/v1/workspaces/staged/branches", ""), http.StatusOK)["branches"].([]any)
	if len(got) != 2 {
		t.Fatalf("branches %v", got)
	}
	dev, main := got[0].(map[string]any), got[1].(map[string]any)
	if dev["name"] != "dev" || dev["into"] != "main" || main["name"] != "main" || main["release"] != true {
		t.Fatalf("branches %v", got)
	}
	want(t, call(t, h, "GET", "/v1/workspaces/nope/branches", ""), http.StatusNotFound)
}

func TestMergesOverHTTPNameTheBranchAndGoOneStepAtATime(t *testing.T) {
	h := newAPI(t)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "staged", "name": "Staged", "has_release_stage": true}`), http.StatusCreated)
	p := "/v1/workspaces/staged/stories"
	want(t, call(t, h, "POST", p, `{"title": "A"}`), http.StatusCreated)
	for _, to := range []string{"in_progress", "in_review"} {
		want(t, call(t, h, "POST", p+"/S-1/status", `{"to": "`+to+`"}`), http.StatusOK)
	}
	e := want(t, call(t, h, "POST", p+"/S-1/status", `{"to": "merged", "into": "main"}`), http.StatusConflict)
	if !strings.Contains(e["error"].(string), "dev") {
		t.Fatalf("refusal %v", e)
	}
	s := want(t, call(t, h, "POST", p+"/S-1/status", `{"to": "merged", "commit": "abc1234"}`), http.StatusOK)
	if s["status"] != "merged" || s["into"] != "dev" {
		t.Fatalf("merged %v", s)
	}
	want(t, call(t, h, "POST", p+"/S-1/status", `{"to": "deployed"}`), http.StatusConflict)
	s = want(t, call(t, h, "POST", p+"/S-1/status", `{"to": "merged", "into": "main", "commit": "def5678"}`), http.StatusOK)
	if s["into"] != "main" {
		t.Fatalf("merged %v", s)
	}
	want(t, call(t, h, "POST", p+"/S-1/status", `{"to": "deployed"}`), http.StatusOK)

	log := want(t, call(t, h, "GET", p+"/S-1/log", ""), http.StatusOK)["entries"].([]any)
	if m := log[2].(map[string]any); m["into"] != "dev" || m["commit"] != "abc1234" {
		t.Fatalf("first merge entry %v", m)
	}
	if m := log[3].(map[string]any); m["into"] != "main" || m["commit"] != "def5678" {
		t.Fatalf("second merge entry %v", m)
	}
	// The board has one merged column per branch, in merge order.
	cols := want(t, call(t, h, "GET", "/v1/workspaces/staged/board", ""), http.StatusOK)["columns"].([]any)
	var merged []string
	for _, c := range cols {
		if c := c.(map[string]any); c["status"] == "merged" {
			merged = append(merged, c["branch"].(string))
		}
	}
	if len(merged) != 2 || merged[0] != "dev" || merged[1] != "main" {
		t.Fatalf("merged columns %v", merged)
	}
}

func TestTheOldReleasedStatusIsABadRequestOverHTTP(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusCreated)
	e := want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/status", `{"to": "released"}`), http.StatusBadRequest)
	if !strings.Contains(e["error"].(string), "release branch") {
		t.Fatalf("refusal %v", e)
	}
}

func TestMergeBranchOverHTTPMovesEveryStoryOnIt(t *testing.T) {
	h := newAPI(t)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "staged", "name": "Staged", "has_release_stage": true}`), http.StatusCreated)
	p := "/v1/workspaces/staged/stories"
	for _, title := range []string{"A", "B"} {
		want(t, call(t, h, "POST", p, `{"title": "`+title+`"}`), http.StatusCreated)
	}
	for _, id := range []string{"S-1", "S-2"} {
		for _, to := range []string{"in_progress", "in_review"} {
			want(t, call(t, h, "POST", p+"/"+id+"/status", `{"to": "`+to+`"}`), http.StatusOK)
		}
		want(t, call(t, h, "POST", p+"/"+id+"/status", `{"to": "merged", "commit": "c1"}`), http.StatusOK)
	}
	m := "/v1/workspaces/staged/branches/"
	want(t, call(t, h, "POST", m+"main/merged", `{"commit": "x9"}`), http.StatusConflict)
	want(t, call(t, h, "POST", m+"nope/merged", `{"commit": "x9"}`), http.StatusNotFound)
	want(t, call(t, h, "POST", m+"dev/merged", `{}`), http.StatusBadRequest)
	got := want(t, call(t, h, "POST", m+"dev/merged", `{"commit": "x9"}`), http.StatusOK)["stories"].([]any)
	if len(got) != 2 || got[0].(map[string]any)["into"] != "main" {
		t.Fatalf("moved %v", got)
	}
	again := want(t, call(t, h, "POST", m+"dev/merged", `{"commit": "y1"}`), http.StatusOK)["stories"].([]any)
	if len(again) != 0 {
		t.Fatalf("moved again %v", again)
	}
}
