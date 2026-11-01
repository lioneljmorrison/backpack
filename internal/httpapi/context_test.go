package httpapi

import (
	"net/http"
	"testing"
)

func TestStoryContextOverHTTP(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/phases", `{"title": "P"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/phases/1/goals", `{"text": "it runs"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "Base"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "Top", "phase": 1}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-2/dependencies", `{"on": "S-1"}`), http.StatusOK)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories/S-1/log", `{"kind": "decision", "text": "go", "reason": "why"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "Bug", "kind": "bug", "found_in": "S-1"}`), http.StatusCreated)

	c := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-2/context", ""), http.StatusOK)
	if c["id"] != "S-2" || c["title"] != "Top" {
		t.Fatalf("context %v", c)
	}
	if p := c["phase"].(map[string]any); p["number"] != 1.0 || len(p["goals"].([]any)) != 1 {
		t.Fatalf("phase %v", p)
	}
	deps := c["depends_on"].([]any)
	d := deps[0].(map[string]any)
	if len(deps) != 1 || d["id"] != "S-1" || len(d["decisions"].([]any)) != 1 {
		t.Fatalf("depends on %v", deps)
	}
	base := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-1/context", ""), http.StatusOK)
	if by := base["depended_on_by"].([]any); len(by) != 1 || by[0].(map[string]any)["id"] != "S-2" {
		t.Fatalf("depended on by %v", by)
	}
	if bugs := base["bugs"].([]any); len(bugs) != 1 || bugs[0].(map[string]any)["id"] != "S-3" {
		t.Fatalf("bugs %v", bugs)
	}
}

func TestStoryContextOfAMissingStoryIsNotFound(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-9/context", ""), http.StatusNotFound)
	want(t, call(t, h, "GET", "/v1/workspaces/nope/stories/S-1/context", ""), http.StatusNotFound)
}
