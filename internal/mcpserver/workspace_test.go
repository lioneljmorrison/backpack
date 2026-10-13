package mcpserver_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestListWorkspacesShowsEveryWorkspace(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "alpha", Name: "Alpha", RepoURL: "https://example.com/alpha"}); err != nil {
		t.Fatal(err)
	}
	ws := use(t, cs, "list_workspaces", map[string]any{})["workspaces"].([]any)
	if len(ws) != 2 {
		t.Fatalf("workspaces %v", ws)
	}
	a, d := ws[0].(map[string]any), ws[1].(map[string]any)
	if a["slug"] != "alpha" || a["name"] != "Alpha" || a["repo_url"] != "https://example.com/alpha" || a["created_by"] != "tester" {
		t.Fatalf("first %v", a)
	}
	if d["slug"] != "demo" || d["name"] != "Demo" {
		t.Fatalf("second %v", d)
	}
}

func TestListBranchesAnswersWithTheSameJSONAsTheAPI(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "staged", Name: "Staged", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	got := use(t, cs, "list_branches", map[string]any{"workspace": "staged"})["branches"].([]any)
	if len(got) != 2 {
		t.Fatalf("branches %v", got)
	}
	dev, main := got[0].(map[string]any), got[1].(map[string]any)
	if dev["name"] != "dev" || dev["into"] != "main" || main["name"] != "main" || main["release"] != true {
		t.Fatalf("branches %v", got)
	}
	refused(t, cs, "list_branches", map[string]any{"workspace": "nope"})
}
