package board_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func ids(ss []board.Story) []string {
	out := []string{}
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return out
}

func TestWhatsNextIsTheTopBacklogStoryThatIsNotBlocked(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	addStory(t, b, board.NewStory{Title: "C", Goal: "g", Plan: "p"})
	depend(t, b, "S-2", "S-1")
	walk(t, b, "S-1", "in_progress")
	next, err := b.Next("demo")
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "S-3" || next.Goal != "g" || next.Plan != "p" {
		t.Fatalf("next %+v", next)
	}
}

func TestWhatsNextIsNothingWhenTheBacklogHasNothingReady(t *testing.T) {
	b := newWorkspace(t)
	if next, err := b.Next("demo"); err != nil || next != nil {
		t.Fatalf("next %+v, %v", next, err)
	}
	_, err := b.Next("nope")
	wantKind(t, err, board.NotFound)
}

// whereBoard is a workspace with one story in each place "where are we" reads.
func whereBoard(t *testing.T) *board.Board {
	t.Helper()
	b := newWorkspace(t)
	addPhase(t, b, "Foundations")
	addPhase(t, b, "Features")
	addStory(t, b, board.NewStory{Title: "Done", Phase: 1})      // S-1 merged
	addStory(t, b, board.NewStory{Title: "Building", Phase: 2})  // S-2 in progress
	addStory(t, b, board.NewStory{Title: "Mocked", Phase: 2})    // S-3 in progress, blocked
	addStory(t, b, board.NewStory{Title: "Reviewing", Phase: 2}) // S-4 in review
	addStory(t, b, board.NewStory{Title: "Unclear", Phase: 2})   // S-5 on hold
	for _, title := range []string{"Next one", "Then", "After", "Later"} {
		addStory(t, b, board.NewStory{Title: title, Phase: 2}) // S-6 … S-9
	}
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	walk(t, b, "S-2", "in_progress")
	if _, err := b.UpdateStory("demo", actor, "S-2", board.StoryPatch{Activity: ptr("waiting"), WaitingOn: ptr("the test agent")}); err != nil {
		t.Fatal(err)
	}
	depend(t, b, "S-3", "S-2")
	setStatus(t, b, "S-3", board.StatusChange{To: "in_progress", Workaround: "mock the S-2 API"})
	walk(t, b, "S-4", "in_progress")
	setStatus(t, b, "S-4", board.StatusChange{To: "in_review", PRURL: "https://example.com/pr/4"})
	setStatus(t, b, "S-5", board.StatusChange{To: "on_hold", Reason: "needs more info"})
	depend(t, b, "S-7", "S-6")
	return b
}

func TestWhereAreWeShowsThePhaseTheWorkUnderWayAndWhatComesNext(t *testing.T) {
	b := whereBoard(t)
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if w.Phase == nil || w.Phase.Number != 2 || w.Phase.StoriesTotal != 8 || w.Phase.StoriesComplete != 0 {
		t.Fatalf("phase %+v", w.Phase)
	}
	for name, c := range map[string]struct {
		got  []board.Story
		want []string
	}{
		"in progress": {w.InProgress, []string{"S-2", "S-3"}},
		"in review":   {w.InReview, []string{"S-4"}},
		"on hold":     {w.OnHold, []string{"S-5"}},
		"next":        {w.Next, []string{"S-6", "S-7", "S-8"}},
	} {
		if !slices.Equal(ids(c.got), c.want) {
			t.Errorf("%s %v, want %v", name, ids(c.got), c.want)
		}
	}
}

func TestWhereAreWeReadsAsAFewLinesOfText(t *testing.T) {
	b := whereBoard(t)
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	text := w.Text()
	for _, line := range []string{
		"Phase 2: Features — 0 of 8 stories complete",
		"  S-2 Building — waiting on the test agent",
		"  S-3 Mocked — planning; blocked by S-2, working around it: mock the S-2 API",
		"  S-4 Reviewing — https://example.com/pr/4",
		"  S-5 Unclear — needs more info (was in backlog)",
		"  S-6 Next one",
		"  S-7 Then — blocked by S-6",
	} {
		if !strings.Contains(text, line+"\n") {
			t.Errorf("missing %q in:\n%s", line, text)
		}
	}
	if strings.Contains(text, "S-9") {
		t.Errorf("more than three upcoming stories:\n%s", text)
	}
}

