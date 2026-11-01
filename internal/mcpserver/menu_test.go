package mcpserver_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// menu is what ListTools offers: the tools every agent loads.
//
// This is the growth guard. Adding a tool to the menu needs the owner's
// approval; a new action goes on request by default: a change runs through
// batch, a read through actions, and `actions` describes both.
var menu = []string{"actions", "add_dependency", "add_idea", "add_story", "batch", "get_story",
	"list_workspaces", "log", "search", "set_status", "update_story", "whats_next", "where_are_we"}

func menuNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	slices.Sort(names)
	return names
}

func TestTheServerOffersExactlyTheSettledMenu(t *testing.T) {
	_, cs := connect(t)
	if got := menuNames(t, cs); !slices.Equal(got, menu) {
		t.Fatalf("tools %v, want %v", got, menu)
	}
}

func TestAnOnRequestChangeIsNotOnTheMenuButBatchRunsIt(t *testing.T) {
	_, cs := connect(t)
	if slices.Contains(menuNames(t, cs), "add_phase") {
		t.Fatal("add_phase is on the menu")
	}
	rs := results(t, cs, action("add_phase", map[string]any{"title": "Foundations"}))
	if r, _ := rs[0]["result"].(map[string]any); rs[0]["ok"] != true || r["number"] != 1.0 {
		t.Fatalf("result %v", rs[0])
	}
}

func TestAnOnRequestToolCalledDirectlyIsUnknown(t *testing.T) {
	_, cs := connect(t)
	_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "add_phase",
		Arguments: map[string]any{"workspace": "demo", "title": "Foundations"}})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("err %v, want an unknown tool error", err)
	}
}

func TestActionsListsEachOnRequestActionOnceAndNoMenuTool(t *testing.T) {
	_, cs := connect(t)
	got := say(t, cs, "actions", map[string]any{})
	count := func(name string) int {
		n := 0
		for _, l := range strings.Split(got, "\n") {
			if strings.HasPrefix(l, name+": ") {
				n++
			}
		}
		return n
	}
	for _, set := range []map[string]bool{onRequest, onRequestReads} {
		for name := range set {
			if n := count(name); n != 1 {
				t.Errorf("%s listed %d times:\n%s", name, n, got)
			}
		}
	}
	for _, name := range menu {
		if count(name) != 0 {
			t.Errorf("menu tool %s is listed:\n%s", name, got)
		}
	}
	// The reads come first, under their own heading, then the changes.
	reads, changes := strings.Index(got, "Reads"), strings.Index(got, "Changes")
	if reads < 0 || changes < reads || strings.Index(got, "list_ideas: ") > changes || strings.Index(got, "add_phase: ") < changes {
		t.Errorf("reads and changes are not marked apart:\n%s", got)
	}
	if len(got) > 1800 {
		t.Errorf("the list is %d bytes, want under 1,800", len(got))
	}
	t.Logf("actions listed %d bytes", len(got))
}

