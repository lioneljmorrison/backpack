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
// approval; a new action goes on request by default (it runs through batch
// and is described by `actions`). Step 2 of "Fewer tools" moves the rarer
// reads off the menu too.
var menu = []string{"actions", "add_dependency", "add_idea", "add_story", "batch", "get_idea", "get_release",
	"get_story", "list_branches", "list_ideas", "list_phases", "list_releases", "list_workspaces", "log",
	"read_log", "search", "set_status", "update_story", "whats_next", "where_are_we"}

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
	lines := strings.Split(strings.TrimSpace(got), "\n")
	// One line for each action, then one line on how to run them.
	if len(lines) != len(onRequest)+1 {
		t.Fatalf("%d lines, want %d:\n%s", len(lines), len(onRequest)+1, got)
	}
	for name := range onRequest {
		n := 0
		for _, l := range lines {
			if strings.HasPrefix(l, name+": ") {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s listed %d times:\n%s", name, n, got)
		}
	}
	for _, name := range menu {
		for _, l := range lines {
			if strings.HasPrefix(l, name+": ") {
				t.Errorf("menu tool %s is listed: %s", name, l)
			}
		}
	}
	if len(got) > 1300 {
		t.Errorf("the list is %d bytes, want about 1 KB", len(got))
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
