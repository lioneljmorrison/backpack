package main

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/search"
	"github.com/lioneljmorrison/backpack/internal/search/typesensetest"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestUsageErrorsExitWithTwo(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"bogus"},
		{"token"},
		{"token", "create"},
		{"token", "revoke"},
		{"token", "rotate", "--actor", "x"},
		{"reindex", "--bogus"},
		{"reindex", "extra"},
		{"join"},
		{"join", "--into", "demo", "--name", "Demo", "demo-api", "demo-web"},
		{"join", "--into", "demo", "--actor", "tester", "demo-api", "demo-web"},
		{"join", "--name", "Demo", "--actor", "tester", "demo-api", "demo-web"},
		{"join", "--into", "demo", "--name", "Demo", "--actor", "tester", "demo-api"},
		{"join", "--bogus"},
		{"join", "--abort"},
		{"join", "--abort", "--apply", "--into", "demo"},
		{"join", "--abort", "--into", "demo", "demo-api"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, env(nil), &out, &errOut); code != 2 {
			t.Errorf("run(%q) = %d, want 2", args, code)
		}
		if errOut.Len() == 0 {
			t.Errorf("run(%q) printed no usage", args)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	c := loadConfig(env(nil))
	if c.Addr != ":8780" || c.FalkorURL != "falkor://localhost:6379" ||
		c.TypesenseURL != "http://localhost:8108" || c.SystemGraph != "backpack" ||
		c.WorkspacePrefix != "ws:" || c.CollectionPrefix != "ws_" {
		t.Fatalf("defaults = %+v", c)
	}
}

func TestConfigReadsTheEnvironment(t *testing.T) {
	c := loadConfig(env(map[string]string{
		"BACKPACK_ADDR":              ":9000",
		"BACKPACK_FALKORDB_URL":      "falkor://db:6379",
		"BACKPACK_TYPESENSE_URL":     "http://ts:8108",
		"BACKPACK_TYPESENSE_API_KEY": "k",
		"BACKPACK_SYSTEM_GRAPH":      "other",
	}))
	if c.Addr != ":9000" || c.FalkorURL != "falkor://db:6379" ||
		c.TypesenseURL != "http://ts:8108" || c.TypesenseKey != "k" || c.SystemGraph != "other" {
		t.Fatalf("config = %+v", c)
	}
}

func TestTokenCreatePrintsAWorkingTokenAndRevokeDisablesIt(t *testing.T) {
	url := falkortest.URL(t)
	g := falkortest.Graph(t) // only for its unique name and cleanup
	e := env(map[string]string{"BACKPACK_FALKORDB_URL": url, "BACKPACK_SYSTEM_GRAPH": g.Id})

	var out, errOut bytes.Buffer
	if code := run([]string{"token", "create", "--actor", "lionel"}, e, &out, &errOut); code != 0 {
		t.Fatalf("create exit %d: %s", code, errOut.String())
	}
	tok := strings.TrimSpace(out.String())

	db, _ := falkor.Connect(url)
	defer db.Conn.Close()
	store, err := auth.NewStore(db.SelectGraph(g.Id))
	if err != nil {
		t.Fatal(err)
	}
	if actor, ok, _ := store.ActorFor(tok); !ok || actor != "lionel" {
		t.Fatalf("printed token %q does not identify lionel", tok)
	}

	out.Reset()
	if code := run([]string{"token", "revoke", "--actor", "lionel"}, e, &out, &errOut); code != 0 {
		t.Fatalf("revoke exit %d: %s", code, errOut.String())
	}
	if _, ok, _ := store.ActorFor(tok); ok {
		t.Fatal("token still works after revoke")
	}
	if !strings.Contains(out.String(), "1") {
		t.Fatalf("revoke output %q does not say how many tokens", out.String())
	}
}

// reindexed returns a config on throwaway graphs and collections, and its
// board, with workspace "demo" holding one story that is not yet indexed.
func reindexed(t *testing.T) (config, *board.Board) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	url, key, cprefix := typesensetest.Prefix(t)
	cfg := config{FalkorURL: falkortest.URL(t), TypesenseURL: url, TypesenseKey: key,
		SystemGraph: prefix + "system", WorkspacePrefix: prefix + "ws:", CollectionPrefix: cprefix}
	b, err := board.New(db, cfg.SystemGraph, cfg.WorkspacePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddStory("demo", "tester", board.NewStory{Title: "Export invoices"}); err != nil {
		t.Fatal(err)
	}
	return cfg, b
}