func TestActionsWithANameAnswersItsArguments(t *testing.T) {
	_, cs := connect(t)
	got := say(t, cs, "actions", map[string]any{"name": "cut_release"})
	for _, want := range []string{"version: ", "tag: ", "commit?: ", "notes?: ", "batch"} {
		if !strings.Contains(got, want) {
			t.Errorf("answer lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "workspace") {
		t.Errorf("answer names the workspace:\n%s", got)
	}
	// Required arguments come before optional ones, in the tool's own order.
	if strings.Index(got, "version") > strings.Index(got, "tag") || strings.Index(got, "tag") > strings.Index(got, "commit") {
		t.Errorf("arguments out of order:\n%s", got)
	}
	// A hint comes from the field's own description.
	if !strings.Contains(got, "git tag") {
		t.Errorf("no hint for tag:\n%s", got)
	}
}

func TestActionsAnswersAnEmbeddedAndAnOptionalArgument(t *testing.T) {
	_, cs := connect(t)
	got := say(t, cs, "actions", map[string]any{"name": "move_story"})
	for _, want := range []string{"id: ", "before?: ", "after?: "} {
		if !strings.Contains(got, want) {
			t.Errorf("answer lacks %q:\n%s", want, got)
		}
	}
}

func TestActionsRefusesAnUnknownNameAndAMenuTool(t *testing.T) {
	_, cs := connect(t)
	if why := refused(t, cs, "actions", map[string]any{"name": "nope"}); !strings.Contains(why, "nope") {
		t.Fatalf("reason %q", why)
	}
	why := refused(t, cs, "actions", map[string]any{"name": "set_status"})
	if !strings.Contains(why, "set_status") || !strings.Contains(why, "batch") {
		t.Fatalf("reason %q", why)
	}
}

func TestActionsNeverRunsAChange(t *testing.T) {
	b, cs := connect(t)
	say(t, cs, "actions", map[string]any{"name": "add_phase"})
	if ps, err := b.Phases("demo"); err != nil || len(ps) != 0 {
		t.Fatalf("phases %v, %v", ps, err)
	}
}

func TestActionsRunsAnOnRequestRead(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_idea", map[string]any{"workspace": "demo", "title": "Try it", "context": "why"})
	list := use(t, cs, "actions", map[string]any{"name": "list_ideas", "args": map[string]any{"workspace": "demo"}})
	is, _ := list["ideas"].([]any)
	if len(is) != 1 || is[0].(map[string]any)["title"] != "Try it" || is[0].(map[string]any)["state"] != "open" {
		t.Fatalf("ideas %v", list)
	}
	one := use(t, cs, "actions", map[string]any{"name": "get_idea", "args": map[string]any{"workspace": "demo", "id": "I-1"}})
	if one["id"] != "I-1" || one["context"] != "why" {
		t.Fatalf("idea %v", one)
	}
}

func TestAnOnRequestReadWithBadArgumentsIsRefusedWithItsReason(t *testing.T) {
	_, cs := connect(t)
	for name, args := range map[string]map[string]any{
		"missing workspace": {},
		"missing id":        {"workspace": "demo"},
		"unknown argument":  {"workspace": "demo", "id": "I-1", "colour": "red"},
	} {
		why := refused(t, cs, "actions", map[string]any{"name": "get_idea", "args": args})
		if !strings.Contains(why, "arguments") {
			t.Errorf("%s: reason %q", name, why)
		}
	}
	if why := refused(t, cs, "actions", map[string]any{"name": "get_idea", "args": map[string]any{"workspace": "nope", "id": "I-1"}}); !strings.Contains(why, "nope") {
		t.Errorf("reason %q", why)
	}
}

func TestActionsWithOnlyAReadsNameDescribesIt(t *testing.T) {
	_, cs := connect(t)
	got := say(t, cs, "actions", map[string]any{"name": "get_idea"})
	if !strings.Contains(got, "workspace: ") || !strings.Contains(got, "id: ") {
		t.Fatalf("answer:\n%s", got)
	}
}

func TestActionsRefusesToRunAChange(t *testing.T) {
	b, cs := connect(t)
	why := refused(t, cs, "actions", map[string]any{"name": "add_phase", "args": map[string]any{"workspace": "demo", "title": "Sneaky"}})
	if !strings.Contains(why, "batch") {
		t.Fatalf("reason %q", why)
	}
	if ps, err := b.Phases("demo"); err != nil || len(ps) != 0 {
		t.Fatalf("phases %v, %v", ps, err)
	}
}

func TestAMovedReadCalledDirectlyIsUnknown(t *testing.T) {
	_, cs := connect(t)
	for name := range onRequestReads {
		_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"workspace": "demo"}})
		if err == nil || !strings.Contains(err.Error(), "unknown tool") {
			t.Errorf("%s: err %v, want an unknown tool error", name, err)
		}
	}
}
