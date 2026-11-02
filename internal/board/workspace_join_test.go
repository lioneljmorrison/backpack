package board_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// Fixtures are invented: demo and demo-api are joined into demo-web.

// joinedBoard returns a board with workspaces demo, demo-api and demo-web,
// and the db and prefix behind it so a test can mark entries directly.
func joinedBoard(t *testing.T) (*board.Board, func(slug string, props map[string]any)) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"demo", "demo-api", "demo-web"} {
		if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: s, Name: s}); err != nil {
			t.Fatal(err)
		}
	}
	return b, func(slug string, props map[string]any) {
		falkortest.MarkEntry(t, db, prefix+"system", slug, props)
	}
}

func wantRefusal(t *testing.T, err error, kind board.Kind, msg string) {
	t.Helper()
	var be *board.Error
	if !errors.As(err, &be) || be.Kind != kind || be.Msg != msg {
		t.Fatalf("error = %v, want kind %d %q", err, kind, msg)
	}
}

func TestAJoinedWorkspaceRefusesWithAPointer(t *testing.T) {
	b, mark := joinedBoard(t)
	mark("demo-api", map[string]any{"joined_into": "demo-web", "joined_repos": []string{"api"}})
	mark("demo", map[string]any{"joined_into": "demo-web", "joined_repos": []string{"api", "web"}})

	_, err := b.Repos("demo-api")
	wantRefusal(t, err, board.NotFound, "demo-api was joined into demo-web; use workspace demo-web with repo api")
	_, err = b.Repos("demo")
	wantRefusal(t, err, board.NotFound, "demo was joined into demo-web; use workspace demo-web with repo api or web")
	_, err = b.Story("demo-api", "S-1")
	wantRefusal(t, err, board.NotFound, "demo-api was joined into demo-web; use workspace demo-web with repo api")
	// An unjoined workspace and an unknown one are as before.
	if _, err := b.Repos("demo-web"); err != nil {
		t.Fatal(err)
	}
	_, err = b.Repos("nowhere")
	wantRefusal(t, err, board.NotFound, `no workspace "nowhere"`)
}

func TestAWorkspaceBeingJoinedAsksToTryAgain(t *testing.T) {
	b, mark := joinedBoard(t)
	mark("demo-api", map[string]any{"joining": "demo-web"})
	_, err := b.Repos("demo-api")
	wantRefusal(t, err, board.Conflict, "demo-api is being joined into demo-web; try again shortly")
}

func TestJoinedWorkspacesLeaveTheListsButBeingJoinedOnesStay(t *testing.T) {
	b, mark := joinedBoard(t)
	mark("demo-api", map[string]any{"joined_into": "demo-web", "joined_repos": []string{"api"}})
	mark("demo", map[string]any{"joining": "demo-web"})
	got, err := b.Workspaces()
	if err != nil {
		t.Fatal(err)
	}
	var slugs []string
	for _, w := range got {
		slugs = append(slugs, w.Slug)
	}
	if strings.Join(slugs, ",") != "demo,demo-web" {
		t.Fatalf("listed %v", slugs)
	}
}

func TestStartingUpDoesNotMigrateAJoinedWorkspace(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, _ := board.New(db, prefix+"system", prefix+"ws:")
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo-api", Name: "Demo API"}); err != nil {
		t.Fatal(err)
	}
	falkortest.MarkEntry(t, db, prefix+"system", "demo-api", map[string]any{"joined_into": "demo-web"})
	g := db.SelectGraph(prefix + "ws:demo-api")
	if _, err := g.Query("MATCH (s:SchemaVersion) SET s.v = 0", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := board.New(db, prefix+"system", prefix+"ws:"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	res, err := g.ROQuery("MATCH (s:SchemaVersion) RETURN s.v", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Next()
	if v, _ := res.Record().GetByIndex(0); v != int64(0) {
		t.Fatalf("schema version %v: the joined graph was migrated", v)
	}
}

func TestAJoinedOrBeingBuiltSlugCannotBeCreatedAndItsGraphIsKept(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, _ := board.New(db, prefix+"system", prefix+"ws:")
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo-api", Name: "Demo API"}); err != nil {
		t.Fatal(err)
	}
	falkortest.MarkEntry(t, db, prefix+"system", "demo-api", map[string]any{"joined_into": "demo-web"})
	// A target being built: another entry's joining names a slug not yet registered.
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	falkortest.MarkEntry(t, db, prefix+"system", "demo", map[string]any{"joining": "demo-new"})

	g := db.SelectGraph(prefix + "ws:demo-api")
	before := nodeCount(t, g)
	for _, slug := range []string{"demo-api", "demo-new"} {
		_, err := b.CreateWorkspace(actor, board.Workspace{Slug: slug, Name: "Again"})
		wantRefusal(t, err, board.Conflict, `workspace "`+slug+`" already exists`)
	}
	if after := nodeCount(t, g); after != before {
		t.Fatalf("the kept graph changed: %d nodes, was %d", after, before)
	}
	if names, _ := db.ListGraphs(); slices.Contains(names, prefix+"ws:demo-new") {
		t.Fatal("a graph was made for the slug being built")
	}
}

func nodeCount(t *testing.T, g *falkordb.Graph) int64 {
	t.Helper()
	res, err := g.ROQuery("MATCH (n) RETURN count(n)", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Next()
	v, _ := res.Record().GetByIndex(0)
	return v.(int64)
}
