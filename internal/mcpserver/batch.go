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

// stepFunc runs one batched action, its arguments including the workspace.
type stepFunc func(ctx context.Context, args map[string]any) (any, error)

// registry is the tools being registered: the server they go on, the steps
// a batch can run, and the actions that are on request.
type registry struct {
	s     *mcp.Server
	steps map[string]stepFunc
	later []request
}

// request is an on-request action, not on the menu. Its definition is what
// `actions` answers from. A change runs through batch; a read (run set) runs
// through actions.
type request struct {
	name        string
	description string
	schema      *jsonschema.Schema
	read        bool
	run         stepFunc
}

// write registers a tool that changes the board and is on the menu, and lets
// a batch run it. A batched action is checked against the same schema as a
// call to the tool, so a missing or unknown argument is refused as it would
// be alone.
func write[In any](r *registry, name, description string, h func(context.Context, In) (any, error)) {
	tool(r.s, name, description, h)
	st, _ := checked(h)
	r.steps[name] = st
}

// later registers a change that is on request: it is not on the menu, and
// runs only through batch, with the same schema check, answers and refusals
// as a tool of its own. `actions` describes it.
func later[In any](r *registry, name, description string, h func(context.Context, In) (any, error)) {
	st, schema := checked(h)
	r.steps[name] = st
	r.later = append(r.later, request{name: name, description: description, schema: schema})
}

// laterRead registers a read that is on request: it is not on the menu and a
// batch refuses it. `actions` describes it and runs it, its arguments (the
// workspace included) checked against its own schema.
func laterRead[In any](r *registry, name, description string, h func(context.Context, In) (any, error)) {
	st, schema := checked(h)
	r.later = append(r.later, request{name: name, description: description, schema: schema, read: true, run: st})
}

// checked wraps h so its arguments, workspace included, are checked against
// the schema of In first, and returns that schema.
func checked[In any](h func(context.Context, In) (any, error)) (stepFunc, *jsonschema.Schema) {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	}
	resolved, err := schema.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		panic(err)
	}
	return func(ctx context.Context, args map[string]any) (any, error) {
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
	}, schema
}

// batch runs each action in order through the tool it names. An action that
// fails is answered with its reason and doesn't stop the rest.
func batch(steps map[string]stepFunc) func(context.Context, batchIn) (any, error) {
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

func run(ctx context.Context, steps map[string]stepFunc, ws string, act batchAction) (any, error) {
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
