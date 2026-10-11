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
	"github.com/lioneljmorrison/backpack/internal/search"
)

// New returns the MCP handler, searching with x, the index of b. It keeps no
// sessions: every call carries its own token, checked before it arrives here.
func New(b *board.Board, x *search.Index) http.Handler {
	s := mcp.NewServer(&mcp.Implementation{Name: "backpack", Version: "0.4.0"}, nil)
	addTools(s, tools{b, x})
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true})
}

// answer turns a board result into a tool result: a refusal comes back with
// its reason, search being down as such, and anything else is logged and
// reported without detail.
func answer(v any, err error) (*mcp.CallToolResult, any, error) {
	if err == nil {
		return nil, v, nil
	}
	if errors.Is(err, search.ErrUnavailable) {
		log.Printf("search: %v", err)
		return nil, nil, search.ErrUnavailable
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
