package mcpserver_test

import (
	"strings"
	"testing"
)

func TestAChangedPlanMarksDependentsAndGetStoryAndWhereShowIt(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	if _, err := b.AddDependency("demo", "tester", "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "plan": "new plan"})
	s := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-2"})
	up, _ := s["upstream_changed"].(map[string]any)
	if up["story"] != "S-1" || up["at"] == "" {
		t.Fatalf("story %v", s)
	}
	if s := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"}); s["upstream_changed"] != nil {
		t.Fatalf("the changed story is marked: %v", s)
	}
	got := say(t, cs, "where_are_we", map[string]any{"workspace": "demo"})
	if !strings.Contains(got, "Upstream changed:\n  S-2 Second — after S-1") {
		t.Fatalf("where:\n%s", got)
	}
}

func TestMarkReviewedClearsTheMarkAndAnswersWithTheId(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	if _, err := b.AddDependency("demo", "tester", "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "goal": "new goal"})
	got := use(t, cs, "mark_reviewed", map[string]any{"workspace": "demo", "id": "S-2", "reason": "still holds"})
	if len(got) != 1 || got["id"] != "S-2" {
		t.Fatalf("answered %v", got)
	}
	if s := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-2"}); s["upstream_changed"] != nil {
		t.Fatalf("still marked: %v", s)
	}
	l, err := b.Log("demo", "S-2")
	if err != nil {
		t.Fatal(err)
	}
	if last := l[len(l)-1]; last.Kind != "note" || !strings.Contains(last.Text, "still holds") {
		t.Fatalf("log %+v", l)
	}
}

func TestMarkReviewedRefusalsCarryTheirReason(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	if got := refused(t, cs, "mark_reviewed", map[string]any{"workspace": "demo", "id": "S-1", "reason": "x"}); !strings.Contains(got, "not marked") {
		t.Fatalf("unmarked: %q", got)
	}
	if got := refused(t, cs, "mark_reviewed", map[string]any{"workspace": "demo", "id": "S-9", "reason": "x"}); !strings.Contains(got, "S-9") {
		t.Fatalf("unknown: %q", got)
	}
	refused(t, cs, "mark_reviewed", map[string]any{"workspace": "demo", "id": "S-1"})
}

func TestBatchMarksStoriesReviewedAndReportsEachOutcome(t *testing.T) {
	b, cs := connect(t)
	for _, title := range []string{"First", "Second", "Third"} {
		addStory(t, b, title)
	}
	for _, id := range []string{"S-2", "S-3"} {
		if _, err := b.AddDependency("demo", "tester", id, "S-1"); err != nil {
			t.Fatal(err)
		}
	}
	use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "plan": "v2"})
	got := results(t, cs,
		action("mark_reviewed", map[string]any{"id": "S-2", "reason": "fine"}),
		action("mark_reviewed", map[string]any{"id": "S-1", "reason": "not marked"}),
		action("mark_reviewed", map[string]any{"id": "S-3", "reason": "fine"}))
	if got[0]["ok"] != true || got[1]["ok"] != false || got[2]["ok"] != true {
		t.Fatalf("results %v", got)
	}
	if e, _ := got[1]["error"].(string); !strings.Contains(e, "not marked") {
		t.Fatalf("error %q", e)
	}
}