func TestReindexRebuildsEveryWorkspaceFromTheBoard(t *testing.T) {
	cfg, b := reindexed(t)
	var out, errOut bytes.Buffer
	if code := dispatch(cfg, []string{"reindex"}, &out, &errOut); code != 0 {
		t.Fatalf("reindex exit %d: %s", code, errOut.String())
	}
	x := search.New(cfg.TypesenseURL, cfg.TypesenseKey, cfg.CollectionPrefix, b)
	got, err := x.Search(context.Background(), "demo", "invoices")
	if err != nil || len(got) != 1 || got[0].ID != "S-1" {
		t.Fatalf("after reindex, search found %+v, %v", got, err)
	}
	if !strings.Contains(out.String(), "1 workspace") {
		t.Fatalf("reindex printed %q", out.String())
	}
}

func TestReindexOfAnUnknownWorkspaceFails(t *testing.T) {
	cfg, _ := reindexed(t)
	var out, errOut bytes.Buffer
	if code := dispatch(cfg, []string{"reindex", "--workspace", "nope"}, &out, &errOut); code != 1 {
		t.Fatalf("reindex exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "nope") {
		t.Fatalf("reindex printed %q", errOut.String())
	}
}

// joinable returns a config on throwaway graphs with workspaces demo-api and
// demo-web, each holding one story. The names are invented.
func joinable(t *testing.T) config {
	t.Helper()
	cfg, b := reindexed(t)
	for _, s := range []string{"demo-api", "demo-web"} {
		if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: s, Name: s}); err != nil {
			t.Fatal(err)
		}
		if _, err := b.AddStory(s, "tester", board.NewStory{Title: "Story of " + s}); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func TestJoinPrintsAPreviewAndWritesNothing(t *testing.T) {
	cfg := joinable(t)
	var out, errOut bytes.Buffer
	args := []string{"join", "--into", "demo-both", "--name", "Demo Both", "--actor", "tester", "demo-api", "demo-web"}
	if code := dispatch(cfg, args, &out, &errOut); code != 0 {
		t.Fatalf("join exit %d: %s", code, errOut.String())
	}
	for _, want := range []string{"join demo-api, demo-web into demo-both", "preview only", "stories S-1 -> S-2", "top of the merged backlog: S-1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	db, _ := falkor.Connect(cfg.FalkorURL)
	defer db.Conn.Close()
	names, _ := db.ListGraphs()
	for _, n := range names {
		if n == cfg.WorkspacePrefix+"demo-both" {
			t.Fatal("the preview created the target graph")
		}
	}
}

func TestAJoinTheBoardRefusesExitsWithOne(t *testing.T) {
	cfg := joinable(t)
	var out, errOut bytes.Buffer
	args := []string{"join", "--into", "demo-both", "--name", "Demo", "--actor", "tester", "demo-api", "nowhere"}
	if code := dispatch(cfg, args, &out, &errOut); code != 1 {
		t.Fatalf("join exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), `no workspace "nowhere"`) || out.Len() != 0 {
		t.Fatalf("stdout %q stderr %q", out.String(), errOut.String())
	}
}

func TestJoinApplyJoinsAndAbortRefusesAFinishedJoin(t *testing.T) {
	cfg := joinable(t)
	var out, errOut bytes.Buffer
	args := []string{"join", "--apply", "--into", "demo-both", "--name", "Demo Both", "--actor", "tester", "demo-api", "demo-web"}
	if code := dispatch(cfg, args, &out, &errOut); code != 0 {
		t.Fatalf("join exit %d: %s", code, errOut.String())
	}
	for _, want := range []string{"joined demo-api, demo-web into demo-both", "stories S-1 -> S-2", "search: rebuilt demo-both, dropped demo-api, demo-web"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "preview only") {
		t.Errorf("an applied join says it is a preview:\n%s", out.String())
	}
	if strings.Contains(out.String(), "make reindex") {
		t.Errorf("a join that rebuilt search still says to reindex:\n%s", out.String())
	}
	db, _ := falkor.Connect(cfg.FalkorURL)
	defer db.Conn.Close()
	b, err := board.New(db, cfg.SystemGraph, cfg.WorkspacePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := b.Stories("demo-both"); err != nil || len(st) != 2 {
		t.Fatalf("stories %d %v", len(st), err)
	}
	out.Reset()
	errOut.Reset()
	if code := dispatch(cfg, []string{"join", "--abort", "--into", "demo-both"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "nothing to abort") {
		t.Fatalf("abort of a finished join: exit %d %q", code, errOut.String())
	}
}

func TestJoinAbortUndoesAFrozenJoin(t *testing.T) {
	cfg := joinable(t)
	db, _ := falkor.Connect(cfg.FalkorURL)
	defer db.Conn.Close()
	falkortest.MarkEntry(t, db, cfg.SystemGraph, "demo-api", map[string]any{"joining": "demo-both"})
	falkortest.MarkEntry(t, db, cfg.SystemGraph, "demo-web", map[string]any{"joining": "demo-both"})
	var out, errOut bytes.Buffer
	if code := dispatch(cfg, []string{"join", "--abort", "--into", "demo-both"}, &out, &errOut); code != 0 {
		t.Fatalf("abort exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "aborted the join into demo-both") {
		t.Fatalf("output %q", out.String())
	}
	b, err := board.New(db, cfg.SystemGraph, cfg.WorkspacePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddStory("demo-api", "tester", board.NewStory{Title: "Writable again"}); err != nil {
		t.Fatalf("the source is still frozen: %v", err)
	}
}

// indexed builds the search index of the given workspaces of cfg's board.
func indexed(t *testing.T, cfg config, b *board.Board, workspaces ...string) *search.Index {
	t.Helper()
	x := search.New(cfg.TypesenseURL, cfg.TypesenseKey, cfg.CollectionPrefix, b)
	for _, w := range workspaces {
		if err := x.Rebuild(context.Background(), w); err != nil {
			t.Fatal(err)
		}
	}
	return x
}

func collectionStatus(cfg config, ws string) int {
	return typesensetest.Do(cfg.TypesenseURL, cfg.TypesenseKey, http.MethodGet, "/collections/"+cfg.CollectionPrefix+ws, "")
}

func TestJoinApplyMakesTheNewWorkspaceSearchableAndDropsTheOldIndexes(t *testing.T) {
	cfg := joinable(t)
	db, _ := falkor.Connect(cfg.FalkorURL)
	defer db.Conn.Close()
	b, err := board.New(db, cfg.SystemGraph, cfg.WorkspacePrefix)
	if err != nil {
		t.Fatal(err)
	}
	x := indexed(t, cfg, b, "demo", "demo-api", "demo-web")
	var out, errOut bytes.Buffer
	args := []string{"join", "--apply", "--into", "demo-both", "--name", "Demo Both", "--actor", "tester", "demo-api", "demo-web"}
	if code := dispatch(cfg, args, &out, &errOut); code != 0 {
		t.Fatalf("join exit %d: %s", code, errOut.String())
	}
	// The join note names the source: only demo-web's story (S-2 now) has it.
	got, err := x.Search(context.Background(), "demo-both", "Joined into demo-both from demo-web")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range got {
		found = found || r.ID == "S-2"
	}
	if !found {
		t.Fatalf("search of the new workspace found %+v, want S-2", got)
	}
	for _, ws := range []string{"demo-api", "demo-web"} {
		if code := collectionStatus(cfg, ws); code != http.StatusNotFound {
			t.Errorf("source %s's collection answers %d, want 404", ws, code)
		}
	}
	if code := collectionStatus(cfg, "demo"); code != http.StatusOK {
		t.Errorf("an unrelated workspace's collection answers %d, want 200", code)
	}
}

func TestJoinApplySucceedsWhenSearchIsUnreachable(t *testing.T) {
	cfg := joinable(t)
	cfg.TypesenseURL = "http://127.0.0.1:1" // nothing listens here
	var out, errOut bytes.Buffer
	args := []string{"join", "--apply", "--into", "demo-both", "--name", "Demo Both", "--actor", "tester", "demo-api", "demo-web"}
	if code := dispatch(cfg, args, &out, &errOut); code != 0 {
		t.Fatalf("join exit %d, want 0: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "search: run make reindex") {
		t.Errorf("output lacks the reindex hint:\n%s", out.String())
	}
	db, _ := falkor.Connect(cfg.FalkorURL)
	defer db.Conn.Close()
	b, err := board.New(db, cfg.SystemGraph, cfg.WorkspacePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := b.Stories("demo-both"); err != nil || len(st) != 2 {
		t.Fatalf("stories %d %v", len(st), err)
	}
}
