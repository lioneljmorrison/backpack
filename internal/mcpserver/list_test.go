package mcpserver_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestListIdeasAnswersIdTitleAndStateOnly(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.AddIdea("demo", "tester", board.NewIdea{Title: "A", Context: "long background"}); err != nil {
		t.Fatal(err)
	}
	is := use(t, cs, "list_ideas", map[string]any{"workspace": "demo"})["ideas"].([]any)
	i := is[0].(map[string]any)
	if len(i) != 3 || i["id"] != "I-1" || i["title"] != "A" || i["state"] != "open" {
		t.Fatalf("idea %v, want id, title and state only", i)
	}
	absent(t, i, "context", "created_by", "created_at", "became_stories")
}

func TestListPhasesAnswersGoalsWithMetAndTheDescriptionButNoAuthors(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Foundations", "description": "long text"})
	use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "it runs"})
	use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "it ships"})
	use(t, cs, "mark_goal_met", map[string]any{"workspace": "demo", "id": "G-1", "met": true})
	p := use(t, cs, "list_phases", map[string]any{"workspace": "demo"})["phases"].([]any)[0].(map[string]any)
	absent(t, p, "created_by", "created_at", "updated_by", "goals_met_early")
	if p["number"] != 1.0 || p["title"] != "Foundations" || p["complete"] != false ||
		p["stories_total"] != 0.0 || p["stories_complete"] != 0.0 {
		t.Fatalf("phase %v", p)
	}
	if p["description"] != "long text" {
		t.Fatalf("phase %v, want its description", p)
	}
	goals := p["goals"].([]any)
	g1, g2 := goals[0].(map[string]any), goals[1].(map[string]any)
	if len(g1) != 3 || g1["id"] != "G-1" || g1["text"] != "it runs" || g1["met"] != true || g2["met"] != false {
		t.Fatalf("goals %v", goals)
	}
	absent(t, g1, "created_by", "created_at", "met_by", "met_at")
}

func TestListPhasesOmitsAnEmptyDescription(t *testing.T) {
	_, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Bare"})
	p := use(t, cs, "list_phases", map[string]any{"workspace": "demo"})["phases"].([]any)[0].(map[string]any)
	absent(t, p, "description")
}

func TestListPhasesFlagsGoalsMetEarlyWhenTrue(t *testing.T) {
	b, cs := connect(t)
	use(t, cs, "add_phase", map[string]any{"workspace": "demo", "title": "Foundations"})
	use(t, cs, "add_goal", map[string]any{"workspace": "demo", "phase": 1, "text": "it runs"})
	if _, err := b.AddStory("demo", "tester", board.NewStory{Title: "Open", Phase: 1}); err != nil {
		t.Fatal(err)
	}
	use(t, cs, "mark_goal_met", map[string]any{"workspace": "demo", "id": "G-1", "met": true})
	p := use(t, cs, "list_phases", map[string]any{"workspace": "demo"})["phases"].([]any)[0].(map[string]any)
	if p["goals_met_early"] != true {
		t.Fatalf("phase %v, want goals_met_early", p)
	}
}

func TestListReleasesAnswersAStoryCountNotNotesOrIds(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	mergedIntoDev(t, b, "Second")
	if _, err := b.MergeBranch("staged", "tester", "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	use(t, cs, "cut_release", map[string]any{"workspace": "staged", "version": "v1.0.0", "tag": "v1.0.0", "commit": "abc1234", "notes": "First cut"})
	r := use(t, cs, "list_releases", map[string]any{"workspace": "staged"})["releases"].([]any)[0].(map[string]any)
	if len(r) != 4 || r["id"] != "R-1" || r["version"] != "v1.0.0" || r["tag"] != "v1.0.0" || r["stories"] != 2.0 {
		t.Fatalf("release %v, want id, version, tag and a count of 2", r)
	}
	absent(t, r, "notes", "commit", "created_by", "created_at")
}

func TestGetReleaseStoriesAreLeanAndTheReleaseCarriesNoAuthor(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	if _, err := b.MergeBranch("staged", "tester", "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	use(t, cs, "cut_release", map[string]any{"workspace": "staged", "version": "v1.0.0", "tag": "v1.0.0", "notes": "n"})
	rel := use(t, cs, "get_release", map[string]any{"workspace": "staged", "id": "R-1"})
	absent(t, rel, "created_by", "created_at")
	if rel["version"] != "v1.0.0" || rel["tag"] != "v1.0.0" || rel["notes"] != "n" {
		t.Fatalf("release %v", rel)
	}
	s := rel["stories"].([]any)[0].(map[string]any)
	absent(t, s, "created_by", "created_at", "updated_by", "updated_at", "blocked", "blocked_by", "blocks")
	if s["id"] != "S-1" || s["title"] != "First" || s["status"] != "released" {
		t.Fatalf("story %v", s)
	}
}

func TestListWorkspacesAndBranchesCarryNoAuthorOrDate(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "staged", Name: "Staged", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	for _, w := range use(t, cs, "list_workspaces", map[string]any{})["workspaces"].([]any) {
		absent(t, w.(map[string]any), "created_by", "created_at")
	}
	for _, br := range use(t, cs, "list_branches", map[string]any{"workspace": "staged"})["branches"].([]any) {
		absent(t, br.(map[string]any), "created_by", "created_at")
	}
}
