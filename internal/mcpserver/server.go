// Package mcpserver serves the board to agents as MCP tools. Each tool calls
// the same board operation as its HTTP route, and answers with one text block
// of compact JSON (not always the HTTP route's shape; see docs/PLAN.md).
package mcpserver

import (
	"context"
	"encoding/json"
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
	s := mcp.NewServer(&mcp.Implementation{Name: "backpack", Version: "0.5.0"}, nil)
	addTools(s, tools{b, x})
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true})
}

// plain is a result a handler wants sent as it is, not as JSON: the one text
// block carries the text itself.
type plain string

// answer turns a board result into a tool result: one text block of compact
// JSON. It never hands the library a value to return, because the library
// would then add a structured copy of the same content.
func answer(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, public(err)
	}
	if p, ok := v.(plain); ok {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(p)}}}, nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, nil, public(err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

// public is what the caller is told of err: a refusal comes back with its
// reason, search being down as such, and anything else is logged and
// reported without detail.
func public(err error) error {
	if errors.Is(err, search.ErrUnavailable) {
		log.Printf("search: %v", err)
		return search.ErrUnavailable
	}
	var be *board.Error
	if errors.As(err, &be) {
		return be
	}
	log.Printf("board: %v", err)
	return errors.New("internal error")
}

// tool registers a typed tool.
func tool[In any](s *mcp.Server, name, description string, h func(context.Context, In) (any, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: description},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			return answer(h(ctx, in))
		})
}
