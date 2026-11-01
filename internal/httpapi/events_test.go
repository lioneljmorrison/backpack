package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestEventsOverHTTPAreTheLatestOldestFirst(t *testing.T) {
	h := withDemo(t)
	events := func(query string) []any {
		t.Helper()
		return want(t, call(t, h, "GET", "/v1/workspaces/demo/events"+query, ""), http.StatusOK)["events"].([]any)
	}
	if got := events(""); got == nil || len(got) != 0 {
		t.Fatalf("an empty board: %v", got)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "Base"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas", `{"title": "Rough"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/log", `{"kind": "decision", "text": "go", "reason": "why"}`), http.StatusCreated)

	got := events("")
	if len(got) != 3 {
		t.Fatalf("events %v", got)
	}
	ids := map[string]string{}
	for _, e := range got {
		m := e.(map[string]any)
		ids[m["id"].(string)] = m["line"].(string)
		if m["at"] == "" || m["kind"] == "" {
			t.Fatalf("event %v", m)
		}
	}
	if !strings.Contains(ids["S-1"], "story added: Base") || !strings.Contains(ids["I-1"], "idea added: Rough") ||
		!strings.Contains(ids["L-1"], "S-1 decision: go — why") {
		t.Fatalf("lines %v", ids)
	}
	if got := events("?before=S-1"); len(got) != 0 {
		t.Fatalf("before the first: %v", got)
	}
	if got := events("?after=S-1"); len(got) < 2 {
		t.Fatalf("after the first: %v", got)
	}
	if got := events("?since=2000-01-01"); len(got) != 3 {
		t.Fatalf("since a date: %v", got)
	}
}

func TestEventsOverHTTPRefuseBadOptions(t *testing.T) {
	h := withDemo(t)
	for _, q := range []string{"?before=S-1&after=S-1", "?since=yesterday", "?before=banana", "?before=S-1&since=2026-01-01"} {
		want(t, call(t, h, "GET", "/v1/workspaces/demo/events"+q, ""), http.StatusBadRequest)
	}
	for _, q := range []string{"?before=S-9", "?after=L-9"} {
		want(t, call(t, h, "GET", "/v1/workspaces/demo/events"+q, ""), http.StatusNotFound)
	}
	want(t, call(t, h, "GET", "/v1/workspaces/nope/events", ""), http.StatusNotFound)
}
