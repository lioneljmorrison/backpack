package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// maxBatch is the most actions one batch may carry.
const maxBatch = 100

type batchIn struct {
	in
	Actions []batchAction `json:"actions" jsonschema:"changes, in order"`
}

type batchAction struct {
	Tool string         `json:"tool" jsonschema:"a changing tool, such as set_status"`
	Args map[string]any `json:"args,omitempty" jsonschema:"its arguments, without workspace"`
}

// outcome is one action's answer: the tool's own answer, or why it failed.
type outcome struct {
	Tool   string `json:"tool"`
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// step runs one batched action, its arguments including the workspace.
type step func(ctx context.Context, args map[string]any) (any, error)

// write registers a tool that changes the board, and lets a batch run it. A
// batched action is checked against the same schema as a call to the tool,
// so a missing or unknown argument is refused as it would be alone.
func write[In any](s *mcp.Server, steps map[string]step, name, description string, h func(context.Context, In) (any, error)) {
	tool(s, name, description, h)
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	}
	resolved, err := schema.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		panic(err)
	}
	steps[name] = func(ctx context.Context, args map[string]any) (any, error) {
		if err := resolved.Validate(args); err != nil {
			return nil, &board.Error{Kind: board.Invalid, Msg: "arguments: " + err.Error()}
		}
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		var a In
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, &board.Error{Kind: board.Invalid, Msg: "arguments: " + err.Error()}
		}
		return h(ctx, a)
	}
}

// batch runs each action in order through the tool it names. An action that
// fails is answered with its reason and doesn't stop the rest.
func batch(steps map[string]step) func(context.Context, batchIn) (any, error) {
	return func(ctx context.Context, a batchIn) (any, error) {
		if len(a.Actions) == 0 || len(a.Actions) > maxBatch {
			return nil, &board.Error{Kind: board.Invalid, Msg: fmt.Sprintf("a batch has 1 to %d actions", maxBatch)}
		}
		out := make([]outcome, len(a.Actions))
		for i, act := range a.Actions {
			v, err := run(ctx, steps, a.Workspace, act)
			out[i] = outcome{Tool: act.Tool, OK: err == nil, Result: v}
			if err != nil {
				out[i].Result, out[i].Error = nil, public(err).Error()
			}
		}
		return map[string]any{"results": out}, nil
	}
}

func run(ctx context.Context, steps map[string]step, ws string, act batchAction) (any, error) {
	st, ok := steps[act.Tool]
	if !ok {
		return nil, &board.Error{Kind: board.Invalid, Msg: fmt.Sprintf("%q is not a tool that changes the board", act.Tool)}
	}
	if _, ok := act.Args["workspace"]; ok {
		return nil, &board.Error{Kind: board.Invalid, Msg: "the workspace is given once, for the whole batch"}
	}
	args := map[string]any{"workspace": ws}
	maps.Copy(args, act.Args)
	return st(ctx, args)
}
