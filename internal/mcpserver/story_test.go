package mcpserver_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestGetStoryAnswersWithTheStory(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	s := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"})
	if s["id"] != "S-1" || s["title"] != "First" {
		t.Fatalf("story %v", s)
	}
}

func TestGetStoryRefusesAStoryThatDoesNotExist(t *testing.T) {
	_, cs := connect(t)
	refused(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-9"})
}

func TestAddStoryProposesTheStoryWithTheTokensActor(t *testing.T) {
	b, cs := connect(t)
	s := use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "First",
		"description": "d", "goal": "g", "plan": "p", "reasoning": "nothing else yet"})
	if s["id"] != "S-1" || s["status"] != "proposed" || s["created_by"] != "tester" || s["goal"] != "g" || s["plan"] != "p" {
		t.Fatalf("story %v", s)
	}
	l, err := b.Log("demo", "S-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 1 || l[0].Kind != "decision" || l[0].Reason != "nothing else yet" {
		t.Fatalf("log %+v", l)
	}
}

func TestAddStoryProposesASpotBeforeAnother(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "Urgent", "before": "S-1",
		"reasoning": "it unblocks First"})
	ss, err := b.Stories("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 2 || ss[0].ID != "S-2" || ss[0].Status != board.Proposed {
		t.Fatalf("stories %+v", ss)
	}
}

func TestAddStoryNeedsATitleAndReasoning(t *testing.T) {
	_, cs := connect(t)
	refused(t, cs, "add_story", map[string]any{"workspace": "demo", "reasoning": "why"})
	refused(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "First"})
}

func TestUpdateStoryChangesOnlyTheFieldsGiven(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	s := use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1",
		"plan": "new plan", "branch": "feature/x"})
	if s["title"] != "First" || s["plan"] != "new plan" || s["branch"] != "feature/x" || s["updated_by"] != "tester" {
		t.Fatalf("story %v", s)
	}
}

func TestUpdateStorySetsWhatAnInProgressStoryIsDoing(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	if _, err := b.SetStatus("demo", "tester", "S-1", board.StatusChange{To: board.InProgress}); err != nil {
		t.Fatal(err)
	}
	s := use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1",
		"activity": "waiting", "waiting_on": "the test agent"})
	if s["activity"] != "waiting" || s["waiting_on"] != "the test agent" {
		t.Fatalf("story %v", s)
	}
}

func TestUpdateStoryRefusesAnActivityOutsideInProgress(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	refused(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "activity": "building"})
}

func TestMoveStoryChangesItsPlaceInTheBacklog(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	s := use(t, cs, "move_story", map[string]any{"workspace": "demo", "id": "S-2", "before": "S-1"})
	if s["moved_by"] != "tester" {
		t.Fatalf("story %v", s)
	}
	next := use(t, cs, "whats_next", map[string]any{"workspace": "demo"})
	if n := next["story"].(map[string]any); n["id"] != "S-2" {
		t.Fatalf("next %v", n)
	}
}
