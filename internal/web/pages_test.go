package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/search"
	"github.com/lioneljmorrison/backpack/internal/search/typesensetest"
)

// must fails the test on a board refusal and returns v.
func must[T any](t *testing.T) func(v T, err error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

// withDemo serves the pages over a board with workspace "demo".
func withDemo(t *testing.T) (http.Handler, *board.Board) {
	t.Helper()
	h, b := newSite(t)
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}))
	return h, b
}

func story(t *testing.T, b *board.Board, s board.NewStory) board.Story {
	t.Helper()
	return must[board.Story](t)(b.AddStory("demo", "tester", s))
}

// before says whether a appears before b on the page.
func before(page, a, b string) bool {
	i, j := strings.Index(page, a), strings.Index(page, b)
	return i >= 0 && j >= 0 && i < j
}

func TestTheBoardShowsEachColumnWithItsStoriesInDeliveryOrder(t *testing.T) {
	h, b := withDemo(t)
	story(t, b, board.NewStory{Title: "First in line"})
	story(t, b, board.NewStory{Title: "Second in line"})
	started := story(t, b, board.NewStory{Title: "Under way"})
	must[board.Story](t)(b.SetStatus("demo", "tester", started.ID, board.StatusChange{To: board.InProgress, Branch: "feature/x"}))

	w := get(h, "/w/demo", "bp_good")
	wantPage(t, w, "Demo", "Backlog", "In progress", "Merged", "Deployed", `href="/w/demo/stories/S-1"`)
	page := w.Body.String()
	if !before(page, `aria-label="Backlog"`, `aria-label="In progress"`) || !before(page, `aria-label="In progress"`, `aria-label="Deployed"`) {
		t.Error("columns are not in board order")
	}
	if !before(page, "First in line", "Second in line") {
		t.Error("backlog is not in delivery order")
	}
	if strings.Contains(page, "Released") {
		t.Error("Released shown for a workspace without a release stage")
	}
}

func TestTheBoardRunsFromIdeasThroughOnHoldAndBacklogToClosed(t *testing.T) {
	h, _ := withDemo(t)
	page := get(h, "/w/demo", "bp_good").Body.String()
	order := []string{"Ideas", "Proposed", "On hold", "Backlog", "In progress", "Deployed", "Closed"}
	for i := 1; i < len(order); i++ {
		if !before(page, `aria-label="`+order[i-1]+`"`, `aria-label="`+order[i]+`"`) {
			t.Errorf("%s is not before %s", order[i-1], order[i])
		}
	}
}

func TestTheBoardShowsOpenIdeasInTheirOwnColumn(t *testing.T) {
	h, b := withDemo(t)
	open := must[board.Idea](t)(b.AddIdea("demo", "tester", board.NewIdea{Title: "Dark mode"}))
	dropped := must[board.Idea](t)(b.AddIdea("demo", "tester", board.NewIdea{Title: "Sound effects"}))
	must[board.Idea](t)(b.DropIdea("demo", "tester", dropped.ID, "nobody wants it"))

	page := get(h, "/w/demo", "bp_good").Body.String()
	if !before(page, `aria-label="Ideas"`, "Dark mode") || !before(page, "Dark mode", `aria-label="Proposed"`) {
		t.Error("an open idea is not in the Ideas column")
	}
	wantPage(t, get(h, "/w/demo", "bp_good"), `href="/w/demo/ideas#`+open.ID+`"`)
	if strings.Contains(page, "Sound effects") {
		t.Error("a dropped idea is on the board")
	}
}

func TestClosedStoriesHaveAColumnOnTheBoard(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Not needed after all"})
	must[board.Story](t)(b.SetStatus("demo", "tester", s.ID, board.StatusChange{To: board.Closed, Reason: "superseded"}))

	page := get(h, "/w/demo", "bp_good").Body.String()
	if !before(page, `aria-label="Closed"`, "Not needed after all") {
		t.Error("a closed story is not in the Closed column")
	}
	if strings.Contains(page, "<details") {
		t.Error("closed stories are still folded away")
	}
}

