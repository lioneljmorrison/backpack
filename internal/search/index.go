// Package search keeps a Typesense index of each workspace and answers
// searches by keyword and by meaning. The index is only ever rebuilt from the
// board; nothing reads it to make a board decision.
package search

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// ErrUnavailable means Typesense could not answer.
var ErrUnavailable = errors.New("search unavailable")

const (
	// model embeds text inside Typesense itself, so tracked content never
	// leaves the host.
	model = "ts/all-MiniLM-L12-v2"
	// maxDistance leaves out matches by meaning weaker than this. Measured on
	// invented fixtures: paraphrases came in under 0.72, unrelated items at
	// 0.83 and above. Keyword matches are kept whatever their distance.
	maxDistance = 0.75
	maxResults  = 10
	// reportTimeout bounds syncing one change, embedding included.
	reportTimeout = 10 * time.Second
)

// Result is one match.
type Result struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Title string `json:"title"`
	// State is a story's status or an idea's state; a phase has none. A merged
	// story's reads "merged:main", with the branch it reached; a released
	// story's is plain "released".
	State string `json:"state,omitempty"`
}

// doc is one entry in a workspace's collection. Meaning is read from the
// title and summary only: the model reads just the first few hundred words,
// and a long log would drown out what the item is about. Keywords match
// every text field.
type doc struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
	State   string `json:"state"`
}

// schema is a workspace collection's shape. A rebuild always recreates the
// collection, so a change here applies at the next start or reindex.
func schema(name string) map[string]any {
	text := func(n string) map[string]any { return map[string]any{"name": n, "type": "string"} }
	stored := func(n string) map[string]any {
		return map[string]any{"name": n, "type": "string", "index": false, "optional": true}
	}
	return map[string]any{
		"name": name,
		"fields": []map[string]any{
			stored("kind"), stored("ref"), stored("state"),
			text("title"), text("summary"), text("detail"),
			{"name": "embedding", "type": "float[]", "embed": map[string]any{
				"from": []string{"title", "summary"}, "model_config": map[string]any{"model_name": model},
			}},
		},
	}
}

// Index is every workspace's search index: one collection per workspace,
// named by its slug after a fixed prefix.
type Index struct {
	c      client
	b      *board.Board
	prefix string
}

// New returns the index of b on the Typesense at baseURL.
func New(baseURL, key, prefix string, b *board.Board) *Index {
	return &Index{c: client{baseURL: baseURL, key: key}, b: b, prefix: prefix}
}

func (x *Index) collection(ws string) string { return x.prefix + ws }

// Report syncs a change, for the board's OnChange. The board change has
// already been saved, so a failure is only logged: a rebuild repairs it.
func (x *Index) Report(c board.Change) {
	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()
	if err := x.Sync(ctx, c); err != nil {
		log.Printf("search: %s: %v (reindex needed)", strings.TrimSpace(c.Workspace+" "+c.ID), err)
	}
}

// Sync reads the changed item back from the board and replaces its entry.
// A new workspace gets its collection.
func (x *Index) Sync(ctx context.Context, c board.Change) error {
	var d doc
	switch c.Kind {
	case board.KindWorkspace:
		return x.Rebuild(ctx, c.Workspace)
	case board.KindBranch:
		return nil // branches are not searched; the stories that moved have their own notices
	case board.KindStory:
		s, err := x.b.Story(c.Workspace, c.ID)
		if err != nil {
			return err
		}
		entries, err := x.b.Log(c.Workspace, c.ID)
		if err != nil {
			return err
		}
		d = storyDoc(s, entries)
	case board.KindIdea:
		i, err := x.b.Idea(c.Workspace, c.ID)
		if err != nil {
			return err
		}
		d = ideaDoc(i)
	case board.KindPhase:
		ps, err := x.b.Phases(c.Workspace)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(ps, func(p board.Phase) bool { return strconv.Itoa(p.Number) == c.ID })
		if i < 0 {
			return fmt.Errorf("no phase %s", c.ID)
		}
		d = phaseDoc(ps[i])
	default:
		return fmt.Errorf("unknown change kind %q", c.Kind)
	}
	err := x.c.upsert(ctx, x.collection(c.Workspace), d)
	if errors.Is(err, errNoCollection) {
		return fmt.Errorf("%w: %s has no index", ErrUnavailable, c.Workspace)
	}
	return err
}

