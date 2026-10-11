package board

import "fmt"

// AddDependency records that story id cannot be done until story on is
// complete. The dependency must point to a story ranked above, in the same
// or an earlier phase when both have one, and must not close a cycle.
// Recording a dependency that already exists changes nothing.
func (b *Board) AddDependency(ws, actor, id, on string) (Story, error) {
	if id == on {
		return Story{}, refuse(Invalid, "a story cannot depend on itself")
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Story{}, err
	}
	res, err := g.Query(`
		MATCH (d:Story {id: $id})
		OPTIONAL MATCH (t:Story {id: $on})
		OPTIONAL MATCH (dp:Phase)-[:CONTAINS]->(d)
		OPTIONAL MATCH (tp:Phase)-[:CONTAINS]->(t)
		// FalkorDB's exists(pattern) is true even with no match; count instead.
		WITH d, t, dp, tp, CASE
			WHEN t IS NULL THEN 'missing'
			WHEN size([p = (d)-[:DEPENDS_ON]->(t) | p]) > 0 THEN 'exists'
			WHEN dp IS NOT NULL AND tp IS NOT NULL AND tp.number > dp.number THEN 'phase'
			WHEN size([p = (t)-[:DEPENDS_ON*]->(d) | p]) > 0 THEN 'cycle'
			WHEN t.rank > d.rank THEN 'rank'
		END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
			CREATE (d)-[:DEPENDS_ON {created_by: $actor, created_at: $now}]->(t))
		RETURN violation, dp.number AS dphase, tp.number AS tphase`,
		map[string]any{"id": id, "on": on, "actor": actor, "now": now()}, nil)
	if err != nil {
		return Story{}, fmt.Errorf("add dependency: %w", err)
	}
	if !res.Next() {
		return Story{}, refuse(NotFound, "no story %q", id)
	}
	r := res.Record()
	switch str(r, "violation") {
	case "", "exists":
		return story(g, id)
	case "missing":
		return Story{}, refuse(NotFound, "no story %q", on)
	case "phase":
		return Story{}, refuse(Conflict, "%s is in phase %d and cannot depend on %s in the later phase %d",
			id, num(r, "dphase"), on, num(r, "tphase"))
	case "cycle":
		return Story{}, refuse(Conflict, "%s cannot depend on %s, which already depends on %s", id, on, id)
	default: // rank
		return Story{}, refuse(Conflict, "%s cannot depend on %s, which ranks below it; move %s above %s first",
			id, on, on, id)
	}
}

// RemoveDependency deletes the dependency of story id on story on.
func (b *Board) RemoveDependency(ws, id, on string) error {
	g, err := b.workspace(ws)
	if err != nil {
		return err
	}
	res, err := g.Query(`
		MATCH (:Story {id: $id})-[r:DEPENDS_ON]->(:Story {id: $on})
		DELETE r
		RETURN true`,
		map[string]any{"id": id, "on": on}, nil)
	if err != nil {
		return fmt.Errorf("remove dependency: %w", err)
	}
	if !res.Next() {
		return refuse(NotFound, "%s does not depend on %s", id, on)
	}
	return nil
}
