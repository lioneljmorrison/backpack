package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/live"
)

// twoRepoSite serves a workspace "demo" whose repository api has dev merging
// into main, and, once addWeb is called, a second one, web, with main, dev
// and staging. web is written straight into the graph, as no action adds a
// repository yet. Invented names throughout.
func twoRepoSite(t *testing.T) (http.Handler, *board.Board, func()) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	hub := live.New()
	b.OnChange(hub.Publish)
	h := New(known, b, nil, hub)
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo", RepoName: "api", HasReleaseStage: true}))
	addWeb := func() {
		t.Helper()
		if _, err := db.SelectGraph(prefix+"ws:demo").Query(`CREATE (r:Repo {name: 'web', created_by: 'test', created_at: 'x'}),
			(r)-[:HAS_BRANCH]->(:Branch {name: 'main', created_by: 'test', created_at: 'x'}),
			(r)-[:HAS_BRANCH]->(:Branch {name: 'dev', into: 'main', created_by: 'test', created_at: 'x'}),
			(r)-[:HAS_BRANCH]->(:Branch {name: 'staging', into: 'main', created_by: 'test', created_at: 'x'})`, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	return h, b, addWeb
}

// inReview takes a new story of repo to review.
func inReview(t *testing.T, b *board.Board, repo, title string) board.Story {
	t.Helper()
	s := must[board.Story](t)(b.AddStory("demo", "tester", board.NewStory{Title: title, Repo: repo}))
	must[board.Story](t)(b.SetStatus("demo", "tester", s.ID, board.StatusChange{To: board.InProgress, Branch: "feature/x"}))
	return must[board.Story](t)(b.SetStatus("demo", "tester", s.ID, board.StatusChange{To: board.InReview, PRURL: "https://example.invalid/pr/1"}))
}

func TestOneRepositoryBoardHasNoFilterColoursOrLabels(t *testing.T) {
	h, b, _ := twoRepoSite(t)
	s := inReview(t, b, "", "Solo")
	page := get(h, "/w/demo", "bp_good").Body.String()
	for _, banned := range []string{`data-repo`, `api · `, `class="repos"`, `repo-0`, `tag repo`} {
		if strings.Contains(page, banned) {
			t.Errorf("a one-repository board shows %q", banned)
		}
	}
	if !strings.Contains(page, `aria-label="Merged (dev)" data-status="merged" data-into="dev"`) {
		t.Error("the Merged column is not as it was")
	}
	if tag := cardTag(t, page, s.ID); strings.Contains(tag, "repo") {
		t.Errorf("card %s", tag)
	}
	one := get(h, "/w/demo/stories/"+s.ID, "bp_good").Body.String()
	if strings.Contains(one, "api") && strings.Contains(one, `class="tag repo`) {
		t.Error("the story page names the only repository")
	}
}

func TestWithTwoRepositoriesMergedColumnsAreGroupedAndKeyedByRepository(t *testing.T) {
	h, b, addWeb := twoRepoSite(t)
	addWeb()
	page := get(h, "/w/demo", "bp_good").Body.String()
	for _, want := range []string{
		`aria-label="api · Merged (dev)" data-status="merged" data-repo="api" data-into="dev"`,
		`aria-label="api · Merged (main)" data-status="merged" data-repo="api" data-into="main"`,
		`aria-label="web · Merged (dev)" data-status="merged" data-repo="web" data-into="dev"`,
		`aria-label="web · Merged (staging)" data-status="merged" data-repo="web" data-into="staging"`,
		`aria-label="Backlog" data-status="backlog"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("board lacks %s", want)
		}
	}
	if !before(page, `api · Merged (main)"`, `web · Merged (dev)"`) || !before(page, `web · Merged (staging)"`, `aria-label="Closed"`) {
		t.Error("the Merged columns are not grouped by repository, before Closed")
	}
	_ = b
}

func TestACardsMovesNeverNameAnotherRepository(t *testing.T) {
	h, b, addWeb := twoRepoSite(t)
	addWeb()
	a := inReview(t, b, "api", "Api work")
	w := inReview(t, b, "web", "Web work")
	page := get(h, "/w/demo", "bp_good").Body.String()
	at, wt := cardTag(t, page, a.ID), cardTag(t, page, w.ID)
	if !strings.Contains(at, `data-repo="api"`) || !strings.Contains(at, `data-moves="on_hold in_progress merged:dev closed"`) {
		t.Errorf("api card %s", at)
	}
	if !strings.Contains(wt, `data-repo="web"`) || !strings.Contains(wt, `data-moves="on_hold in_progress merged:dev merged:staging closed"`) {
		t.Errorf("web card %s", wt)
	}
}

func TestEachRepositorysCardsHaveTheirOwnColourAndNameTheRepository(t *testing.T) {
	h, b, addWeb := twoRepoSite(t)
	addWeb()
	a := inReview(t, b, "api", "Api work")
	w := inReview(t, b, "web", "Web work")
	page := get(h, "/w/demo", "bp_good").Body.String()
	if at := cardTag(t, page, a.ID); !strings.Contains(at, "repo-0") {
		t.Errorf("api card %s", at)
	}
	if wt := cardTag(t, page, w.ID); !strings.Contains(wt, "repo-1") {
		t.Errorf("web card %s", wt)
	}
	if !strings.Contains(page, `<span class="tag repo repo-0">api</span>`) || !strings.Contains(page, `<span class="tag repo repo-1">web</span>`) {
		t.Error("the cards do not name their repositories")
	}
}

func TestTheFilterShowsOnlyOneRepository(t *testing.T) {
	h, b, addWeb := twoRepoSite(t)
	addWeb()
	a := inReview(t, b, "api", "Api work")
	w := inReview(t, b, "web", "Web work")
	all := get(h, "/w/demo", "bp_good").Body.String()
	for _, want := range []string{`class="repos"`, `href="/w/demo?repo=api"`, `href="/w/demo?repo=web"`, a.Title, w.Title} {
		if !strings.Contains(all, want) {
			t.Errorf("the whole board lacks %s", want)
		}
	}
	page := get(h, "/w/demo?repo=web", "bp_good").Body.String()
	if strings.Contains(page, a.Title) || !strings.Contains(page, w.Title) {
		t.Error("the filter does not show only web's cards")
	}
	if strings.Contains(page, `data-repo="api"`) || !strings.Contains(page, `data-repo="web"`) {
		t.Error("the filter does not show only web's Merged columns")
	}
	if !strings.Contains(page, `aria-current="true"`) {
		t.Error("the filter does not mark the chosen repository")
	}
	if code := get(h, "/w/demo?repo=ios", "bp_good").Code; code != http.StatusNotFound {
		t.Errorf("an unknown repository gave %d, want 404", code)
	}
}

func TestABranchAliasIsPerRepository(t *testing.T) {
	h, b, addWeb := twoRepoSite(t)
	addWeb()
	prod := "Production"
	must[board.Branch](t)(b.UpdateBranch("demo", "tester", "api", "main", board.BranchPatch{Alias: &prod}))
	page := get(h, "/w/demo", "bp_good").Body.String()
	if !strings.Contains(page, `aria-label="api · Merged (Production)"`) || !strings.Contains(page, `aria-label="web · Merged (main)"`) {
		t.Error("an alias did not stay with its own repository's branch")
	}
}

func TestTheStoryPageNamesItsRepositoryOnlyWithSeveral(t *testing.T) {
	h, b, addWeb := twoRepoSite(t)
	s := must[board.Story](t)(b.AddStory("demo", "tester", board.NewStory{Title: "Alone"}))
	if p := get(h, "/w/demo/stories/"+s.ID, "bp_good").Body.String(); strings.Contains(p, `tag repo`) {
		t.Error("the story page names the only repository")
	}
	addWeb()
	w := must[board.Story](t)(b.AddStory("demo", "tester", board.NewStory{Title: "Web", Repo: "web"}))
	wantPage(t, get(h, "/w/demo/stories/"+w.ID, "bp_good"), `<span class="tag repo repo-1">web</span>`)
}

func TestTheNewStoryFormOffersARepositoryOnlyWithSeveralAndAddsIntoIt(t *testing.T) {
	h, b, addWeb := twoRepoSite(t)
	if p := get(h, "/w/demo/stories/new", "bp_good").Body.String(); strings.Contains(p, `name="repo"`) {
		t.Error("a one-repository form offers a repository")
	}
	addWeb()
	wantPage(t, get(h, "/w/demo/stories/new", "bp_good"), `name="repo"`, `<option value="api">`, `<option value="web">`)
	// None chosen is the board's refusal, shown on the form.
	page := act(t, h, "/w/demo/stories", url.Values{"title": {"Which?"}}, "/w/demo/stories/new")
	if !strings.Contains(page, "name the repo: api or web") {
		t.Errorf("no refusal naming the choices: %s", page)
	}
	wantRedirect(t, post(h, host, "/w/demo/stories", "bp_good", url.Values{"title": {"For web"}, "repo": {"web"}}), "/w/demo/stories/S-1")
	if s := must[board.Story](t)(b.Story("demo", "S-1")); s.Repo != "web" {
		t.Fatalf("repo %q, want web", s.Repo)
	}
}

func TestADropOnAnotherRepositorysColumnIsRefused(t *testing.T) {
	h, b, addWeb := twoRepoSite(t)
	addWeb()
	a := inReview(t, b, "api", "Api work")
	path := "/w/demo/stories/" + a.ID + "/status"
	stays := func() {
		t.Helper()
		if s := must[board.Story](t)(b.Story("demo", a.ID)); s.Status != board.InReview {
			t.Fatalf("status %q, want in_review", s.Status)
		}
	}
	// A branch only web has.
	page := act(t, h, path, url.Values{"to": {"merged"}, "into": {"staging"}, "repo": {"web"}, "back": {"board"}}, "/w/demo")
	if !strings.Contains(page, `class="error"`) {
		t.Error("the drop was not refused on the board")
	}
	stays()
	// A branch both have: the repository decides, and it is not the story's.
	page = act(t, h, path, url.Values{"to": {"merged"}, "into": {"dev"}, "repo": {"web"}, "back": {"board"}}, "/w/demo")
	if !strings.Contains(page, "is in api, not web") {
		t.Errorf("the refusal does not say which repository: %s", page)
	}
	stays()
	// Its own repository's column works.
	act(t, h, path, url.Values{"to": {"merged"}, "into": {"dev"}, "repo": {"api"}, "back": {"board"}}, "/w/demo")
	if s := must[board.Story](t)(b.Story("demo", a.ID)); s.Into != "dev" {
		t.Fatalf("into %q, want dev", s.Into)
	}
}
