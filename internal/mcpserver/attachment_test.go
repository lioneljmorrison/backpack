package mcpserver_test

import (
	"path/filepath"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/files"
)

// An agent's reads list the files kept on an item, by id, name, type and
// size, and leave the list out when there are none. Adding and removing is
// not offered over MCP yet.
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
