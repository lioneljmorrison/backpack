package httpapi

import (
	"net/http"
	"testing"
)

func TestAWorkspaceIsCreatedWithAnOptionalRepoAndListsItsRepositories(t *testing.T) {
	h := withDemo(t)
	made := want(t, call(t, h, "POST", "/v1/workspaces",
		`{"slug": "shop", "name": "Shop", "repo": "storefront", "repo_url": "https://example.com/shop"}`), http.StatusCreated)
	repos := made["repos"].([]any)
	if r := repos[0].(map[string]any); len(repos) != 1 || r["name"] != "storefront" || r["url"] != "https://example.com/shop" {
		t.Fatalf("created %v", made)
	}
	if _, ok := made["repo_url"]; ok {
		t.Fatalf("the single URL is gone from the answer: %v", made)
	}
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "bad", "name": "Bad", "repo": "Not A Slug"}`), http.StatusBadRequest)

	list := want(t, call(t, h, "GET", "/v1/workspaces", ""), http.StatusOK)["workspaces"].([]any)
	demo := list[0].(map[string]any)["repos"].([]any)
	if r := demo[0].(map[string]any); len(demo) != 1 || r["name"] != "demo" {
		t.Fatalf("demo's repositories %v", demo)
	}

	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "A"}`), http.StatusCreated)
	s := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-1", ""), http.StatusOK)
	if s["repo"] != "demo" {
		t.Fatalf("story %v", s)
	}
}
