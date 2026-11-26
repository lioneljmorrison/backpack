package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestThePhasesPageIsAnIndexLinkingToEachPhase(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "Foundations", "Everything else stands on it"))
	must[board.Phase](t)(b.AddPhase("demo", "tester", "Features", ""))
	closed := true
	must[board.Phase](t)(b.UpdatePhase("demo", "tester", 2, board.PhasePatch{Closed: &closed}))
	story(t, b, board.NewStory{Title: "Lay the slab", Phase: 1})
	w := get(h, "/w/demo/phases", "bp_good")
	wantPage(t, w, "Foundations", `href="/w/demo/phases/1"`, `href="/w/demo/phases/2"`, "Closed", `action="/w/demo/phases"`)
	if strings.Contains(w.Body.String(), "Lay the slab") {
		t.Error("the index lists a phase's stories")
	}
}

func TestAPhaseCanBeCreatedFromThePhasesPage(t *testing.T) {
	h, b := withDemo(t)
	act(t, h, "/w/demo/phases", url.Values{"title": {"Foundations"}, "description": {"first"}}, "/w/demo/phases")
	ps := must[[]board.Phase](t)(b.Phases("demo"))
	if len(ps) != 1 || ps[0].Title != "Foundations" || ps[0].Description != "first" {
		t.Fatalf("phases %+v", ps)
	}
	page := act(t, h, "/w/demo/phases", url.Values{"title": {""}}, "/w/demo/phases")
	if !strings.Contains(page, "title is required") {
		t.Fatal("the refusal is not shown")
	}
}

func TestAPhasePageShowsItsGoalsAndWhatIsLeftInRankOrder(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "Foundations", "Everything else stands on it"))
	must[board.Goal](t)(b.AddGoal("demo", "tester", 1, "The slab is level"))
	first := story(t, b, board.NewStory{Title: "Pour footings", Phase: 1})
	second := story(t, b, board.NewStory{Title: "Lay the slab", Phase: 1})
	third := story(t, b, board.NewStory{Title: "Frame the walls", Phase: 1})
	done := story(t, b, board.NewStory{Title: "Survey the plot", Phase: 1})
	story(t, b, board.NewStory{Title: "Unphased chore"})
	must[board.Story](t)(b.AddDependency("demo", "tester", second.ID, first.ID))
	must[board.Story](t)(b.SetStatus("demo", "tester", done.ID, board.StatusChange{To: board.InProgress, Branch: "feature/x"}))
	must[board.Story](t)(b.SetStatus("demo", "tester", done.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/1"}))
	must[board.Story](t)(b.SetStatus("demo", "tester", done.ID, board.StatusChange{To: board.Merged, Commit: "abc123", Into: "main"}))
	// the first story is under way, so the next ready one is the third
	must[board.Story](t)(b.SetStatus("demo", "tester", first.ID, board.StatusChange{To: board.InProgress, Branch: "feature/y"}))

	w := get(h, "/w/demo/phases/1", "bp_good")
	wantPage(t, w, "Foundations", "Everything else stands on it", "The slab is level", "Pour footings", "Lay the slab", "Frame the walls",
		"Survey the plot", "Blocked by "+first.ID, "Next")
	page := w.Body.String()
	if !before(page, "Pour footings", "Lay the slab") || !before(page, "Lay the slab", "Frame the walls") {
		t.Error("what is left is not in rank order")
	}
	if !strings.Contains(page, `data-next="`+third.ID+`"`) {
		t.Errorf("the next ready story is not %s", third.ID)
	}
	if !before(page, "Frame the walls", "Done (1)") || !before(page, "Done (1)", "Survey the plot") {
		t.Error("the finished story is not listed apart, after what is left")
	}
	if !before(page, "Add stories", "Unphased chore") {
		t.Error("the unphased story is not offered for adding")
	}
}

func TestAPhasePageThatDoesNotExistIsNotFound(t *testing.T) {
	h, _ := withDemo(t)
	for _, p := range []string{"/w/demo/phases/9", "/w/demo/phases/one"} {
		if w := get(h, p, "bp_good"); w.Code != 404 {
			t.Errorf("%s: status %d", p, w.Code)
		}
	}
}

func TestAPhaseIsEditedFromItsPage(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "Old", "old text"))
	back := "/w/demo/phases/1"
	wantPage(t, get(h, back, "bp_good"), `action="`+back+`/edit"`)
	act(t, h, back+"/edit", url.Values{"title": {"New"}, "description": {"new text"}}, back)
	p := must[board.Phase](t)(b.Phase("demo", 1))
	if p.Title != "New" || p.Description != "new text" {
		t.Fatalf("phase %+v", p)
	}
	page := act(t, h, back+"/edit", url.Values{"title": {""}, "description": {"x"}}, back)
	if !strings.Contains(page, "title cannot be empty") {
		t.Fatal("the refusal is not shown")
	}
}

