package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	srv := httptest.NewServer(httpapi.New(map[string]health.Check{}, tester, b, x, nil))
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

// say calls a tool that answers with plain text and returns that text exactly.
func say(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) string {
	t.Helper()
	res := callTool(t, cs, tool, args)
	if res.IsError {
		t.Fatalf("%s: %s", tool, text(res))
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s answered %d blocks, want 1", tool, len(res.Content))
	}
	return text(res)
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

// onRequest names the changes that are not on the menu: they run only
// through batch. callTool sends them that way, as a one-action batch, and
// unwraps the answer, so a test reads them as it reads any tool.
var onRequest = map[string]bool{
	"add_branch": true, "update_branch": true, "remove_branch": true, "merge_branch": true,
	"cut_release": true, "add_phase": true, "update_phase": true, "add_goal": true,
	"update_goal": true, "mark_goal_met": true, "refine_idea": true, "drop_idea": true,
	"update_idea": true, "move_idea": true, "move_story": true, "remove_dependency": true,
	"mark_reviewed": true,
}

// onRequestReads names the reads that are not on the menu: they run only
// through actions (name, and args with the workspace). callTool sends them
// that way.
var onRequestReads = map[string]bool{
	"list_branches": true, "list_releases": true, "get_release": true, "read_log": true,
	"list_phases": true, "list_ideas": true, "get_idea": true,
}

func callTool(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	switch {
	case onRequest[tool]:
		return callOnRequest(t, cs, tool, args)
	case onRequestReads[tool]:
		if args == nil {
			args = map[string]any{}
		}
		return callDirect(t, cs, "actions", map[string]any{"name": tool, "args": args})
	}
	return callDirect(t, cs, tool, args)
}

func callDirect(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

// callOnRequest runs an on-request change as a one-action batch. A failed
// action comes back as an error result carrying its reason.
func callOnRequest(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	rest := map[string]any{}
	for k, v := range args {
		if k != "workspace" {
			rest[k] = v
		}
	}
	res := callDirect(t, cs, "batch", map[string]any{"workspace": args["workspace"],
		"actions": []any{map[string]any{"tool": tool, "args": rest}}})
	if res.IsError {
		return res
	}
	var out struct {
		Results []struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil || len(out.Results) != 1 {
		t.Fatalf("%s: batch answered %q: %v", tool, text(res), err)
	}
	r := out.Results[0]
	if !r.OK {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: r.Error}}}
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(r.Result)}}}
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

func TestWhatsNextAnswersWithTheLeanStory(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	next := use(t, cs, "whats_next", map[string]any{"workspace": "demo"})
	s, _ := next["story"].(map[string]any)
	absent(t, s, "created_at", "created_by", "plan", "blocked", "blocks", "blocked_by")
}

func TestWhatsNextIsNullWhenNothingIsReady(t *testing.T) {
	_, cs := connect(t)
	next := use(t, cs, "whats_next", map[string]any{"workspace": "demo"})
	if s, ok := next["story"]; !ok || s != nil {
		t.Fatalf("next %v", next)
	}
}

// An invented large board: one phase, 3 in progress, 2 in review, 2 on hold,
// 10 in the backlog, 5 proposed and 30 merged and waiting for a release.
func largeBoard(t *testing.T, b *board.Board) {
	t.Helper()
	if _, err := b.AddPhase("demo", "tester", "Invented phase", "Made up for a test."); err != nil {
		t.Fatal(err)
	}
	add := func(title string, proposed bool, moves ...board.StatusChange) {
		n := board.NewStory{Title: title, Phase: 1, Proposed: proposed}
		if proposed {
			n.Reasoning = "invented"
		}
		s, err := b.AddStory("demo", "tester", n)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range moves {
			if m.To == "merged" {
				m.Commit = "c-" + s.ID
			}
			if _, err := b.SetStatus("demo", "tester", s.ID, m); err != nil {
				t.Fatal(err)
			}
		}
	}
	start, review := board.StatusChange{To: "in_progress"}, board.StatusChange{To: "in_review", PRURL: "https://example.com/pr/1"}
	hold, merge := board.StatusChange{To: "on_hold", Reason: "waiting on an invented decision"}, board.StatusChange{To: "merged"}
	for i := 0; i < 30; i++ {
		add(fmt.Sprintf("Merged story number %d with a fairly descriptive title", i), false, start, review, merge)
	}
	for i := 0; i < 3; i++ {
		add(fmt.Sprintf("In progress story %d", i), false, start)
	}
	for i := 0; i < 2; i++ {
		add(fmt.Sprintf("In review story %d", i), false, start, review)
	}
	for i := 0; i < 2; i++ {
		add(fmt.Sprintf("On hold story %d", i), false, hold)
	}
	for i := 0; i < 10; i++ {
		add(fmt.Sprintf("Backlog story %d", i), false)
	}
	for i := 0; i < 5; i++ {
		add(fmt.Sprintf("Proposed story %d", i), true)
	}
}

func TestWhereAreWeAnswersWithTheHooksTextUnderTwoKilobytes(t *testing.T) {
	b, cs := connect(t)
	largeBoard(t, b)
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	got := say(t, cs, "where_are_we", map[string]any{"workspace": "demo"})
	if got != w.Text() {
		t.Fatalf("answered\n%s\nwant\n%s", got, w.Text())
	}
	if len(got) >= 2000 {
		t.Fatalf("%d bytes, want under 2000:\n%s", len(got), got)
	}
	t.Logf("where_are_we answered %d bytes", len(got))
}

func TestWhereAreWeRefusesAnUnknownWorkspaceWithItsReason(t *testing.T) {
	_, cs := connect(t)
	if why := refused(t, cs, "where_are_we", map[string]any{"workspace": "nope"}); !strings.Contains(why, "nope") {
		t.Fatalf("reason %q", why)
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
