package board_test

import (
	"errors"
	"testing"

	"github.com/FalkorDB/falkordb-go/v2"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

const actor = "tester"

func newBoard(t *testing.T) *board.Board {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatalf("new board: %v", err)
	}
	return b
}

// newWorkspace returns a board with one empty workspace, "demo".
func newWorkspace(t *testing.T) *board.Board {
	t.Helper()
	b := newBoard(t)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	return b
}

// newWorkspaceGraph is newWorkspace with direct access to the workspace's graph.
func newWorkspaceGraph(t *testing.T) (*board.Board, *falkordb.Graph) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	return b, db.SelectGraph(prefix + "ws:demo")
}

func wantKind(t *testing.T, err error, want board.Kind) {
	t.Helper()
	var be *board.Error
	if !errors.As(err, &be) || be.Kind != want {
		t.Fatalf("error = %v, want a board error of kind %v", err, want)
	}
}
