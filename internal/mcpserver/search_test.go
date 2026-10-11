package mcpserver_test

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/search"
	"github.com/lioneljmorrison/backpack/internal/search/typesensetest"
)

// connectIndexed is connect with a board whose every change is indexed by
// the index index returns.
func connectIndexed(t *testing.T, index func(*board.Board) *search.Index) (*board.Board, *mcp.ClientSession) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	x := index(b)
	b.OnChange(x.Report)
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	return b, session(t, serve(t, b, x))
}

func TestSearchFindsAStoryThatAlreadyCoversSomething(t *testing.T) {
	b, cs := connectIndexed(t, func(b *board.Board) *search.Index {
		url, key, prefix := typesensetest.Prefix(t)
		return search.New(url, key, prefix, b)
	})
	addStory(t, b, "Sign-in with passkeys")
	got := use(t, cs, "search", map[string]any{"workspace": "demo", "query": "login"})
	rs, _ := got["results"].([]any)
	if len(rs) != 1 || rs[0].(map[string]any)["id"] != "S-1" {
		t.Fatalf("results %v", got)
	}
}

func TestSearchWithTypesenseUnreachableSaysSo(t *testing.T) {
	_, cs := connectIndexed(t, func(b *board.Board) *search.Index {
		return search.New("http://127.0.0.1:1", "none", "nothing_", b)
	})
	why := refused(t, cs, "search", map[string]any{"workspace": "demo", "query": "login"})
	if !strings.Contains(why, "search unavailable") {
		t.Fatalf("reason %q", why)
	}
}
