package board_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestARepositoryIsAddedWithMainAloneAsItsReleaseBranch(t *testing.T) {
	b, _ := twoRepos(t)
	r, err := b.AddRepo("demo", actor, board.Repo{Name: "web", URL: "https://example.com/web"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "web" || r.URL != "https://example.com/web" {
		t.Fatalf("added %+v", r)
	}
	rs, err := b.Repos("demo")
	if err != nil || len(rs) != 2 || rs[0].Name != "api" || rs[1].Name != "web" {
		t.Fatalf("repos %v %v", rs, err)
	}
	bs, err := b.Branches("demo", "web")
	if err != nil || len(bs) != 1 || bs[0].Name != "main" || !bs[0].Release || bs[0].Repo != "web" || bs[0].CreatedBy != actor {
		t.Fatalf("web's branches %+v %v", bs, err)
	}
	// The other repository is untouched.
	if got, _ := b.Branches("demo", "api"); layout(got) != "api/dev>main api/main" {
		t.Fatalf("api's branches %s", layout(got))
	}
}

func TestARepositoryNeedsAFreeSlugName(t *testing.T) {
	b, _ := twoRepos(t)
	for _, name := range []string{"", "Web", "web_2", "-web", "web-", "we--b", strings.Repeat("a", 65)} {
		_, err := b.AddRepo("demo", actor, board.Repo{Name: name})
		wantKind(t, err, board.Invalid)
	}
	_, err := b.AddRepo("demo", actor, board.Repo{Name: "web", URL: "bad\x00url"})
	wantKind(t, err, board.Invalid)
	if _, err := b.AddRepo("demo", actor, board.Repo{Name: "web"}); err != nil {
		t.Fatal(err)
	}
	_, err = b.AddRepo("demo", actor, board.Repo{Name: "web"})
	wantMessage(t, err, board.Conflict, `demo already has a repo named "web"`)
	_, err = b.AddRepo("demo", actor, board.Repo{Name: "api"})
	wantKind(t, err, board.Conflict)
	_, err = b.AddRepo("nope", actor, board.Repo{Name: "web"})
	wantKind(t, err, board.NotFound)
	if rs, _ := b.Repos("demo"); len(rs) != 2 {
		t.Fatalf("repos %v", rs)
	}
}

// With a second repository added for real, the whole feature works: a story
// goes where it is told, branches are shaped per repository, and an add that
// names no repository is refused with the choice.
func TestAnAddedRepositoryWorksEndToEnd(t *testing.T) {
	b, g := twoRepos(t)
	if _, err := b.AddRepo("demo", actor, board.Repo{Name: "web"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddBranch("demo", actor, board.Branch{Repo: "web", Name: "staging", Into: "main"}); err != nil {
		t.Fatal(err)
	}
	bs, _ := b.Branches("demo", "web")
	if layout(bs) != "web/staging>main web/main" {
		t.Fatalf("web: %s", layout(bs))
	}
	s, err := b.AddStory("demo", actor, board.NewStory{Title: "Web story", Repo: "web"})
	if err != nil || s.Repo != "web" {
		t.Fatalf("story %+v %v", s, err)
	}
	if count(t, g, `MATCH (:Repo {name: 'web'})-[:HOLDS]->(:Story {id: '`+s.ID+`'}) RETURN count(*)`) != 1 {
		t.Fatal("the story is not held by web")
	}
	_, err = b.AddStory("demo", actor, board.NewStory{Title: "Nowhere"})
	wantMessage(t, err, board.Invalid, "demo has several repos; name the repo: api or web")
}

func TestAddingARepositorySendsABranchNotice(t *testing.T) {
	b, _ := twoRepos(t)
	got := recorder(b)
	if _, err := b.AddRepo("demo", actor, board.Repo{Name: "web"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*got, []board.Change{branchNotice()}) {
		t.Fatalf("notices %v", *got)
	}
	*got = nil
	if _, err := b.AddRepo("demo", actor, board.Repo{Name: "web"}); err == nil {
		t.Fatal("added twice")
	}
	if len(*got) != 0 {
		t.Fatalf("a refused add sent %v", *got)
	}
}
