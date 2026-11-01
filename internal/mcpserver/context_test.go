package mcpserver_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// aNeighbourhood is an invented one around S-3: a phase with goals, an idea
// that became it, a merged and an open dependency (S-1, S-2) with decisions,
// one corrected, a dependent S-5, a bug found in it (S-4) and an open bug
// found in the merged dependency (S-6).
func aNeighbourhood(t *testing.T, b *board.Board) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := b.AddPhase("demo", "tester", "Foundations", "")
	must(err)
	_, err = b.AddGoal("demo", "tester", 1, "it runs")
	must(err)
	for _, s := range []board.NewStory{
		{Title: "First"}, {Title: "Second"}, {Title: "Target", Phase: 1},
		{Title: "Found bug", Kind: "bug", FoundIn: "S-3"},
		{Title: "After"}, {Title: "Dependency bug", Kind: "bug", FoundIn: "S-1"},
	} {
		_, err := b.AddStory("demo", "tester", s)
		must(err)
	}
	for _, d := range [][2]string{{"S-3", "S-1"}, {"S-3", "S-2"}, {"S-5", "S-3"}} {
		_, err := b.AddDependency("demo", "tester", d[0], d[1])
		must(err)
	}
	_, err = b.AddIdea("demo", "tester", board.NewIdea{Title: "Rough"})
	must(err)
	_, err = b.RefineIdea("demo", "tester", "I-1", board.Refinement{Stories: []string{"S-3"}})
	must(err)
	wrong, err := b.AddLogEntry("demo", "tester", "S-2", board.NewLogEntry{Kind: "decision", Text: "wrong", Reason: "r1"})
	must(err)
	_, err = b.AddLogEntry("demo", "tester", "S-2", board.NewLogEntry{Kind: "correction", Text: "not so", Corrects: wrong.ID})
	must(err)
	for _, to := range []string{"in_progress", "in_review", "merged"} {
		c := board.StatusChange{To: to}
		if to == "merged" {
			c.Commit = "c1"
		}
		_, err := b.SetStatus("demo", "tester", "S-1", c)
		must(err)
	}
}

func TestGetContextAnswersEachSectionAsSummaries(t *testing.T) {
	b, cs := connect(t)
	aNeighbourhood(t, b)
	c := use(t, cs, "get_context", map[string]any{"workspace": "demo", "id": "S-3"})
	if c["id"] != "S-3" || c["title"] != "Target" || c["status"] != "backlog" {
		t.Fatalf("context %v", c)
	}
	phase := c["phase"].(map[string]any)
	goals := phase["goals"].([]any)
	if phase["number"] != 1.0 || phase["title"] != "Foundations" || len(goals) != 1 ||
		goals[0].(map[string]any)["text"] != "it runs" || goals[0].(map[string]any)["met"] != false {
		t.Fatalf("phase %v", phase)
	}
	if idea := c["idea"].(map[string]any); idea["id"] != "I-1" || idea["title"] != "Rough" {
		t.Fatalf("idea %v", idea)
	}
	deps := c["depends_on"].([]any)
	first, second := deps[0].(map[string]any), deps[1].(map[string]any)
	if first["id"] != "S-1" || first["status"] != "merged" {
		t.Fatalf("the merged dependency %v", first)
	}
	absent(t, first, "decisions") // none logged: nothing is sent
	ds := second["decisions"].([]any)
	d := ds[0].(map[string]any)
	if second["id"] != "S-2" || len(ds) != 1 || d["text"] != "wrong" || d["reason"] != "r1" || d["corrected"] != true {
		t.Fatalf("the open dependency %v", second)
	}
	absent(t, d, "at", "actor", "created_at", "created_by")
	if by := c["depended_on_by"].([]any); len(by) != 1 || by[0].(map[string]any)["id"] != "S-5" {
		t.Fatalf("depended on by %v", by)
	}
	if f := c["found"].([]any); len(f) != 1 || f[0].(map[string]any)["id"] != "S-4" {
		t.Fatalf("found %v", f)
	}
	bugs := c["bugs"].([]any)
	if len(bugs) != 2 || bugs[0].(map[string]any)["id"] != "S-4" || bugs[1].(map[string]any)["found_in"] != "S-1" {
		t.Fatalf("bugs %v", bugs)
	}
	absent(t, c, "found_in")
}

func TestGetContextLeavesOutWhatIsEmpty(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "Alone")
	c := use(t, cs, "get_context", map[string]any{"workspace": "demo", "id": "S-1"})
	if len(c) != 3 {
		t.Fatalf("context %v, want id, title and status only", c)
	}
}

func TestGetContextIsOnRequestAndRefusesWhatIsMissing(t *testing.T) {
	_, cs := connect(t)
	if slices.Contains(menuNames(t, cs), "get_context") {
		t.Fatal("get_context is on the menu")
	}
	if list := say(t, cs, "actions", map[string]any{}); !strings.Contains(list, "get_context: ") {
		t.Fatalf("actions does not list get_context:\n%s", list)
	}
	if got := say(t, cs, "actions", map[string]any{"name": "get_context"}); !strings.Contains(got, "id: string") {
		t.Fatalf("actions get_context:\n%s", got)
	}
	if why := refused(t, cs, "get_context", map[string]any{"workspace": "demo", "id": "S-9"}); !strings.Contains(why, "S-9") {
		t.Fatalf("refusal %q", why)
	}
	if why := refused(t, cs, "get_context", map[string]any{"workspace": "demo"}); why == "" {
		t.Fatal("a read with no id was not refused")
	}
}
