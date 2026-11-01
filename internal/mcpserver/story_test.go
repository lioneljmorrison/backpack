package mcpserver_test

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
	got := use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "First",
		"description": "d", "goal": "g", "plan": "p", "reasoning": "nothing else yet"})
	if len(got) != 2 || got["id"] != "S-1" || got["status"] != "proposed" {
		t.Fatalf("answer %v", got)
	}
	s := readStory(t, cs, "S-1")
	if s["goal"] != "g" || s["plan"] != "p" || s["created_by"] != nil {
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
	use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1",
		"plan": "new plan", "branch": "feature/x"})
	s := readStory(t, cs, "S-1")
	if s["title"] != "First" || s["plan"] != "new plan" || s["branch"] != "feature/x" || s["updated_by"] != nil {
		t.Fatalf("story %v", s)
	}
}

func TestUpdateStorySetsWhatAnInProgressStoryIsDoing(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	if _, err := b.SetStatus("demo", "tester", "S-1", board.StatusChange{To: board.InProgress}); err != nil {
		t.Fatal(err)
	}
	use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1",
		"activity": "waiting", "waiting_on": "the test agent"})
	s := readStory(t, cs, "S-1")
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
	if len(s) != 1 || s["id"] != "S-2" {
		t.Fatalf("answer %v", s)
	}
	next := use(t, cs, "whats_next", map[string]any{"workspace": "demo"})
	if n := next["story"].(map[string]any); n["id"] != "S-2" {
		t.Fatalf("next %v", n)
	}
}

func TestGetStoryOfAReleasedStoryIncludesItsRelease(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	mergedIntoDev(t, b, "Second")
	if _, err := b.MergeBranch("staged", "tester", "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CutRelease("staged", "tester", board.NewRelease{Version: "v1.0.0", Tag: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	s := use(t, cs, "get_story", map[string]any{"workspace": "staged", "id": "S-1"})
	rel, _ := s["release"].(map[string]any)
	if rel["id"] != "R-1" || rel["version"] != "v1.0.0" {
		t.Fatalf("story %v", s)
	}
	addStory(t, b, "Unreleased")
	if u := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"}); u["release"] != nil {
		t.Fatalf("unreleased story %v", u)
	}
}

func absent(t *testing.T, s map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := s[k]; ok {
			t.Errorf("story has %q: %v", k, s)
		}
	}
}

func TestGetStoryLeavesOutBookkeepingAndEmptyFields(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	s := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"})
	absent(t, s, "created_at", "created_by", "moved_at", "moved_by", "updated_at", "updated_by",
		"description", "goal", "plan", "phase", "blocked", "blocks", "blocked_by")
	if s["id"] != "S-1" || s["title"] != "First" || s["status"] != "backlog" {
		t.Fatalf("story %v", s)
	}
}

func TestGetStoryShowsBlockedOnlyWhenBlocked(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	if _, err := b.AddDependency("demo", "tester", "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	s := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-2"})
	by, _ := s["blocked_by"].([]any)
	if s["blocked"] != true || len(by) != 1 || by[0] != "S-1" {
		t.Fatalf("story %v", s)
	}
	first := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"})
	bl, _ := first["blocks"].([]any)
	if len(bl) != 1 || bl[0] != "S-2" {
		t.Fatalf("story %v", first)
	}
	absent(t, first, "blocked", "blocked_by")
}

func TestGetStoryKeepsFieldsThatSaySomething(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.AddStory("demo", "tester", board.NewStory{Title: "First", Description: "d", Goal: "g", Plan: "p"}); err != nil {
		t.Fatal(err)
	}
	s := use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": "S-1"})
	if s["description"] != "d" || s["goal"] != "g" || s["plan"] != "p" {
		t.Fatalf("story %v", s)
	}
}

func readStory(t *testing.T, cs *mcp.ClientSession, id string) map[string]any {
	t.Helper()
	return use(t, cs, "get_story", map[string]any{"workspace": "demo", "id": id})
}