func TestTheBoardMarksBlockedStoriesAndWhatInProgressWorkIsDoing(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "Groundwork"})
	second := story(t, b, board.NewStory{Title: "Built on it"})
	must[board.Story](t)(b.AddDependency("demo", "tester", second.ID, first.ID))
	must[board.Story](t)(b.SetStatus("demo", "tester", first.ID, board.StatusChange{To: board.InProgress, Branch: "feature/x"}))
	activity := "testing"
	must[board.Story](t)(b.UpdateStory("demo", "tester", first.ID, board.StoryPatch{Activity: &activity}))

	wantPage(t, get(h, "/w/demo", "bp_good"), "Blocked by S-1", "Testing")
}

func TestTheBoardSaysWhereWeAre(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "Foundations", ""))
	story(t, b, board.NewStory{Title: "Lay the slab", Phase: 1})
	wantPage(t, get(h, "/w/demo", "bp_good"), "Foundations", "0 of 1 complete", "Next", "Lay the slab")
}

func TestAStoryShowsItsDetailDependenciesAndLog(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "Groundwork"})
	second := story(t, b, board.NewStory{Title: "Built on it", Description: "What it is", Goal: "Why it matters", Plan: "How it goes"})
	must[board.Story](t)(b.AddDependency("demo", "tester", second.ID, first.ID))
	note := must[board.LogEntry](t)(b.AddLogEntry("demo", "tester", second.ID, board.NewLogEntry{Kind: "note", Text: "Looked at the old one"}))
	must[board.LogEntry](t)(b.AddLogEntry("demo", "tester", second.ID, board.NewLogEntry{Kind: "correction", Text: "It was the new one", Corrects: note.ID}))

	wantPage(t, get(h, "/w/demo/stories/"+second.ID, "bp_good"),
		"Built on it", "What it is", "Why it matters", "How it goes",
		"Depends on", `href="/w/demo/stories/S-1"`, "Groundwork",
		"Looked at the old one", "It was the new one", "Corrects "+note.ID, `id="`+note.ID+`"`)
	wantPage(t, get(h, "/w/demo/stories/"+first.ID, "bp_good"), "Blocks", `href="/w/demo/stories/S-2"`)
}

func TestAnUnknownWorkspaceOrStoryIsNotFound(t *testing.T) {
	h, _ := withDemo(t)
	for _, path := range []string{"/w/nope", "/w/demo/stories/S-99", "/w/nope/phases", "/w/nope/ideas"} {
		if w := get(h, path, "bp_good"); w.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, w.Code)
		}
	}
}

func TestPhasesShowTheirGoalsAndStories(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "Foundations", "Everything else stands on it"))
	must[board.Goal](t)(b.AddGoal("demo", "tester", 1, "The slab is level"))
	story(t, b, board.NewStory{Title: "Lay the slab", Phase: 1})
	story(t, b, board.NewStory{Title: "Unphased chore"})
	w := get(h, "/w/demo/phases", "bp_good")
	wantPage(t, w, "Foundations", "Everything else stands on it", "The slab is level", "Lay the slab")
	if strings.Contains(w.Body.String(), "Unphased chore") {
		t.Error("a story with no phase is listed under a phase")
	}
}

func TestIdeasAreListedWithTheirContext(t *testing.T) {
	h, b := withDemo(t)
	must[board.Idea](t)(b.AddIdea("demo", "tester", board.NewIdea{Title: "Dark mode", Context: "Asked for twice"}))
	wantPage(t, get(h, "/w/demo/ideas", "bp_good"), "Dark mode", "Asked for twice")
}

func TestSearchWithoutAnIndexSaysItIsUnavailable(t *testing.T) {
	h, _ := withDemo(t)
	wantPage(t, get(h, "/w/demo/search?q=slab", "bp_good"), "Search is unavailable")
}

func TestSearchLinksEachMatchToItsPage(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b := must[*board.Board](t)(board.New(db, prefix+"system", prefix+"ws:"))
	url, key, cprefix := typesensetest.Prefix(t)
	x := search.New(url, key, cprefix, b)
	b.OnChange(x.Report)
	h := New(known, b, x, nil)
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}))
	story(t, b, board.NewStory{Title: "Export invoices as PDF"})

	wantPage(t, get(h, "/w/demo/search?q=invoices", "bp_good"), "Export invoices as PDF", `href="/w/demo/stories/S-1"`)
}

