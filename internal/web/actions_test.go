package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

const host = "backpack.example.com"

// act submits form to path as the signed-in tester, checks it is sent back,
// and returns the page it is sent back to, with any message it left there.
func act(t *testing.T, h http.Handler, path string, form url.Values, back string) string {
	t.Helper()
	w := post(h, host, path, "bp_good", form)
	wantRedirect(t, w, back)
	r := httptest.NewRequest("GET", back, nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "bp_good"})
	for _, c := range w.Result().Cookies() {
		if c.Name != cookieName && c.MaxAge >= 0 {
			r.AddCookie(c)
		}
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, r)
	return page.Body.String()
}

func status(t *testing.T, b *board.Board, id string) string {
	t.Helper()
	return must[board.Story](t)(b.Story("demo", id)).Status
}

func TestProposedStoriesCanBeApprovedTogether(t *testing.T) {
	h, b := withDemo(t)
	one := story(t, b, board.NewStory{Title: "One", Proposed: true, Reasoning: "first"})
	two := story(t, b, board.NewStory{Title: "Two", Proposed: true, Reasoning: "second"})
	wantPage(t, get(h, "/w/demo", "bp_good"), "Waiting for approval", `value="`+one.ID+`"`)

	act(t, h, "/w/demo/approve", url.Values{"id": {one.ID, two.ID}, "decision": {"approve"}}, "/w/demo")
	if status(t, b, one.ID) != board.Backlog || status(t, b, two.ID) != board.Backlog {
		t.Fatal("approved stories are not in the backlog")
	}
}

func TestDecliningProposedStoriesNeedsAReason(t *testing.T) {
	h, b := withDemo(t)
	one := story(t, b, board.NewStory{Title: "One", Proposed: true, Reasoning: "first"})
	page := act(t, h, "/w/demo/approve", url.Values{"id": {one.ID}, "decision": {"decline"}}, "/w/demo")
	if !strings.Contains(page, `class="error"`) || status(t, b, one.ID) != board.Proposed {
		t.Fatal("a decline without a reason was not refused on the page")
	}
	act(t, h, "/w/demo/approve", url.Values{"id": {one.ID}, "decision": {"decline"}, "reason": {"Not needed"}}, "/w/demo")
	if status(t, b, one.ID) != board.Closed {
		t.Fatal("declined story is not closed")
	}
}

func TestABacklogStoryCanBeMovedUp(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "First"})
	second := story(t, b, board.NewStory{Title: "Second"})
	page := get(h, "/w/demo", "bp_good").Body.String()
	if !strings.Contains(page, `action="/w/demo/stories/`+second.ID+`/move"`) {
		t.Fatal("the board offers no move for a backlog story")
	}
	act(t, h, "/w/demo/stories/"+second.ID+"/move", url.Values{"before": {first.ID}}, "/w/demo")
	all := must[[]board.Story](t)(b.Stories("demo"))
	if all[0].ID != second.ID {
		t.Fatalf("order %s, %s; want %s first", all[0].ID, all[1].ID, second.ID)
	}
}

func TestAMoveThatBreaksADependencyShowsWhy(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "First"})
	second := story(t, b, board.NewStory{Title: "Second"})
	must[board.Story](t)(b.AddDependency("demo", "tester", second.ID, first.ID))
	page := act(t, h, "/w/demo/stories/"+second.ID+"/move", url.Values{"before": {first.ID}}, "/w/demo")
	if !strings.Contains(page, `class="error"`) || !strings.Contains(page, first.ID) {
		t.Fatal("the refusal's reason is not on the page")
	}
	if again := get(h, "/w/demo", "bp_good").Body.String(); strings.Contains(again, `class="error"`) {
		t.Fatal("the message is shown again on the next page")
	}
}

