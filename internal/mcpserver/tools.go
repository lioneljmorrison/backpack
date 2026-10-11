package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
)

type tools struct{ b *board.Board }

// in is what every tool needs: the workspace it acts on.
type in struct {
	Workspace string `json:"workspace" jsonschema:"the workspace's slug, from the project's .backpack.json"`
}

func addTools(s *mcp.Server, t tools) {
	tool(s, "whats_next", "The next story to deliver: the highest-ranked backlog story whose dependencies are all complete, with its description, goal and plan. The story is null when nothing is ready.", t.whatsNext)
	tool(s, "where_are_we", "The current phase and how far along it is, what is in progress and what each story is doing, what is in review, what is on hold and why, and the next three in the backlog.", t.whereAreWe)
}

func (t tools) whatsNext(_ context.Context, a in) (any, error) {
	s, err := t.b.Next(a.Workspace)
	return map[string]any{"story": s}, err
}

func (t tools) whereAreWe(_ context.Context, a in) (any, error) {
	return t.b.Where(a.Workspace)
}