// cardTag is the opening tag of the card for story id on page.
func cardTag(t *testing.T, page, id string) string {
	t.Helper()
	i := strings.Index(page, `data-id="`+id+`"`)
	if i < 0 {
		i = strings.Index(page, `<span class="id">`+id+`</span>`)
	}
	if i < 0 {
		t.Fatalf("no card for %s", id)
	}
	start := strings.LastIndex(page[:i], "<article")
	return page[start : start+strings.Index(page[start:], ">")+1]
}

func TestEachCardSaysWhichColumnsItMayBeDroppedOn(t *testing.T) {
	h, b := withDemo(t)
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "staged", Name: "Staged", HasReleaseStage: true}))
	mk := func(ws string, n board.NewStory) board.Story {
		return must[board.Story](t)(b.AddStory(ws, "tester", n))
	}
	set := func(ws, id string, c board.StatusChange) {
		must[board.Story](t)(b.SetStatus(ws, "tester", id, c))
	}
	prop := mk("demo", board.NewStory{Title: "Maybe", Proposed: true, Reasoning: "x"})
	back := mk("demo", board.NewStory{Title: "Queued"})
	held := mk("demo", board.NewStory{Title: "Parked"})
	set("demo", held.ID, board.StatusChange{To: board.InProgress, Branch: "feature/h"})
	set("demo", held.ID, board.StatusChange{To: board.OnHold, Reason: "waiting"})
	merged := func(ws string) board.Story {
		s := mk(ws, board.NewStory{Title: "Landed"})
		set(ws, s.ID, board.StatusChange{To: board.InProgress, Branch: "feature/m"})
		set(ws, s.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/2"})
		set(ws, s.ID, board.StatusChange{To: board.Merged, Commit: "abc1234"})
		return s
	}
	plain, staged := merged("demo"), merged("staged")

	page := get(h, "/w/demo", "bp_good").Body.String()
	for _, c := range []struct {
		id, status, moves string
	}{
		{prop.ID, "proposed", "on_hold backlog closed"},
		{back.ID, "backlog", "on_hold in_progress closed"},
		{held.ID, "on_hold", "in_progress closed"},
		{plain.ID, "merged", "deployed"},
	} {
		tag := cardTag(t, page, c.id)
		for _, want := range []string{`draggable="true"`, `data-status="` + c.status + `"`, `data-moves="` + c.moves + `"`} {
			if !strings.Contains(tag, want) {
				t.Errorf("%s: %s lacks %s", c.id, tag, want)
			}
		}
	}
	sp := get(h, "/w/staged", "bp_good").Body.String()
	if tag := cardTag(t, sp, staged.ID); !strings.Contains(tag, `data-moves="released"`) {
		t.Errorf("merged card with a release stage: %s", tag)
	}
	for _, status := range []string{"proposed", "backlog", "on_hold", "in_progress", "in_review", "merged", "deployed", "closed"} {
		if !strings.Contains(page, `data-status="`+status+`"`) {
			t.Errorf("no column marked %s", status)
		}
	}
}

func TestDeployedClosedAndIdeaCardsAreNotDraggable(t *testing.T) {
	h, b := withDemo(t)
	set := func(id string, c board.StatusChange) {
		must[board.Story](t)(b.SetStatus("demo", "tester", id, c))
	}
	done := story(t, b, board.NewStory{Title: "Shipped"})
	set(done.ID, board.StatusChange{To: board.InProgress, Branch: "feature/d"})
	set(done.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/3"})
	set(done.ID, board.StatusChange{To: board.Merged, Commit: "abc1234"})
	set(done.ID, board.StatusChange{To: board.Deployed})
	shut := story(t, b, board.NewStory{Title: "Dropped"})
	set(shut.ID, board.StatusChange{To: board.Closed, Reason: "no"})
	must[board.Idea](t)(b.AddIdea("demo", "tester", board.NewIdea{Title: "Vague"}))

	page := get(h, "/w/demo", "bp_good").Body.String()
	for _, id := range []string{done.ID, shut.ID, "I-1"} {
		if tag := cardTag(t, page, id); strings.Contains(tag, "draggable") || strings.Contains(tag, "data-moves") {
			t.Errorf("%s is draggable: %s", id, tag)
		}
	}
	if strings.Contains(page, `aria-label="Ideas" data-status`) {
		t.Error("Ideas column takes drops")
	}
}
