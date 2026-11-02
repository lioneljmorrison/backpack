package mcpserver_test

import "testing"

func TestReposAreAddedThroughBatchAndListedThroughActions(t *testing.T) {
	_, cs := connect(t)
	if got := use(t, cs, "add_repo", map[string]any{"workspace": "demo", "name": "web", "url": "https://example.com/web"}); got["name"] != "web" {
		t.Fatalf("added %v", got)
	}
	if got := refused(t, cs, "add_repo", map[string]any{"workspace": "demo", "name": "web"}); got != `demo already has a repo named "web"` {
		t.Fatalf("again: %s", got)
	}
	refused(t, cs, "add_repo", map[string]any{"workspace": "demo", "name": "Bad Name"})

	rs := use(t, cs, "list_repos", map[string]any{"workspace": "demo"})["repos"].([]any)
	if len(rs) != 2 || rs[0].(map[string]any)["name"] != "demo" || rs[1].(map[string]any)["url"] != "https://example.com/web" {
		t.Fatalf("repos %v", rs)
	}

	// The tool menu did not grow; both are described by actions.
	if got := say(t, cs, "actions", map[string]any{"name": "add_repo"}); len(got) == 0 {
		t.Fatal("actions does not describe add_repo")
	}
	// A new repository is usable at once: branches shape it, stories land in it.
	use(t, cs, "add_branch", map[string]any{"workspace": "demo", "repo": "web", "name": "dev", "into": "main"})
	if got := refused(t, cs, "add_branch", map[string]any{"workspace": "demo", "name": "x", "into": "main"}); got != "demo has several repos; name the repo: demo or web" {
		t.Fatalf("no repo: %s", got)
	}
}
