package mcpserver_test

import (
	"context"
	"slices"
	"testing"
)

func TestAddPhaseNumbersItAfterTheLast(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Foundations"})
	p := use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Features", "description": "d"})
	if p["number"] != 2.0 || p["created_by"] != "tester" {
		t.Fatalf("phase %v", p)
	}
}

func TestListPhasesShowsGoalsAndProgress(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Foundations"})
	g := use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "it runs"})
	if g["id"] != "G-1" || g["met"] != false {
		t.Fatalf("goal %v", g)
	}
	ps := use(t, cs, "list_phases", map[string]any{"workspace": "demo"})["phases"].([]any)
	p := ps[0].(map[string]any)
	if goals := p["goals"].([]any); len(goals) != 1 || p["complete"] != false {
		t.Fatalf("phase %v", p)
	}
}

func TestAddGoalRefusesAPhaseThatDoesNotExist(t *testing.T) {
	_, cs := connect(t)
	refused(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 3, "text": "it runs"})
}

func TestMarkGoalMetMarksAndUnmarks(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Foundations"})
	use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "it runs"})
	g := use(t, cs, "mark_goal_met", map[string]any{"workspace": "demo", "id": "G-1", "met": true})
	if g["met"] != true || g["met_by"] != "tester" {
		t.Fatalf("goal %v", g)
	}
	g = use(t, cs, "mark_goal_met", map[string]any{"workspace": "demo", "id": "G-1", "met": false})
	if g["met"] != false {
		t.Fatalf("goal %v", g)
	}
}

func TestMarkGoalMetNeedsMet(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Foundations"})
	use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "it runs"})
	refused(t, cs, "mark_goal_met", map[string]any{"workspace": "demo", "id": "G-1"})
}

func TestTheServerOffersExactlyTheSettledTools(t *testing.T) {
	_, cs := connect(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	slices.Sort(names)
	want := []string{"add_dependency", "add_goal", "add_idea", "add_phase", "add_story",
		"batch", "drop_idea", "get_idea", "get_story", "list_ideas", "list_phases", "list_workspaces", "log", "mark_goal_met", "move_idea",
		"move_story", "read_log", "refine_idea", "remove_dependency", "search", "set_status", "update_idea",
		"update_story", "whats_next", "where_are_we"}
	if !slices.Equal(names, want) {
		t.Fatalf("tools %v, want %v", names, want)
	}
}
