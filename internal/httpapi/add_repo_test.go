package httpapi

import (
	"net/http"
	"testing"
)

func TestReposAreAddedAndListedOverHTTP(t *testing.T) {
	h := withDemo(t)
	made := want(t, call(t, h, "POST", "/v1/workspaces/demo/repos", `{"name": "web", "url": "https://example.com/web"}`), http.StatusCreated)
	if made["name"] != "web" || made["url"] != "https://example.com/web" {
		t.Fatalf("created %v", made)
	}
	want(t, call(t, h, "POST", "/v1/workspaces/demo/repos", `{"name": "web"}`), http.StatusConflict)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/repos", `{"name": "Not A Slug"}`), http.StatusBadRequest)
	want(t, call(t, h, "POST", "/v1/workspaces/none/repos", `{"name": "x"}`), http.StatusNotFound)

	list := want(t, call(t, h, "GET", "/v1/workspaces/demo/repos", ""), http.StatusOK)["repos"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["name"] != "demo" || list[1].(map[string]any)["name"] != "web" {
		t.Fatalf("repos %v", list)
	}
	want(t, call(t, h, "GET", "/v1/workspaces/none/repos", ""), http.StatusNotFound)

	// The new repository is usable at once, and a story must now name one.
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusBadRequest)
	s := want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A", "repo": "web"}`), http.StatusCreated)
	if s["repo"] != "web" {
		t.Fatalf("story %v", s)
	}
}
