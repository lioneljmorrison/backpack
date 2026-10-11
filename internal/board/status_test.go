package board_test

import (
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func setStatus(t *testing.T, b *board.Board, id string, c board.StatusChange) board.Story {
	t.Helper()
	s, err := b.SetStatus("demo", actor, id, c)
	if err != nil {
		t.Fatalf("%s to %s: %v", id, c.To, err)
	}
	return s
}

// walk moves a story through each status in turn.
func walk(t *testing.T, b *board.Board, id string, to ...string) {
	t.Helper()
	for _, s := range to {
		setStatus(t, b, id, board.StatusChange{To: s})
	}
}

func readLog(t *testing.T, b *board.Board, id string) []board.LogEntry {
	t.Helper()
	l, err := b.Log("demo", id)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func wantStatus(t *testing.T, b *board.Board, id, want string) board.Story {
	t.Helper()
	s, err := b.Story("demo", id)
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != want {
		t.Fatalf("%s is %s, want %s", id, s.Status, want)
	}
	return s
}

// newReleasingWorkspace returns a board whose workspace "demo" has a release stage.
func newReleasingWorkspace(t *testing.T) *board.Board {
	t.Helper()
	b := newBoard(t)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAStoryMovesForwardThroughEveryColumnAndEachMoveIsLogged(t *testing.T) {
	b := newReleasingWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	cols := []string{"in_progress", "in_review", "merged", "released", "deployed"}
	walk(t, b, "S-1", cols...)
	wantStatus(t, b, "S-1", "deployed")

	log := readLog(t, b, "S-1")
	if len(log) != len(cols) {
		t.Fatalf("log %+v, want %d status entries", log, len(cols))
	}
	from := "backlog"
	for i, e := range log {
		if e.Kind != "status" || e.From != from || e.To != cols[i] || e.Actor != actor || e.At == "" || e.ID == "" {
			t.Fatalf("entry %d %+v", i, e)
		}
		from = cols[i]
	}
}

func TestWithoutAReleaseStageMergedGoesStraightToDeployed(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "released"})
	wantKind(t, err, board.Conflict)
	walk(t, b, "S-1", "deployed")
}

func TestWithAReleaseStageMergedMustBeReleasedBeforeDeployed(t *testing.T) {
	b := newReleasingWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "deployed"})
	wantKind(t, err, board.Conflict)
}

func TestAStoryCanStepBackOneColumnBeforeItIsMerged(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	walk(t, b, "S-1", "in_progress", "in_review", "in_progress", "backlog")
	wantStatus(t, b, "S-1", "backlog")
}

func TestStatusChangesThatSkipOrGoBackTooFarAreRefused(t *testing.T) {
	cases := []struct {
		name     string
		before   []string
		to       string
		wantKind board.Kind
	}{
		{"skip to review", nil, "in_review", board.Conflict},
		{"skip to merged", []string{"in_progress"}, "merged", board.Conflict},
		{"back from merged", []string{"in_progress", "in_review", "merged"}, "in_review", board.Conflict},
		{"out of deployed", []string{"in_progress", "in_review", "merged", "deployed"}, "backlog", board.Conflict},
		{"back two", []string{"in_progress", "in_review"}, "backlog", board.Conflict},
		{"same column", nil, "backlog", board.Conflict},
		{"unknown column", nil, "done", board.Invalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newWorkspace(t)
			addStory(t, b, board.NewStory{Title: "A"})
			walk(t, b, "S-1", c.before...)
			before := len(readLog(t, b, "S-1"))
			_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: c.to})
			wantKind(t, err, c.wantKind)
			if got := len(readLog(t, b, "S-1")); got != before {
				t.Fatalf("a refused change wrote %d log entries", got-before)
			}
		})
	}
}

func TestSetStatusOnAMissingStoryIsNotFound(t *testing.T) {
	b := newWorkspace(t)
	_, err := b.SetStatus("demo", actor, "S-9", board.StatusChange{To: "in_progress"})
	wantKind(t, err, board.NotFound)
}

