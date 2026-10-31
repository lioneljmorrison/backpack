package mcpserver_test

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// results calls batch on workspace demo and returns each action's answer.
func results(t *testing.T, cs *mcp.ClientSession, actions ...map[string]any) []map[string]any {
	t.Helper()
	return resultsIn(t, cs, "demo", actions...)
}

func resultsIn(t *testing.T, cs *mcp.ClientSession, ws string, actions ...map[string]any) []map[string]any {
	t.Helper()
	list := make([]any, len(actions))
	for i, a := range actions {
		list[i] = a
	}
	got := use(t, cs, "batch", map[string]any{"workspace": ws, "actions": list})
	raw, _ := got["results"].([]any)
	if len(raw) != len(actions) {
		t.Fatalf("results %v, want %d", got, len(actions))
	}
	out := make([]map[string]any, len(raw))
	for i, r := range raw {
		out[i], _ = r.(map[string]any)
	}
	return out
}

func action(tool string, args map[string]any) map[string]any {
	return map[string]any{"tool": tool, "args": args}
}

func approve(id string) map[string]any {
	return action("set_status", map[string]any{"id": id, "to": "backlog"})
}

func proposeStory(t *testing.T, cs *mcp.ClientSession, title string) {
	t.Helper()
	use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": title, "reasoning": "at the bottom"})
}

func TestBatchApprovesSeveralProposedStoriesInOneCall(t *testing.T) {
	_, cs := connect(t)
	for _, title := range []string{"First", "Second", "Third"} {
		proposeStory(t, cs, title)
	}
	for i, r := range results(t, cs, approve("S-1"), approve("S-2"), approve("S-3")) {
		s, _ := r["result"].(map[string]any)
		if r["ok"] != true || r["tool"] != "set_status" || s["status"] != "backlog" {
			t.Fatalf("action %d: %v", i, r)
		}
	}
}

func TestBatchReportsAFailedActionsReasonAndAppliesTheRest(t *testing.T) {
	_, cs := connect(t)
	proposeStory(t, cs, "First")
	proposeStory(t, cs, "Second")
	rs := results(t, cs, approve("S-1"), approve("S-9"), approve("S-2"))
	if rs[0]["ok"] != true || rs[2]["ok"] != true {
		t.Fatalf("results %v", rs)
	}
	if why, _ := rs[1]["error"].(string); rs[1]["ok"] != false || !strings.Contains(why, "S-9") {
		t.Fatalf("failed action %v", rs[1])
	}
	if _, ok := rs[1]["result"]; ok {
		t.Fatalf("failed action has a result: %v", rs[1])
	}
}

func TestAStoryAddedThroughABatchIsProposed(t *testing.T) {
	_, cs := connect(t)
	rs := results(t, cs, action("add_story", map[string]any{"title": "First", "reasoning": "at the bottom"}))
	if s, _ := rs[0]["result"].(map[string]any); s["status"] != "proposed" || s["created_by"] != nil {
		t.Fatalf("result %v", rs[0])
	}
}

func TestBatchRefusesReadsUnknownToolsAndNestedBatchesOneByOne(t *testing.T) {
	_, cs := connect(t)
	proposeStory(t, cs, "First")
	rs := results(t, cs,
		action("get_story", map[string]any{"id": "S-1"}),
		action("no_such_tool", map[string]any{}),
		action("batch", map[string]any{"actions": []any{}}),
		approve("S-1"))
	for i, r := range rs[:3] {
		if why, _ := r["error"].(string); r["ok"] != false || why == "" {
			t.Fatalf("action %d: %v", i, r)
		}
	}
	if rs[3]["ok"] != true {
		t.Fatalf("approval %v", rs[3])
	}
}

func TestBatchRefusesAnActionThatNamesItsOwnWorkspace(t *testing.T) {
	_, cs := connect(t)
	proposeStory(t, cs, "First")
	rs := results(t, cs, action("set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "backlog"}))
	if why, _ := rs[0]["error"].(string); rs[0]["ok"] != false || !strings.Contains(why, "workspace") {
		t.Fatalf("result %v", rs[0])
	}
}

func TestBatchRefusesAnActionWithAnUnknownArgument(t *testing.T) {
	_, cs := connect(t)
	proposeStory(t, cs, "First")
	rs := results(t, cs, action("set_status", map[string]any{"id": "S-1", "to": "backlog", "colour": "red"}))
	if why, _ := rs[0]["error"].(string); rs[0]["ok"] != false || !strings.Contains(why, "colour") {
		t.Fatalf("result %v", rs[0])
	}
}

func TestBatchNeedsBetweenOneAndAHundredActions(t *testing.T) {
	_, cs := connect(t)
	refused(t, cs, "batch", map[string]any{"workspace": "demo", "actions": []any{}})
	many := make([]any, 101)
	for i := range many {
		many[i] = approve("S-1")
	}
	if why := refused(t, cs, "batch", map[string]any{"workspace": "demo", "actions": many}); !strings.Contains(why, "100") {
		t.Fatalf("reason %q", why)
	}
}

func TestEachActionInABatchOnAnUnknownWorkspaceFailsWithTheReason(t *testing.T) {
	_, cs := connect(t)
	rs := resultsIn(t, cs, "nope", approve("S-1"))
	if why, _ := rs[0]["error"].(string); rs[0]["ok"] != false || !strings.Contains(why, "nope") {
		t.Fatalf("result %v", rs[0])
	}
}

func TestBatchRefusesAnActionMissingAnArgumentItsToolRequires(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.AddPhase("demo", "tester", "One", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddGoal("demo", "tester", 1, "Ship it"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SetGoalMet("demo", "tester", "G-1", true); err != nil {
		t.Fatal(err)
	}
	rs := results(t, cs,
		action("mark_goal_met", map[string]any{"id": "G-1"}),
		action("add_story", map[string]any{"title": "No reasoning"}))
	for i, r := range rs {
		if why, _ := r["error"].(string); r["ok"] != false || why == "" {
			t.Fatalf("action %d: %v", i, r)
		}
	}
	ps, err := b.Phases("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !ps[0].Goals[0].Met {
		t.Fatalf("goal unmarked: %+v", ps[0].Goals[0])
	}
}
