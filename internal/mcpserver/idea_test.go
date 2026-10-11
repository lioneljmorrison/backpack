package mcpserver_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestAddIdeaSuggestsItWithItsReasoning(t *testing.T) {
	_, cs := connect(t)
	i := use(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "A", "notes": "rough",
		"reasoning": "nothing else yet"})
	if i["id"] != "I-1" || i["state"] != "proposed" || i["reasoning"] != "nothing else yet" || i["created_by"] != "tester" {
		t.Fatalf("idea %v", i)
	}
	refused(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "B"})
}

func TestApproveIdeaOpensIt(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "A", "reasoning": "r"})
	i := use(t, cs, "approve_idea", map[string]any{"workspace": "demo", "id": "I-1"})
	if i["state"] != "open" || i["approved_by"] != "tester" {
		t.Fatalf("idea %v", i)
	}
	refused(t, cs, "approve_idea", map[string]any{"workspace": "demo", "id": "I-1"})
}

func TestListGetMoveAndUpdateIdeas(t *testing.T) {
	b, cs := connect(t)
	for _, title := range []string{"A", "B"} {
		if _, err := b.AddIdea("demo", "tester", board.NewIdea{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	use(t, cs, "move_idea", map[string]any{"workspace": "demo", "id": "I-2", "before": "I-1"})
	list := use(t, cs, "list_ideas", map[string]any{"workspace": "demo"})
	if is := list["ideas"].([]any); len(is) != 2 || is[0].(map[string]any)["id"] != "I-2" {
		t.Fatalf("ideas %v", list)
	}
	use(t, cs, "update_idea", map[string]any{"workspace": "demo", "id": "I-1", "notes": "grown"})
	if i := use(t, cs, "get_idea", map[string]any{"workspace": "demo", "id": "I-1"}); i["notes"] != "grown" {
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
	i := use(t, cs, "refine_idea", map[string]any{"workspace": "demo", "id": "I-1", "stories": []string{"S-1"}})
	if i["state"] != "closed" || i["became_stories"].([]any)[0] != "S-1" {
		t.Fatalf("idea %v", i)
	}
	refused(t, cs, "drop_idea", map[string]any{"workspace": "demo", "id": "I-2"})
	if i := use(t, cs, "drop_idea", map[string]any{"workspace": "demo", "id": "I-2", "reason": "no"}); i["drop_reason"] != "no" {
		t.Fatalf("idea %v", i)
	}
}

func TestWhereAreWeListsIdeasWaitingForApproval(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "A", "reasoning": "r"})
	w := use(t, cs, "where_are_we", map[string]any{"workspace": "demo"})
	if is := w["ideas_proposed"].([]any); len(is) != 1 {
		t.Fatalf("where %v", w)
	}
}
