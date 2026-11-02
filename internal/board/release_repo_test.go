package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// releasing is workspace demo with repositories api and web. api holds S-1
// and S-2, merged into main; web holds W-1 and W-2, merged into main, and W-3,
// merged into dev. Stories are added before web exists, because adding to a
// chosen repository is S-70.
func releasing(t *testing.T) *board.Board {
	t.Helper()
	b, g := twoRepos(t)
	mergedIntoMain(t, b, "Api one")
	mergedIntoMain(t, b, "Api two")
	addWeb(t, b)
	webStory(t, g, "W-1", "merged", "main")
	webStory(t, g, "W-2", "merged", "main")
	webStory(t, g, "W-3", "merged", "dev")
	return b
}

func cutIn(t *testing.T, b *board.Board, repo, version string) board.Release {
	t.Helper()
	r, err := b.CutRelease("demo", actor, board.NewRelease{Repo: repo, Version: version, Tag: version})
	if err != nil {
		t.Fatalf("cut %s in %s: %v", version, repo, err)
	}
	return r
}

func TestTwoRepositoriesEachCutV1WithOnlyTheirOwnStories(t *testing.T) {
	b := releasing(t)

	web := cutIn(t, b, "web", "v1.0.0")
	if web.Repo != "web" || web.ID != "R-1" || !slices.Equal(web.Stories, []string{"W-1", "W-2"}) {
		t.Fatalf("web release %+v", web)
	}
	// R-n is one series for the workspace, and v1.0.0 is free in api.
	api := cutIn(t, b, "api", "v1.0.0")
	if api.Repo != "api" || api.ID != "R-2" || !slices.Equal(api.Stories, []string{"S-1", "S-2"}) {
		t.Fatalf("api release %+v", api)
	}
	wantStatus(t, b, "W-1", board.Released)
	wantStatus(t, b, "S-1", board.Released)
	// web's story merged into dev is not in any release.
	wantStatus(t, b, "W-3", board.Merged)

	d, err := b.GetRelease("demo", "R-1")
	if err != nil || d.Repo != "web" || len(d.Entries) != 2 || d.Entries[0].Repo != "web" {
		t.Fatalf("R-1 %+v, %v", d, err)
	}
}

func TestACutInOneRepositoryLeavesTheOthersMergedStoriesAlone(t *testing.T) {
	b := releasing(t)
	cutIn(t, b, "api", "v1.0.0")
	wantStatus(t, b, "W-1", board.Merged)
	wantStatus(t, b, "W-2", board.Merged)
	// api has nothing left, though web does.
	_, err := b.CutRelease("demo", actor, board.NewRelease{Repo: "api", Version: "v1.1.0", Tag: "v1.1.0"})
	wantKind(t, err, board.Conflict)
}

func TestACutNeedsARepositoryWhenThereAreSeveralAndKnowsOnlyRealOnes(t *testing.T) {
	b := releasing(t)
	_, err := b.CutRelease("demo", actor, v1)
	wantMessage(t, err, board.Invalid, "demo has several repos; name the repo: api or web")
	_, err = b.CutRelease("demo", actor, board.NewRelease{Repo: "x", Version: "v1.0.0", Tag: "v1.0.0"})
	wantMessage(t, err, board.NotFound, `demo has no repo "x"`)
	// The refusals changed nothing and took no release id.
	wantStatus(t, b, "S-1", board.Merged)
	if r := cutIn(t, b, "api", "v1.0.0"); r.ID != "R-1" {
		t.Fatalf("release %+v, want R-1", r)
	}
}

func TestAVersionIsUniqueWithinItsRepositoryOnly(t *testing.T) {
	b := releasing(t)
	cutIn(t, b, "api", "v1.0.0")
	mergedIntoMain(t, b, "Api later")
	_, err := b.CutRelease("demo", actor, board.NewRelease{Repo: "api", Version: "v1.0.0", Tag: "other"})
	wantMessage(t, err, board.Conflict, "api already has a release v1.0.0")
	wantStatus(t, b, "S-3", board.Merged)
	// web has none yet, so it may use the version.
	cutIn(t, b, "web", "v1.0.0")
}