func TestAStoryCanBeStartedAndItsActivitySet(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Work"})
	wantPage(t, get(h, "/w/demo/stories/"+s.ID, "bp_good"), `value="in_progress"`)
	back := "/w/demo/stories/" + s.ID
	act(t, h, back+"/status", url.Values{"to": {"in_progress"}, "branch": {"feature/x"}}, back)
	act(t, h, back+"/activity", url.Values{"activity": {"waiting"}, "waiting_on": {"review"}}, back)
	got := must[board.Story](t)(b.Story("demo", s.ID))
	if got.Status != board.InProgress || got.Branch != "feature/x" || got.Activity != "waiting" || got.WaitingOn != "review" {
		t.Fatalf("story %+v", got)
	}
}

func TestAnEmptyStatusFieldIsLeftOut(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Work"})
	back := "/w/demo/stories/" + s.ID
	act(t, h, back+"/status", url.Values{"to": {"on_hold"}, "reason": {"Needs info"}, "branch": {""}, "pr_url": {""}}, back)
	if status(t, b, s.ID) != board.OnHold {
		t.Fatal("story is not on hold")
	}
}

func TestALogEntryCanBeAdded(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Work"})
	back := "/w/demo/stories/" + s.ID
	page := act(t, h, back+"/log", url.Values{"kind": {"decision"}, "text": {"Use cookies"}, "reason": {"Browsers can't send headers"}}, back)
	if !strings.Contains(page, "Use cookies") || !strings.Contains(page, "Browsers can&#39;t send headers") {
		t.Fatal("the decision is not in the log")
	}
}

func TestAStoryCanBeAddedAfterAnother(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "First"})
	story(t, b, board.NewStory{Title: "Second"})
	wantPage(t, get(h, "/w/demo/stories/new", "bp_good"), `name="title"`, `value="`+first.ID+`"`)
	w := post(h, host, "/w/demo/stories", "bp_good", url.Values{"title": {"Squeezed in"}, "after": {first.ID}, "phase": {""}})
	wantRedirect(t, w, "/w/demo/stories/S-3")
	all := must[[]board.Story](t)(b.Stories("demo"))
	if all[1].Title != "Squeezed in" || all[1].Status != board.Backlog {
		t.Fatalf("second story %+v, want the new one in the backlog", all[1])
	}
}

func TestAStoryWithoutATitleIsRefusedOnTheForm(t *testing.T) {
	h, _ := withDemo(t)
	page := act(t, h, "/w/demo/stories", url.Values{"title": {""}}, "/w/demo/stories/new")
	if !strings.Contains(page, `class="error"`) {
		t.Fatal("no reason shown")
	}
}

func TestIdeasCanBeAddedRefinedAndDropped(t *testing.T) {
	h, b := withDemo(t)
	act(t, h, "/w/demo/ideas", url.Values{"title": {"Dark mode"}, "context": {"Asked twice"}}, "/w/demo/ideas")
	act(t, h, "/w/demo/ideas", url.Values{"title": {"Sounds"}, "context": {""}}, "/w/demo/ideas")
	s := story(t, b, board.NewStory{Title: "Theme switch"})

	act(t, h, "/w/demo/ideas/I-1/refine", url.Values{"stories": {s.ID}}, "/w/demo/ideas")
	act(t, h, "/w/demo/ideas/I-2/drop", url.Values{"reason": {"Nobody wants it"}}, "/w/demo/ideas")
	if left := must[[]board.Idea](t)(b.Ideas("demo")); len(left) != 0 {
		t.Fatalf("ideas still open: %+v", left)
	}
	if i := must[board.Idea](t)(b.Idea("demo", "I-1")); len(i.BecameStories) != 1 || i.BecameStories[0] != s.ID {
		t.Fatalf("I-1 %+v", i)
	}
}

func TestActionsNeedASignedInBrowser(t *testing.T) {
	h, _ := withDemo(t)
	wantRedirect(t, post(h, host, "/w/demo/approve", "", url.Values{"id": {"S-1"}}), "/sign-in")
}