// Rebuild drops a workspace's collection and refills it from the board.
func (x *Index) Rebuild(ctx context.Context, ws string) error {
	ds, err := x.docs(ws)
	if err != nil {
		return err
	}
	name := x.collection(ws)
	if err := x.c.drop(ctx, name); err != nil {
		return err
	}
	if err := x.c.create(ctx, schema(name)); err != nil {
		return err
	}
	return x.c.importDocs(ctx, name, ds)
}

// Warm makes sure Typesense has the embedding model, by creating and
// dropping a collection that uses it. On a new Typesense volume this
// downloads the model, which can take longer than syncing one change is
// allowed; done at start, before the first workspace exists, it means no
// sync ever waits on it. The name starts with an underscore, which no
// workspace slug can.
func (x *Index) Warm(ctx context.Context) error {
	name := x.prefix + "_warm"
	if err := x.c.create(ctx, schema(name)); err != nil {
		return err
	}
	return x.c.drop(ctx, name)
}

// RebuildAll rebuilds every workspace, going on past one that fails.
func (x *Index) RebuildAll(ctx context.Context) error {
	ws, err := x.b.Workspaces()
	if err != nil {
		return err
	}
	var errs []error
	for _, w := range ws {
		if err := x.Rebuild(ctx, w.Slug); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", w.Slug, err))
		}
	}
	return errors.Join(errs...)
}

// docs is every entry a workspace's collection holds.
func (x *Index) docs(ws string) ([]doc, error) {
	stories, err := x.b.Stories(ws)
	if err != nil {
		return nil, err
	}
	var ds []doc
	for _, s := range stories {
		entries, err := x.b.Log(ws, s.ID)
		if err != nil {
			return nil, err
		}
		ds = append(ds, storyDoc(s, entries))
	}
	ideas, err := x.b.Ideas(ws)
	if err != nil {
		return nil, err
	}
	for _, i := range ideas {
		ds = append(ds, ideaDoc(i))
	}
	phases, err := x.b.Phases(ws)
	if err != nil {
		return nil, err
	}
	for _, p := range phases {
		ds = append(ds, phaseDoc(p))
	}
	return ds, nil
}

// Search finds a workspace's stories, ideas and phases by keyword and by
// meaning, best first.
func (x *Index) Search(ctx context.Context, ws, q string) ([]Result, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, &board.Error{Kind: board.Invalid, Msg: "a search needs a query"}
	}
	all, err := x.b.Workspaces()
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(all, func(w board.Workspace) bool { return w.Slug == ws }) {
		return nil, &board.Error{Kind: board.NotFound, Msg: fmt.Sprintf("no workspace %q", ws)}
	}
	var res struct {
		Hits []struct {
			Document doc `json:"document"`
		} `json:"hits"`
	}
	err = x.c.search(ctx, x.collection(ws), url.Values{
		"q":              {q},
		"query_by":       {"title,summary,detail,embedding"},
		"vector_query":   {fmt.Sprintf("embedding:([], distance_threshold:%v)", maxDistance)},
		"exclude_fields": {"embedding"},
		"per_page":       {strconv.Itoa(maxResults)},
	}, &res)
	if errors.Is(err, errNoCollection) {
		return nil, fmt.Errorf("%w: %s has no index; reindex needed", ErrUnavailable, ws)
	}
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(res.Hits))
	for _, h := range res.Hits {
		d := h.Document
		out = append(out, Result{Kind: d.Kind, ID: d.Ref, Title: d.Title, State: d.State})
	}
	return out, nil
}

func storyDoc(s board.Story, entries []board.LogEntry) doc {
	detail := []string{s.Plan, s.HoldReason, s.Workaround}
	for _, e := range entries {
		detail = append(detail, e.Text, e.Reason)
	}
	state := board.Move{To: s.Status}
	if s.Status == board.Merged {
		state.Into = s.Into // only a merged story is in a branch's column
	}
	return doc{ID: s.ID, Kind: board.KindStory, Ref: s.ID, Title: s.Title,
		Summary: join(s.Description, s.Goal), Detail: join(detail...),
		State: state.String()}
}

func ideaDoc(i board.Idea) doc {
	return doc{ID: i.ID, Kind: board.KindIdea, Ref: i.ID, Title: i.Title,
		Summary: i.Context, Detail: i.DropReason, State: i.State}
}

func phaseDoc(p board.Phase) doc {
	goals := []string{p.Description}
	for _, g := range p.Goals {
		goals = append(goals, g.Text)
	}
	n := strconv.Itoa(p.Number)
	return doc{ID: "P-" + n, Kind: board.KindPhase, Ref: n, Title: p.Title, Summary: join(goals...)}
}

// join keeps the non-empty parts, one per line.
func join(parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), "\n")
}
