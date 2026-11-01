package mcpserver_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// someStories adds n invented stories.
func someStories(t *testing.T, b *board.Board, n int) {
	t.Helper()
	for i := range n {
		if _, err := b.AddStory("demo", "tester", board.NewStory{Title: fmt.Sprintf("Story %d", i+1)}); err != nil {
			t.Fatal(err)
		}
	}
}

// positions is the first word of each line.
func positions(t *testing.T, lines string) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(lines), "\n") {
		out = append(out, strings.Fields(l)[0])
	}
	return out
}

func TestListEventsIsOnRequestAndSaysHowToUseIt(t *testing.T) {
	_, cs := connect(t)
	if slices.Contains(menuNames(t, cs), "list_events") {
		t.Fatal("list_events is on the menu")
	}
	got := say(t, cs, "actions", map[string]any{"name": "list_events"})
	for _, want := range []string{"list_events: ", "before?: ", "after?: ", "since?: "} {
		if !strings.Contains(got, want) {
			t.Errorf("detail lacks %q:\n%s", want, got)
		}
	}
	first, _, _ := strings.Cut(got, "\n")
	if n := len([]rune(strings.TrimPrefix(first, "list_events: "))); n >= 300 {
		t.Errorf("the description is %d characters, want under 300", n)
	}
	t.Logf("list_events description is %d characters", len([]rune(strings.TrimPrefix(first, "list_events: "))))
	list := say(t, cs, "actions", nil)
	if !strings.Contains(list, "list_events: The latest events, however old\n") {
		t.Errorf("not listed as one short line:\n%s", list)
	}
}

func TestListEventsAnswersTheLatestAsLinesOldestFirst(t *testing.T) {
	b, cs := connect(t)
	aNeighbourhood(t, b)
	got := say(t, cs, "list_events", map[string]any{"workspace": "demo"})
	lines := strings.Split(got, "\n")
	if len(lines) != 10 || strings.HasPrefix(got, "{") {
		t.Fatalf("%d lines, want 10 plain ones:\n%s", len(lines), got)
	}
	for _, want := range []string{" story added: Target", " idea added: Rough", " decision: wrong — r1", " correction of L-1: not so", " merged into main (c1)"} {
		if !strings.Contains(got, want) {
			t.Errorf("no line %q:\n%s", want, got)
		}
	}
}

func TestListEventsPagesBackAndCatchesUp(t *testing.T) {
	b, cs := connect(t)
	someStories(t, b, 25)
	page1 := positions(t, say(t, cs, "list_events", map[string]any{"workspace": "demo"}))
	if len(page1) != 20 {
		t.Fatalf("first page %v", page1)
	}
	page2 := positions(t, say(t, cs, "list_events", map[string]any{"workspace": "demo", "before": page1[0]}))
	if len(page2) != 5 {
		t.Fatalf("second page %v", page2)
	}
	all := append(page2, page1...)
	if len(slices.Compact(slices.Clone(all))) != 25 {
		t.Fatalf("pages overlap or leave a gap: %v", all)
	}
	since := positions(t, say(t, cs, "list_events", map[string]any{"workspace": "demo", "after": all[22]}))
	if !slices.Contains(since, all[24]) || slices.Contains(since, all[22]) {
		t.Fatalf("after %s: %v", all[22], since)
	}
	from := positions(t, say(t, cs, "list_events", map[string]any{"workspace": "demo", "since": "2000-01-01"}))
	if len(from) != 25 {
		t.Fatalf("since a date: %d events, want all 25", len(from))
	}
}

func TestListEventsSaysPlainlyWhenThereIsNothing(t *testing.T) {
	b, cs := connect(t)
	if got := say(t, cs, "list_events", map[string]any{"workspace": "demo"}); got != "Nothing has happened yet." {
		t.Fatalf("answer %q", got)
	}
	someStories(t, b, 1)
	if got := say(t, cs, "list_events", map[string]any{"workspace": "demo", "before": "S-1"}); got != "Nothing before S-1." {
		t.Fatalf("answer %q", got)
	}
	if got := say(t, cs, "list_events", map[string]any{"workspace": "demo", "after": "S-1"}); got != "Nothing after S-1." {
		t.Fatalf("answer %q", got)
	}
	if got := say(t, cs, "list_events", map[string]any{"workspace": "demo", "since": "2999-01-01"}); got != "Nothing since 2999-01-01." {
		t.Fatalf("answer %q", got)
	}
}

func TestListEventsRefusesWhatIsWrong(t *testing.T) {
	_, cs := connect(t)
	for name, args := range map[string]map[string]any{
		"two options":     {"workspace": "demo", "before": "S-1", "since": "2026-01-01"},
		"an unknown spot": {"workspace": "demo", "before": "S-99"},
		"a bad date":      {"workspace": "demo", "since": "yesterday"},
		"no workspace":    {"workspace": "nope"},
	} {
		if why := refused(t, cs, "list_events", args); why == "" {
			t.Errorf("%s: no reason", name)
		}
	}
	// A change cannot be smuggled in through the name.
	_, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_events", Arguments: map[string]any{"workspace": "demo"}})
	if err == nil {
		t.Error("list_events is callable directly")
	}
}
