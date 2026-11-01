package board

import (
	"fmt"
	"slices"
)

// Neighbourhood is what lies near a story, as ids: the story itself, its
// dependencies and dependents, the story it was found in, what was found
// while building it, the open bugs Context lists, and the other stories of
// its phase; the ideas it came from; and its phase (0 for none).
type Neighbourhood struct {
	Stories []string
	Ideas   []string
	Phase   int
}

// Near reads a story's neighbourhood in one query. It is Context's set with
// the phase's other stories added, as ids only.
func (b *Board) Near(ws, id string) (Neighbourhood, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return Neighbourhood{}, err
	}
	res, err := g.ROQuery(`
		MATCH (s:Story {id: $id})
		OPTIONAL MATCH (s)-[:DEPENDS_ON]->(d:Story)
		WITH s, collect(d.id) AS deps
		OPTIONAL MATCH (u:Story)-[:DEPENDS_ON]->(s)
		WITH s, deps, collect(u.id) AS users
		OPTIONAL MATCH (s)-[:FOUND_IN]->(fi:Story)
		WITH s, deps, users, collect(fi.id) AS foundIn
		OPTIONAL MATCH (fd:Story)-[:FOUND_IN]->(s)
		WITH s, deps, users, foundIn, collect(fd.id) AS found
		OPTIONAL MATCH (b:Story)-[:FOUND_IN]->(x:Story)
			WHERE b.kind = 'bug' AND NOT b.status IN $done AND (x.id = s.id OR x.id IN deps)
		WITH s, deps, users, foundIn, found, b
		ORDER BY b.rank
		WITH s, deps, users, foundIn, found, collect(b.id)[0..$bugs] AS bugs
		OPTIONAL MATCH (i:Idea)-[:BECAME]->(s)
		WITH s, deps, users, foundIn, found, bugs, collect(i.id) AS ideas
		OPTIONAL MATCH (p:Phase)-[:CONTAINS]->(s)
		OPTIONAL MATCH (p)-[:CONTAINS]->(sib:Story)
		RETURN s.id AS id, deps, users, foundIn, found, bugs, ideas, p.number AS number, collect(sib.id) AS siblings`,
		map[string]any{"id": id, "done": append(slices.Clone(complete), Closed), "bugs": maxContextBugs}, nil)
	if err != nil {
		return Neighbourhood{}, fmt.Errorf("read neighbourhood: %w", err)
	}
	if !res.Next() {
		return Neighbourhood{}, refuse(NotFound, "no story %q", id)
	}
	r := res.Record()
	n := Neighbourhood{Phase: num(r, "number"), Ideas: strs(r, "ideas")}
	n.Stories = append(n.Stories, str(r, "id"))
	for _, k := range []string{"deps", "users", "foundIn", "found", "bugs", "siblings"} {
		n.Stories = append(n.Stories, strs(r, k)...)
	}
	slices.Sort(n.Stories)
	n.Stories = slices.Compact(n.Stories)
	return n, nil
}
