package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/live"
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
	wantPage(t, w, "Demo", "Backlog", "In progress", "Merged (main)", `href="/w/demo/stories/S-1"`)
	page := w.Body.String()
	if !before(page, `aria-label="Backlog"`, `aria-label="In progress"`) || !before(page, `aria-label="In progress"`, `aria-label="Merged (main)"`) {
		t.Error("columns are not in board order")
	}
	if !before(page, "First in line", "Second in line") {
		t.Error("backlog is not in delivery order")
	}
	if strings.Contains(page, "Merged (dev)") {
		t.Error("a column for a branch the workspace does not have")
	}
}

func TestTheBoardRunsFromIdeasThroughOnHoldAndBacklogToClosed(t *testing.T) {
	h, _ := withDemo(t)
	page := get(h, "/w/demo", "bp_good").Body.String()
	order := []string{"Ideas", "Proposed", "On hold", "Backlog", "In progress", "Merged (main)", "Closed"}
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
	merged("demo")
	staged := merged("staged")

	page := get(h, "/w/demo", "bp_good").Body.String()
	for _, c := range []struct {
		id, status, moves string
	}{
		{prop.ID, "proposed", "on_hold backlog closed"},
		{back.ID, "backlog", "on_hold in_progress closed"},
		{held.ID, "on_hold", "in_progress closed"},
	} {
		tag := cardTag(t, page, c.id)
		for _, want := range []string{`draggable="true"`, `data-status="` + c.status + `"`, `data-moves="` + c.moves + `"`} {
			if !strings.Contains(tag, want) {
				t.Errorf("%s: %s lacks %s", c.id, tag, want)
			}
		}
	}
	sp := get(h, "/w/staged", "bp_good").Body.String()
	if tag := cardTag(t, sp, staged.ID); !strings.Contains(tag, `data-moves="merged:main"`) {
		t.Errorf("a card merged into dev may be dropped on Merged (main): %s", tag)
	}
	for _, want := range []string{`aria-label="Merged (dev)" data-status="merged" data-into="dev"`, `aria-label="Merged (main)" data-status="merged" data-into="main"`} {
		if !strings.Contains(sp, want) {
			t.Errorf("staged board lacks %s", want)
		}
	}
	if !before(sp, `aria-label="Merged (dev)"`, `aria-label="Merged (main)"`) || !before(sp, `aria-label="Merged (main)"`, `aria-label="Closed"`) {
		t.Error("merged columns are not in merge order, or Closed does not follow them")
	}
	for _, status := range []string{"proposed", "backlog", "on_hold", "in_progress", "in_review", "merged", "closed"} {
		if !strings.Contains(page, `data-status="`+status+`"`) {
			t.Errorf("no column marked %s", status)
		}
	}
}