func TestAPhaseIsClosedAndReopenedFromItsPage(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "One", ""))
	back := "/w/demo/phases/1"
	wantPage(t, get(h, back, "bp_good"), `action="`+back+`/closed"`, "Close this phase")
	act(t, h, back+"/closed", url.Values{"closed": {"true"}}, back)
	if !must[board.Phase](t)(b.Phase("demo", 1)).Closed {
		t.Fatal("not closed")
	}
	wantPage(t, get(h, back, "bp_good"), "Reopen this phase", "Closed")
	act(t, h, back+"/closed", url.Values{"closed": {"false"}}, back)
	if must[board.Phase](t)(b.Phase("demo", 1)).Closed {
		t.Fatal("not reopened")
	}
}

func TestGoalsAreAddedEditedAndMarkedFromThePhasePage(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "One", ""))
	back := "/w/demo/phases/1"
	act(t, h, back+"/goals", url.Values{"text": {"It runs"}}, back)
	wantPage(t, get(h, back, "bp_good"), "It runs", `action="`+back+`/goals/G-1/text"`, `action="`+back+`/goals/G-1/met"`)
	act(t, h, back+"/goals/G-1/text", url.Values{"text": {"It runs fast"}}, back)
	act(t, h, back+"/goals/G-1/met", url.Values{"met": {"true"}}, back)
	g := must[board.Phase](t)(b.Phase("demo", 1)).Goals[0]
	if g.Text != "It runs fast" || !g.Met {
		t.Fatalf("goal %+v", g)
	}
	act(t, h, back+"/goals/G-1/met", url.Values{"met": {"false"}}, back)
	if must[board.Phase](t)(b.Phase("demo", 1)).Goals[0].Met {
		t.Fatal("not unmarked")
	}
	page := act(t, h, back+"/goals", url.Values{"text": {""}}, back)
	if !strings.Contains(page, "text is required") {
		t.Fatal("the refusal is not shown")
	}
}

func TestStoriesAreAddedToAPhaseInBulkAndARefusalDoesNotStopTheRest(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "One", ""))
	must[board.Phase](t)(b.AddPhase("demo", "tester", "Two", ""))
	on := story(t, b, board.NewStory{Title: "Base", Phase: 2})
	dep := story(t, b, board.NewStory{Title: "Needs base"})
	ok := story(t, b, board.NewStory{Title: "Free"})
	must[board.Story](t)(b.AddDependency("demo", "tester", dep.ID, on.ID))
	back := "/w/demo/phases/1"
	wantPage(t, get(h, back, "bp_good"), `action="`+back+`/stories"`, `value="`+dep.ID+`"`, `value="`+ok.ID+`"`)
	if strings.Contains(get(h, back, "bp_good").Body.String(), `value="`+on.ID+`"`) {
		t.Error("a story that has a phase is offered")
	}
	// dep may not join phase 1: it depends on a story in the later phase 2
	page := act(t, h, back+"/stories", url.Values{"id": {dep.ID, ok.ID}}, back)
	if !strings.Contains(page, dep.ID+": ") || !strings.Contains(page, "later phase 2") {
		t.Fatal("the refusal is not reported")
	}
	if must[board.Story](t)(b.Story("demo", ok.ID)).Phase != 1 || must[board.Story](t)(b.Story("demo", dep.ID)).Phase != 0 {
		t.Fatal("the allowed story did not move, or the refused one did")
	}
	page = act(t, h, back+"/stories", url.Values{}, back)
	if !strings.Contains(page, "Choose at least one story") {
		t.Fatal("no message for an empty choice")
	}
}

func TestThePhasePagesWatchForChanges(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "One", ""))
	for _, p := range []string{"/w/demo/phases", "/w/demo/phases/1"} {
		wantPage(t, get(h, p, "bp_good"), `data-live="/w/demo/events"`, `data-live-kinds="story phase"`)
	}
}

func TestLinksToAPhaseGoToItsPage(t *testing.T) {
	h, b := withDemo(t)
	must[board.Phase](t)(b.AddPhase("demo", "tester", "One", ""))
	s := story(t, b, board.NewStory{Title: "Work", Phase: 1})
	for _, p := range []string{"/w/demo", "/w/demo/stories/" + s.ID} {
		page := get(h, p, "bp_good").Body.String()
		if strings.Contains(page, "phases#phase-") || !strings.Contains(page, `href="/w/demo/phases/1"`) {
			t.Errorf("%s links to the wrong place", p)
		}
	}
}
