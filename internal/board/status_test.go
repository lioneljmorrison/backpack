package board_test

import (
	"slices"
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
	steps := []board.StatusChange{
		{To: "in_progress"}, {To: "in_review"}, {To: "merged", Commit: "c1"},
		{To: "merged", Into: "main", Commit: "c2"},
	}
	for _, c := range steps {
		setStatus(t, b, "S-1", c)
	}
	s := wantStatus(t, b, "S-1", "merged")
	if s.Into != "main" {
		t.Fatalf("a merged story is in the branch it reached: into %q", s.Into)
	}

	log := readLog(t, b, "S-1")
	if len(log) != len(steps) {
		t.Fatalf("log %+v, want %d status entries", log, len(steps))
	}
	from := "backlog"
	for i, e := range log {
		if e.Kind != "status" || e.From != from || e.To != steps[i].To || e.Actor != actor || e.At == "" || e.ID == "" {
			t.Fatalf("entry %d %+v", i, e)
		}
		from = steps[i].To
	}
	// Each merge's entry names the branch and the commit.
	if log[2].Into != "dev" || log[2].Commit != "c1" || log[3].Into != "main" || log[3].Commit != "c2" {
		t.Fatalf("merge entries %+v %+v", log[2], log[3])
	}
}

// merge is a table of merges and refusals, in a workspace of two branches
// (dev into main) and in one of a single branch (main).
func TestMergingRecordsTheBranchReachedAndGoesOneStepAtATime(t *testing.T) {
	type step struct {
		c      board.StatusChange
		into   string // the story's into afterwards; "" when the step is refused
		refuse string // part of the refusal, when the step is refused
	}
	two := []string{"dev", "main"}
	for _, c := range []struct {
		name     string
		branches []string
		steps    []step
	}{
		{"no branch named reaches the first branch", two, []step{{c: board.StatusChange{To: "merged"}, into: "dev"}}},
		{"dev named from review", two, []step{{c: board.StatusChange{To: "merged", Into: "dev"}, into: "dev"}}},
		{"main from dev", two, []step{
			{c: board.StatusChange{To: "merged"}, into: "dev"},
			{c: board.StatusChange{To: "merged", Into: "main", Commit: "m1"}, into: "main"},
		}},
		{"main straight from review is refused", two, []step{
			{c: board.StatusChange{To: "merged", Into: "main"}, refuse: "dev"},
		}},
		{"an unknown branch is refused", two, []step{
			{c: board.StatusChange{To: "merged", Into: "nope"}, refuse: "no branch"},
		}},
		{"a merge from dev must name the next branch", two, []step{
			{c: board.StatusChange{To: "merged"}, into: "dev"},
			{c: board.StatusChange{To: "merged"}, refuse: "main"},
			{c: board.StatusChange{To: "merged", Into: "dev"}, refuse: "main"},
		}},
		{"nothing merges past the release branch", two, []step{
			{c: board.StatusChange{To: "merged"}, into: "dev"},
			{c: board.StatusChange{To: "merged", Into: "main"}, into: "main"},
			{c: board.StatusChange{To: "merged", Into: "main"}, refuse: "merges nowhere"},
		}},
		{"one branch: merged reaches it", []string{"main"}, []step{{c: board.StatusChange{To: "merged"}, into: "main"}}},
		{"one branch: main named", []string{"main"}, []step{{c: board.StatusChange{To: "merged", Into: "main"}, into: "main"}}},
		{"one branch: nothing merges past it", []string{"main"}, []step{
			{c: board.StatusChange{To: "merged"}, into: "main"},
			{c: board.StatusChange{To: "merged", Into: "main"}, refuse: "merges nowhere"},
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var b *board.Board
			if len(c.branches) == 2 {
				b = newReleasingWorkspace(t)
			} else {
				b = newWorkspace(t)
			}
			addStory(t, b, board.NewStory{Title: "A"})
			walk(t, b, "S-1", "in_progress", "in_review")
			for i, st := range c.steps {
				before := len(readLog(t, b, "S-1"))
				got, err := b.SetStatus("demo", actor, "S-1", st.c)
				if st.refuse != "" {
					wantKind(t, err, board.Conflict)
					if !strings.Contains(err.Error(), st.refuse) {
						t.Fatalf("step %d: refusal %q lacks %q", i, err, st.refuse)
					}
					if n := len(readLog(t, b, "S-1")); n != before {
						t.Fatalf("step %d: a refused change wrote %d log entries", i, n-before)
					}
					continue
				}
				if err != nil {
					t.Fatalf("step %d: %v", i, err)
				}
				if got.Into != st.into {
					t.Fatalf("step %d: into %q, want %q", i, got.Into, st.into)
				}
				if st.c.To == "merged" {
					e := readLog(t, b, "S-1")
					if last := e[len(e)-1]; last.Into != st.into || last.Commit != st.c.Commit {
						t.Fatalf("step %d: log entry %+v", i, last)
					}
				}
			}
		})
	}
}

