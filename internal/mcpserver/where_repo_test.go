package mcpserver_test

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// severalReposBoard is the large invented board in repository api plus a
// second repository, web, holding invented stories of its own.
func severalReposBoard(t *testing.T, large bool) (*board.Board, *mcp.ClientSession) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo", RepoName: "api"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SelectGraph(prefix+"ws:demo").Query(`CREATE (r:Repo {name: 'web', created_by: 'test', created_at: 'x'}),
		(r)-[:HAS_BRANCH]->(:Branch {name: 'main', created_by: 'test', created_at: 'x'}),
		(r)-[:HAS_BRANCH]->(:Branch {name: 'dev', into: 'main', created_by: 'test', created_at: 'x'})`, nil, nil); err != nil {
		t.Fatal(err)
	}
	if large {
		largeBoard(t, b, "api")
	}
	var webIDs []string
	for _, title := range []string{"Web first", "Web second"} {
		s, err := b.AddStory("demo", "tester", board.NewStory{Title: title, Repo: "web"})
		if err != nil {
			t.Fatal(err)
		}
		webIDs = append(webIDs, s.ID)
	}
	if large { // a web story under way, so the long text carries both tags
		if _, err := b.SetStatus("demo", "tester", webIDs[1], board.StatusChange{To: "in_progress"}); err != nil {
			t.Fatal(err)
		}
	}
	return b, session(t, serve(t, b, nil))
}

func TestWhereAreWeTakesARepositoryAndNarrowsOnlyNext(t *testing.T) {
	_, cs := severalReposBoard(t, false)
	whole := say(t, cs, "where_are_we", map[string]any{"workspace": "demo"})
	if whole != "Next:\n  S-1 [web] Web first\n  S-2 [web] Web second\n" {
		t.Fatalf("whole:\n%s", whole)
	}
	if got := say(t, cs, "where_are_we", map[string]any{"workspace": "demo", "repo": "api"}); got != "Nothing in progress, in review or in the backlog.\n" {
		t.Fatalf("api:\n%s", got)
	}
	if got := say(t, cs, "where_are_we", map[string]any{"workspace": "demo", "repo": "web"}); got != whole {
		t.Fatalf("web:\n%s", got)
	}
	if why := refused(t, cs, "where_are_we", map[string]any{"workspace": "demo", "repo": "ios"}); why != `demo has no repo "ios"` {
		t.Fatalf("reason %q", why)
	}
}

func TestWhereAreWeWithSeveralRepositoriesStaysUnderTwoKilobytes(t *testing.T) {
	_, cs := severalReposBoard(t, true)
	got := say(t, cs, "where_are_we", map[string]any{"workspace": "demo"})
	if !strings.Contains(got, "[api]") || !strings.Contains(got, "[web]") || !strings.Contains(got, "waiting for a release in api: 30") {
		t.Fatalf("not tagged:\n%s", got)
	}
	if len(got) >= 2000 {
		t.Fatalf("%d bytes, want under 2000:\n%s", len(got), got)
	}
	t.Logf("where_are_we answered %d bytes with two repositories", len(got))
}
