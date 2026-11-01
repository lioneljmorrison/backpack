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

func TestSearchGivenAStoryLooksNearItFirstAndSaysSo(t *testing.T) {
	b, cs := connectIndexed(t, func(b *board.Board) *search.Index {
		url, key, prefix := typesensetest.Prefix(t)
		return search.New(url, key, prefix, b)
	})
	addStory(t, b, "Invoices in the work")
	addStory(t, b, "Invoices dependency")
	addStory(t, b, "Invoices far away")
	if _, err := b.AddDependency("demo", "tester", "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	got := use(t, cs, "search", map[string]any{"workspace": "demo", "query": "invoices", "story": "S-1"})
	rs, _ := got["results"].([]any)
	if len(rs) != 2 || got["scope"] != "story" {
		t.Fatalf("answer %v", got)
	}
	// Nothing near matches, so it widens and says so.
	addStory(t, b, "Gardening notes about tomatoes")
	got = use(t, cs, "search", map[string]any{"workspace": "demo", "query": "tomatoes", "story": "S-1"})
	if rs, _ := got["results"].([]any); len(rs) != 1 || got["scope"] != "workspace" {
		t.Fatalf("answer %v", got)
	}
}

func TestSearchWithoutAStoryNamesNoScope(t *testing.T) {
	b, cs := connectIndexed(t, func(b *board.Board) *search.Index {
		url, key, prefix := typesensetest.Prefix(t)
		return search.New(url, key, prefix, b)
	})
	addStory(t, b, "Invoices in the work")
	got := use(t, cs, "search", map[string]any{"workspace": "demo", "query": "invoices"})
	if _, ok := got["scope"]; ok {
		t.Fatalf("answer %v", got)
	}
}

func TestSearchNearAnUnknownStoryIsRefused(t *testing.T) {
	_, cs := connectIndexed(t, func(b *board.Board) *search.Index {
		url, key, prefix := typesensetest.Prefix(t)
		return search.New(url, key, prefix, b)
	})
	why := refused(t, cs, "search", map[string]any{"workspace": "demo", "query": "invoices", "story": "S-99"})
	if !strings.Contains(why, "S-99") {
		t.Fatalf("reason %q", why)
	}
}