func TestTheReleaseListNarrowsToOneRepository(t *testing.T) {
	b := releasing(t)
	cutIn(t, b, "web", "v1.0.0")
	cutIn(t, b, "api", "v1.0.0")

	all, err := b.ListReleases("demo", "")
	if err != nil || len(all) != 2 || all[0].ID != "R-2" || all[0].Repo != "api" || all[1].Repo != "web" {
		t.Fatalf("all %+v, %v", all, err)
	}
	web, err := b.ListReleases("demo", "web")
	if err != nil || len(web) != 1 || web[0].ID != "R-1" || web[0].Repo != "web" {
		t.Fatalf("web %+v, %v", web, err)
	}
	_, err = b.ListReleases("demo", "x")
	wantMessage(t, err, board.NotFound, `demo has no repo "x"`)
}

func TestAOneRepositoryWorkspaceCutsWithoutNamingItsRepository(t *testing.T) {
	b := threeInMainOneInDev(t)
	r, err := b.CutRelease("demo", actor, v1)
	if err != nil || r.Repo != "demo" || r.ID != "R-1" {
		t.Fatalf("release %+v, %v", r, err)
	}
	// Naming the only repository is accepted too.
	mergedIntoMain(t, b, "Later")
	if _, err := b.CutRelease("demo", actor, board.NewRelease{Repo: "demo", Version: "v1.1.0", Tag: "v1.1.0"}); err != nil {
		t.Fatal(err)
	}
	// And a version already released is a refusal that names the repository.
	mergedIntoMain(t, b, "Later still")
	_, err = b.CutRelease("demo", actor, v1)
	wantMessage(t, err, board.Conflict, "demo already has a release v1.0.0")
}

func TestWhereAreWeGathersWhatWaitsForEachRepositorysOwnReleaseBranch(t *testing.T) {
	b, g := twoRepos(t)
	mergedIntoMain(t, b, "Api in main")
	addWeb(t, b)
	// web's release branch is trunk, not main.
	run(t, g, `MATCH (r:Repo {name: 'web'})-[:HAS_BRANCH]->(m:Branch {name: 'main'}) SET m.name = 'trunk'
		WITH r MATCH (r)-[:HAS_BRANCH]->(d:Branch {name: 'dev'}) SET d.into = 'trunk'`)
	webStory(t, g, "W-1", "merged", "trunk")
	webStory(t, g, "W-2", "merged", "dev")
	webStory(t, g, "W-3", "merged", "main")

	w, err := b.Where("demo")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range w.Unreleased {
		got = append(got, s.Repo+"/"+s.ID)
	}
	if want := []string{"api/S-1", "web/W-1"}; !slices.Equal(got, want) {
		t.Fatalf("unreleased %v, want %v", got, want)
	}
}

func TestTheReleaseStageIsOnWhenAnyRepositoryHasAMergeStep(t *testing.T) {
	b, g := twoRepos(t)
	if on, err := b.ReleaseStage("demo"); err != nil || !on {
		t.Fatalf("api merges dev into main: %v, %v", on, err)
	}
	// Only web has a merge step.
	run(t, g, `MATCH (b:Branch {name: 'dev'}) DELETE b`)
	if on, err := b.ReleaseStage("demo"); err != nil || on {
		t.Fatalf("no repository merges: %v, %v", on, err)
	}
	addWeb(t, b)
	if on, err := b.ReleaseStage("demo"); err != nil || !on {
		t.Fatalf("web merges dev into main: %v, %v", on, err)
	}
}

func TestReposListsTheWorkspacesRepositories(t *testing.T) {
	b, _ := twoRepos(t)
	addWeb(t, b)
	rs, err := b.Repos("demo")
	if err != nil || len(rs) != 2 || rs[0].Name != "api" || rs[1].Name != "web" {
		t.Fatalf("repos %+v, %v", rs, err)
	}
	_, err = b.Repos("nope")
	wantKind(t, err, board.NotFound)
}
