package mcpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/health"
	"github.com/lioneljmorrison/backpack/internal/httpapi"
	"github.com/lioneljmorrison/backpack/internal/search"
)

// bearer adds the test's token to every request the client sends.
type bearer struct{}

func (bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer bp_good")
	return http.DefaultTransport.RoundTrip(r)
}

// demo serves the whole API over a real, empty board with workspace "demo",
// and returns its address. Only the token bp_good is accepted, as "tester".
func demo(t *testing.T) (*board.Board, string) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	return b, serve(t, b, nil)
}

// serve serves the whole API over b, searching with x, and returns the MCP
// address.
func serve(t *testing.T, b *board.Board, x *search.Index) string {
	t.Helper()
	tester := func(tok string) (string, bool, error) { return "tester", tok == "bp_good", nil }
	srv := httptest.NewServer(httpapi.New(map[string]health.Check{}, tester, b, x))
	t.Cleanup(srv.Close)
	return srv.URL + "/mcp"
}

// connect returns an MCP client session on the board, sending the token.
func connect(t *testing.T) (*board.Board, *mcp.ClientSession) {
	t.Helper()
	b, url := demo(t)
	return b, session(t, url)
}

func session(t *testing.T, url string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: &http.Client{Transport: bearer{}},
		DisableStandaloneSSE: true}
	cs, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// use calls a tool that must succeed and returns its JSON answer.
func use(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	res := callTool(t, cs, tool, args)
	if res.IsError {
		t.Fatalf("%s: %s", tool, text(res))
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text(res)), &m); err != nil {
		t.Fatalf("%s answered %q: %v", tool, text(res), err)
	}
	return m
}

// refused calls a tool that must fail and returns its reason.
func refused(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) string {
	t.Helper()
	res := callTool(t, cs, tool, args)
	if !res.IsError {
		t.Fatalf("%s succeeded: %s", tool, text(res))
	}
	return text(res)
}

func callTool(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func addStory(t *testing.T, b *board.Board, title string) {
	t.Helper()
	if _, err := b.AddStory("demo", "tester", board.NewStory{Title: title}); err != nil {
		t.Fatal(err)
	}
}

func TestTheMCPAddressRequiresAToken(t *testing.T) {
	_, url := demo(t)
	r, err := http.Post(url, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", r.StatusCode)
	}
}

func TestWhatsNextAnswersWithTheNextStory(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	next := use(t, cs, "whats_next", map[string]any{"workspace": "demo"})
	if s, _ := next["story"].(map[string]any); s == nil || s["id"] != "S-1" {
		t.Fatalf("next %v", next)
	}
}

func TestWhatsNextIsNullWhenNothingIsReady(t *testing.T) {
	_, cs := connect(t)
	next := use(t, cs, "whats_next", map[string]any{"workspace": "demo"})
	if s, ok := next["story"]; !ok || s != nil {
		t.Fatalf("next %v", next)
	}
}

func TestWhereAreWeAnswersWithTheBoardsJSON(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	where := use(t, cs, "where_are_we", map[string]any{"workspace": "demo"})
	if next, _ := where["next"].([]any); len(next) != 1 {
		t.Fatalf("where %v", where)
	}
}

func TestAnUnknownWorkspaceIsRefusedWithItsReason(t *testing.T) {
	_, cs := connect(t)
	if why := refused(t, cs, "whats_next", map[string]any{"workspace": "nope"}); !strings.Contains(why, "nope") {
		t.Fatalf("reason %q", why)
	}
}

func TestAToolNeedsAWorkspace(t *testing.T) {
	_, cs := connect(t)
	refused(t, cs, "where_are_we", map[string]any{})
}
