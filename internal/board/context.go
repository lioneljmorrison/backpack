package board

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
)

// StoryContext is a story's neighbourhood in summaries: enough to start
// work on it without a call for each neighbour. Detail stays with Story and
// Log. Unlike a story read, DependsOn lists every dependency, finished ones
// included, and DependedOnBy every dependent; each carries its status.
type StoryContext struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Phase is the phase the story is in; Idea the idea it came from.
	Phase *ContextPhase `json:"phase,omitempty"`
	Idea  *ContextIdea  `json:"idea,omitempty"`
	// DependsOn and DependedOnBy are in delivery order.
	DependsOn    []ContextLink `json:"depends_on"`
	DependedOnBy []ContextLink `json:"depended_on_by"`
	// FoundIn is where the story turned up; Found is what turned up while
	// building it.
	FoundIn *ContextStory  `json:"found_in,omitempty"`
	Found   []ContextStory `json:"found"`
	// Bugs are the open bugs found in the story or in one of its direct
	// dependencies, in delivery order, at most maxContextBugs.
	Bugs []ContextBug `json:"bugs"`
}

// ContextPhase is a phase with its goals and whether each is met.
type ContextPhase struct {
	Number int           `json:"number"`
	Title  string        `json:"title"`
	Goals  []ContextGoal `json:"goals"`
}

// ContextGoal is a goal of the story's phase.
type ContextGoal struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Met  bool   `json:"met"`
}

// ContextIdea is the idea a story came from.
type ContextIdea struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ContextStory is a story in one line.
type ContextStory struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// ContextLink is a dependency or dependent: the story in one line and its
// last decisions, oldest first.
type ContextLink struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Status    string            `json:"status"`
	Decisions []ContextDecision `json:"decisions"`
}

// ContextDecision is a decision from a log. Corrected is set when a later
// correction entry points at it.
type ContextDecision struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Reason    string `json:"reason"`
	Corrected bool   `json:"corrected,omitempty"`
}

// ContextBug is an open bug and the story it was found in.
type ContextBug struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	FoundIn string `json:"found_in"`
}

const (
	maxContextBugs      = 5
	maxContextDecisions = 3
)