func TestAStoryIsAddedAsABugAndTheMarkIsChangedAndCleared(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "Crash", "kind": "bug", "reasoning": "r"})
	if s := readStory(t, cs, "S-1"); s["kind"] != "bug" {
		t.Fatalf("story %v", s)
	}
	got := use(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "kind": ""})
	if c, _ := got["changed"].([]any); len(c) != 1 || c[0] != "kind" {
		t.Fatalf("answer %v", got)
	}
	if s := readStory(t, cs, "S-1"); s["kind"] != nil {
		t.Fatalf("story %v", s)
	}
	refused(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "kind": "chore"})
	refused(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "X", "kind": "chore", "reasoning": "r"})
}

func TestAStoryFoundInReadsBothWaysOverMCP(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "A", "reasoning": "r"})
	use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "B", "found_in": "S-1", "reasoning": "r"})
	if s := readStory(t, cs, "S-2"); s["found_in"] != "S-1" {
		t.Fatalf("B %v", s)
	}
	a := readStory(t, cs, "S-1")
	if f, _ := a["found"].([]any); len(f) != 1 || f[0] != "S-2" || a["found_in"] != nil {
		t.Fatalf("A %v", a)
	}
	refused(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "C", "found_in": "S-99", "reasoning": "r"})
}

func bugFix(m map[string]any) map[string]any {
	a := map[string]any{"workspace": "demo", "title": "Crash on save", "kind": "bug", "found_in": "S-1",
		"after": "S-1", "reasoning": "next to where it was found", "fix": "Check the length first",
		"no_design_choice": true, "no_contract_change": true, "nothing_else": true}
	for k, v := range m {
		if v == nil {
			delete(a, k)
		} else {
			a[k] = v
		}
	}
	return a
}

func TestAddBugFixPutsAClearBugInTheBacklogAndWhereAreWeShowsIt(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "Build A")
	got := use(t, cs, "add_bug_fix", bugFix(nil))
	if got["id"] != "S-2" || got["status"] != "backlog" {
		t.Fatalf("answer %v", got)
	}
	if s := readStory(t, cs, "S-2"); s["straight_in"] != true || s["found_in"] != "S-1" {
		t.Fatalf("story %v", s)
	}
	l, err := b.Log("demo", "S-2")
	if err != nil || len(l) != 1 || l[0].Kind != "decision" || !strings.Contains(l[0].Text, "Check the length first") {
		t.Fatalf("log %+v, %v", l, err)
	}
	if w := say(t, cs, "where_are_we", map[string]any{"workspace": "demo"}); !strings.Contains(w, "S-2 Crash on save — found in S-1") {
		t.Fatalf("where:\n%s", w)
	}
}

func TestAddBugFixIsRefusedWhenAClaimIsNotMadeOrACheckFails(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "Build A")
	for name, edit := range map[string]map[string]any{
		"a claim left out":  {"nothing_else": nil},
		"a claim false":     {"no_design_choice": false},
		"no fix":            {"fix": nil},
		"not a bug":         {"kind": nil},
		"no place":          {"after": nil},
		"no found-in story": {"found_in": nil},
	} {
		t.Run(name, func(t *testing.T) { refused(t, cs, "add_bug_fix", bugFix(edit)) })
	}
	if ss, err := b.Stories("demo"); err != nil || len(ss) != 1 {
		t.Fatalf("stories %+v, %v", ss, err)
	}
}

func TestAddStoryStillProposesABugAndTakesNoClaims(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "Build A")
	args := map[string]any{"workspace": "demo", "title": "Crash", "kind": "bug", "found_in": "S-1", "reasoning": "r"}
	if got := use(t, cs, "add_story", args); got["status"] != "proposed" {
		t.Fatalf("answer %v", got)
	}
	args["fix"] = "x"
	refused(t, cs, "add_story", args)
}

func TestActionsDescribesAddBugFixWithItsClaims(t *testing.T) {
	_, cs := connect(t)
	got := say(t, cs, "actions", map[string]any{"name": "add_bug_fix"})
	for _, want := range []string{"fix: string", "no_design_choice: boolean", "no_contract_change: boolean", "nothing_else: boolean", "title: ", "batch"} {
		if !strings.Contains(got, want) {
			t.Errorf("answer lacks %q:\n%s", want, got)
		}
	}
}
