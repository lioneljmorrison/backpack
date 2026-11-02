package mcpserver_test

import "testing"

const severalRepos = "demo has several repos; name the repo: api or web"

func TestAddStoryTakesARepositoryAndNeedsOneWhenThereAreSeveral(t *testing.T) {
	cs := twoRepos(t)
	if got := refused(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "Which", "reasoning": "r"}); got != severalRepos {
		t.Fatalf("no repo: %s", got)
	}
	if got := refused(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "Nowhere", "reasoning": "r", "repo": "ios"}); got != `demo has no repo "ios"` {
		t.Fatalf("unknown repo: %s", got)
	}
	got := use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "Web one", "reasoning": "r", "repo": "web"})
	if got["status"] != "proposed" {
		t.Fatalf("answer %v", got)
	}
	// With several repositories a story read says which it is in.
	if s := readStory(t, cs, got["id"].(string)); s["repo"] != "web" {
		t.Fatalf("story %v", s)
	}
	if s := readStory(t, cs, "W-1"); s["repo"] != "web" {
		t.Fatalf("W-1 %v", s)
	}
}

func TestAddBugFixTakesARepositoryDirectAndThroughBatch(t *testing.T) {
	cs := twoRepos(t)
	fix := func(repo any) map[string]any {
		m := bugFix(map[string]any{"found_in": "W-1", "after": "W-1", "repo": repo})
		return m
	}
	if got := refused(t, cs, "add_bug_fix", fix(nil)); got != severalRepos {
		t.Fatalf("no repo: %s", got)
	}
	direct := use(t, cs, "add_bug_fix", fix("web"))
	if direct["status"] != "backlog" {
		t.Fatalf("direct %v", direct)
	}
	args := fix("api")
	delete(args, "workspace")
	rs := results(t, cs,
		action("add_bug_fix", args),
		action("add_story", map[string]any{"title": "Batch web", "reasoning": "r", "repo": "web"}),
		action("add_story", map[string]any{"title": "Batch none", "reasoning": "r"}))
	r0, _ := rs[0]["result"].(map[string]any)
	r1, _ := rs[1]["result"].(map[string]any)
	if r0["status"] != "backlog" || r1["status"] != "proposed" {
		t.Fatalf("batch %v", rs)
	}
	if rs[2]["ok"] != false || rs[2]["error"] != severalRepos {
		t.Fatalf("a batched add with no repo succeeded: %v", rs[2])
	}
	if s := readStory(t, cs, direct["id"].(string)); s["repo"] != "web" {
		t.Fatalf("direct story %v", s)
	}
	if s := readStory(t, cs, r0["id"].(string)); s["repo"] != "api" {
		t.Fatalf("batched story %v", s)
	}
}

func TestOneRepositoryAnswersCarryNoRepoAndUpdateStoryCannotMoveAStory(t *testing.T) {
	b, cs := connect(t)
	addStory(t, b, "Build A")
	if s := readStory(t, cs, "S-1"); s["repo"] != nil {
		t.Fatalf("one-repository story shows a repo: %v", s)
	}
	got := use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "Named", "reasoning": "r"})
	if got["status"] != "proposed" {
		t.Fatalf("answer %v", got)
	}
	// The repository is fixed: update_story has no such field.
	refused(t, cs, "update_story", map[string]any{"workspace": "demo", "id": "S-1", "repo": "web"})
}

func TestWhatsNextTakesARepositoryAndAnswersItsTopReadyStory(t *testing.T) {
	cs := twoRepos(t)
	for _, repo := range []string{"api", "web"} {
		got := use(t, cs, "add_story", map[string]any{"workspace": "demo", "title": "In " + repo, "reasoning": "r", "repo": repo})
		use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": got["id"], "to": "backlog"})
	}
	// Without a repository the answer is the whole workspace's, as before.
	if s := use(t, cs, "whats_next", map[string]any{"workspace": "demo"})["story"].(map[string]any); s["id"] != "S-1" || s["repo"] != "api" {
		t.Fatalf("whole workspace %v", s)
	}
	if s := use(t, cs, "whats_next", map[string]any{"workspace": "demo", "repo": "web"})["story"].(map[string]any); s["id"] != "S-2" || s["repo"] != "web" {
		t.Fatalf("web %v", s)
	}
	if got := refused(t, cs, "whats_next", map[string]any{"workspace": "demo", "repo": "ios"}); got != `demo has no repo "ios"` {
		t.Fatalf("unknown repo: %s", got)
	}
	use(t, cs, "set_status", map[string]any{"workspace": "demo", "id": "S-2", "to": "in_progress"})
	if got := use(t, cs, "whats_next", map[string]any{"workspace": "demo", "repo": "web"}); got["story"] != nil {
		t.Fatalf("nothing ready in web: %v", got)
	}
}
