// Package mcpserver serves the board to agents as MCP tools. Each tool calls
// the same board operation as its HTTP route, and answers with the same JSON.
package mcpserver

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// New returns the MCP handler. It keeps no sessions: every call carries its
// own token, checked before it arrives here.
func New(b *board.Board) http.Handler {
	s := mcp.NewServer(&mcp.Implementation{Name: "backpack", Version: "0.4.0"}, nil)
	addTools(s, tools{b})
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true})
}

// answer turns a board result into a tool result: a refusal comes back with
// its reason, and anything else is logged and reported without detail.
func answer(v any, err error) (*mcp.CallToolResult, any, error) {
	if err == nil {
		return nil, v, nil
	}
	var be *board.Error
	if errors.As(err, &be) {
		return nil, nil, be
	}
	log.Printf("board: %v", err)
	return nil, nil, errors.New("internal error")
}

// tool registers a typed tool.
func tool[In any](s *mcp.Server, name, description string, h func(context.Context, In) (any, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: description},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			return answer(h(ctx, in))
		})
}
