package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/lioneljmorrison/backpack/internal/board"
)

type actionsIn struct {
	Name string `json:"name,omitempty" jsonschema:"an action from the list, for its arguments"`
}

// actions answers from the on-request actions' own registered definitions,
// so what it says cannot drift from what a batch accepts. It never runs one.
func actions(r *registry) func(context.Context, actionsIn) (any, error) {
	return func(_ context.Context, a actionsIn) (any, error) {
		if a.Name == "" {
			var b strings.Builder
			for _, q := range r.later {
				fmt.Fprintf(&b, "%s: %s\n", q.name, summary(q.description))
			}
			b.WriteString("Run one with batch (tool and args). Name one here for its arguments.")
			return plain(b.String()), nil
		}
		for _, q := range r.later {
			if q.name == a.Name {
				return plain(detail(q)), nil
			}
		}
		if _, ok := r.steps[a.Name]; ok {
			return nil, &board.Error{Kind: board.Invalid, Msg: fmt.Sprintf("%s is not an on-request action: it is a tool, called directly or run with batch", a.Name)}
		}
		return nil, &board.Error{Kind: board.Invalid, Msg: fmt.Sprintf("%q is not an action: call actions with no name to list them", a.Name)}
	}
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
	fmt.Fprintf(&b, "%s: %s\nRun it with batch. Arguments (? is optional):\n", q.name, q.description)
	required := map[string]bool{}
	for _, n := range q.schema.Required {
		required[n] = true
	}
	for _, pass := range []bool{true, false} {
		for _, n := range q.schema.PropertyOrder {
			p := q.schema.Properties[n]
			if n == "workspace" || p == nil || required[n] != pass {
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