func TestWhereAreWeOnAnEmptyWorkspaceSaysSo(t *testing.T) {
	b := newWorkspace(t)
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if w.Phase != nil || w.Text() != "Nothing in progress, in review or in the backlog.\n" {
		t.Fatalf("empty %+v\n%s", w, w.Text())
	}
}

func TestTheCurrentPhaseIsTheFirstThatIsNotCompleteAndItsFlagShows(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addPhase(t, b, "Two")
	addStory(t, b, board.NewStory{Title: "A", Phase: 1})
	addStory(t, b, board.NewStory{Title: "B", Phase: 2})
	addGoal(t, b, 2, "fast")
	if _, err := b.SetGoalMet("demo", actor, "G-1", true); err != nil {
		t.Fatal(err)
	}
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if w.Phase == nil || w.Phase.Number != 2 || !strings.Contains(w.Text(), "every goal is met but stories are still open") {
		t.Fatalf("phase %+v\n%s", w.Phase, w.Text())
	}
}

func TestTheBoardGroupsStoriesByColumnInDeliveryOrder(t *testing.T) {
	b := whereBoard(t)
	cols, err := b.Board("demo")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	names := []string{}
	for _, c := range cols {
		names = append(names, c.Status)
		got[c.Status] = ids(c.Stories)
	}
	if !slices.Equal(names, []string{"proposed", "on_hold", "backlog", "in_progress", "in_review", "merged", "released", "closed"}) || cols[5].Branch != "main" {
		t.Fatalf("columns %v", names)
	}
	if !slices.Equal(got["backlog"], []string{"S-6", "S-7", "S-8", "S-9"}) || !slices.Equal(got["merged"], []string{"S-1"}) ||
		len(got["closed"]) != 0 {
		t.Fatalf("board %v", got)
	}
}

func TestTheBoardHasOneMergedColumnPerBranchInMergeOrder(t *testing.T) {
	b := newReleasingWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	walk(t, b, "S-2", "in_progress", "in_review", "merged")
	setStatus(t, b, "S-2", board.StatusChange{To: "merged", Into: "main"})
	cols, err := b.Board("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 9 || cols[5].Status != "merged" || cols[5].Branch != "dev" || cols[6].Status != "merged" || cols[6].Branch != "main" ||
		cols[7].Status != "released" || cols[7].Branch != "" || cols[6].Stories == nil {
		t.Fatalf("columns %+v", cols)
	}
	if !slices.Equal(ids(cols[5].Stories), []string{"S-1"}) || !slices.Equal(ids(cols[6].Stories), []string{"S-2"}) {
		t.Fatalf("dev %v, main %v", ids(cols[5].Stories), ids(cols[6].Stories))
	}
}

// A phase with nothing left in it is complete once its goals are met.
func TestAPhaseWithNothingLeftIsCompleteOnceItsGoalsAreMet(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addPhase(t, b, "Two")
	addGoal(t, b, 1, "fast")
	addStory(t, b, board.NewStory{Title: "A", Phase: 1})
	setStatus(t, b, "S-1", board.StatusChange{To: "closed", Reason: "dropped"})
	if p := phase(t, b, 1); p.Complete {
		t.Fatalf("complete with its goal unmet %+v", p)
	}
	if _, err := b.SetGoalMet("demo", actor, "G-1", true); err != nil {
		t.Fatal(err)
	}
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !phase(t, b, 1).Complete || w.Phase == nil || w.Phase.Number != 2 {
		t.Fatalf("phase 1 %+v, current %+v", phase(t, b, 1), w.Phase)
	}
}

func TestAPhaseWithNothingInItAndNoGoalsIsNotComplete(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addStory(t, b, board.NewStory{Title: "A", Phase: 1})
	setStatus(t, b, "S-1", board.StatusChange{To: "closed", Reason: "dropped"})
	if p := phase(t, b, 1); p.Complete {
		t.Fatalf("complete %+v", p)
	}
}