func TestABranchIsNamedOnlyWhenMerging(t *testing.T) {
	b := newReleasingWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	_, err := b.SetStatus("demo", actor, "S-1", board.StatusChange{To: "in_progress", Into: "dev"})
	wantKind(t, err, board.Invalid)
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

func TestAStoryWithNoDependenciesIsNotBlocked(t *testing.T) {
	b := newWorkspace(t)
	if s := addStory(t, b, board.NewStory{Title: "A"}); s.Blocked {
		t.Fatalf("added %+v", s)
	}
}

func TestAWorkaroundIsGivenOnlyWhenStartingFromTheBacklog(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	depend(t, b, "S-2", "S-1")
	setStatus(t, b, "S-2", board.StatusChange{To: "in_progress", Workaround: "mocks"})
	walk(t, b, "S-2", "in_review")
	_, err := b.SetStatus("demo", actor, "S-2", board.StatusChange{To: "in_progress", Workaround: "other mocks"})
	wantKind(t, err, board.Conflict)
	if s := wantStatus(t, b, "S-2", "in_review"); s.Workaround != "mocks" {
		t.Fatalf("workaround replaced %+v", s)
	}
}

func TestAClosedDependentDoesNotStopAStoryBeingClosed(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	depend(t, b, "S-2", "S-1")
	setStatus(t, b, "S-2", board.StatusChange{To: "closed", Reason: "dropped"})
	setStatus(t, b, "S-1", board.StatusChange{To: "closed", Reason: "dropped too"})
}

func TestMovesListsWhereAStoryCanGoNext(t *testing.T) {
	one := []board.Branch{{Name: "main", Release: true}}
	two := []board.Branch{{Name: "dev", Into: "main"}, {Name: "main", Release: true}}
	for _, c := range []struct {
		s    board.Story
		bs   []board.Branch
		want []string
	}{
		{board.Story{Status: board.Proposed}, one, []string{"on_hold", "backlog", "closed"}},
		{board.Story{Status: board.OnHold, HeldFrom: board.Proposed}, one, []string{"proposed", "closed"}},
		{board.Story{Status: board.Backlog}, one, []string{"on_hold", "in_progress", "closed"}},
		{board.Story{Status: board.InReview}, one, []string{"on_hold", "in_progress", "merged:main", "closed"}},
		{board.Story{Status: board.InReview}, two, []string{"on_hold", "in_progress", "merged:dev", "closed"}},
		{board.Story{Status: board.Merged, Into: "main"}, one, nil},
		{board.Story{Status: board.Merged, Into: "dev"}, two, []string{"merged:main"}},
		{board.Story{Status: board.Merged, Into: "main"}, two, nil},
		{board.Story{Status: board.OnHold, HeldFrom: board.InProgress}, one, []string{"in_progress", "closed"}},
		{board.Story{Status: board.Released, Into: "main"}, one, nil},
	} {
		var got []string
		for _, m := range board.Moves(c.s, c.bs) {
			got = append(got, m.String())
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s into %q (%d branches): %v, want %v", c.s.Status, c.s.Into, len(c.bs), got, c.want)
		}
	}
}