func TestMergedIntoTheReleaseBranchReleasedClosedAndIdeaCardsAreNotDraggable(t *testing.T) {
	h, b := withDemo(t)
	set := func(id string, c board.StatusChange) {
		must[board.Story](t)(b.SetStatus("demo", "tester", id, c))
	}
	done := story(t, b, board.NewStory{Title: "Shipped"})
	set(done.ID, board.StatusChange{To: board.InProgress, Branch: "feature/d"})
	set(done.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/3"})
	set(done.ID, board.StatusChange{To: board.Merged, Commit: "abc1234"})
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1", Tag: "v1"}))
	shut := story(t, b, board.NewStory{Title: "Dropped"})
	set(shut.ID, board.StatusChange{To: board.Closed, Reason: "no"})
	must[board.Idea](t)(b.AddIdea("demo", "tester", board.NewIdea{Title: "Vague"}))
	// A story merged into the release branch waits for a release, and may
	// not be moved to Released by hand.
	waiting := story(t, b, board.NewStory{Title: "Waiting for a release"})
	set(waiting.ID, board.StatusChange{To: board.InProgress, Branch: "feature/w"})
	set(waiting.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/4"})
	set(waiting.ID, board.StatusChange{To: board.Merged, Commit: "abc1235"})

	page := get(h, "/w/demo", "bp_good").Body.String()
	for _, id := range []string{shut.ID, "I-1", waiting.ID} {
		if tag := cardTag(t, page, id); strings.Contains(tag, "draggable") || strings.Contains(tag, "data-moves") {
			t.Errorf("%s is draggable: %s", id, tag)
		}
	}
	if strings.Contains(page, `aria-label="Ideas" data-status`) {
		t.Error("Ideas column takes drops")
	}
}

// The web board ends at the last Merged column, then Closed: a released
// story is read on the Releases pages, not as a card.
func TestTheBoardShowsNoReleasedColumnAndNoReleasedCards(t *testing.T) {
	h, b := withDemo(t)
	set := func(id string, c board.StatusChange) {
		must[board.Story](t)(b.SetStatus("demo", "tester", id, c))
	}
	done := story(t, b, board.NewStory{Title: "Shipped in a release"})
	set(done.ID, board.StatusChange{To: board.InProgress, Branch: "feature/d"})
	set(done.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/3"})
	set(done.ID, board.StatusChange{To: board.Merged, Commit: "abc1234"})
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1", Tag: "v1"}))

	page := get(h, "/w/demo", "bp_good").Body.String()
	if strings.Contains(page, `aria-label="Released"`) || strings.Contains(page, `data-status="released"`) {
		t.Error("the board has a Released column")
	}
	if strings.Contains(page, "Shipped in a release") || strings.Contains(page, `href="/w/demo/stories/`+done.ID+`"`) {
		t.Error("the board shows a released card")
	}
	if !before(page, `aria-label="Merged (main)"`, `aria-label="Closed"`) {
		t.Error("Closed does not follow the last Merged column")
	}
	if rel := get(h, "/w/demo/releases/R-1", "bp_good").Body.String(); !strings.Contains(rel, "Shipped in a release") {
		t.Error("the release page does not list the released story")
	}
}

func TestEveryCardOnTheBoardCarriesItsID(t *testing.T) {
	h, b := withDemo(t)
	set := func(id string, c board.StatusChange) {
		must[board.Story](t)(b.SetStatus("demo", "tester", id, c))
	}
	done := story(t, b, board.NewStory{Title: "Shipped"})
	set(done.ID, board.StatusChange{To: board.InProgress, Branch: "feature/d"})
	set(done.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/3"})
	set(done.ID, board.StatusChange{To: board.Merged, Commit: "abc1234"})
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1", Tag: "v1"}))
	shut := story(t, b, board.NewStory{Title: "Dropped"})
	set(shut.ID, board.StatusChange{To: board.Closed, Reason: "no"})
	idea := must[board.Idea](t)(b.AddIdea("demo", "tester", board.NewIdea{Title: "Vague"}))

	page := get(h, "/w/demo", "bp_good").Body.String()
	for _, id := range []string{shut.ID, idea.ID} {
		if tag := cardTag(t, page, id); !strings.Contains(tag, `data-id="`+id+`"`) {
			t.Errorf("%s card does not carry its id: %s", id, tag)
		}
	}
}

func TestTheBoardCarriesTheMovePrompt(t *testing.T) {
	h, _ := withDemo(t)
	page := get(h, "/w/demo", "bp_good").Body.String()
	d := strings.Index(page, `<dialog`)
	if d < 0 {
		t.Fatal("no move prompt on the board")
	}
	if strings.Index(page, "</main>") > d {
		t.Error("the prompt is inside main, which a live refresh replaces")
	}
	prompt := page[d : d+strings.Index(page[d:], "</dialog>")]
	for _, want := range []string{`name="reason"`, `name="branch"`, `name="workaround"`, `name="pr_url"`, `name="commit"`, `<label`, `required`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %s", want)
		}
	}
}

func TestABlockedCardSaysSo(t *testing.T) {
	h, b := withDemo(t)
	first := story(t, b, board.NewStory{Title: "First"})
	second := story(t, b, board.NewStory{Title: "Second"})
	free := story(t, b, board.NewStory{Title: "Free"})
	must[board.Story](t)(b.AddDependency("demo", "tester", second.ID, first.ID))
	page := get(h, "/w/demo", "bp_good").Body.String()
	if tag := cardTag(t, page, second.ID); !strings.Contains(tag, "data-blocked") {
		t.Errorf("blocked card: %s", tag)
	}
	for _, id := range []string{first.ID, free.ID} {
		if tag := cardTag(t, page, id); strings.Contains(tag, "data-blocked") {
			t.Errorf("%s is not blocked: %s", id, tag)
		}
	}
}

// Search answers a merged story's state as "merged:main"; a page reads it as
// the column is named.
func TestASearchStateWithABranchReadsAsTheColumnIsNamed(t *testing.T) {
	for state, want := range map[string]string{"merged:main": "Merged (main)", "merged:dev": "Merged (dev)", "in_review": "In review"} {
		if got := label(state); got != want {
			t.Errorf("label(%q) = %q, want %q", state, got, want)
		}
	}
}

func TestABranchAliasRenamesOnlyItsColumnHeading(t *testing.T) {
	h, b := withDemo(t)
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "staged", Name: "Staged", HasReleaseStage: true}))
	s := must[board.Story](t)(b.AddStory("staged", "tester", board.NewStory{Title: "Landed"}))
	for _, c := range []board.StatusChange{
		{To: board.InProgress, Branch: "feature/a"},
		{To: board.InReview, PRURL: "https://example.invalid/pr/3"},
		{To: board.Merged, Commit: "abc1234"},
	} {
		must[board.Story](t)(b.SetStatus("staged", "tester", s.ID, c))
	}
	prod := "Production"
	must[board.Branch](t)(b.UpdateBranch("staged", "tester", "main", board.BranchPatch{Alias: &prod}))

	page := get(h, "/w/staged", "bp_good").Body.String()
	for _, want := range []string{
		`aria-label="Merged (Production)" data-status="merged" data-into="main"`,
		`<h2>Merged (Production) `,
		`aria-label="Merged (dev)" data-status="merged" data-into="dev"`,
		`data-moves="merged:main"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("board lacks %s", want)
		}
	}
	if strings.Contains(page, "Merged (main)") {
		t.Error("the heading still shows the branch name")
	}
	// The story page keeps the real name.
	if sp := get(h, "/w/staged/stories/"+s.ID, "bp_good").Body.String(); strings.Contains(sp, "Production") {
		t.Error("the story page shows the alias")
	}

	empty := ""
	must[board.Branch](t)(b.UpdateBranch("staged", "tester", "main", board.BranchPatch{Alias: &empty}))
	page = get(h, "/w/staged", "bp_good").Body.String()
	if !strings.Contains(page, `aria-label="Merged (main)"`) || strings.Contains(page, "Production") {
		t.Error("clearing the alias does not restore the heading")
	}
}

// A story page for a released story says Released, offers no move, and reads
// the release's log entry and an old "deployed" one.
func TestAReleasedStoryReadsAsReleasedAndOffersNoMove(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Shipped"})
	for _, c := range []board.StatusChange{{To: board.InProgress}, {To: board.InReview}, {To: board.Merged, Commit: "abc1234"}} {
		must[board.Story](t)(b.SetStatus("demo", "tester", s.ID, c))
	}
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1.0.0", Tag: "v1.0.0"}))
	page := get(h, "/w/demo/stories/"+s.ID, "bp_good").Body.String()
	for _, want := range []string{"Released", "Released in R-1 (v1.0.0)"} {
		if !strings.Contains(page, want) {
			t.Errorf("the story page lacks %q", want)
		}
	}
	if strings.Contains(page, `name="to"`) {
		t.Error("a released story offers a move")
	}
}

func TestAReleasedStoryPageLinksToItsReleaseAndAnUnreleasedOneDoesNot(t *testing.T) {
	h, b := withDemo(t)
	one, two := shipped(t, b, "Shipped"), story(t, b, board.NewStory{Title: "Waiting"})
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1.0.0", Tag: "v1.0.0"}))
	page := get(h, "/w/demo/stories/"+one.ID, "bp_good").Body.String()
	if !strings.Contains(page, `href="/w/demo/releases/R-1"`) || !strings.Contains(page, "v1.0.0") {
		t.Error("the released story's page does not link to its release")
	}
	if page := get(h, "/w/demo/stories/"+two.ID, "bp_good").Body.String(); strings.Contains(page, "/releases/") {
		t.Error("an unreleased story's page links to a release")
	}
}

// Old log entries and old search states may still say "deployed".
func TestAnOldDeployedEntryStillReadsSensibly(t *testing.T) {
	if got := label("deployed"); got != "Deployed" {
		t.Errorf("label(deployed) = %q", got)
	}
	if got := label("released:main"); got != "Released" {
		t.Errorf("label(released:main) = %q, want Released without a branch", got)
	}
}

// shipped walks a story to merged (into the release branch of a workspace
// with one branch) and returns it.
func shipped(t *testing.T, b *board.Board, title string) board.Story {
	t.Helper()
	s := story(t, b, board.NewStory{Title: title})
	for _, c := range []board.StatusChange{{To: board.InProgress}, {To: board.InReview}, {To: board.Merged, Commit: "abc1234"}} {
		must[board.Story](t)(b.SetStatus("demo", "tester", s.ID, c))
	}
	return s
}

func TestAReleasePageShowsTheVersionTagDateNotesAndItsStoriesAsAChangelog(t *testing.T) {
	h, b := withDemo(t)
	one, two := shipped(t, b, "Shipped one"), shipped(t, b, "Shipped <two>")
	r := must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1.2.0", Tag: "rel-1.2.0", Commit: "9f8e7d6", Notes: "The first <b>cut</b>"}))
	page := get(h, "/w/demo/releases/"+r.ID, "bp_good")
	if page.Code != http.StatusOK {
		t.Fatalf("status %d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{"v1.2.0", "rel-1.2.0", "9f8e7d6", r.CreatedAt[:10], "The first &lt;b&gt;cut&lt;/b&gt;",
		`href="/w/demo/stories/` + one.ID + `"`, "Shipped one", `href="/w/demo/stories/` + two.ID + `"`, "Shipped &lt;two&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("the release page lacks %q", want)
		}
	}
	if !before(body, "Shipped one", "Shipped &lt;two&gt;") {
		t.Error("stories are not in delivery order")
	}
	if got := get(h, "/w/demo/releases/R-9", "bp_good"); got.Code != http.StatusNotFound {
		t.Errorf("unknown release: status %d", got.Code)
	}
	if got := get(h, "/w/demo/releases/"+r.ID, ""); got.Code == http.StatusOK {
		t.Error("a release page opens without signing in")
	}
}

func TestTheReleasesPageListsReleasesNewestFirstAndTheNavLinksToIt(t *testing.T) {
	h, b := withDemo(t)
	empty := get(h, "/w/demo/releases", "bp_good")
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), "No releases yet") {
		t.Fatalf("empty list: %d", empty.Code)
	}
	shipped(t, b, "One")
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1.0.0", Tag: "v1.0.0"}))
	shipped(t, b, "Two")
	must[board.Release](t)(b.CutRelease("demo", "tester", board.NewRelease{Version: "v1.1.0", Tag: "v1.1.0"}))
	body := get(h, "/w/demo/releases", "bp_good").Body.String()
	for _, want := range []string{`href="/w/demo/releases/R-1"`, `href="/w/demo/releases/R-2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the list lacks %q", want)
		}
	}
	if !before(body, "v1.1.0", "v1.0.0") {
		t.Error("the list is not newest first")
	}
	if !strings.Contains(get(h, "/w/demo", "bp_good").Body.String(), `href="/w/demo/releases"`) {
		t.Error("the navigation does not link to the releases")
	}
}

