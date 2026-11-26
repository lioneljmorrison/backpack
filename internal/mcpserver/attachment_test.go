package mcpserver_test

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/files"
)

// An agent's reads list the files kept on an item, by id, name, type and
// size, and leave the list out when there are none.
func TestAgentReadsListAttachmentsAndOmitThemWhenThereAreNone(t *testing.T) {
	b, cs := connect(t)
	s, err := files.New(filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	b.SetFiles(s)
	for _, in := range []board.NewStory{{Title: "S"}, {Title: "Bare"}} {
		if _, err := b.AddStory("demo", "tester", in); err != nil {
			t.Fatal(err)
		}
	}
	b.AddIdea("demo", "tester", board.NewIdea{Title: "I"})
	b.AddPhase("demo", "tester", "P", "")
	png := []byte("\x89PNG\r\n\x1a\n invented")
	for kind, id := range map[string]string{board.KindStory: "S-1", board.KindIdea: "I-1", board.KindPhase: "1"} {
		if _, err := b.AddAttachment("demo", "tester", kind, id, "shot.png", png); err != nil {
			t.Fatal(err)
		}
	}
	has := func(what string, m map[string]any) {
		t.Helper()
		as, _ := m["attachments"].([]any)
		if len(as) != 1 {
			t.Fatalf("%s: %v", what, m)
		}
		a := as[0].(map[string]any)
		if a["name"] != "shot.png" || a["type"] != "image/png" || a["size"] != float64(len(png)) || a["id"] == "" || a["key"] != nil {
			t.Fatalf("%s: attachment %v", what, a)
		}
	}
	has("get_story", use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"}))
	has("get_context", use(t, cs, "get_context", map[string]any{"workspace": "demo", "id": "S-1"}))
	has("get_idea", use(t, cs, "get_idea", map[string]any{"workspace": "demo", "id": "I-1"}))
	has("list_phases", use(t, cs, "list_phases", map[string]any{"workspace": "demo"})["phases"].([]any)[0].(map[string]any))

	if bare := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-2"}); bare["attachments"] != nil {
		t.Fatalf("an empty list is sent: %v", bare)
	}
	if bare := use(t, cs, "get_context", map[string]any{"workspace": "demo", "id": "S-2"}); bare["attachments"] != nil {
		t.Fatalf("an empty list is sent: %v", bare)
	}
}

func withFiles(t *testing.T) (*board.Board, *mcp.ClientSession) {
	t.Helper()
	b, cs := connect(t)
	s, err := files.New(filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	b.SetFiles(s)
	return b, cs
}

// An agent attaches a markdown note to a story, an idea or a phase, naming the
// item by id (S-n, I-n) or, for a phase, its number, and reads it back as text.
func TestAgentAttachesMarkdownAndReadsItBack(t *testing.T) {
	b, cs := withFiles(t)
	b.AddStory("demo", "tester", board.NewStory{Title: "S"})
	b.AddIdea("demo", "tester", board.NewIdea{Title: "I"})
	b.AddPhase("demo", "tester", "P", "")
	for _, item := range []string{"S-1", "I-1", "1"} {
		got := use(t, cs, "attach_markdown", map[string]any{"workspace": "demo", "item": item, "name": "note.md", "text": "# Hi\nbody"})
		if got["id"] == nil || got["name"] != "note.md" || got["type"] != "text/markdown; charset=utf-8" {
			t.Fatalf("%s: answer %v", item, got)
		}
	}
	res := callTool(t, cs, "get_attachment", map[string]any{"workspace": "demo", "id": "A-1"})
	if res.IsError || len(res.Content) != 1 || text(res) != "# Hi\nbody" {
		t.Fatalf("markdown came back as %v", res.Content)
	}
	if as := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"})["attachments"].([]any); len(as) != 1 {
		t.Fatalf("story attachments %v", as)
	}
}

func TestAttachMarkdownRefusesWithTheBoardsReason(t *testing.T) {
	b, cs := withFiles(t)
	b.AddStory("demo", "tester", board.NewStory{Title: "S"})
	for why, args := range map[string]map[string]any{
		"markdown":  {"item": "S-1", "name": "note.txt", "text": "x"},
		"no such":   {"item": "S-9", "name": "n.md", "text": "x"},
		"item form": {"item": "story one", "name": "n.md", "text": "x"},
	} {
		args["workspace"] = "demo"
		if r := refused(t, cs, "attach_markdown", args); r == "" {
			t.Errorf("%s: empty reason", why)
		}
	}
}

// An image comes back as image content an agent can look at, with its type.
func TestAgentGetsAnImageAsImageContent(t *testing.T) {
	b, cs := withFiles(t)
	b.AddStory("demo", "tester", board.NewStory{Title: "S"})
	png := []byte("\x89PNG\r\n\x1a\n invented")
	if _, err := b.AddAttachment("demo", "tester", board.KindStory, "S-1", "shot.png", png); err != nil {
		t.Fatal(err)
	}
	res := callTool(t, cs, "get_attachment", map[string]any{"workspace": "demo", "id": "A-1"})
	if res.IsError || len(res.Content) != 1 {
		t.Fatalf("result %v", res.Content)
	}
	img, ok := res.Content[0].(*mcp.ImageContent)
	if !ok || img.MIMEType != "image/png" || string(img.Data) != string(png) {
		t.Fatalf("content %#v", res.Content[0])
	}
	if r := refused(t, cs, "get_attachment", map[string]any{"workspace": "demo", "id": "A-9"}); !strings.Contains(r, "A-9") {
		t.Fatalf("reason %q", r)
	}
}

func TestAgentRemovesAnAttachmentThroughBatch(t *testing.T) {
	b, cs := withFiles(t)
	b.AddStory("demo", "tester", board.NewStory{Title: "S"})
	use(t, cs, "attach_markdown", map[string]any{"workspace": "demo", "item": "S-1", "name": "n.md", "text": "x"})
	use(t, cs, "remove_attachment", map[string]any{"workspace": "demo", "id": "A-1"})
	if bare := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"}); bare["attachments"] != nil {
		t.Fatalf("still listed: %v", bare)
	}
	if r := refused(t, cs, "remove_attachment", map[string]any{"workspace": "demo", "id": "A-1"}); !strings.Contains(r, "A-1") {
		t.Fatalf("reason %q", r)
	}
	if !strings.Contains(say(t, cs, "actions", map[string]any{}), "attach_markdown: ") {
		t.Fatal("actions does not list attach_markdown")
	}
}

// A note of the largest size, as JSON in a request, fits the server's body
// limit even when every character needs an escape.
func TestATenMiBNoteFitsTheRequestLimit(t *testing.T) {
	b, cs := withFiles(t)
	b.AddStory("demo", "tester", board.NewStory{Title: "S"})
	note := strings.Repeat("\n", board.MaxAttachment)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "batch", Arguments: map[string]any{
		"workspace": "demo", "actions": []any{map[string]any{"tool": "attach_markdown",
			"args": map[string]any{"item": "S-1", "name": "big.md", "text": note}}}}})
	if err != nil || res.IsError || !strings.Contains(text(res), `"ok":true`) {
		t.Fatalf("err %v, result %v", err, res)
	}
}
