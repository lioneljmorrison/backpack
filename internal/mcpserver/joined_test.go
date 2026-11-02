package mcpserver_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// Fixtures are invented: demo-api is joined into demo-web, demo is being joined.
func TestAJoinedWorkspaceRefusesEveryToolWithAPointer(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"demo", "demo-api", "demo-web"} {
		if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: s, Name: s}); err != nil {
			t.Fatal(err)
		}
	}
	falkortest.MarkEntry(t, db, prefix+"system", "demo-api", map[string]any{"joined_into": "demo-web", "joined_repos": []string{"api", "web"}})
	falkortest.MarkEntry(t, db, prefix+"system", "demo", map[string]any{"joining": "demo-web"})
	cs := session(t, serve(t, b, nil))

	const pointer = "demo-api was joined into demo-web; use workspace demo-web with repo api or web"
	for tool, args := range map[string]map[string]any{
		"get_story":    {"workspace": "demo-api", "id": "S-1"},
		"whats_next":   {"workspace": "demo-api"},
		"where_are_we": {"workspace": "demo-api"},
		"update_story": {"workspace": "demo-api", "id": "S-1", "title": "A"},
		"add_story":    {"workspace": "demo-api", "title": "A", "reasoning": "r"},
	} {
		got := refused(t, cs, tool, args)
		if got != pointer {
			t.Fatalf("%s: %q", tool, got)
		}
	}
	if got := refused(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"}); got != "demo is being joined into demo-web; try again shortly" {
		t.Fatalf("being joined: %q", got)
	}
	ws := use(t, cs, "list_workspaces", map[string]any{})["workspaces"].([]any)
	if len(ws) != 2 {
		t.Fatalf("listed %v", ws)
	}
}