func TestStartingReviewAndMergingRecordTheBranchPullRequestAndCommit(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	setStatus(t, b, "S-1", board.StatusChange{To: "in_progress", Branch: "feature/a"})
	setStatus(t, b, "S-1", board.StatusChange{To: "in_review", PRURL: "https://example.com/pr/1"})
	s := setStatus(t, b, "S-1", board.StatusChange{To: "merged", Commit: "abc1234"})
	if s.Branch != "feature/a" || s.PRURL != "https://example.com/pr/1" {
		t.Fatalf("story %+v", s)
	}
	log := readLog(t, b, "S-1")
	if log[0].Branch != "feature/a" || log[1].PRURL != "https://example.com/pr/1" || log[2].Commit != "abc1234" {
		t.Fatalf("log %+v", log)
	}
}

func TestDetailsThatDoNotBelongToTheChangeAreRefused(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	for _, c := range []board.StatusChange{
		{To: "in_progress", Commit: "abc"},
		{To: "in_progress", PRURL: "https://example.com/pr/1"},
		{To: "in_review", Branch: "feature/a"},
		{To: "in_review", Workaround: "mocks"},
		{To: "in_progress", Reason: "because"},
		{To: "in_progress", Branch: "bad\x00branch"},
	} {
		_, err := b.SetStatus("demo", actor, "S-1", c)
		wantKind(t, err, board.Invalid)
	}
	wantStatus(t, b, "S-1", "backlog")
}

func TestAStoryOnHoldNeedsAReasonAndResumesInTheColumnItLeft(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	walk(t, b, "S-1", "in_progress", "in_review")
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "on_hold"})
	wantKind(t, err, board.Invalid)

	s := setStatus(t, b, "S-1", board.StatusChange{To: "on_hold", Reason: "needs more info"})
	if s.HeldFrom != "in_review" || s.HoldReason != "needs more info" {
		t.Fatalf("held %+v", s)
	}
	_, err = b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "in_progress"})
	wantKind(t, err, board.Conflict)

	s = setStatus(t, b, "S-1", board.StatusChange{To: "in_review"})
	if s.HeldFrom != "" || s.HoldReason != "" {
		t.Fatalf("resumed %+v", s)
	}
	if e := readLog(t, b, "S-1")[2]; e.To != "on_hold" || e.Reason != "needs more info" {
		t.Fatalf("hold entry %+v", e)
	}
}

func TestClosingNeedsAReasonAndIsFinal(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "closed"})
	wantKind(t, err, board.Invalid)
	setStatus(t, b, "S-1", board.StatusChange{To: "closed", Reason: "no longer needed"})
	_, err = b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "backlog"})
	wantKind(t, err, board.Conflict)
}

func TestAStoryOnHoldCanBeClosed(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	setStatus(t, b, "S-1", board.StatusChange{To: "on_hold", Reason: "unclear"})
	setStatus(t, b, "S-1", board.StatusChange{To: "closed", Reason: "dropped"})
}

func TestAMergedStoryCannotBeClosed(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "closed", Reason: "x"})
	wantKind(t, err, board.Conflict)
}

func TestClosingIsRefusedWhileOtherStoriesDependOnIt(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	addStory(t, b, board.NewStory{Title: "C"})
	depend(t, b, "S-2", "S-1")
	depend(t, b, "S-3", "S-1")
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "closed", Reason: "dropped"})
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), "S-2 and S-3") {
		t.Fatalf("the refusal does not name the dependents: %v", err)
	}
	wantStatus(t, b, "S-1", "backlog")
}

func TestAStoryIsBlockedUntilEveryDependencyIsComplete(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	depend(t, b, "S-2", "S-1")
	if s := wantStatus(t, b, "S-2", "backlog"); !s.Blocked {
		t.Fatal("S-2 is not blocked by the incomplete S-1")
	}
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	if s := wantStatus(t, b, "S-2", "backlog"); s.Blocked {
		t.Fatal("S-2 is still blocked after S-1 merged")
	}
}

