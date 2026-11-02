package mcpserver_test

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// twoRepos serves workspace "demo" with repositories api and web, each with
// dev merging into main, and a story merged into web's dev. The invented
// story W-1 is written straight into the graph, because no tool puts a story
// in a status and branch in one step.
func twoRepos(t *testing.T) *mcp.ClientSession {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo", RepoName: "api", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddRepo("demo", "tester", board.Repo{Name: "web"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddBranch("demo", "tester", board.Branch{Repo: "web", Name: "dev", Into: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SelectGraph(prefix+"ws:demo").Query(`MATCH (r:Repo {name: 'web'})
		CREATE (r)-[:HOLDS]->(:Story {id: 'W-1', title: 'Web', status: 'merged', into: 'dev', rank: 'n'})`, nil, nil); err != nil {
		t.Fatal(err)
	}
	return session(t, serve(t, b, nil))
}

func branchNames(t *testing.T, cs *mcp.ClientSession, args map[string]any) string {
	t.Helper()
	args["workspace"] = "demo"
	var out []string
	for _, x := range use(t, cs, "list_branches", args)["branches"].([]any) {
		b := x.(map[string]any)
		repo, _ := b["repo"].(string)
		out = append(out, repo+"/"+b["name"].(string))
	}
	return strings.Join(out, " ")
}

func TestBranchToolsTakeARepositoryAndNeedOneWhenThereAreSeveral(t *testing.T) {
	cs := twoRepos(t)
	const several = "demo has several repos; name the repo: api or web"

	// list_branches, through actions: every repository's, labelled, or one's.
	if got := branchNames(t, cs, map[string]any{}); got != "api/dev api/main web/dev web/main" {
		t.Fatalf("all: %s", got)
	}
	// One repository's list needs no label: it is the one asked for.
	if got := branchNames(t, cs, map[string]any{"repo": "web"}); got != "/dev /main" {
		t.Fatalf("web: %s", got)
	}
	if got := refused(t, cs, "list_branches", map[string]any{"workspace": "demo", "repo": "x"}); got != `demo has no repo "x"` {
		t.Fatalf("unknown repo: %s", got)
	}

	// add_branch, through batch.
	if got := refused(t, cs, "add_branch", map[string]any{"workspace": "demo", "name": "hotfix", "into": "main"}); got != several {
		t.Fatalf("add without repo: %s", got)
	}
	use(t, cs, "add_branch", map[string]any{"workspace": "demo", "name": "hotfix", "into": "main", "repo": "web"})
	use(t, cs, "add_branch", map[string]any{"workspace": "demo", "name": "hotfix", "into": "main", "repo": "api"})

	// update_branch.
	if got := refused(t, cs, "update_branch", map[string]any{"workspace": "demo", "branch": "hotfix", "alias": "Fix"}); got != several {
		t.Fatalf("update without repo: %s", got)
	}
	if got := use(t, cs, "update_branch", map[string]any{"workspace": "demo", "repo": "web", "branch": "hotfix", "alias": "Fix"}); got["name"] != "hotfix" {
		t.Fatalf("updated %v", got)
	}

	// merge_branch moves only the named repository's stories.
	if got := refused(t, cs, "merge_branch", map[string]any{"workspace": "demo", "branch": "dev", "commit": "x9"}); got != several {
		t.Fatalf("merge without repo: %s", got)
	}
	if got := use(t, cs, "merge_branch", map[string]any{"workspace": "demo", "repo": "api", "branch": "dev", "commit": "x9"})["stories"].([]any); len(got) != 0 {
		t.Fatalf("api moved %v", got)
	}
	if got := use(t, cs, "merge_branch", map[string]any{"workspace": "demo", "repo": "web", "branch": "dev", "commit": "x9"})["stories"].([]any); len(got) != 1 || got[0] != "W-1" {
		t.Fatalf("web moved %v", got)
	}

	// remove_branch.
	if got := refused(t, cs, "remove_branch", map[string]any{"workspace": "demo", "branch": "hotfix"}); got != several {
		t.Fatalf("remove without repo: %s", got)
	}
	if got := refused(t, cs, "remove_branch", map[string]any{"workspace": "demo", "repo": "web", "branch": "main"}); !strings.Contains(got, "release branch") {
		t.Fatalf("remove web's main: %s", got)
	}
	use(t, cs, "remove_branch", map[string]any{"workspace": "demo", "repo": "web", "branch": "hotfix"})
	if got := branchNames(t, cs, map[string]any{}); got != "api/dev api/hotfix api/main web/dev web/main" {
		t.Fatalf("after: %s", got)
	}
}

func TestBranchesOfASingleRepositoryAnswerWithoutRepoLabels(t *testing.T) {
	b, cs := connect(t)
	_ = b
	for _, x := range use(t, cs, "list_branches", map[string]any{"workspace": "demo"})["branches"].([]any) {
		absent(t, x.(map[string]any), "repo")
	}
}
