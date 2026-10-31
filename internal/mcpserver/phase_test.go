package mcpserver_test

import (
	"context"
	"slices"
	"strings"
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
	want := []string{"add_branch", "add_dependency", "add_goal", "add_idea", "add_phase", "add_story",
		"batch", "cut_release", "drop_idea", "get_idea", "get_release", "get_story", "list_branches", "list_ideas", "list_phases", "list_releases", "list_workspaces", "log", "mark_goal_met", "mark_reviewed", "merge_branch", "move_idea",
		"move_story", "read_log", "refine_idea", "remove_branch", "remove_dependency", "search", "set_status", "update_branch", "update_goal", "update_idea",
		"update_phase", "update_story", "whats_next", "where_are_we"}
	if !slices.Equal(names, want) {
		t.Fatalf("tools %v, want %v", names, want)
	}
}

func TestUpdatePhaseChangesOnlyTheFieldsGivenAndAnswersWithTheNumber(t *testing.T) {
	b, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Old", "description": "keep"})
	got := use(t, cs, "update_phase", map[string]any{"workspace": "demo", "number": 1, "title": "New"})
	if len(got) != 1 || got["number"] != 1.0 {
		t.Fatalf("answer %v, want only the number", got)
	}
	ps, _ := b.Phases("demo")
	if ps[0].Title != "New" || ps[0].Description != "keep" {
		t.Fatalf("phase %+v", ps[0])
	}
	l, _ := b.PhaseLog("demo", 1)
	if len(l) != 1 || l[0].From != "Old" || l[0].To != "New" || l[0].Actor != "tester" {
		t.Fatalf("log %+v", l)
	}
}

func TestUpdatePhaseRefusesAnEmptyTitleAndAnUnknownPhase(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "One"})
	refused(t, cs, "update_phase", map[string]any{"workspace": "demo", "number": 1, "title": ""})
	refused(t, cs, "update_phase", map[string]any{"workspace": "demo", "number": 9, "title": "x"})
}

func TestUpdateGoalChangesTheTextAndAnswersWithTheID(t *testing.T) {
	b, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "One"})
	use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "old"})
	got := use(t, cs, "update_goal", map[string]any{"workspace": "demo", "id": "G-1", "text": "new"})
	if len(got) != 1 || got["id"] != "G-1" {
		t.Fatalf("answer %v, want only the id", got)
	}
	ps, _ := b.Phases("demo")
	if ps[0].Goals[0].Text != "new" {
		t.Fatalf("goal %+v", ps[0].Goals[0])
	}
	refused(t, cs, "update_goal", map[string]any{"workspace": "demo", "id": "G-1", "text": ""})
	refused(t, cs, "update_goal", map[string]any{"workspace": "demo", "id": "G-9", "text": "x"})
	refused(t, cs, "update_goal", map[string]any{"workspace": "demo", "id": "G-1"})
}

func TestReadLogReadsAPhasesAndAGoalsLog(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Old"})
	use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "old"})
	use(t, cs, "update_phase", map[string]any{"workspace": "demo", "number": 1, "title": "New"})
	use(t, cs, "update_goal", map[string]any{"workspace": "demo", "id": "G-1", "text": "new"})
	pl := use(t, cs, "read_log", map[string]any{"workspace": "demo", "phase": 1})["entries"].([]any)
	if len(pl) != 1 || pl[0].(map[string]any)["field"] != "title" || pl[0].(map[string]any)["to"] != "New" {
		t.Fatalf("phase log %v", pl)
	}
	gl := use(t, cs, "read_log", map[string]any{"workspace": "demo", "id": "G-1"})["entries"].([]any)
	if len(gl) != 1 || gl[0].(map[string]any)["field"] != "text" || gl[0].(map[string]any)["from"] != "old" {
		t.Fatalf("goal log %v", gl)
	}
	refused(t, cs, "read_log", map[string]any{"workspace": "demo"})
	refused(t, cs, "read_log", map[string]any{"workspace": "demo", "id": "G-1", "phase": 1})
	refused(t, cs, "read_log", map[string]any{"workspace": "demo", "phase": 9})
}

func TestBatchEditsAPhaseAndAGoal(t *testing.T) {
	b, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Old"})
	use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "old"})
	rs := results(t, cs,
		action("update_phase", map[string]any{"number": 1, "description": "d"}),
		action("update_goal", map[string]any{"id": "G-1", "text": "new"}),
		action("update_goal", map[string]any{"id": "G-9", "text": "x"}))
	if rs[0]["ok"] != true || rs[1]["ok"] != true || rs[2]["ok"] != false {
		t.Fatalf("results %v", rs)
	}
	ps, _ := b.Phases("demo")
	if ps[0].Description != "d" || ps[0].Goals[0].Text != "new" {
		t.Fatalf("phase %+v", ps[0])
	}
}

func TestReadLogSaysWhyItRefusesBothOrNeitherOfIDAndPhase(t *testing.T) {
	_, cs := connect(t)
	if why := refused(t, cs, "read_log", map[string]any{"workspace": "demo", "id": "G-1", "phase": 1}); !strings.Contains(why, "not both") {
		t.Fatalf("both: %q", why)
	}
	if why := refused(t, cs, "read_log", map[string]any{"workspace": "demo"}); !strings.Contains(why, "number of a phase") {
		t.Fatalf("neither: %q", why)
	}
}