func TestStartingABlockedStoryIsRefusedWithoutAWorkaround(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	depend(t, b, "S-2", "S-1")
	_, err := b.SetStatus("demo", actor, "S-2", board.StatusChange{To: "in_progress"})
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), "blocked by S-1") {
		t.Fatalf("refusal %v", err)
	}
}

func TestABlockedStoryCanStartWithAWorkaroundWhichIsLoggedAsADecision(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	depend(t, b, "S-2", "S-1")
	s := setStatus(t, b, "S-2", board.StatusChange{To: "in_progress", Workaround: "build against mock data"})
	if s.Workaround != "build against mock data" || !s.Blocked {
		t.Fatalf("started %+v", s)
	}
	log := readLog(t, b, "S-2")
	if len(log) != 2 || log[0].Kind != "status" || log[1].Kind != "decision" ||
		log[1].Reason != "build against mock data" || !strings.Contains(log[1].Text, "S-1") {
		t.Fatalf("log %+v", log)
	}
}

func TestABlockedStoryCannotBeMergedEvenWithAWorkaround(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	depend(t, b, "S-2", "S-1")
	setStatus(t, b, "S-2", board.StatusChange{To: "in_progress", Workaround: "mocks"})
	walk(t, b, "S-2", "in_review")
	_, err := b.SetStatus("demo", actor, "S-2", board.StatusChange{To: "merged"})
	wantKind(t, err, board.Conflict)
}

func TestAWorkaroundIsRefusedWhenTheStoryIsNotBlocked(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "in_progress", Workaround: "mocks"})
	wantKind(t, err, board.Conflict)
}

func TestReturningToTheBacklogDropsTheWorkaround(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	depend(t, b, "S-2", "S-1")
	setStatus(t, b, "S-2", board.StatusChange{To: "in_progress", Workaround: "mocks"})
	if s := setStatus(t, b, "S-2", board.StatusChange{To: "backlog"}); s.Workaround != "" {
		t.Fatalf("back in the backlog %+v", s)
	}
}

func TestAStartedStoryIsPlanningAndLeavingInProgressClearsItsActivity(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	if s := setStatus(t, b, "S-1", board.StatusChange{To: "in_progress"}); s.Activity != "planning" {
		t.Fatalf("started %+v", s)
	}
	if s := setStatus(t, b, "S-1", board.StatusChange{To: "in_review"}); s.Activity != "" {
		t.Fatalf("in review %+v", s)
	}
}

func TestAnInProgressStorySaysWhatItIsDoing(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	walk(t, b, "S-1", "in_progress")
	s, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Activity: ptr("waiting"), WaitingOn: ptr("the review agent")})
	if err != nil {
		t.Fatal(err)
	}
	if s.Activity != "waiting" || s.WaitingOn != "the review agent" {
		t.Fatalf("waiting %+v", s)
	}
	s, err = b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Activity: ptr("testing")})
	if err != nil {
		t.Fatal(err)
	}
	if s.Activity != "testing" || s.WaitingOn != "" {
		t.Fatalf("testing %+v", s)
	}
}

func TestActivityIsRefusedOutsideInProgressAndMustBeKnown(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	_, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Activity: ptr("building")})
	wantKind(t, err, board.Conflict)
	walk(t, b, "S-1", "in_progress")
	_, err = b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Activity: ptr("napping")})
	wantKind(t, err, board.Invalid)
	_, err = b.UpdateStory("demo", actor, "S-1", board.StoryPatch{WaitingOn: ptr("someone")})
	wantKind(t, err, board.Invalid)
}

func TestTheBranchAndPullRequestCanBeCorrected(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	s, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Branch: ptr("feature/a"), PRURL: ptr("https://example.com/pr/2")})
	if err != nil {
		t.Fatal(err)
	}
	if s.Branch != "feature/a" || s.PRURL != "https://example.com/pr/2" {
		t.Fatalf("edited %+v", s)
	}
}