func TestActionsAreRecordedUnderTheTokensActor(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Work"})
	back := "/w/demo/stories/" + s.ID
	act(t, h, back+"/log", url.Values{"kind": {"note"}, "text": {"hello"}}, back)
	log := must[[]board.LogEntry](t)(b.Log("demo", s.ID))
	if last := log[len(log)-1]; last.Actor != "tester" {
		t.Fatalf("entry by %q, want tester", last.Actor)
	}
}

func TestAWaitingStoryCanGoBackToWork(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Work"})
	back := "/w/demo/stories/" + s.ID
	act(t, h, back+"/status", url.Values{"to": {"in_progress"}, "branch": {"feature/x"}}, back)
	act(t, h, back+"/activity", url.Values{"activity": {"waiting"}, "waiting_on": {"review"}}, back)
	// The form still holds what it was waiting on when the activity changes.
	page := act(t, h, back+"/activity", url.Values{"activity": {"building"}, "waiting_on": {"review"}}, back)
	if strings.Contains(page, `class="error"`) {
		t.Fatal("leaving waiting was refused")
	}
	if got := must[board.Story](t)(b.Story("demo", s.ID)); got.Activity != "building" {
		t.Fatalf("activity %q, want building", got.Activity)
	}
}

func TestAProposalCanBeHeldFromItsPage(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Maybe", Proposed: true, Reasoning: "it might help"})
	back := "/w/demo/stories/" + s.ID
	wantPage(t, get(h, back, "bp_good"), `value="on_hold"`)
	act(t, h, back+"/status", url.Values{"to": {"on_hold"}, "reason": {"Wait for the design"}}, back)
	if status(t, b, s.ID) != board.OnHold {
		t.Fatal("proposal is not on hold")
	}
	wantPage(t, get(h, back, "bp_good"), `value="proposed"`)
}

// order is every story's id in delivery order.
func order(t *testing.T, b *board.Board) string {
	t.Helper()
	var ids []string
	for _, s := range must[[]board.Story](t)(b.Stories("demo")) {
		ids = append(ids, s.ID)
	}
	return strings.Join(ids, " ")
}

func TestAStatusChangeFromTheBoardReturnsToTheBoard(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Work"})
	act(t, h, "/w/demo/stories/"+s.ID+"/status", url.Values{"to": {"in_progress"}, "branch": {"feature/x"}, "back": {"board"}}, "/w/demo")
	if status(t, b, s.ID) != board.InProgress {
		t.Fatal("story was not started")
	}
	// A refusal is shown on the board too.
	page := act(t, h, "/w/demo/stories/"+s.ID+"/status", url.Values{"to": {"deployed"}, "back": {"board"}}, "/w/demo")
	if !strings.Contains(page, `class="error"`) {
		t.Fatal("the refusal is not shown on the board")
	}
}

func TestBackIsOnlyEverTheBoard(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Work"})
	story := "/w/demo/stories/" + s.ID
	for _, back := range []string{"https://evil.example/", "/w/other", "//evil.example", ""} {
		act(t, h, story+"/status", url.Values{"to": {"on_hold"}, "reason": {"r"}, "back": {back}}, story)
		act(t, h, story+"/status", url.Values{"to": {"backlog"}}, story)
	}
}

func TestAProposalDroppedInTheBacklogIsPlacedThenApproved(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "First"})
	second := story(t, b, board.NewStory{Title: "Second"})
	p := story(t, b, board.NewStory{Title: "Maybe", Proposed: true, Reasoning: "might help"})
	act(t, h, "/w/demo/stories/"+p.ID+"/status", url.Values{"to": {"backlog"}, "before": {second.ID}, "back": {"board"}}, "/w/demo")
	all := must[[]board.Story](t)(b.Stories("demo"))
	if len(all) != 3 || all[0].ID != first.ID || all[1].ID != p.ID || all[1].Status != board.Backlog || all[2].ID != second.ID {
		t.Fatalf("order %+v, want First, the proposal in the backlog, Second", all)
	}
}

