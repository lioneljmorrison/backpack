package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestEachRepositorysStoryMergesFromReviewIntoItsOwnStartingBranch(t *testing.T) {
	b, g := twoRepos(t)
	// Stories are added before web exists: adding to a chosen repository is S-70.
	// Counting the workspace's branches together would see two starting branches.
	a := addStory(t, b, board.NewStory{Title: "Api story"})
	walk(t, b, a.ID, board.InProgress, board.InReview)
	addWeb(t, b)
	webStory(t, g, "W-1", "in_review", "")

	if got := setStatus(t, b, a.ID, board.StatusChange{To: board.Merged, Commit: "c1"}); got.Into != "dev" {
		t.Fatalf("api story into %q, want dev", got.Into)
	}
	if got := setStatus(t, b, "W-1", board.StatusChange{To: board.Merged, Commit: "c2"}); got.Into != "dev" {
		t.Fatalf("web story into %q, want dev", got.Into)
	}
}

func TestAnIntoFromTheOtherRepositoryIsRefused(t *testing.T) {
	b, g := twoRepos(t)
	a := addStory(t, b, board.NewStory{Title: "Api story"})
	walk(t, b, a.ID, board.InProgress, board.InReview)
	addWeb(t, b)
	run(t, g, `MATCH (r:Repo {name: 'web'}) CREATE (r)-[:HAS_BRANCH]->(:Branch {name: 'staging', into: 'main', created_by: 'test', created_at: 'x'})`)
	_, err := b.SetStatus("demo", actor, a.ID, board.StatusChange{To: board.Merged, Into: "staging", Commit: "c1"})
	wantMessage(t, err, board.Conflict, `api has no branch "staging"`)
	wantStatus(t, b, a.ID, board.InReview)

	// Web has two starting branches, so its story must name one; api's does not.
	webStory(t, g, "W-1", "in_review", "")
	_, err = b.SetStatus("demo", actor, "W-1", board.StatusChange{To: board.Merged, Commit: "c2"})
	wantKind(t, err, board.Conflict)
	if got := setStatus(t, b, a.ID, board.StatusChange{To: board.Merged, Commit: "c3"}); got.Into != "dev" {
		t.Fatalf("api story into %q, want dev", got.Into)
	}
}

func TestTheOneStepRuleFollowsTheStorysOwnChain(t *testing.T) {
	b, g := twoRepos(t)
	a := addStory(t, b, board.NewStory{Title: "Api story"})
	walk(t, b, a.ID, board.InProgress, board.InReview)
	setStatus(t, b, a.ID, board.StatusChange{To: board.Merged, Commit: "c1"})
	addWeb(t, b)
	// web's chain is dev, staging, main; api's stays dev, main.
	run(t, g, `MATCH (r:Repo {name: 'web'})-[:HAS_BRANCH]->(d:Branch {name: 'dev'})
		CREATE (r)-[:HAS_BRANCH]->(:Branch {name: 'staging', into: 'main', created_by: 'test', created_at: 'x'})
		SET d.into = 'staging'`)
	webStory(t, g, "W-1", "merged", "dev")

	// api's next step is main, web's is staging.
	_, err := b.SetStatus("demo", actor, "W-1", board.StatusChange{To: board.Merged, Into: "main", Commit: "c2"})
	wantMessage(t, err, board.Conflict, "W-1 is merged into dev; its next merge is into staging")
	if got := setStatus(t, b, a.ID, board.StatusChange{To: board.Merged, Into: "main", Commit: "c3"}); got.Into != "main" {
		t.Fatalf("api story into %q, want main", got.Into)
	}
	if got := setStatus(t, b, "W-1", board.StatusChange{To: board.Merged, Into: "staging", Commit: "c4"}); got.Into != "staging" {
		t.Fatalf("web story into %q, want staging", got.Into)
	}
}

func TestMovesListOnlyTheStorysRepositorysBranches(t *testing.T) {
	bs := []board.Branch{
		{Name: "dev", Repo: "api", Into: "main"}, {Name: "main", Repo: "api", Release: true},
		{Name: "staging", Repo: "web", Into: "main"}, {Name: "main", Repo: "web", Release: true},
	}
	var got []string
	for _, m := range board.Moves(board.Story{Repo: "api", Status: board.InReview}, bs) {
		got = append(got, m.String())
	}
	if want := []string{"on_hold", "in_progress", "merged:dev", "closed"}; !slices.Equal(got, want) {
		t.Fatalf("api moves %v, want %v", got, want)
	}
	got = nil
	for _, m := range board.Moves(board.Story{Repo: "web", Status: board.Merged, Into: "staging"}, bs) {
		got = append(got, m.String())
	}
	if want := []string{"merged:main"}; !slices.Equal(got, want) {
		t.Fatalf("web moves %v, want %v", got, want)
	}
}
