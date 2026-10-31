package board_test

import (
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// chain adds S-1 <- S-2 <- S-3 (S-3 depends on S-2, which depends on S-1)
// and S-4, which depends on S-1 as well. S-5 depends on nothing.
func chain(t *testing.T) *board.Board {
	t.Helper()
	b := newWorkspace(t)
	for _, title := range []string{"A", "B", "C", "D", "E"} {
		addStory(t, b, board.NewStory{Title: title, Goal: "g", Plan: "p"})
	}
	depend(t, b, "S-2", "S-1")
	depend(t, b, "S-3", "S-2")
	depend(t, b, "S-4", "S-1")
	return b
}

func read(t *testing.T, b *board.Board, id string) board.Story {
	t.Helper()
	s, err := b.Story("demo", id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func wantMarked(t *testing.T, b *board.Board, id, upstream string) {
	t.Helper()
	s := read(t, b, id)
	if s.UpstreamChanged == nil || s.UpstreamChanged.Story != upstream || s.UpstreamChanged.At == "" {
		t.Fatalf("%s upstream_changed = %+v, want after %s", id, s.UpstreamChanged, upstream)
	}
}

func wantUnmarked(t *testing.T, b *board.Board, id string) {
	t.Helper()
	if s := read(t, b, id); s.UpstreamChanged != nil {
		t.Fatalf("%s upstream_changed = %+v, want none", id, s.UpstreamChanged)
	}
}

func TestChangingAPlanMarksDirectAndIndirectDependents(t *testing.T) {
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("new plan")}); err != nil {
		t.Fatal(err)
	}
	wantMarked(t, b, "S-2", "S-1")
	wantMarked(t, b, "S-3", "S-1")
	wantMarked(t, b, "S-4", "S-1")
	wantUnmarked(t, b, "S-1")
	wantUnmarked(t, b, "S-5")
	log := readLog(t, b, "S-3")
	last := log[len(log)-1]
	if last.Kind != "note" || !strings.Contains(last.Text, "S-1") || !strings.Contains(last.Text, "plan") {
		t.Fatalf("S-3 log %+v", log)
	}
}

func TestChangingAGoalMarksDependentsToo(t *testing.T) {
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Goal: ptr("new goal")}); err != nil {
		t.Fatal(err)
	}
	wantMarked(t, b, "S-2", "S-1")
	log := readLog(t, b, "S-2")
	if last := log[len(log)-1]; !strings.Contains(last.Text, "goal") {
		t.Fatalf("log %+v", log)
	}
}

func TestCompleteAndClosedDependentsAreNotMarked(t *testing.T) {
	b := newWorkspace(t)
	for _, title := range []string{"A", "B", "C", "D", "E"} {
		addStory(t, b, board.NewStory{Title: title, Plan: "p"})
	}
	walk(t, b, "S-1", "in_progress", "in_review", "merged")
	for _, id := range []string{"S-2", "S-3", "S-4"} {
		depend(t, b, id, "S-1")
	}
	depend(t, b, "S-5", "S-2") // behind a story that will be merged
	walk(t, b, "S-2", "in_progress", "in_review", "merged")
	setStatus(t, b, "S-3", board.StatusChange{To: "closed", Reason: "not needed"})
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("changed after merge")}); err != nil {
		t.Fatal(err)
	}
	wantUnmarked(t, b, "S-2")
	wantUnmarked(t, b, "S-3")
	wantMarked(t, b, "S-4", "S-1")
	// S-5 is only reached through the merged S-2, whose work is done.
	wantUnmarked(t, b, "S-5")
	if log := readLog(t, b, "S-2"); log[len(log)-1].Kind != "status" {
		t.Fatalf("merged story was logged to: %+v", log)
	}
}

func TestChangingOtherFieldsMarksNothing(t *testing.T) {
	b := chain(t)
	walk(t, b, "S-1", "in_progress")
	_, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Title: ptr("T"), Description: ptr("d"),
		Activity: ptr("building"), Branch: ptr("feature/x"), PRURL: ptr("https://example.invalid/pr/1")})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"S-2", "S-3", "S-4", "S-5"} {
		wantUnmarked(t, b, id)
	}
}

