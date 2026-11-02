package board_test

import (
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// severalBoard is two repositories, api and web, with invented stories:
// S-1 and S-2 merged into main (api, web), S-3 building in web, S-4 and S-5
// ready in api and web.
func severalBoard(t *testing.T) *board.Board {
	t.Helper()
	b, _ := twoRepos(t)
	addWeb(t, b)
	for _, repo := range []string{"api", "web"} {
		id := readyIn(t, b, repo, strings.ToUpper(repo[:1])+repo[1:]+" done")
		walk(t, b, id, board.InProgress, board.InReview)
		setStatus(t, b, id, board.StatusChange{To: board.Merged, Commit: "d-" + id})
		setStatus(t, b, id, board.StatusChange{To: board.Merged, Into: "main", Commit: "m-" + id})
	}
	building := readyIn(t, b, "web", "Web building")
	walk(t, b, building, board.InProgress)
	readyIn(t, b, "api", "Api next")
	readyIn(t, b, "web", "Web next")
	return b
}

func TestWhereAreWeWithSeveralRepositoriesTagsEachStoryAndWaitsPerRepository(t *testing.T) {
	b := severalBoard(t)
	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	const want = `In progress:
  S-3 [web] Web building — planning
Merged, waiting for a release in api: 1 (latest: S-1 Api done)
Merged, waiting for a release in web: 1 (latest: S-2 Web done)
Next:
  S-4 [api] Api next
  S-5 [web] Web next
`
	if got := w.Text(); got != want {
		t.Fatalf("text:\n%s\nwant:\n%s", got, want)
	}
}

func TestWhereAreWeForARepositoryNarrowsOnlyNext(t *testing.T) {
	b := severalBoard(t)
	whole, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	web, err := b.WhereFor("demo", "web")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(web.Next), " "); got != "S-5" {
		t.Fatalf("web next %q", got)
	}
	if got := strings.Join(ids(whole.Next), " "); got != "S-4 S-5" {
		t.Fatalf("whole next %q", got)
	}
	// Everything else is the whole workspace's, tags and waiting lines included.
	wholeText, webText := whole.Text(), web.Text()
	if strings.ReplaceAll(wholeText, "  S-4 [api] Api next\n", "") != webText {
		t.Fatalf("only Next should differ:\n%s\nvs:\n%s", wholeText, webText)
	}
	if len(web.InProgress) != 1 || len(web.Unreleased) != 2 {
		t.Fatalf("in progress %d, unreleased %d", len(web.InProgress), len(web.Unreleased))
	}
}

func TestWhereAreWeForAnUnknownRepositoryIsNotFound(t *testing.T) {
	b := severalBoard(t)
	_, err := b.WhereFor("demo", "ios")
	wantMessage(t, err, board.NotFound, `demo has no repo "ios"`)
}

func TestWhereAreWeNamingTheOnlyRepositoryChangesNothing(t *testing.T) {
	b := whereBoard(t)
	whole, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	named, err := b.WhereFor("demo", "demo")
	if err != nil || named.Text() != whole.Text() {
		t.Fatalf("named:\n%s\n%v\nwhole:\n%s", named.Text(), err, whole.Text())
	}
}

func TestEventsMergeLinesNameTheRepositoryOnlyWithSeveralRepositories(t *testing.T) {
	b, _ := twoRepos(t)
	id := readyIn(t, b, "", "Only api")
	walk(t, b, id, board.InProgress, board.InReview)
	setStatus(t, b, id, board.StatusChange{To: board.Merged, Commit: "abc1234"})
	if lines := eventLines(latest(t, b, board.EventsQuery{})); !strings.Contains(lines, "merged into dev (abc1234)") {
		t.Fatalf("one repository:\n%s", lines)
	}
	addWeb(t, b)
	setStatus(t, b, id, board.StatusChange{To: board.Merged, Into: "main", Commit: "ffff000"})
	web := readyIn(t, b, "web", "In web")
	walk(t, b, web, board.InProgress, board.InReview)
	setStatus(t, b, web, board.StatusChange{To: board.Merged, Commit: "9999999"})
	lines := eventLines(latest(t, b, board.EventsQuery{}))
	for _, want := range []string{"S-1 merged into api/dev (abc1234)", "S-1 merged into api/main (ffff000)", "S-2 merged into web/dev (9999999)"} {
		if !strings.Contains(lines, want) {
			t.Errorf("no %q in:\n%s", want, lines)
		}
	}
}
