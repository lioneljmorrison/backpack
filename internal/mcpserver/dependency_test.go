package mcpserver_test

import (
	"testing"
)

func TestAddDependencyIsReadFromBothEnds(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	s := use(t, cs, "add_dependency", map[string]any{"workspace": "demo", "id": "S-2", "on": "S-1"})
	if by, _ := s["blocked_by"].([]any); len(by) != 1 || by[0] != "S-1" {
		t.Fatalf("story %v", s)
	}
	first := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"})
	if blocks, _ := first["blocks"].([]any); len(blocks) != 1 || blocks[0] != "S-2" {
		t.Fatalf("story %v", first)
	}
}

func TestAddDependencyRefusesToBreakTheOrderWithTheBoardsReason(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	_, err := b.AddDependency("demo", "tester", "S-1", "S-2")
	if err == nil {
		t.Fatal("the board accepted a dependency on a story ranked below")
	}
	if why := refused(t, cs, "add_dependency", map[string]any{"workspace": "demo", "id": "S-1", "on": "S-2"}); why != err.Error() {
		t.Fatalf("reason %q, want the board's %q", why, err)
	}
}

func TestRemoveDependencyUnblocksTheStory(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	use(t, cs, "add_dependency", map[string]any{"workspace": "demo", "id": "S-2", "on": "S-1"})
	if r := use(t, cs, "remove_dependency", map[string]any{"workspace": "demo", "id": "S-2", "on": "S-1"}); r["removed"] != true {
		t.Fatalf("answer %v", r)
	}
	s := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-2"})
	if by, _ := s["blocked_by"].([]any); len(by) != 0 {
		t.Fatalf("story %v", s)
	}
}

func TestRemoveDependencyRefusesOneThatDoesNotExist(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	refused(t, cs, "remove_dependency", map[string]any{"workspace": "demo", "id": "S-2", "on": "S-1"})
}
