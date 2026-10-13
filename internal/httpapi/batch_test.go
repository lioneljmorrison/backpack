package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// batch sends requests as one batch on workspace demo, and returns each one's
// status and body.
func batch(t *testing.T, h http.Handler, requests ...string) []map[string]any {
	t.Helper()
	got := want(t, call(t, h, "POST", "/v1/workspaces/demo/batch",
		`{"requests": [`+strings.Join(requests, ",")+`]}`), http.StatusOK)
	raw, _ := got["results"].([]any)
	if len(raw) != len(requests) {
		t.Fatalf("results %v, want %d", got, len(requests))
	}
	out := make([]map[string]any, len(raw))
	for i, r := range raw {
		out[i], _ = r.(map[string]any)
	}
	return out
}

func status(r map[string]any) int {
	n, _ := r["status"].(float64)
	return int(n)
}

func body(r map[string]any) map[string]any {
	b, _ := r["body"].(map[string]any)
	return b
}

func TestABatchAnswersEachRequestAsItsOwnRouteWouldAndAppliesWhatWorks(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "First"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "Second"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-2/dependencies", `{"on": "S-1"}`), http.StatusOK)
	rs := batch(t, h,
		`{"method": "POST", "path": "stories/S-1/status", "body": {"to": "in_progress", "branch": "feature/20261011-first"}}`,
		`{"method": "POST", "path": "stories/S-9/status", "body": {"to": "in_progress"}}`,
		`{"method": "POST", "path": "stories/S-2/log", "body": {"kind": "note", "text": "next up"}}`,
		`{"method": "DELETE", "path": "stories/S-2/dependencies/S-1"}`)
	if status(rs[0]) != http.StatusOK || body(rs[0])["status"] != "in_progress" {
		t.Fatalf("start %v", rs[0])
	}
	if why, _ := body(rs[1])["error"].(string); status(rs[1]) != http.StatusNotFound || !strings.Contains(why, "S-9") {
		t.Fatalf("unknown story %v", rs[1])
	}
	if status(rs[2]) != http.StatusCreated || body(rs[2])["actor"] != "tester" {
		t.Fatalf("log %v", rs[2])
	}
	if _, ok := rs[3]["body"]; status(rs[3]) != http.StatusNoContent || ok {
		t.Fatalf("delete %v", rs[3])
	}
}

func TestABatchRefusesReadsAndPathsOutsideTheWorkspaceOneByOne(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "First"}`), http.StatusCreated)
	rs := batch(t, h,
		`{"method": "GET", "path": "stories/S-1"}`,
		`{"method": "POST", "path": "../other/stories", "body": {"title": "X"}}`,
		`{"method": "POST", "path": "stories/../../other/stories", "body": {"title": "X"}}`,
		`{"method": "POST", "path": "/v1/workspaces/other/stories", "body": {"title": "X"}}`,
		`{"method": "POST", "path": "http://elsewhere/stories", "body": {"title": "X"}}`,
		`{"method": "POST", "path": "batch", "body": {"requests": []}}`,
		`{"method": "POST", "path": "stories/S-1/log", "body": {"kind": "note", "text": "still runs"}}`)
	for i, r := range rs[:6] {
		if status(r) != http.StatusBadRequest || body(r)["error"] == nil {
			t.Fatalf("request %d: %v", i, r)
		}
	}
	if status(rs[6]) != http.StatusCreated {
		t.Fatalf("log %v", rs[6])
	}
}

func TestABatchNeedsBetweenOneAndAHundredRequests(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/batch", `{"requests": []}`), http.StatusBadRequest)
	many := make([]string, 101)
	for i := range many {
		many[i] = fmt.Sprintf(`{"method": "POST", "path": "stories", "body": {"title": "S%d"}}`, i)
	}
	w := want(t, call(t, h, "POST", "/v1/workspaces/demo/batch", `{"requests": [`+strings.Join(many, ",")+`]}`),
		http.StatusBadRequest)
	if why, _ := w["error"].(string); !strings.Contains(why, "100") {
		t.Fatalf("reason %v", w)
	}
}

func TestABatchRefusesUnknownFields(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/batch",
		`{"requests": [{"method": "POST", "path": "stories", "colour": "red"}]}`), http.StatusBadRequest)
}

func TestABatchCanRecordABranchMerge(t *testing.T) {
	h := newAPI(t)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "staged", "name": "Staged", "has_release_stage": true}`), http.StatusCreated)
	p := "/v1/workspaces/staged/stories"
	want(t, call(t, h, "POST", p, `{"title": "A"}`), http.StatusCreated)
	for _, to := range []string{"in_progress", "in_review", "merged"} {
		want(t, call(t, h, "POST", p+"/S-1/status", `{"to": "`+to+`"}`), http.StatusOK)
	}
	got := want(t, call(t, h, "POST", "/v1/workspaces/staged/batch", `{"requests": [
		{"method": "POST", "path": "branches/dev/merged", "body": {"commit": "x9"}},
		{"method": "POST", "path": "branches/main/merged", "body": {"commit": "x9"}}]}`), http.StatusOK)
	rs, _ := got["results"].([]any)
	first, _ := rs[0].(map[string]any)
	second, _ := rs[1].(map[string]any)
	if status(first) != http.StatusOK || len(body(first)["stories"].([]any)) != 1 || status(second) != http.StatusConflict {
		t.Fatalf("results %v", rs)
	}
}
