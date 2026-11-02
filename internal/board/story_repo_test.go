package board_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

const severalRepos = "demo has several repos; name the repo: api or web"

func TestAStoryIsAddedIntoTheRepositoryItNames(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	w := addStory(t, b, board.NewStory{Title: "Web story", Repo: "web"})
	a := addStory(t, b, board.NewStory{Title: "Api story", Repo: "api"})
	if w.Repo != "web" || a.Repo != "api" {
		t.Fatalf("repos %q, %q", w.Repo, a.Repo)
	}
	// Each is held by exactly one repository.
	if n := count(t, g, "MATCH (:Repo)-[:HOLDS]->(s:Story) RETURN count(s)"); n != 2 {
		t.Fatalf("held stories %d, want 2", n)
	}
	if got, err := b.Story("demo", w.ID); err != nil || got.Repo != "web" {
		t.Fatalf("read back %+v, %v", got, err)
	}
}

func TestAStoryWithNoRepositoryNamedIsRefusedWhenThereAreTwo(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	_, err := b.AddStory("demo", actor, board.NewStory{Title: "Which?"})
	wantMessage(t, err, board.Invalid, severalRepos)
	// The refusal comes before the others, and nothing is written.
	_, err = b.AddStory("demo", actor, board.NewStory{Title: "Which?", Phase: 9})
	wantMessage(t, err, board.Invalid, severalRepos)
	if ss, err := b.Stories("demo"); err != nil || len(ss) != 0 {
		t.Fatalf("stories %+v, %v", ss, err)
	}
	if n := count(t, g, "MATCH (w:Workspace) RETURN w.next_story"); n != 1 {
		t.Fatalf("next_story %d, want 1", n)
	}
}

func TestAStoryInAnUnknownRepositoryIsNotFound(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	_, err := b.AddStory("demo", actor, board.NewStory{Title: "Nowhere", Repo: "ios"})
	wantMessage(t, err, board.NotFound, `demo has no repo "ios"`)
	// With one repository too.
	b2, _ := twoRepos(t)
	_, err = b2.AddStory("demo", actor, board.NewStory{Title: "Nowhere", Repo: "ios"})
	wantMessage(t, err, board.NotFound, `demo has no repo "ios"`)
}

func TestWithOneRepositoryTheNameCanBeLeftOutOrGiven(t *testing.T) {
	b, g := twoRepos(t)
	s := addStory(t, b, board.NewStory{Title: "Plain"})
	if s.Repo != "api" {
		t.Fatalf("repo %q, want api", s.Repo)
	}
	if s2 := addStory(t, b, board.NewStory{Title: "Named", Repo: "api"}); s2.Repo != "api" {
		t.Fatalf("repo %q", s2.Repo)
	}
	if n := count(t, g, "MATCH (:Repo)-[:HOLDS]->(s:Story) RETURN count(s)"); n != 2 {
		t.Fatalf("held %d, want 2", n)
	}
}

func TestADependencyAcrossRepositoriesWorksAndTheRankRuleHolds(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	w := addStory(t, b, board.NewStory{Title: "Web", Repo: "web"})
	a := addStory(t, b, board.NewStory{Title: "Api", Repo: "api"})
	// w ranks above a, and w now depends on a: refused until a is moved up.
	if _, err := b.AddDependency("demo", actor, w.ID, a.ID); err == nil {
		t.Fatal("a dependency on a story ranked below it was accepted")
	}
	if _, err := b.MoveStory("demo", actor, a.ID, board.Position{Before: w.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := b.AddDependency("demo", actor, w.ID, a.ID)
	if err != nil || len(got.BlockedBy) != 1 || got.BlockedBy[0] != a.ID {
		t.Fatalf("dependency %+v, %v", got, err)
	}
	// And the rule still holds: a cannot move below what depends on it.
	if _, err := b.MoveStory("demo", actor, a.ID, board.Position{After: w.ID}); err == nil {
		t.Fatal("a story moved below its dependent")
	}
}

func TestAStorysRepositoryDoesNotChangeWhenItIsUpdatedOrMoved(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	w := addStory(t, b, board.NewStory{Title: "Web", Repo: "web"})
	title := "Renamed"
	if _, err := b.UpdateStory("demo", actor, w.ID, board.StoryPatch{Title: &title}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.MoveStory("demo", actor, w.ID, board.Position{}); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Story("demo", w.ID); err != nil || got.Repo != "web" {
		t.Fatalf("after update %+v, %v", got, err)
	}
}

func TestABugFixStraightInNamesItsRepositoryLikeAnyOtherAdd(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	found := addStory(t, b, board.NewStory{Title: "Found here", Repo: "web"})
	fix := func(repo string) board.NewStory {
		return board.NewStory{Title: "Crash", Kind: "bug", FoundIn: found.ID, Repo: repo,
			Position: board.Position{After: found.ID}, Reasoning: "next to it",
			StraightIn: &board.StraightIn{Fix: "Check the length", NoDesignChoice: true, NoContractChange: true, NothingElse: true}}
	}
	_, err := b.AddStory("demo", actor, fix(""))
	wantMessage(t, err, board.Invalid, severalRepos)
	got, err := b.AddStory("demo", actor, fix("web"))
	if err != nil || got.Repo != "web" || got.Status != board.Backlog {
		t.Fatalf("bug fix %+v, %v", got, err)
	}
}