func TestAPlacementThatBreaksADependencyLeavesTheProposalUnchanged(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "First"})
	p := story(t, b, board.NewStory{Title: "Maybe", Proposed: true, Reasoning: "might help"})
	must[board.Story](t)(b.AddDependency("demo", "tester", p.ID, first.ID))
	before := order(t, b)
	page := act(t, h, "/w/demo/stories/"+p.ID+"/status", url.Values{"to": {"backlog"}, "before": {first.ID}, "back": {"board"}}, "/w/demo")
	if !strings.Contains(page, `class="error"`) {
		t.Fatal("the refusal is not shown")
	}
	if status(t, b, p.ID) != board.Proposed || order(t, b) != before {
		t.Fatalf("proposal changed: %s -> %s, status %s", before, order(t, b), status(t, b, p.ID))
	}
}

func TestADropThatTheStatusFlowRefusesDoesNotMoveTheStory(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "First"})
	second := story(t, b, board.NewStory{Title: "Second"})
	must[board.Story](t)(b.SetStatus("demo", "tester", second.ID, board.StatusChange{To: board.InProgress, Branch: "feature/x"}))
	must[board.Story](t)(b.SetStatus("demo", "tester", second.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/1"}))
	before := order(t, b)
	// In review cannot go straight back to the backlog.
	act(t, h, "/w/demo/stories/"+second.ID+"/status", url.Values{"to": {"backlog"}, "before": {first.ID}, "back": {"board"}}, "/w/demo")
	if status(t, b, second.ID) != board.InReview || order(t, b) != before {
		t.Fatalf("story changed: %s -> %s", before, order(t, b))
	}
}

func TestABoardDropWithAReasonHoldsClosesAndDeclines(t *testing.T) {
	h, b := withDemo(t)
	drop := func(id, to string) {
		act(t, h, "/w/demo/stories/"+id+"/status", url.Values{"to": {to}, "reason": {"Not now"}, "back": {"board"}}, "/w/demo")
	}
	held := story(t, b, board.NewStory{Title: "Held"})
	drop(held.ID, "on_hold")
	prop := story(t, b, board.NewStory{Title: "Proposal", Proposed: true, Reasoning: "x"})
	drop(prop.ID, "closed")
	for id, want := range map[string]string{held.ID: board.OnHold, prop.ID: board.Closed} {
		if got := status(t, b, id); got != want {
			t.Errorf("%s is %s, want %s", id, got, want)
		}
	}
	drop(held.ID, "closed")
	if status(t, b, held.ID) != board.Closed {
		t.Error("a held story is not closed")
	}
}

func TestABoardDropWithoutAReasonIsRefusedOnTheBoard(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Work"})
	page := act(t, h, "/w/demo/stories/"+s.ID+"/status", url.Values{"to": {"on_hold"}, "back": {"board"}}, "/w/demo")
	if !strings.Contains(page, `class="error"`) || status(t, b, s.ID) != board.Backlog {
		t.Fatal("a hold without a reason was not refused on the board")
	}
}

func TestStartingABlockedStoryByDropNeedsAWorkaround(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "First"})
	second := story(t, b, board.NewStory{Title: "Second"})
	must[board.Story](t)(b.AddDependency("demo", "tester", second.ID, first.ID))
	path := "/w/demo/stories/" + second.ID + "/status"
	page := act(t, h, path, url.Values{"to": {"in_progress"}, "back": {"board"}}, "/w/demo")
	if !strings.Contains(page, `class="error"`) || status(t, b, second.ID) != board.Backlog {
		t.Fatal("starting a blocked story without a workaround was not refused on the board")
	}
	act(t, h, path, url.Values{"to": {"in_progress"}, "workaround": {"Stub it"}, "back": {"board"}}, "/w/demo")
	if status(t, b, second.ID) != board.InProgress {
		t.Fatal("a blocked story with a workaround did not start")
	}
}
