package board_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

func TestACreatedWorkspaceIsListedWithItsCreator(t *testing.T) {
	b := newBoard(t)
	in := board.Workspace{Slug: "my-app", Name: "My app", RepoURL: "https://example.com/my-app"}
	if _, err := b.CreateWorkspace(actor, in); err != nil {
		t.Fatal(err)
	}
	got, err := b.Workspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d workspaces, want 1", len(got))
	}
	w := got[0]
	if w.Slug != in.Slug || w.Name != in.Name || w.RepoURL != in.RepoURL || w.CreatedBy != actor || w.CreatedAt == "" {
		t.Fatalf("listed %+v", w)
	}
}

func TestWorkspacesAreListedBySlug(t *testing.T) {
	b := newBoard(t)
	for _, s := range []string{"zeta", "alpha"} {
		if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: s, Name: s}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := b.Workspaces()
	if len(got) != 2 || got[0].Slug != "alpha" || got[1].Slug != "zeta" {
		t.Fatalf("listed %+v", got)
	}
}

func TestCreateWorkspaceRejectsInvalidInput(t *testing.T) {
	b := newBoard(t)
	for _, w := range []board.Workspace{
		{Slug: "", Name: "x"},
		{Slug: "Has-Caps", Name: "x"},
		{Slug: "has space", Name: "x"},
		{Slug: "-leading", Name: "x"},
		{Slug: "ok", Name: ""},
	} {
		_, err := b.CreateWorkspace(actor, w)
		wantKind(t, err, board.Invalid)
	}
}

func TestASlugCanBeUsedOnlyOnce(t *testing.T) {
	b := newWorkspace(t)
	_, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Again"})
	wantKind(t, err, board.Conflict)
}

// A create interrupted after the workspace's own graph was written, but
// before it was registered, leaves no workspace. Repeating it succeeds.
func TestRepeatingAnInterruptedCreateSucceeds(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SelectGraph(prefix+"ws:demo").Query("CREATE (:Workspace {slug: 'demo', next_story: 1, next_goal: 1})", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Workspaces(); len(got) != 0 {
		t.Fatalf("an unregistered workspace was listed: %+v", got)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatalf("repeat: %v", err)
	}
}

func TestStartingUpMigratesEveryRegisteredWorkspace(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, _ := board.New(db, prefix+"system", prefix+"ws:")
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	g := db.SelectGraph(prefix + "ws:demo")
	if _, err := g.Query("MATCH (s:SchemaVersion) SET s.v = 0", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := board.New(db, prefix+"system", prefix+"ws:"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if v, _ := falkor.Version(g); v == 0 {
		t.Fatal("the workspace graph was not migrated at start-up")
	}
}

func TestAWorkspaceHasNoReleaseStageUnlessCreatedWithOne(t *testing.T) {
	b := newBoard(t)
	for _, w := range []board.Workspace{{Slug: "plain", Name: "Plain"}, {Slug: "staged", Name: "Staged", HasReleaseStage: true}} {
		if _, err := b.CreateWorkspace(actor, w); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := b.Workspaces()
	if got[0].Slug != "plain" || got[0].HasReleaseStage || !got[1].HasReleaseStage {
		t.Fatalf("listed %+v", got)
	}
}

// A create that loses a race for the slug, or repeats an interrupted one,
// prepares the workspace graph again. It must not change the release stage
// the graph already has, and the registry must agree with the graph.
func TestPreparingAnExistingWorkspaceGraphKeepsItsReleaseStage(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SelectGraph(prefix+"ws:demo").Query(
		"CREATE (:Workspace {slug: 'demo', next_story: 1, next_goal: 1, next_log: 1, has_release_stage: true})", nil, nil); err != nil {
		t.Fatal(err)
	}
	w, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo", HasReleaseStage: false})
	if err != nil {
		t.Fatal(err)
	}
	listed, _ := b.Workspaces()
	if !w.HasReleaseStage || !listed[0].HasReleaseStage {
		t.Fatalf("created %+v, listed %+v", w, listed)
	}
	if cols, _ := b.Board("demo"); len(cols) != 9 {
		t.Fatalf("the graph lost its release stage: %d columns", len(cols))
	}
}
