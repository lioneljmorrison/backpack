package board_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// markedInReview is chain() with S-2 (blocked by S-1) marked upstream changed
// and then moved to in review. S-1 is merged first so S-2 can be.
func markedInReview(t *testing.T) *board.Board {
	t.Helper()
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("new plan")}); err != nil {
		t.Fatal(err)
	}
	walk(t, b, "S-1", board.InProgress, board.InReview)
	setStatus(t, b, "S-1", board.StatusChange{To: board.Merged, Commit: "c1"})
	walk(t, b, "S-2", board.InProgress, board.InReview)
	wantMarked(t, b, "S-2", "S-1")
	return b
}

func TestMergingAMarkedStoryDropsItsUpstreamMark(t *testing.T) {
	b := markedInReview(t)
	setStatus(t, b, "S-2", board.StatusChange{To: board.Merged, Commit: "c2"})
	wantUnmarked(t, b, "S-2")
	wantMarked(t, b, "S-4", "S-1") // others keep theirs
}

func TestClosingAMarkedStoryDropsItsUpstreamMark(t *testing.T) {
	b := chain(t)
	if _, err := b.UpdateStory("demo", actor, "S-1", board.StoryPatch{Plan: ptr("new plan")}); err != nil {
		t.Fatal(err)
	}
	setStatus(t, b, "S-4", board.StatusChange{To: "closed", Reason: "not needed"})
	wantUnmarked(t, b, "S-4")
	wantMarked(t, b, "S-2", "S-1")
}

func TestMovingAMarkedStoryToReviewOrOnHoldKeepsItsMark(t *testing.T) {
	b := markedInReview(t) // S-2 is in review and marked
	wantMarked(t, b, "S-2", "S-1")
	setStatus(t, b, "S-4", board.StatusChange{To: board.InProgress})
	setStatus(t, b, "S-4", board.StatusChange{To: board.OnHold, Reason: "waiting"})
	wantMarked(t, b, "S-4", "S-1")
}

// markedBehindTheAPI builds a releasing workspace with S-1 merged into dev
// and S-2 merged into main, each given a mark directly in the graph: the API
// never marks a complete story, so this is the only way to reach the case.
func markedBehindTheAPI(t *testing.T) *board.Board {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	mergedIntoDev(t, b, "In dev", "d1")
	mergedIntoMain(t, b, "In main")
	g := db.SelectGraph(prefix + "ws:demo")
	if _, err := g.Query("MATCH (s:Story) SET s.upstream_id = 'S-9', s.upstream_at = '2026-01-01T00:00:00Z'", nil, nil); err != nil {
		t.Fatal(err)
	}
	wantMarked(t, b, "S-1", "S-9")
	wantMarked(t, b, "S-2", "S-9")
	return b
}

func TestMergingABranchDropsTheMarkOfEveryStoryItMoves(t *testing.T) {
	b := markedBehindTheAPI(t)
	if _, err := b.MergeBranch("demo", actor, "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	wantUnmarked(t, b, "S-1")
	wantMarked(t, b, "S-2", "S-9") // not moved by this merge
}

func TestCuttingAReleaseDropsTheMarkOfEveryStoryItReleases(t *testing.T) {
	b := markedBehindTheAPI(t)
	if _, err := b.MergeBranch("demo", actor, "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CutRelease("demo", actor, v1); err != nil {
		t.Fatal(err)
	}
	wantUnmarked(t, b, "S-1")
	wantUnmarked(t, b, "S-2")
}
