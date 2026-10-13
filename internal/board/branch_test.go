package board_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

func names(bs []board.Branch) (out [][2]string) {
	for _, b := range bs {
		out = append(out, [2]string{b.Name, b.Into})
	}
	return out
}

func TestANewWorkspaceWithoutAReleaseStageHasOnlyMain(t *testing.T) {
	b := newWorkspace(t)
	got, err := b.Branches("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "main" || got[0].Into != "" || !got[0].Release || got[0].CreatedBy != actor || got[0].CreatedAt == "" {
		t.Fatalf("branches %+v", got)
	}
}

func TestANewWorkspaceWithAReleaseStageHasDevIntoMainAndMain(t *testing.T) {
	b := newBoard(t)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "staged", Name: "Staged", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	got, err := b.Branches("staged")
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]string{{"dev", "main"}, {"main", ""}}
	if g := names(got); len(g) != 2 || g[0] != want[0] || g[1] != want[1] || got[0].Release || !got[1].Release {
		t.Fatalf("branches %+v", got)
	}
}

func TestBranchesOfAnUnknownWorkspaceAreRefused(t *testing.T) {
	b := newBoard(t)
	if _, err := b.Branches("nope"); err == nil {
		t.Fatal("want a refusal")
	}
}

// An old-style graph keeps its release stage as a flag on the Workspace node.
// Opening the board turns the flag into branches, and doing it again, as after
// a crash before the version was recorded, changes nothing.
func TestMigrationBuildsBranchesFromTheOldReleaseStageFlag(t *testing.T) {
	for _, c := range []struct {
		flag string
		want [][2]string
	}{
		{"true", [][2]string{{"dev", "main"}, {"main", ""}}},
		{"false", [][2]string{{"main", ""}}},
		{"null", [][2]string{{"main", ""}}},
	} {
		db, prefix := falkortest.Prefix(t)
		sys, ws := db.SelectGraph(prefix+"system"), db.SelectGraph(prefix+"ws:old")
		if _, err := sys.Query("CREATE (:WorkspaceEntry {slug: 'old', name: 'Old', has_release_stage: "+c.flag+", created_by: 'x', created_at: 'y'})", nil, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := ws.Query("CREATE (:Workspace {slug: 'old', next_story: 1, next_goal: 1, next_log: 1, has_release_stage: "+c.flag+"})", nil, nil); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			b, err := board.New(db, prefix+"system", prefix+"ws:")
			if err != nil {
				t.Fatalf("%s: %v", c.flag, err)
			}
			got, err := b.Branches("old")
			if err != nil {
				t.Fatal(err)
			}
			g := names(got)
			if len(g) != len(c.want) || g[0] != c.want[0] || (len(g) == 2 && g[1] != c.want[1]) {
				t.Fatalf("flag %s: branches %+v, want %v", c.flag, got, c.want)
			}
			// Make the next open run the migration again.
			if _, err := ws.Query("MATCH (s:SchemaVersion) SET s.v = 6", nil, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// An old-style graph holds stories merged, released and deployed. Migration 8
// gives each the branch it reached: merged is in the first branch, released
// becomes merged into the release branch, deployed stays deployed in the
// release branch. Running it again changes nothing.
func TestMigrationGivesCompleteStoriesTheBranchTheyReached(t *testing.T) {
	for _, c := range []struct {
		name, flag string
		want       map[string][2]string // story id -> status, into
	}{
		{"two branches", "true", map[string][2]string{
			"S-1": {"merged", "dev"}, "S-2": {"merged", "main"}, "S-3": {"deployed", "main"}, "S-4": {"backlog", ""}}},
		{"one branch", "false", map[string][2]string{
			"S-1": {"merged", "main"}, "S-2": {"merged", "main"}, "S-3": {"deployed", "main"}, "S-4": {"backlog", ""}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			db, prefix := falkortest.Prefix(t)
			sys, ws := db.SelectGraph(prefix+"system"), db.SelectGraph(prefix+"ws:old")
			if _, err := sys.Query("CREATE (:WorkspaceEntry {slug: 'old', name: 'Old', created_by: 'x', created_at: 'y'})", nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := ws.Query("CREATE (:Workspace {slug: 'old', next_story: 5, next_goal: 1, next_log: 1, has_release_stage: "+c.flag+"})", nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := board.New(db, prefix+"system", prefix+"ws:"); err != nil {
				t.Fatal(err)
			}
			// Back to the graph as it was before migration 8: statuses only.
			for q, params := range map[string]map[string]any{
				"CREATE (:Story {id: 'S-1', title: 'a', status: 'merged', rank: 'a'})":   nil,
				"CREATE (:Story {id: 'S-2', title: 'b', status: 'released', rank: 'b'})": nil,
				"CREATE (:Story {id: 'S-3', title: 'c', status: 'deployed', rank: 'c'})": nil,
				"CREATE (:Story {id: 'S-4', title: 'd', status: 'backlog', rank: 'd'})":  nil,
			} {
				if _, err := ws.Query(q, params, nil); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if _, err := ws.Query("MATCH (s:SchemaVersion) SET s.v = 7", nil, nil); err != nil {
					t.Fatal(err)
				}
				b, err := board.New(db, prefix+"system", prefix+"ws:")
				if err != nil {
					t.Fatal(err)
				}
				for id, want := range c.want {
					s, err := b.Story("old", id)
					if err != nil {
						t.Fatal(err)
					}
					if s.Status != want[0] || s.Into != want[1] {
						t.Fatalf("%s: %s into %q, want %v", id, s.Status, s.Into, want)
					}
				}
			}
		})
	}
}
