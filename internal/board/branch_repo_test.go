package board_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FalkorDB/falkordb-go/v2"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// twoRepos is workspace "demo" with a release stage in repository "api". The
// second repository is added with addWeb, directly in the graph, because no
// action adds a repository yet.
func twoRepos(t *testing.T) (*board.Board, *falkordb.Graph) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", RepoName: "api", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	return b, db.SelectGraph(prefix + "ws:demo")
}

// addWeb gives the workspace a second repository, web, with its own dev
// merging into its own main.
func addWeb(t *testing.T, g *falkordb.Graph) {
	t.Helper()
	run(t, g, `CREATE (r:Repo {name: 'web', created_by: 'test', created_at: 'x'}),
		(r)-[:HAS_BRANCH]->(:Branch {name: 'main', created_by: 'test', created_at: 'x'}),
		(r)-[:HAS_BRANCH]->(:Branch {name: 'dev', into: 'main', created_by: 'test', created_at: 'x'})`)
}

// webStory puts a story in web, as if it had been added there.
func webStory(t *testing.T, g *falkordb.Graph, id, status, into string) {
	t.Helper()
	run(t, g, fmt.Sprintf(`MATCH (r:Repo {name: 'web'})
		CREATE (r)-[:HOLDS]->(:Story {id: '%s', title: 'Web story', status: '%s', into: '%s', rank: 'n'})`, id, status, into))
}

