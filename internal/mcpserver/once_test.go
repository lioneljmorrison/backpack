package mcpserver_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// onlyText fails unless res carries exactly one text block and no structured
// copy of it.
func onlyText(t *testing.T, what string, res *mcp.CallToolResult) {
	t.Helper()
	if res.StructuredContent != nil {
		t.Errorf("%s: structured copy %v, want none", what, res.StructuredContent)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s: %d content blocks, want 1", what, len(res.Content))
	}
	if _, ok := res.Content[0].(*mcp.TextContent); !ok {
		t.Errorf("%s: content is %T, want text", what, res.Content[0])
	}
}

func TestEveryAnswerIsSentOnce(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	onlyText(t, "read", callTool(t, cs, "whats_next", map[string]any{"workspace": "demo"}))
	onlyText(t, "write", callTool(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "Thought"}))
	onlyText(t, "batch", callTool(t, cs, "batch", map[string]any{"workspace": "demo",
		"actions": []any{action("log", map[string]any{"id": "S-1", "kind": "note", "text": "hi"})}}))
	refusal := callTool(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-999"})
	if !refusal.IsError {
		t.Fatal("unknown story was not refused")
	}
	onlyText(t, "refusal", refusal)
}

func TestToolsDeclareNoOutputSchema(t *testing.T) {
	_, cs := connect(t)
	n := 0
	for tl, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		n++
		if tl.OutputSchema != nil {
			t.Errorf("%s declares an output schema", tl.Name)
		}
	}
	if n == 0 {
		t.Fatal("no tools listed")
	}
}
