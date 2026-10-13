package mcpserver_test

import (
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestSetStatusStartsAStoryOnItsBranch(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	s := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "in_progress",
		"branch": "feature/20261010-first"})
	if s["status"] != "in_progress" || s["branch"] != "feature/20261010-first" || s["activity"] != "planning" {
		t.Fatalf("story %v", s)
	}
}

func TestSetStatusRefusesToStartABlockedStoryWithoutAWorkaround(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	addStory(t, b, "Second")
	if _, err := b.AddDependency("demo", "tester", "S-2", "S-1"); err != nil {
		t.Fatal(err)
	}
	refused(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-2", "to": "in_progress"})
	s := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-2", "to": "in_progress",
		"workaround": "build against mock data"})
	if s["workaround"] != "build against mock data" {
		t.Fatalf("story %v", s)
	}
}

func TestSetStatusNeedsAReasonToHold(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	if why := refused(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "on_hold"}); !strings.Contains(why, "reason") {
		t.Fatalf("reason %q", why)
	}
	s := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "on_hold",
		"reason": "needs more info"})
	if s["hold_reason"] != "needs more info" {
		t.Fatalf("story %v", s)
	}
}

func TestLogAddsAnEntryByTheTokensActor(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	e := use(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "decision",
		"text": "use the standard library", "reason": "fewer dependencies"})
	if e["kind"] != "decision" || e["actor"] != "tester" || e["reason"] != "fewer dependencies" {
		t.Fatalf("entry %v", e)
	}
}

func TestLogRefusesAStatusEntry(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	refused(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "status", "text": "done"})
}

func TestReadLogListsStatusChangesAndCorrections(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "First")
	use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "in_progress"})
	note := use(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "note", "text": "built on Tuesday"})
	use(t, cs, "log", map[string]any{"workspace": "demo", "id": "S-1", "kind": "correction",
		"text": "it was Wednesday", "corrects": note["id"]})
	l := use(t, cs, "read_log", map[string]any{"workspace": "demo", "id": "S-1"})
	entries := l["entries"].([]any)
	var kinds []string
	for _, e := range entries {
		kinds = append(kinds, e.(map[string]any)["kind"].(string))
	}
	if strings.Join(kinds, ",") != "status,note,correction" {
		t.Fatalf("entries %v", entries)
	}
	if by := entries[1].(map[string]any)["corrected_by"]; by == nil {
		t.Fatalf("note %v is not marked corrected", entries[1])
	}
}

func TestSetStatusHoldsAProposalAndResumesItToProposed(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.AddStory("demo", "tester", board.NewStory{Title: "Maybe", Proposed: true, Reasoning: "it might help"}); err != nil {
		t.Fatal(err)
	}
	refused(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "on_hold"})
	s := use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "on_hold", "reason": "wait for the design"})
	if s["status"] != "on_hold" || s["held_from"] != "proposed" {
		t.Fatalf("held %v", s)
	}
	s = use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-1", "to": "proposed"})
	if s["status"] != "proposed" {
		t.Fatalf("resumed %v", s)
	}
}

func TestSetStatusMergesIntoANamedBranchOneStepAtATime(t *testing.T) {
	b, cs := connect(t)
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "staged", Name: "Staged", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddStory("staged", "tester", board.NewStory{Title: "First"}); err != nil {
		t.Fatal(err)
	}
	set := func(args map[string]any) map[string]any {
		args["workspace"], args["id"] = "staged", "S-1"
		return use(t, cs, "set_status", args)
	}
	set(map[string]any{"to": "in_progress"})
	set(map[string]any{"to": "in_review"})
	refused(t, cs, "set_status", map[string]any{"workspace": "staged", "id": "S-1", "to": "merged", "into": "main"})
	if s := set(map[string]any{"to": "merged", "commit": "abc1234"}); s["into"] != "dev" {
		t.Fatalf("a merge naming no branch: %v", s)
	}
	if s := set(map[string]any{"to": "merged", "into": "main", "commit": "def5678"}); s["into"] != "main" {
		t.Fatalf("a merge into main: %v", s)
	}
	for _, to := range []string{"released", "deployed"} {
		why := refused(t, cs, "set_status", map[string]any{"workspace": "staged", "id": "S-1", "to": to})
		if !strings.Contains(why, "cut_release") {
			t.Fatalf("refusal %q", why)
		}
	}
}