func run(t *testing.T, g *falkordb.Graph, q string) {
	t.Helper()
	if _, err := g.Query(q, nil, nil); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// storyInto reads the branch a story is in, straight from the graph.
func storyInto(t *testing.T, g *falkordb.Graph, id string) string {
	t.Helper()
	res, err := g.ROQuery("MATCH (s:Story {id: $id}) RETURN s.into", map[string]any{"id": id}, nil)
	if err != nil || !res.Next() {
		t.Fatalf("story %s: %v", id, err)
	}
	v, _ := res.Record().GetByIndex(0)
	s, _ := v.(string)
	return s
}

// layout lists branches as repo/name[>into], in the order given.
func layout(bs []board.Branch) string {
	var out []string
	for _, b := range bs {
		s := b.Repo + "/" + b.Name
		if b.Into != "" {
			s += ">" + b.Into
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

func wantMessage(t *testing.T, err error, kind board.Kind, msg string) {
	t.Helper()
	wantKind(t, err, kind)
	if err.Error() != msg {
		t.Fatalf("message %q, want %q", err.Error(), msg)
	}
}

func TestTwoRepositoriesCanEachHaveTheSameBranchNames(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	for _, repo := range []string{"api", "web"} {
		br, err := b.AddBranch("demo", actor, board.Branch{Name: "hotfix", Into: "main", Repo: repo})
		if err != nil || br.Repo != repo || br.Name != "hotfix" {
			t.Fatalf("add hotfix to %s: %+v, %v", repo, br, err)
		}
	}
	// A name is taken only within its own repository.
	_, err := b.AddBranch("demo", actor, board.Branch{Name: "hotfix", Into: "main", Repo: "web"})
	wantKind(t, err, board.Conflict)

	// Every repository's branches, by repository and then in merge order.
	all, err := b.Branches("demo", "")
	if err != nil {
		t.Fatal(err)
	}
	want := "api/dev>main api/hotfix>main api/main web/dev>main web/hotfix>main web/main"
	if got := layout(all); got != want {
		t.Fatalf("branches\n got %s\nwant %s", got, want)
	}
	one, err := b.Branches("demo", "web")
	if err != nil || layout(one) != "web/dev>main web/hotfix>main web/main" {
		t.Fatalf("web's branches %s, %v", layout(one), err)
	}
	_, err = b.Branches("demo", "nope")
	wantMessage(t, err, board.NotFound, `demo has no repo "nope"`)
}

func TestAddingABranchMergesIntoThatRepositorysBranchesOnly(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	run(t, g, `MATCH (r:Repo {name: 'web'}) CREATE (r)-[:HAS_BRANCH]->(:Branch {name: 'staging', into: 'main'})`)
	// api has no staging to merge into, whatever web has.
	_, err := b.AddBranch("demo", actor, board.Branch{Name: "x", Into: "staging", Repo: "api"})
	wantKind(t, err, board.NotFound)
	if _, err := b.AddBranch("demo", actor, board.Branch{Name: "x", Into: "staging", Repo: "web"}); err != nil {
		t.Fatal(err)
	}
}

func TestRenamingOneRepositorysBranchLeavesTheOthersAlone(t *testing.T) {
	b, g := twoRepos(t)
	s := addStory(t, b, board.NewStory{Title: "Api story"})
	walk(t, b, s.ID, board.InProgress, board.InReview)
	setStatus(t, b, s.ID, board.StatusChange{To: board.Merged, Commit: "c1"})
	addWeb(t, g)
	webStory(t, g, "W-1", "merged", "dev")
	run(t, g, `MATCH (r:Repo {name: 'web'}) CREATE (r)-[:HAS_BRANCH]->(:Branch {name: 'hotfix', into: 'dev'})`)
	run(t, g, `MATCH (r:Repo {name: 'api'}) CREATE (r)-[:HAS_BRANCH]->(:Branch {name: 'hotfix', into: 'dev'})`)

	to := "develop"
	br, err := b.UpdateBranch("demo", actor, "api", "dev", board.BranchPatch{Name: &to})
	if err != nil || br.Name != "develop" || br.Repo != "api" {
		t.Fatalf("rename: %+v, %v", br, err)
	}
	if got := storyInto(t, g, s.ID); got != "develop" {
		t.Fatalf("api's story is into %q, want develop", got)
	}
	if got := storyInto(t, g, "W-1"); got != "dev" {
		t.Fatalf("web's story is into %q, want dev", got)
	}
	all, _ := b.Branches("demo", "")
	want := "api/hotfix>develop api/develop>main api/main web/hotfix>dev web/dev>main web/main"
	if got := layout(all); got != want {
		t.Fatalf("branches\n got %s\nwant %s", got, want)
	}
	// The new name is free to use in the other repository, and the old one in this.
	if _, err := b.UpdateBranch("demo", actor, "web", "dev", board.BranchPatch{Name: &to}); err != nil {
		t.Fatalf("rename web's dev to develop: %v", err)
	}
	if got := storyInto(t, g, "W-1"); got != "develop" {
		t.Fatalf("web's story is into %q, want develop", got)
	}
}

func TestRePointingCannotWalkIntoTheOtherRepository(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	// api: x merges into main. web: x merges into dev. In api, dev merging into
	// x is no cycle; web's x leading back to dev must not count.
	for _, br := range []board.Branch{{Name: "x", Into: "main", Repo: "api"}, {Name: "x", Into: "dev", Repo: "web"}} {
		if _, err := b.AddBranch("demo", actor, br); err != nil {
			t.Fatal(err)
		}
	}
	into := "x"
	br, err := b.UpdateBranch("demo", actor, "api", "dev", board.BranchPatch{Into: &into})
	if err != nil || br.Into != "x" {
		t.Fatalf("api's dev into x: %+v, %v", br, err)
	}
	// In web the same move is a real cycle.
	_, err = b.UpdateBranch("demo", actor, "web", "dev", board.BranchPatch{Into: &into})
	wantKind(t, err, board.Conflict)
	// A branch only the other repository has is no target.
	run(t, g, `MATCH (r:Repo {name: 'web'}) CREATE (r)-[:HAS_BRANCH]->(:Branch {name: 'staging', into: 'main'})`)
	staging := "staging"
	_, err = b.UpdateBranch("demo", actor, "api", "dev", board.BranchPatch{Into: &staging})
	wantKind(t, err, board.NotFound)
}

func TestRemovingABranchChecksOnlyItsOwnRepository(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	for _, repo := range []string{"api", "web"} {
		if _, err := b.AddBranch("demo", actor, board.Branch{Name: "stage", Into: "main", Repo: repo}); err != nil {
			t.Fatal(err)
		}
	}
	webStory(t, g, "W-1", "merged", "stage")

	// web's main is web's release branch.
	err := b.RemoveBranch("demo", actor, "web", "main")
	wantMessage(t, err, board.Conflict, "main is the release branch and is never removed")
	// api's stage holds nothing, though web's does.
	if err := b.RemoveBranch("demo", actor, "api", "stage"); err != nil {
		t.Fatalf("remove api's stage: %v", err)
	}
	err = b.RemoveBranch("demo", actor, "web", "stage")
	wantKind(t, err, board.Conflict)
	if !strings.Contains(err.Error(), "W-1") {
		t.Fatalf("refusal %q does not name W-1", err)
	}
	all, _ := b.Branches("demo", "")
	if got, want := layout(all), "api/dev>main api/main web/dev>main web/stage>main web/main"; got != want {
		t.Fatalf("branches\n got %s\nwant %s", got, want)
	}
}

func TestMergingABranchMovesOnlyThatRepositorysStories(t *testing.T) {
	b, g := twoRepos(t)
	s := addStory(t, b, board.NewStory{Title: "Api story"})
	walk(t, b, s.ID, board.InProgress, board.InReview)
	setStatus(t, b, s.ID, board.StatusChange{To: board.Merged, Commit: "c1"})
	addWeb(t, g)
	webStory(t, g, "W-1", "merged", "dev")

	moved, err := b.MergeBranch("demo", actor, "api", "dev", "x9")
	if err != nil || len(moved) != 1 || moved[0].ID != s.ID {
		t.Fatalf("moved %v, %v", moved, err)
	}
	if got := storyInto(t, g, s.ID); got != "main" {
		t.Fatalf("api's story is into %q, want main", got)
	}
	if got := storyInto(t, g, "W-1"); got != "dev" {
		t.Fatalf("web's story is into %q, want dev", got)
	}
	// Merging web's dev finds its story, and web's main is its release branch.
	if moved, err = b.MergeBranch("demo", actor, "web", "dev", "y1"); err != nil || len(moved) != 1 || moved[0].ID != "W-1" {
		t.Fatalf("web moved %v, %v", moved, err)
	}
	_, err = b.MergeBranch("demo", actor, "web", "main", "y2")
	wantKind(t, err, board.Conflict)
}

func TestWithSeveralRepositoriesOneMustBeNamed(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	const msg = "demo has several repos; name the repo: api or web"
	before, _ := b.Branches("demo", "")
	name := "other"
	_, err := b.AddBranch("demo", actor, board.Branch{Name: "x", Into: "main"})
	wantMessage(t, err, board.Invalid, msg)
	_, err = b.UpdateBranch("demo", actor, "", "dev", board.BranchPatch{Name: &name})
	wantMessage(t, err, board.Invalid, msg)
	wantMessage(t, b.RemoveBranch("demo", actor, "", "dev"), board.Invalid, msg)
	_, err = b.MergeBranch("demo", actor, "", "dev", "x9")
	wantMessage(t, err, board.Invalid, msg)
	after, _ := b.Branches("demo", "")
	if layout(before) != layout(after) {
		t.Fatalf("a refusal changed the branches:\n%s\n%s", layout(before), layout(after))
	}
}

func TestAnUnknownRepositoryIsNotFound(t *testing.T) {
	b, g := twoRepos(t)
	check := func(err error) {
		t.Helper()
		wantMessage(t, err, board.NotFound, `demo has no repo "x"`)
	}
	for range 2 { // with one repository, then with two
		name := "other"
		_, err := b.AddBranch("demo", actor, board.Branch{Name: "n", Into: "main", Repo: "x"})
		check(err)
		_, err = b.UpdateBranch("demo", actor, "x", "dev", board.BranchPatch{Name: &name})
		check(err)
		check(b.RemoveBranch("demo", actor, "x", "dev"))
		_, err = b.MergeBranch("demo", actor, "x", "dev", "x9")
		check(err)
		_, err = b.Branches("demo", "x")
		check(err)
		addWeb(t, g)
	}
}

func TestAOneRepositoryWorkspaceNeedsNoRepositoryButAcceptsItsName(t *testing.T) {
	b, _ := twoRepos(t)
	br, err := b.AddBranch("demo", actor, board.Branch{Name: "hotfix", Into: "main"})
	if err != nil || br.Repo != "api" {
		t.Fatalf("add without a repo: %+v, %v", br, err)
	}
	if _, err := b.AddBranch("demo", actor, board.Branch{Name: "other", Into: "main", Repo: "api"}); err != nil {
		t.Fatal(err)
	}
	if err := b.RemoveBranch("demo", actor, "api", "other"); err != nil {
		t.Fatal(err)
	}
	all, _ := b.Branches("demo", "api")
	if got, want := layout(all), "api/dev>main api/hotfix>main api/main"; got != want {
		t.Fatalf("branches\n got %s\nwant %s", got, want)
	}
}
