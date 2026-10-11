package httpapi

import (
	"net/http"
	"testing"
)

func TestIdeasOverHTTP(t *testing.T) {
	h := withDemo(t)
	i := want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas", `{"title": "A", "context": "rough"}`), http.StatusCreated)
	if i["id"] != "I-1" || i["state"] != "open" || i["context"] != "rough" || i["created_by"] != "tester" {
		t.Fatalf("added %v", i)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas", `{"title": "B", "before": "I-1"}`), http.StatusCreated)
	list := want(t, call(t, h, "GET", "/v1/workspaces/demo/ideas", ""), http.StatusOK)
	if is := list["ideas"].([]any); len(is) != 2 || is[0].(map[string]any)["id"] != "I-2" {
		t.Fatalf("ideas %v", list)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas/I-2/move", `{"after": "I-1"}`), http.StatusOK)
	i = want(t, call(t, h, "PATCH", "/v1/workspaces/demo/ideas/I-1", `{"context": "grown"}`), http.StatusOK)
	if i["context"] != "grown" || i["title"] != "A" {
		t.Fatalf("updated %v", i)
	}
	i = want(t, call(t, h, "GET", "/v1/workspaces/demo/ideas/I-1", ""), http.StatusOK)
	if i["context"] != "grown" {
		t.Fatalf("read %v", i)
	}
}

func TestIdeasHaveNoApprovalOverHTTP(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas", `{"title": "A"}`), http.StatusCreated)
	if w := call(t, h, "POST", "/v1/workspaces/demo/ideas/I-1/approve", ""); w.Code != http.StatusNotFound {
		t.Fatalf("approve: status %d, want 404", w.Code)
	}
}

func TestRefiningAndDroppingIdeasOverHTTP(t *testing.T) {
	h := withDemo(t)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "S"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas", `{"title": "A"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas", `{"title": "B"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas/I-1/refine", `{"stories": ["S-9"]}`), http.StatusNotFound)
	i := want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas/I-1/refine", `{"stories": ["S-1"]}`), http.StatusOK)
	if i["state"] != "closed" || i["became_stories"].([]any)[0] != "S-1" {
		t.Fatalf("refined %v", i)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/phases", `{"title": "P"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas", `{"title": "C"}`), http.StatusCreated)
	i = want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas/I-3/refine", `{"phase": 1}`), http.StatusOK)
	if i["became_phase"] != float64(1) || len(i["became_stories"].([]any)) != 0 {
		t.Fatalf("refined into a phase %v", i)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas/I-2/drop", `{}`), http.StatusBadRequest)
	i = want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas/I-2/drop", `{"reason": "no"}`), http.StatusOK)
	if i["drop_reason"] != "no" {
		t.Fatalf("dropped %v", i)
	}
	list := want(t, call(t, h, "GET", "/v1/workspaces/demo/ideas", ""), http.StatusOK)
	if len(list["ideas"].([]any)) != 0 {
		t.Fatalf("ideas %v", list)
	}
}
