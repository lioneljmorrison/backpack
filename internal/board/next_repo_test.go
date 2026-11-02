package board_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// readyIn adds a story to a repository; a person's story goes straight to the backlog.
func readyIn(t *testing.T, b *board.Board, repo, title string) string {
	t.Helper()
	s := addStory(t, b, board.NewStory{Title: title, Repo: repo})
	return s.ID
}

func TestWhatsNextForARepositoryIsItsTopReadyStoryEvenWhenAnotherRanksHigher(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	a1 := readyIn(t, b, "api", "Api first")
	w1 := readyIn(t, b, "web", "Web second")
	readyIn(t, b, "web", "Web third")

	whole, err := b.Next("demo", "")
	if err != nil || whole == nil || whole.ID != a1 {
		t.Fatalf("whole workspace %+v, %v", whole, err)
	}
	web, err := b.Next("demo", "web")
	if err != nil || web == nil || web.ID != w1 || web.Repo != "web" {
		t.Fatalf("web %+v, %v", web, err)
	}
}

func TestAStoryBlockedByAnotherRepositorysUnfinishedWorkIsBlocked(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	a1 := readyIn(t, b, "api", "Api")
	w1 := readyIn(t, b, "web", "Web needs api")
	w2 := readyIn(t, b, "web", "Web free")
	depend(t, b, w1, a1)

	// w1 ranks above w2 but waits on api's story.
	if next, err := b.Next("demo", "web"); err != nil || next == nil || next.ID != w2 {
		t.Fatalf("while blocked %+v, %v", next, err)
	}
	walk(t, b, a1, "in_progress", "in_review", "merged")
	if next, err := b.Next("demo", "web"); err != nil || next == nil || next.ID != w1 {
		t.Fatalf("once api is complete %+v, %v", next, err)
	}
}

func TestWhatsNextForARepositoryWithNothingReadyIsNull(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	readyIn(t, b, "api", "Api")
	if next, err := b.Next("demo", "web"); err != nil || next != nil {
		t.Fatalf("next %+v, %v", next, err)
	}
}

func TestWhatsNextForAnUnknownRepositoryIsNotFound(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	_, err := b.Next("demo", "ios")
	wantMessage(t, err, board.NotFound, `demo has no repo "ios"`)
}

func TestWhatsNextInAOneRepositoryWorkspaceIsUnchangedAndAcceptsItsName(t *testing.T) {
	b, _ := twoRepos(t)
	s1 := readyIn(t, b, "", "Only")
	for _, repo := range []string{"", "api"} {
		if next, err := b.Next("demo", repo); err != nil || next == nil || next.ID != s1 {
			t.Fatalf("repo %q: %+v, %v", repo, next, err)
		}
	}
}
