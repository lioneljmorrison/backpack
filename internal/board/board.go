package board

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/falkor"
)

// migrations for every workspace graph. Append only. Story.rank is not
// indexed: FalkorDB 4.20 answers a range comparison on an indexed string
// property (rank > $r) with no rows; Idea.rank likewise.
var migrations = []string{
	"CREATE INDEX FOR (s:Story) ON (s.id)",
	"CREATE INDEX FOR (p:Phase) ON (p.number)",
	"CREATE INDEX FOR (g:Goal) ON (g.id)",
	"CREATE INDEX FOR (l:LogEntry) ON (l.id)",
	"CREATE INDEX FOR (i:Idea) ON (i.id)",
	// Ideas lose their approval step and gain context: notes and any
	// reasoning become the context, proposed ideas become open.
	`MATCH (i:Idea) SET
		i.context = CASE
			WHEN i.reasoning IS NULL THEN coalesce(i.notes, '')
			WHEN coalesce(i.notes, '') = '' THEN i.reasoning
			ELSE i.notes + '\n\n' + i.reasoning END,
		i.state = CASE i.state WHEN 'proposed' THEN 'open' ELSE i.state END,
		i.notes = NULL, i.reasoning = NULL, i.approved_by = NULL, i.approved_at = NULL`,
	// Branches replace the release-stage flag: a flag set gives dev merging
	// into main, otherwise main alone. MERGE makes a repeat harmless. A graph
	// with no Workspace node yet is seeded by CreateWorkspace instead.
	`MATCH (w:Workspace)
		WITH w, coalesce(w.has_release_stage, false) AS release
		MERGE (m:Branch {name: 'main'})
			ON CREATE SET m.created_by = 'migration', m.created_at = toString(localdatetime()) + 'Z'
		FOREACH (_ IN CASE WHEN release THEN [1] ELSE [] END |
			MERGE (d:Branch {name: 'dev'})
				ON CREATE SET d.into = 'main', d.created_by = 'migration', d.created_at = toString(localdatetime()) + 'Z')
		SET w.has_release_stage = NULL`,
}

// Kind says why the board refused a request.
type Kind int

const (
	Invalid  Kind = iota + 1 // the request itself is malformed
	NotFound                 // a workspace, phase or story does not exist
	Conflict                 // the change would break a board rule
)

// Error is a refusal, with a reason fit to show the caller.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func refuse(k Kind, format string, args ...any) error {
	return &Error{Kind: k, Msg: fmt.Sprintf(format, args...)}
}

// Board is every workspace: a registry in the system graph, and one graph
// per workspace named by its slug after a fixed prefix.
type Board struct {
	db       *falkordb.FalkorDB
	system   *falkordb.Graph
	prefix   string
	onChange func(Change)
}

// New opens the board and brings every registered workspace's graph up to
// the current migration.
func New(db *falkordb.FalkorDB, systemGraph, workspacePrefix string) (*Board, error) {
	b := &Board{db: db, system: db.SelectGraph(systemGraph), prefix: workspacePrefix}
	// A read-only query on a graph that does not exist yet is an error.
	if _, err := b.system.Query("RETURN 1", nil, nil); err != nil {
		return nil, fmt.Errorf("open system graph: %w", err)
	}
	ws, err := b.Workspaces()
	if err != nil {
		return nil, err
	}
	for _, w := range ws {
		if err := falkor.Migrate(b.graph(w.Slug), migrations); err != nil {
			return nil, fmt.Errorf("workspace %s: %w", w.Slug, err)
		}
	}
	return b, nil
}

func (b *Board) graph(slug string) *falkordb.Graph {
	return b.db.SelectGraph(b.prefix + slug)
}

// workspace returns the graph of a registered workspace.
func (b *Board) workspace(slug string) (*falkordb.Graph, error) {
	res, err := b.system.ROQuery("MATCH (e:WorkspaceEntry {slug: $slug}) RETURN e.slug",
		map[string]any{"slug": slug}, nil)
	if err != nil {
		return nil, fmt.Errorf("look up workspace: %w", err)
	}
	if !res.Next() {
		return nil, refuse(NotFound, "no workspace %q", slug)
	}
	return b.graph(slug), nil
}

// plainText refuses text that would not be stored exactly. The FalkorDB
// client inlines values into the query as Go-quoted literals, and FalkorDB
// does not read Go's \x and \u escapes. Only newlines, carriage returns and
// tabs, among the characters Go escapes, come back unchanged.
func plainText(fields ...string) error {
	for _, f := range fields {
		if !utf8.ValidString(f) || strings.ContainsFunc(f, escaped) {
			return refuse(Invalid, "text cannot contain control or invisible formatting characters")
		}
	}
	return nil
}

func escaped(r rune) bool {
	return !strconv.IsPrint(r) && r != '\n' && r != '\r' && r != '\t'
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// str reads an optional string column; a null reads as "".
func str(r *falkordb.Record, key string) string {
	v, _ := r.Get(key)
	s, _ := v.(string)
	return s
}

func num(r *falkordb.Record, key string) int {
	v, _ := r.Get(key)
	n, _ := v.(int64)
	return int(n)
}

func flag(r *falkordb.Record, key string) bool {
	v, _ := r.Get(key)
	b, _ := v.(bool)
	return b
}

func strs(r *falkordb.Record, key string) []string {
	v, _ := r.Get(key)
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
