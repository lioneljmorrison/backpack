package mcpserver_test

import (
	"encoding/json"
	"reflect"
	"testing"
)

func keys(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestSetStatusAnswersWithTheIdAndStatusOnly(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	got := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "in_progress", "branch": "feature/x"})
	if len(got) != 2 || got["id"] != "S-1" || got["status"] != "in_progress" {
		t.Fatalf("answer %v", got)
	}
}

func TestABatchOfSetStatusNestsTheShortAnswer(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	rs := results(t, cs,
		action("set_status", map[string]any{"id": "S-1", "to": "in_progress"}),
		action("set_status", map[string]any{"id": "S-2", "to": "on_hold", "reason": "later"}))
	for i, want := range []string{"in_progress", "on_hold"} {
		s, _ := rs[i]["result"].(map[string]any)
		if len(s) != 2 || s["status"] != want {
			t.Fatalf("action %d: %v", i, rs[i])
		}
	}
}

func TestUpdateStoryAnswersWithTheFieldsThatChanged(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	got := use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1",
		"title": "First", "branch": "feature/x", "description": "d"})
	changed, _ := got["changed"].([]any)
	if got["id"] != "S-1" || !reflect.DeepEqual(changed, []any{"description", "branch"}) {
		t.Fatalf("answer %v, want description and branch changed but not the unchanged title", got)
	}
	if _, ok := got["marked"]; ok {
		t.Fatalf("answer marks nothing yet: %v", got)
	}
	again := use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "branch": "feature/x"})
	if c, ok := again["changed"].([]any); !ok || len(c) != 0 {
		t.Fatalf("a repeat changed %v, want an empty list", again)
	}
}

func TestUpdateStoryNamesTheDependentsItMarked(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	addStory(t, b, "Third")
	for _, id := range []string{"S-2", "S-3"} {
		if _, err := b.AddDependency("demo", "tester", id, "S-1"); err != nil {
			t.Fatal(err)
		}
	}
	got := use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "plan": "new plan"})
	marked, _ := got["marked"].([]any)
	if !reflect.DeepEqual(got["changed"], []any{"plan"}) || len(marked) != 2 {
		t.Fatalf("answer %v", got)
	}
	ids := map[any]bool{marked[0]: true, marked[1]: true}
	if !ids["S-2"] || !ids["S-3"] {
		t.Fatalf("marked %v", marked)
	}
}

func TestMoveLogAndAddDependencyAnswerBriefly(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	if got := use(t, cs, "move_story", map[string]any{"workspace": "demo", "id": "S-2", "before": "S-1"}); !reflect.DeepEqual(got, map[string]any{"id": "S-2"}) {
		t.Fatalf("move_story %v", got)
	}
	got := use(t, cs, "add_dependency", map[string]any{"workspace": "demo", "id": "S-1", "on": "S-2"})
	if !reflect.DeepEqual(got, map[string]any{"id": "S-1", "blocked": true}) {
		t.Fatalf("add_dependency %v", got)
	}
	got = use(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "note", "text": "hi"})
	if len(got) != 1 || got["id"] == nil || got["id"] == "" {
		t.Fatalf("log %v", got)
	}
}

func TestAddDependencyOnACompleteStoryIsNotBlocked(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "in_progress"})
	use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "in_review"})
	use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "merged", "commit": "c1"})
	got := use(t, cs, "add_dependency", map[string]any{"workspace": "demo", "id": "S-2", "on": "S-1"})
	if got["blocked"] != false {
		t.Fatalf("answer %v", got)
	}
}

func TestShortAnswersAreSmallerThanTheStory(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	res := callTool(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "in_progress", "branch": "feature/x"})
	short := text(res)
	full, _ := json.Marshal(readStory(t, cs, "S-1"))
	if res.IsError || len(short) >= len(full) {
		t.Fatalf("short %d bytes, story %d", len(short), len(full))
	}
	t.Logf("set_status answer: %d bytes %s; the story read back: %d bytes", len(short), short, len(full))
}

func TestABatchOfAddIdeaAndAddGoalNestsTheShortAnswers(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Foundations"})
	rs := results(t, cs,
		action("add_idea", map[string]any{"title": "A", "context": "c"}),
		action("add_goal", map[string]any{"phase": 1, "text": "it runs"}))
	idea, _ := rs[0]["result"].(map[string]any)
	goal, _ := rs[1]["result"].(map[string]any)
	if !reflect.DeepEqual(idea, map[string]any{"id": "I-1", "state": "open"}) {
		t.Fatalf("add_idea %v", rs[0])
	}
	if !reflect.DeepEqual(goal, map[string]any{"id": "G-1", "met": false}) {
		t.Fatalf("add_goal %v", rs[1])
	}
}