// Context reads a story's neighbourhood in one query. Dependencies are read
// from the DEPENDS_ON links directly, so finished ones are included.
func (b *Board) Context(ws, id string) (StoryContext, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return StoryContext{}, err
	}
	res, err := g.ROQuery(`
		MATCH (s:Story {id: $id})
		OPTIONAL MATCH (p:Phase)-[:CONTAINS]->(s)
		OPTIONAL MATCH (p)-[:HAS_GOAL]->(g:Goal)
		WITH s, p, collect(CASE WHEN g IS NULL THEN null ELSE {id: g.id, text: g.text, met: g.met_at IS NOT NULL} END) AS goals
		OPTIONAL MATCH (i:Idea)-[:BECAME]->(s)
		WITH s, p, goals, collect(CASE WHEN i IS NULL THEN null ELSE {id: i.id, title: i.title} END) AS ideas
		OPTIONAL MATCH (s)-[:FOUND_IN]->(fi:Story)
		WITH s, p, goals, ideas, collect(CASE WHEN fi IS NULL THEN null ELSE {id: fi.id, title: fi.title, status: fi.status} END) AS foundIn
		OPTIONAL MATCH (fd:Story)-[:FOUND_IN]->(s)
		WITH s, p, goals, ideas, foundIn, collect(CASE WHEN fd IS NULL THEN null ELSE {id: fd.id, title: fd.title, status: fd.status} END) AS found
		OPTIONAL MATCH (s)-[:DEPENDS_ON]->(t:Story)
		OPTIONAL MATCH (t)-[:LOGGED]->(e:LogEntry {kind: 'decision'})
		OPTIONAL MATCH (c:LogEntry)-[:CORRECTS]->(e)
		WITH s, p, goals, ideas, foundIn, found, t, e, count(c) > 0 AS corrected
		ORDER BY toInteger(substring(e.id, 2)) DESC
		WITH s, p, goals, ideas, foundIn, found, t,
			collect(CASE WHEN e IS NULL THEN null ELSE {id: e.id, text: e.text, reason: e.reason, corrected: corrected} END)[0..$decisions] AS ds
		WITH s, p, goals, ideas, foundIn, found,
			collect(CASE WHEN t IS NULL THEN null ELSE {id: t.id, title: t.title, status: t.status, rank: t.rank, decisions: ds} END) AS deps
		OPTIONAL MATCH (u:Story)-[:DEPENDS_ON]->(s)
		OPTIONAL MATCH (u)-[:LOGGED]->(e:LogEntry {kind: 'decision'})
		OPTIONAL MATCH (c:LogEntry)-[:CORRECTS]->(e)
		WITH s, p, goals, ideas, foundIn, found, deps, u, e, count(c) > 0 AS corrected
		ORDER BY toInteger(substring(e.id, 2)) DESC
		WITH s, p, goals, ideas, foundIn, found, deps, u,
			collect(CASE WHEN e IS NULL THEN null ELSE {id: e.id, text: e.text, reason: e.reason, corrected: corrected} END)[0..$decisions] AS ds
		WITH s, p, goals, ideas, foundIn, found, deps,
			collect(CASE WHEN u IS NULL THEN null ELSE {id: u.id, title: u.title, status: u.status, rank: u.rank, decisions: ds} END) AS users
		OPTIONAL MATCH (b:Story)-[:FOUND_IN]->(x:Story)
			WHERE b.kind = 'bug' AND NOT b.status IN $done AND (x.id = s.id OR x.id IN [d IN deps | d.id])
		WITH s, p, goals, ideas, foundIn, found, deps, users, b, x
		ORDER BY b.rank
		WITH s, p, goals, ideas, foundIn, found, deps, users,
			collect(CASE WHEN b IS NULL THEN null ELSE {id: b.id, title: b.title, status: b.status, found_in: x.id} END)[0..$bugs] AS bugs
		RETURN s.id AS id, s.title AS title, s.status AS status, p.number AS number, p.title AS phase_title,
			goals, ideas, foundIn, found, deps, users, bugs`,
		map[string]any{"id": id, "done": append(slices.Clone(complete), Closed),
			"decisions": maxContextDecisions, "bugs": maxContextBugs}, nil)
	if err != nil {
		return StoryContext{}, fmt.Errorf("read context: %w", err)
	}
	if !res.Next() {
		return StoryContext{}, refuse(NotFound, "no story %q", id)
	}
	r := res.Record()
	c := StoryContext{ID: str(r, "id"), Title: str(r, "title"), Status: str(r, "status"),
		DependsOn: links(maps(r, "deps")), DependedOnBy: links(maps(r, "users")),
		Found: []ContextStory{}, Bugs: []ContextBug{}}
	if n := num(r, "number"); n != 0 {
		c.Phase = &ContextPhase{Number: n, Title: str(r, "phase_title"), Goals: []ContextGoal{}}
		for _, m := range maps(r, "goals") {
			c.Phase.Goals = append(c.Phase.Goals, ContextGoal{ID: field(m, "id"), Text: field(m, "text"), Met: m["met"] == true})
		}
		slices.SortFunc(c.Phase.Goals, func(a, b ContextGoal) int { return seq(a.ID) - seq(b.ID) })
	}
	if ms := maps(r, "ideas"); len(ms) > 0 {
		slices.SortFunc(ms, func(a, b map[string]any) int { return seq(field(a, "id")) - seq(field(b, "id")) })
		c.Idea = &ContextIdea{ID: field(ms[0], "id"), Title: field(ms[0], "title")}
	}
	if ms := maps(r, "foundIn"); len(ms) > 0 {
		c.FoundIn = &ContextStory{ID: field(ms[0], "id"), Title: field(ms[0], "title"), Status: field(ms[0], "status")}
	}
	for _, m := range maps(r, "found") {
		c.Found = append(c.Found, ContextStory{ID: field(m, "id"), Title: field(m, "title"), Status: field(m, "status")})
	}
	slices.SortFunc(c.Found, func(a, b ContextStory) int { return seq(a.ID) - seq(b.ID) })
	for _, m := range maps(r, "bugs") {
		c.Bugs = append(c.Bugs, ContextBug{ID: field(m, "id"), Title: field(m, "title"), Status: field(m, "status"), FoundIn: field(m, "found_in")})
	}
	return c, nil
}

// maps reads a column holding a list of maps.
func maps(r *falkordb.Record, key string) []map[string]any {
	v, _ := r.Get(key)
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func field(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// links turns dependency maps into links in delivery order, each with its
// decisions oldest first.
func links(ms []map[string]any) []ContextLink {
	slices.SortFunc(ms, func(a, b map[string]any) int { return strings.Compare(field(a, "rank"), field(b, "rank")) })
	out := make([]ContextLink, 0, len(ms))
	for _, m := range ms {
		l := ContextLink{ID: field(m, "id"), Title: field(m, "title"), Status: field(m, "status"), Decisions: []ContextDecision{}}
		raw, _ := m["decisions"].([]any)
		for _, x := range raw {
			if d, ok := x.(map[string]any); ok {
				l.Decisions = append(l.Decisions, ContextDecision{ID: field(d, "id"), Text: field(d, "text"),
					Reason: field(d, "reason"), Corrected: d["corrected"] == true})
			}
		}
		slices.SortFunc(l.Decisions, func(a, b ContextDecision) int { return seq(a.ID) - seq(b.ID) })
		out = append(out, l)
	}
	return out
}
