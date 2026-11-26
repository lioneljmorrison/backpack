package mcpserver

import (
	"context"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// maxRequestBytes is the largest request the MCP handler reads. A 10 MiB
// markdown note is JSON in the request, and JSON can double it (every newline
// becomes \n), so the library's 4 MiB default is too small: twice the largest
// attachment, and a MiB for the rest of the request.
const maxRequestBytes = 2*board.MaxAttachment + 1<<20

type attachIn struct {
	in
	Item string `json:"item" jsonschema:"S-n, I-n or a phase number"`
	Name string `json:"name" jsonschema:"ends in .md"`
	Text string `json:"text"`
}

type attachmentIn struct {
	in
	ID string `json:"id" jsonschema:"A-n"`
}

// attachMarkdown attaches a note. The item's kind comes from its id.
func (t tools) attachMarkdown(ctx context.Context, a attachIn) (any, error) {
	kind := board.KindPhase
	switch {
	case strings.HasPrefix(a.Item, "S-"):
		kind = board.KindStory
	case strings.HasPrefix(a.Item, "I-"):
		kind = board.KindIdea
	}
	return t.b.AddAttachment(a.Workspace, actor(ctx), kind, a.Item, a.Name, []byte(a.Text))
}

func (t tools) removeAttachment(ctx context.Context, a attachmentIn) (any, error) {
	return map[string]any{"id": a.ID}, t.b.RemoveAttachment(a.Workspace, actor(ctx), a.ID)
}

// getAttachment answers with the file itself: an image as image content an
// agent can look at, markdown as text. It builds its own result because
// answer only sends text.
func (t tools) getAttachment(_ context.Context, _ *mcp.CallToolRequest, a attachmentIn) (*mcp.CallToolResult, any, error) {
	att, f, err := t.b.OpenAttachment(a.Workspace, a.ID)
	if err != nil {
		return nil, nil, public(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, public(err)
	}
	if strings.HasPrefix(att.Type, "image/") {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: data, MIMEType: att.Type}}}, nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}
