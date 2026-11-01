package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// actionsIn is the name of an action and, to run a read, its arguments. The
// rule is one line: a name alone describes the action; a name with args (even
// empty) runs a read here. A change is never run here, with args or without.
type actionsIn struct {
	Name string         `json:"name,omitempty" jsonschema:"an action from the list"`
	Args map[string]any `json:"args,omitempty" jsonschema:"runs a read: its arguments, with workspace"`
}

// actions answers from the on-request actions' own registered definitions,
// so what it says cannot drift from what runs. It runs only reads.
func actions(r *registry) func(context.Context, actionsIn) (any, error) {
	return func(ctx context.Context, a actionsIn) (any, error) {
		if a.Name == "" {
			if a.Args != nil {
				return nil, &board.Error{Kind: board.Invalid, Msg: "args need the name of a read"}
			}
			return plain(list(r)), nil
		}
		for _, q := range r.later {
			if q.name != a.Name {
				continue
			}
			switch {
			case a.Args == nil:
				return plain(detail(q)), nil
			case !q.read:
				return nil, &board.Error{Kind: board.Invalid, Msg: fmt.Sprintf("%s changes the board: run it with batch, not here", q.name)}
			}
			return q.run(ctx, a.Args)
		}
		if _, ok := r.steps[a.Name]; ok {
			return nil, &board.Error{Kind: board.Invalid, Msg: fmt.Sprintf("%s is not an on-request action: it is a tool, called directly or run with batch", a.Name)}
		}
		return nil, &board.Error{Kind: board.Invalid, Msg: fmt.Sprintf("%q is not an action: call actions with no name to list them", a.Name)}
	}
}

// list is one line for each on-request action: the reads (run here), then the
// changes (run with batch).
func list(r *registry) string {
	var b strings.Builder
	for _, reads := range []bool{true, false} {
		if reads {
			b.WriteString("Reads (run here: name and args):\n")
		} else {
			b.WriteString("Changes (run with batch):\n")
		}
		for _, q := range r.later {
			if q.read == reads {
				fmt.Fprintf(&b, "%s: %s\n", q.name, summary(q.description))
			}
		}
	}
	b.WriteString("Name alone gives an action's arguments.")
	return b.String()
}

// summary is the first sentence of a description, without what it answers with.
func summary(d string) string {
	if i := strings.Index(d, " Answers with"); i >= 0 {
		d = d[:i]
	}
	if i := strings.Index(d, ". "); i >= 0 {
		d = d[:i]
	}
	return strings.TrimSuffix(d, ".")
}

// detail is an action's description and its arguments, in the tool's own
// order, without the workspace. An optional argument ends in "?".
func detail(q request) string {
	var b strings.Builder
	how := "Run it with batch (tool and args)."
	if q.read {
		how = "Run it here: actions with name and args, workspace included."
	}
	fmt.Fprintf(&b, "%s: %s\n%s Arguments (? is optional):\n", q.name, q.description, how)
	required := map[string]bool{}
	for _, n := range q.schema.Required {
		required[n] = true
	}
	for _, pass := range []bool{true, false} {
		for _, n := range q.schema.PropertyOrder {
			p := q.schema.Properties[n]
			if (n == "workspace" && !q.read) || p == nil || required[n] != pass {
				continue
			}
			mark := "?"
			if pass {
				mark = ""
			}
			fmt.Fprintf(&b, "%s%s: %s", n, mark, typeOf(p))
			if p.Description != "" {
				fmt.Fprintf(&b, ", %s", p.Description)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// typeOf is a property's type, such as string or string[].
func typeOf(p *jsonschema.Schema) string {
	t := p.Type
	for _, x := range p.Types {
		if x != "null" {
			t = x
		}
	}
	if t == "array" && p.Items != nil {
		return typeOf(p.Items) + "[]"
	}
	return t
}