func TestSettingTheSamePlanOrGoalMarksNothing(t *testing.T) {
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("p"), Goal: ptr("g")}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"S-2", "S-3", "S-4"} {
		wantUnmarked(t, b, id)
	}
}

func TestUpdatingAMarkedStorysPlanClearsItsMarkAndMarksItsDependents(t *testing.T) {
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("v2")}); err != nil {
		t.Fatal(err)
	}
	wantMarked(t, b, "S-3", "S-1")
	if _, err := b.UpdateStory("demo", actor, "S-2", board.StoryPatch{Plan: ptr("adapted")}); err != nil {
		t.Fatal(err)
	}
	wantUnmarked(t, b, "S-2")
	wantMarked(t, b, "S-3", "S-2") // the most recent upstream change wins
	wantMarked(t, b, "S-4", "S-1") // untouched
}

func TestUpdatingOnlyOtherFieldsKeepsTheMark(t *testing.T) {
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("v2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.UpdateStory("demo", actor, "S-2", board.StoryPatch{Title: ptr("renamed")}); err != nil {
		t.Fatal(err)
	}
	wantMarked(t, b, "S-2", "S-1")
}

func TestARefusedUpdateMarksNothing(t *testing.T) {
	b := chain(t)
	_, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("v2"), Activity: ptr("building")})
	wantKind(t, err, board.Conflict) // S-1 is not in progress
	wantUnmarked(t, b, "S-2")
	if got := read(t, b, "S-1").Plan; got != "p" {
		t.Fatalf("plan %q", got)
	}
}

func TestMarkReviewedClearsTheMarkAndLogsTheReason(t *testing.T) {
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("v2")}); err != nil {
		t.Fatal(err)
	}
	s, err := b.MarkReviewed("demo", actor, "S-2", "the change does not touch this story")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "S-2" || s.UpstreamChanged != nil {
		t.Fatalf("reviewed %+v", s)
	}
	wantUnmarked(t, b, "S-2")
	wantMarked(t, b, "S-3", "S-1") // reviewing does not touch others
	log := readLog(t, b, "S-2")
	last := log[len(log)-1]
	if last.Kind != "note" || last.Actor != actor || !strings.Contains(last.Text, "the change does not touch this story") {
		t.Fatalf("log %+v", log)
	}
}

func TestMarkReviewedIsRefusedWithItsReason(t *testing.T) {
	b := chain(t)
	_, err := b.MarkReviewed("demo", actor, "S-2", "fine")
	wantKind(t, err, board.Conflict) // not marked
	if !strings.Contains(err.Error(), "S-2") {
		t.Fatalf("reason %q", err)
	}
	_, err = b.MarkReviewed("demo", actor, "S-9", "fine")
	wantKind(t, err, board.NotFound)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("v2")}); err != nil {
		t.Fatal(err)
	}
	_, err = b.MarkReviewed("demo", actor, "S-2", "")
	wantKind(t, err, board.Invalid)
	wantMarked(t, b, "S-2", "S-1")
	before := len(readLog(t, b, "S-2"))
	if _, err = b.MarkReviewed("demo", actor, "S-2", "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err = b.MarkReviewed("demo", actor, "S-2", "ok"); err == nil {
		t.Fatal("a second review should be refused")
	}
	if got := len(readLog(t, b, "S-2")); got != before+1 {
		t.Fatalf("log grew from %d to %d", before, got)
	}
}

func TestWhereAreWeListsMarkedStoriesThatAreNotComplete(t *testing.T) {
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("v2")}); err != nil {
		t.Fatal(err)
	}
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(w.UpstreamChanged); strings.Join(got, ",") != "S-2,S-3,S-4" {
		t.Fatalf("upstream changed %v", got)
	}
	text := w.Text()
	if !strings.Contains(text, "Upstream changed:\n  S-2 B — after S-1;") || strings.Contains(text, "S-5 E — after") {
		t.Fatalf("text:\n%s", text)
	}
	// A marked story that closes drops off the list.
	setStatus(t, b, "S-4", board.StatusChange{To: "closed", Reason: "no longer needed"})
	w, _ = b.Where("demo")
	if got := ids(w.UpstreamChanged); strings.Join(got, ",") != "S-2,S-3" {
		t.Fatalf("upstream changed %v", got)
	}
}