func TestTheBoardsWhereAreWeAreaCountsWhatWaitsForARelease(t *testing.T) {
	h, b := withDemo(t)
	shipped(t, b, "Waiting")
	body := get(h, "/w/demo", "bp_good").Body.String()
	i := strings.Index(body, "Waiting for release")
	if i < 0 || !strings.Contains(body[i:i+200], "<strong>1</strong>") {
		t.Fatalf("the where-are-we area does not count the unreleased story")
	}
}

// A story released before releases existed sits in R-0, whose page lists it
// and shows no empty tag.
func TestTheReleasePageForR0ListsItsStoriesAndShowsNoTag(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}))
	g := db.SelectGraph(prefix + "ws:demo")
	for _, q := range []string{
		"CREATE (:Story {id: 'S-1', title: 'Shipped long ago', status: 'released', into: 'main', rank: 'a'})",
		"MATCH (s:SchemaVersion) SET s.v = 9",
	} {
		if _, err := g.Query(q, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	b, err = board.New(db, prefix+"system", prefix+"ws:") // runs the R-0 migration
	if err != nil {
		t.Fatal(err)
	}
	h := New(known, b, nil, live.New())
	page := get(h, "/w/demo/releases/R-0", "bp_good").Body.String()
	for _, want := range []string{"R-0", "Before releases", "Shipped long ago", `href="/w/demo/stories/S-1"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the R-0 page lacks %q", want)
		}
	}
	if strings.Contains(page, "Tag") {
		t.Error("the R-0 page shows a tag")
	}
	if !strings.Contains(get(h, "/w/demo/stories/S-1", "bp_good").Body.String(), `href="/w/demo/releases/R-0"`) {
		t.Error("the story page does not link to R-0")
	}
}

// markedPair adds a story that depends on another, then changes the other's
// plan so the dependent story carries the upstream-changed mark.
func markedPair(t *testing.T) (h http.Handler, up, marked, free board.Story) {
	t.Helper()
	h, b := withDemo(t)
	up = story(t, b, board.NewStory{Title: "Upstream", Goal: "g", Plan: "p"})
	marked = story(t, b, board.NewStory{Title: "Dependent", Goal: "g", Plan: "p"})
	free = story(t, b, board.NewStory{Title: "Free", Goal: "g", Plan: "p"})
	must[board.Story](t)(b.AddDependency("demo", "tester", marked.ID, up.ID))
	plan := "a new plan"
	must[board.Story](t)(b.UpdateStory("demo", "tester", up.ID, board.StoryPatch{Plan: &plan}))
	return h, up, marked, free
}

func TestAMarkedCardShowsTheUpstreamChangedBadgeAndAnUnmarkedOneDoesNot(t *testing.T) {
	h, _, marked, free := markedPair(t)
	page := get(h, "/w/demo", "bp_good").Body.String()
	card := func(id string) string {
		i := strings.Index(page, `data-id="`+id+`"`)
		if i < 0 {
			t.Fatalf("no card for %s", id)
		}
		return page[i : i+strings.Index(page[i:], "</article>")]
	}
	if c := card(marked.ID); !strings.Contains(c, "Upstream changed") {
		t.Errorf("marked card has no badge: %s", c)
	}
	if c := card(free.ID); strings.Contains(c, "Upstream changed") {
		t.Errorf("unmarked card has the badge: %s", c)
	}
}

func TestAMarkedStoryPageNamesTheUpstreamStoryAndWhen(t *testing.T) {
	h, up, marked, free := markedPair(t)
	page := get(h, "/w/demo/stories/"+marked.ID, "bp_good").Body.String()
	if !strings.Contains(page, "Upstream changed") {
		t.Errorf("page does not say upstream changed: %s", page)
	}
	if !strings.Contains(page, `href="/w/demo/stories/`+up.ID+`"`) || !strings.Contains(page, "Upstream") {
		t.Errorf("page does not link the upstream story")
	}
	if !strings.Contains(page, `<time datetime="`) {
		t.Errorf("page has no time")
	}
	other := get(h, "/w/demo/stories/"+free.ID, "bp_good").Body.String()
	if strings.Contains(other, "Upstream changed") {
		t.Errorf("unmarked page says upstream changed")
	}
}

func TestABugShowsABadgeOnItsCardAndItsPage(t *testing.T) {
	h, b := withDemo(t)
	story(t, b, board.NewStory{Title: "Crash on save", Kind: "bug"})
	story(t, b, board.NewStory{Title: "Export"})
	page := get(h, "/w/demo", "bp_good").Body.String()
	if n := strings.Count(page, `class="tag bug"`); n != 1 {
		t.Errorf("the board has %d bug badges, want 1", n)
	}
	wantPage(t, get(h, "/w/demo/stories/S-1", "bp_good"), `class="tag bug"`)
	if strings.Contains(get(h, "/w/demo/stories/S-2", "bp_good").Body.String(), `class="tag bug"`) {
		t.Error("a story that is not a bug has the badge")
	}
}