func TestCutReleaseReleasesWhatIsMergedIntoTheReleaseBranch(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	mergedIntoDev(t, b, "Second")
	if _, err := b.MergeBranch("staged", "tester", "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	mergedIntoDev(t, b, "Third")
	got := use(t, cs, "cut_release", map[string]any{"workspace": "staged", "version": "v1.0.0", "tag": "v1.0.0", "commit": "abc1234", "notes": "First cut"})
	stories, _ := got["stories"].([]any)
	if got["id"] != "R-1" || got["version"] != "v1.0.0" || got["tag"] != "v1.0.0" || got["commit"] != "abc1234" ||
		len(stories) != 2 || stories[0] != "S-1" || stories[1] != "S-2" {
		t.Fatalf("answer %v", got)
	}
	if s, err := b.Story("staged", "S-1"); err != nil || s.Status != "released" {
		t.Fatalf("S-1 %+v, %v", s, err)
	}
	if s, err := b.Story("staged", "S-3"); err != nil || s.Status != "merged" || s.Into != "dev" {
		t.Fatalf("S-3 %+v, %v", s, err)
	}
	// Refusals: a version taken, nothing to release, a missing version or tag.
	if why := refused(t, cs, "cut_release", map[string]any{"workspace": "staged", "version": "v1.0.0", "tag": "t"}); !strings.Contains(why, "v1.0.0") {
		t.Fatalf("refusal %q", why)
	}
	if why := refused(t, cs, "cut_release", map[string]any{"workspace": "staged", "version": "v1.1.0", "tag": "v1.1.0"}); !strings.Contains(why, "nothing to release") {
		t.Fatalf("refusal %q", why)
	}
	refused(t, cs, "cut_release", map[string]any{"workspace": "staged", "version": "v1.2.0"})
	refused(t, cs, "cut_release", map[string]any{"workspace": "staged", "tag": "v1.2.0"})
}

func TestCutReleaseRunsInsideABatch(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	if _, err := b.MergeBranch("staged", "tester", "dev", "x9"); err != nil {
		t.Fatal(err)
	}
	rs := resultsIn(t, cs, "staged",
		action("cut_release", map[string]any{"version": "v1", "tag": "v1"}),
		action("cut_release", map[string]any{"version": "v2", "tag": "v2"}))
	res, _ := rs[0]["result"].(map[string]any)
	if rs[0]["ok"] != true || res["id"] != "R-1" {
		t.Fatalf("cut in a batch: %v", rs[0])
	}
	if why, _ := rs[1]["error"].(string); rs[1]["ok"] != false || !strings.Contains(why, "nothing to release") {
		t.Fatalf("a refused cut in a batch: %v", rs[1])
	}
}

// mergedIntoDev puts a story in workspace staged (dev merging into main) and
// merges it into dev.
func mergedIntoDev(t *testing.T, b *board.Board, title string) {
	t.Helper()
	s, err := b.AddStory("staged", "tester", board.NewStory{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []board.StatusChange{{To: "in_progress"}, {To: "in_review"}, {To: "merged", Commit: "c-" + s.ID}} {
		if _, err := b.SetStatus("staged", "tester", s.ID, c); err != nil {
			t.Fatal(err)
		}
	}
}

func stagedWorkspace(t *testing.T, b *board.Board) {
	t.Helper()
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "staged", Name: "Staged", HasReleaseStage: true}); err != nil {
		t.Fatal(err)
	}
}

func TestMergeBranchMovesEveryStoryOnItAndRefusesTheReleaseBranch(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	mergedIntoDev(t, b, "Second")
	got := use(t, cs, "merge_branch", map[string]any{"workspace": "staged", "branch": "dev", "commit": "x9"})
	stories, _ := got["stories"].([]any)
	if len(stories) != 2 {
		t.Fatalf("answer %v", got)
	}
	for _, s := range stories {
		if m := s.(map[string]any); m["status"] != "merged" || m["into"] != "main" {
			t.Fatalf("story %v", m)
		}
	}
	if why := refused(t, cs, "merge_branch", map[string]any{"workspace": "staged", "branch": "main", "commit": "x9"}); !strings.Contains(why, "release branch") {
		t.Fatalf("refusal %q", why)
	}
	refused(t, cs, "merge_branch", map[string]any{"workspace": "staged", "branch": "nope", "commit": "x9"})
	refused(t, cs, "merge_branch", map[string]any{"workspace": "staged", "branch": "dev"})
	empty := use(t, cs, "merge_branch", map[string]any{"workspace": "staged", "branch": "dev", "commit": "y1"})
	if list, ok := empty["stories"].([]any); !ok || len(list) != 0 {
		t.Fatalf("a branch with nothing on it answered %v, want an empty list", empty)
	}
}

func TestMergeBranchRunsInsideABatch(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	mergedIntoDev(t, b, "Second")
	rs := resultsIn(t, cs, "staged",
		action("merge_branch", map[string]any{"branch": "dev", "commit": "x9"}),
		action("merge_branch", map[string]any{"branch": "main", "commit": "x9"}))
	res, _ := rs[0]["result"].(map[string]any)
	if stories, _ := res["stories"].([]any); rs[0]["ok"] != true || len(stories) != 2 {
		t.Fatalf("merge in a batch: %v", rs[0])
	}
	if why, _ := rs[1]["error"].(string); rs[1]["ok"] != false || !strings.Contains(why, "release branch") {
		t.Fatalf("a refused merge in a batch: %v", rs[1])
	}
	for _, id := range []string{"S-1", "S-2"} {
		if s, err := b.Story("staged", id); err != nil || s.Into != "main" {
			t.Fatalf("%s: %+v, %v", id, s, err)
		}
	}
}

func TestUpdateBranchRenamesABranchAndRefusesATakenOrBadName(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	got := use(t, cs, "update_branch", map[string]any{"workspace": "staged", "branch": "dev", "name": "develop"})
	if got["name"] != "develop" || got["into"] != "main" {
		t.Fatalf("answer %v", got)
	}
	if s, err := b.Story("staged", "S-1"); err != nil || s.Status != "merged" || s.Into != "develop" {
		t.Fatalf("S-1: %+v, %v", s, err)
	}
	if why := refused(t, cs, "update_branch", map[string]any{"workspace": "staged", "branch": "develop", "name": "main"}); !strings.Contains(why, "already") {
		t.Fatalf("refusal %q", why)
	}
	refused(t, cs, "update_branch", map[string]any{"workspace": "staged", "branch": "develop", "name": "two words"})
	refused(t, cs, "update_branch", map[string]any{"workspace": "staged", "branch": "nope", "name": "x"})
	// A field not given is left alone.
	if same := use(t, cs, "update_branch", map[string]any{"workspace": "staged", "branch": "develop"}); same["name"] != "develop" {
		t.Fatalf("an empty patch answered %v", same)
	}
}

func TestUpdateBranchRunsInsideABatch(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	rs := resultsIn(t, cs, "staged",
		action("update_branch", map[string]any{"branch": "dev", "name": "develop"}),
		action("update_branch", map[string]any{"branch": "develop", "name": "main"}))
	if res, _ := rs[0]["result"].(map[string]any); rs[0]["ok"] != true || res["name"] != "develop" {
		t.Fatalf("rename in a batch: %v", rs[0])
	}
	if rs[1]["ok"] != false {
		t.Fatalf("a refused rename in a batch: %v", rs[1])
	}
	if s, err := b.Story("staged", "S-1"); err != nil || s.Into != "develop" {
		t.Fatalf("S-1: %+v, %v", s, err)
	}
}

func TestAddBranchAndRepointViaUpdateBranchMoveNoStory(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	got := use(t, cs, "add_branch", map[string]any{"workspace": "staged", "name": "staging", "into": "main"})
	if got["name"] != "staging" || got["into"] != "main" {
		t.Fatalf("answer %v", got)
	}
	re := use(t, cs, "update_branch", map[string]any{"workspace": "staged", "branch": "dev", "into": "staging"})
	if re["into"] != "staging" {
		t.Fatalf("answer %v", re)
	}
	if s, err := b.Story("staged", "S-1"); err != nil || s.Status != "merged" || s.Into != "dev" {
		t.Fatalf("S-1: %+v, %v", s, err)
	}
	moved := use(t, cs, "merge_branch", map[string]any{"workspace": "staged", "branch": "dev", "commit": "x9"})
	if st, _ := moved["stories"].([]any); len(st) != 1 || st[0].(map[string]any)["into"] != "staging" {
		t.Fatalf("merge answered %v", moved)
	}
	// Refusals: a second branch that merges nowhere, a taken name, an unknown target, a cycle.
	refused(t, cs, "add_branch", map[string]any{"workspace": "staged", "name": "other"})
	refused(t, cs, "add_branch", map[string]any{"workspace": "staged", "name": "dev", "into": "main"})
	refused(t, cs, "add_branch", map[string]any{"workspace": "staged", "name": "x", "into": "nope"})
	if why := refused(t, cs, "update_branch", map[string]any{"workspace": "staged", "branch": "staging", "into": "dev"}); !strings.Contains(why, "leads back") {
		t.Fatalf("refusal %q", why)
	}
}

func TestAddBranchRunsInsideABatch(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	rs := resultsIn(t, cs, "staged",
		action("add_branch", map[string]any{"name": "staging", "into": "main", "alias": "Staging"}),
		action("add_branch", map[string]any{"name": "bad"}))
	if rs[0]["ok"] != true || rs[1]["ok"] != false {
		t.Fatalf("batch: %v", rs)
	}
	bs, err := b.Branches("staged")
	if err != nil || len(bs) != 3 {
		t.Fatalf("branches %+v, %v", bs, err)
	}
}

func TestRemoveBranchTakesAnEmptyBranchOffAndRefusesOneInUse(t *testing.T) {
	b, cs := connect(t)
	stagedWorkspace(t, b)
	mergedIntoDev(t, b, "First")
	if _, err := b.AddBranch("staged", "tester", board.Branch{Name: "staging", Into: "main"}); err != nil {
		t.Fatal(err)
	}
	if why := refused(t, cs, "remove_branch", map[string]any{"workspace": "staged", "branch": "main"}); !strings.Contains(why, "release") {
		t.Fatalf("refusal %q", why)
	}
	if why := refused(t, cs, "remove_branch", map[string]any{"workspace": "staged", "branch": "dev"}); !strings.Contains(why, "S-1") {
		t.Fatalf("refusal %q", why)
	}
	refused(t, cs, "remove_branch", map[string]any{"workspace": "staged", "branch": "nope"})
	if got := use(t, cs, "remove_branch", map[string]any{"workspace": "staged", "branch": "staging"}); got["removed"] != true {
		t.Fatalf("answer %v", got)
	}
	bs, err := b.Branches("staged")
	if err != nil || len(bs) != 2 {
		t.Fatalf("branches %+v, %v", bs, err)
	}
}
