package board_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func addEntry(t *testing.T, b *board.Board, id string, e board.NewLogEntry) board.LogEntry {
	t.Helper()
	got, err := b.AddLogEntry("demo", actor, id, e)
	if err != nil {
		t.Fatalf("log %s on %s: %v", e.Kind, id, err)
	}
	return got
}

func TestEachKindOfEntryIsKeptInOrderWithItsAuthor(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addEntry(t, b, "S-1", board.NewLogEntry{Kind: "note", Text: "looked at it"})
	addEntry(t, b, "S-1", board.NewLogEntry{Kind: "decision", Text: "use a queue", Reason: "bursts"})
	addEntry(t, b, "S-1", board.NewLogEntry{Kind: "commit", Text: "first cut", Commit: "abc1234"})
	addEntry(t, b, "S-1", board.NewLogEntry{Kind: "pr", PRURL: "https://example.com/pr/3"})
	log := readLog(t, b, "S-1")
	if len(log) != 4 {
		t.Fatalf("log %+v", log)
	}
	if log[0].Kind != "note" || log[0].Text != "looked at it" || log[0].Actor != actor || log[0].At == "" ||
		log[1].Reason != "bursts" || log[2].Commit != "abc1234" || log[3].PRURL != "https://example.com/pr/3" {
		t.Fatalf("log %+v", log)
	}
}

func TestEntriesAreNumberedAcrossTheWorkspace(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	walk(t, b, "S-1", "in_progress")
	e := addEntry(t, b, "S-2", board.NewLogEntry{Kind: "note", Text: "x"})
	if e.ID != "L-2" {
		t.Fatalf("entry %+v", e)
	}
}

func TestACorrectionPointsToTheEntryItCorrects(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	wrong := addEntry(t, b, "S-1", board.NewLogEntry{Kind: "commit", Commit: "abc1234"})
	fix := addEntry(t, b, "S-1", board.NewLogEntry{Kind: "correction", Text: "that commit was def5678", Corrects: wrong.ID})
	log := readLog(t, b, "S-1")
	if log[0].Commit != "abc1234" || len(log[0].CorrectedBy) != 1 || log[0].CorrectedBy[0] != fix.ID || log[1].Corrects != wrong.ID {
		t.Fatalf("log %+v", log)
	}
}

func TestACorrectionMustCorrectAnEntryOfTheSameStory(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	addStory(t, b, board.NewStory{Title: "B"})
	other := addEntry(t, b, "S-2", board.NewLogEntry{Kind: "note", Text: "x"})
	for _, corrects := range []string{other.ID, "L-99"} {
		_, err := b.AddLogEntry("demo", actor, "S-1", board.NewLogEntry{Kind: "correction", Text: "y", Corrects: corrects})
		wantKind(t, err, board.NotFound)
	}
	if got := readLog(t, b, "S-1"); len(got) != 0 {
		t.Fatalf("a refused correction was logged: %+v", got)
	}
}

func TestEntriesMissingTheirDetailsOrOfAnUnknownKindAreRefused(t *testing.T) {
	b := newWorkspace(t)
	addStory(t, b, board.NewStory{Title: "A"})
	for _, e := range []board.NewLogEntry{
		{Kind: "status", Text: "moved"},
		{Kind: "gossip", Text: "x"},
		{Kind: "note"},
		{Kind: "decision", Text: "use a queue"},
		{Kind: "commit", Text: "x"},
		{Kind: "pr", Text: "x"},
		{Kind: "correction", Text: "x"},
		{Kind: "correction", Corrects: "L-1"},
		{Kind: "note", Text: "x", Commit: "abc"},
		{Kind: "note", Text: "bad\x07text"},
	} {
		_, err := b.AddLogEntry("demo", actor, "S-1", e)
		wantKind(t, err, board.Invalid)
	}
}

func TestLoggingOnAMissingStoryIsNotFound(t *testing.T) {
	b := newWorkspace(t)
	_, err := b.AddLogEntry("demo", actor, "S-9", board.NewLogEntry{Kind: "note", Text: "x"})
	wantKind(t, err, board.NotFound)
	_, err = b.Log("demo", "S-9")
	wantKind(t, err, board.NotFound)
}

func TestAStoryCreatedBeforeLogsExistedCanStillBeLogged(t *testing.T) {
	b, g := newWorkspaceGraph(t)
	addStory(t, b, board.NewStory{Title: "A"})
	if _, err := g.Query("MATCH (w:Workspace) SET w.next_log = null, w.has_release_stage = null", nil, nil); err != nil {
		t.Fatal(err)
	}
	if e := addEntry(t, b, "S-1", board.NewLogEntry{Kind: "note", Text: "x"}); e.ID != "L-1" {
		t.Fatalf("entry %+v", e)
	}
	walk(t, b, "S-1", "in_progress", "in_review", "merged", "deployed")
}
