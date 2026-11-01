package mcpserver_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestAddIdeaOpensItWithItsContextAndNeedsNoReasoning(t *testing.T) {
	_, cs := connect(t)
	got := use(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "A", "context": "rough, see the notes"})
	if !reflect.DeepEqual(got, map[string]any{"id": "I-1", "state": "open"}) {
		t.Fatalf("answer %v", got)
	}
	// The context and author are no longer in the write answer; read them back.
	i := use(t, cs, "get_idea", map[string]any{"workspace": "demo", "id": "I-1"})
	if i["state"] != "open" || i["context"] != "rough, see the notes" || i["created_by"] != "tester" {
		t.Fatalf("idea %v", i)
	}
	if _, has := i["reasoning"]; has {
		t.Fatalf("idea still has reasoning: %v", i)
	}
	if i := use(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "B"}); i["state"] != "open" || i["id"] != "I-2" {
		t.Fatalf("idea %v", i)
	}
}

func TestListGetMoveAndUpdateIdeas(t *testing.T) {
	b, cs := connect(t)
	for _, title := range []string{"A", "B"} {
		if _, err := b.AddIdea("demo", "tester", board.NewIdea{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	if got := use(t, cs, "move_idea", map[string]any{"workspace": "demo", "id": "I-2", "before": "I-1"}); !reflect.DeepEqual(got, map[string]any{"id": "I-2", "state": "open"}) {
		t.Fatalf("move_idea %v", got)
	}
	list := use(t, cs, "list_ideas", map[string]any{"workspace": "demo"})
	if is := list["ideas"].([]any); len(is) != 2 || is[0].(map[string]any)["id"] != "I-2" {
		t.Fatalf("ideas %v", list)
	}
	if got := use(t, cs, "update_idea", map[string]any{"workspace": "demo", "id": "I-1", "context": "grown"}); !reflect.DeepEqual(got, map[string]any{"id": "I-1", "state": "open"}) {
		t.Fatalf("update_idea %v", got)
	}
	if i := use(t, cs, "get_idea", map[string]any{"workspace": "demo", "id": "I-1"}); i["context"] != "grown" {
		t.Fatalf("idea %v", i)
	}
	refused(t, cs, "get_idea", map[string]any{"workspace": "demo", "id": "I-9"})
}

func TestRefineIdeaLinksWhatItBecameAndDropIdeaNeedsAReason(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "Story")
	for _, title := range []string{"A", "B"} {
		if _, err := b.AddIdea("demo", "tester", board.NewIdea{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	got := use(t, cs, "refine_idea", map[string]any{"workspace": "demo", "id": "I-1", "stories": []string{"S-1"}})
	if !reflect.DeepEqual(got, map[string]any{"id": "I-1", "state": "closed"}) {
		t.Fatalf("refine_idea %v", got)
	}
	// What it became is read back with get_idea, not the write answer.
	if i := use(t, cs, "get_idea", map[string]any{"workspace": "demo", "id": "I-1"}); i["became_stories"].([]any)[0] != "S-1" {
		t.Fatalf("idea %v", i)
	}
	refused(t, cs, "drop_idea", map[string]any{"workspace": "demo", "id": "I-2"})
	got = use(t, cs, "drop_idea", map[string]any{"workspace": "demo", "id": "I-2", "reason": "no"})
	if !reflect.DeepEqual(got, map[string]any{"id": "I-2", "state": "closed"}) {
		t.Fatalf("drop_idea %v", got)
	}
	if i := use(t, cs, "get_idea", map[string]any{"workspace": "demo", "id": "I-2"}); i["drop_reason"] != "no" {
		t.Fatalf("idea %v", i)
	}
}

func TestWhereAreWeHasNoIdeasWaitingForApproval(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "A"})
	w := say(t, cs, "where_are_we", map[string]any{"workspace": "demo"})
	if strings.Contains(strings.ToLower(w), "idea") {
		t.Fatalf("where %q", w)
	}
}
