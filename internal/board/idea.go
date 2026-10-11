package board

import (
	"fmt"
	"slices"

	"github.com/FalkorDB/falkordb-go/v2"
)

// Idea is a rough thought, kept in its own ordered list apart from the
// backlog until it becomes stories or a phase, or is dropped.
type Idea struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Notes string `json:"notes"`
	// State is proposed (waiting for a person to approve its spot), open or
	// closed.
	State string `json:"state"`
	// Reasoning is why an agent suggested the idea's spot.
	Reasoning     string   `json:"reasoning,omitempty"`
	BecameStories []string `json:"became_stories"`
	BecamePhase   int      `json:"became_phase,omitempty"`
	DropReason    string   `json:"drop_reason,omitempty"`
	CreatedBy     string   `json:"created_by"`
	CreatedAt     string   `json:"created_at"`
	ApprovedBy    string   `json:"approved_by,omitempty"`
	ApprovedAt    string   `json:"approved_at,omitempty"`
	MovedBy       string   `json:"moved_by,omitempty"`
	MovedAt       string   `json:"moved_at,omitempty"`
	UpdatedBy     string   `json:"updated_by,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
	ClosedBy      string   `json:"closed_by,omitempty"`
	ClosedAt      string   `json:"closed_at,omitempty"`
}

// NewIdea is what AddIdea needs. A proposed idea waits, with the reasoning
// for its spot, until a person approves it.
type NewIdea struct {
	Title     string `json:"title"`
	Notes     string `json:"notes"`
	Proposed  bool   `json:"-"`
	Reasoning string `json:"-"`
	Position
}

// IdeaPatch changes the fields that are set and leaves the rest.
type IdeaPatch struct {
	Title *string `json:"title"`
	Notes *string `json:"notes"`
}

// Refinement is what an idea became: stories, a phase, or both.
type Refinement struct {
	Stories []string `json:"stories"`
	Phase   int      `json:"phase"`
}

// AddIdea puts an idea into the ideas list at the given position.
func (b *Board) AddIdea(ws, actor string, i NewIdea) (Idea, error) {
	if i.Title == "" {
		return Idea{}, refuse(Invalid, "title is required")
	}
	if i.Before != "" && i.After != "" {
		return Idea{}, refuse(Invalid, "give before or after, not both")
	}
	if i.Proposed && i.Reasoning == "" {
		return Idea{}, refuse(Invalid, "reasoning is required to suggest an idea")
	}
	if !i.Proposed && i.Reasoning != "" {
		return Idea{}, refuse(Invalid, "reasoning is given only when suggesting an idea")
	}
	if err := plainText(i.Title, i.Notes, i.Reasoning); err != nil {
		return Idea{}, err
	}
	state := "open"
	if i.Proposed {
		state = "proposed"
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Idea{}, err
	}
	for range placeAttempts {
		at, err := findGap(g, rankedIdea, i.Position, "")
		if err != nil {
			return Idea{}, err
		}
		res, err := g.Query(`
			MATCH (w:Workspace)
			OPTIONAL MATCH (anchor:Idea {id: $anchor})
			OPTIONAL MATCH (o:Idea) WHERE ($lo = '' OR o.rank > $lo) AND ($hi = '' OR o.rank < $hi)
			WITH w, anchor, count(o) AS between, coalesce(w.next_idea, 1) AS n
			WITH w, n, 'I-' + toString(n) AS id, CASE
				WHEN between > 0 OR ($anchor <> '' AND (anchor IS NULL OR anchor.rank <> $anchorRank)) THEN 'gap'
			END AS violation
			FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
				SET w.next_idea = n + 1
				CREATE (:Idea {id: id, title: $title, notes: $notes, rank: $rank, state: $state,
					reasoning: $reasoning, created_by: $actor, created_at: $now}))
			RETURN violation, id`,
			map[string]any{
				"anchor": at.anchor, "anchorRank": at.anchorRank, "lo": at.lo, "hi": at.hi,
				"rank": Between(at.lo, at.hi), "title": i.Title, "notes": i.Notes, "state": state,
				"reasoning": orNull(i.Reasoning), "actor": actor, "now": now(),
			}, nil)
		if err != nil {
			return Idea{}, fmt.Errorf("add idea: %w", err)
		}
		res.Next()
		if r := res.Record(); str(r, "violation") == "" {
			return idea(g, str(r, "id"))
		}
	}
	return Idea{}, refuse(Conflict, "the ideas list kept changing while the idea was being placed; try again")
}

// MoveIdea moves an idea that is not closed to a new position.
func (b *Board) MoveIdea(ws, actor, id string, to Position) (Idea, error) {
	if to.Before != "" && to.After != "" {
		return Idea{}, refuse(Invalid, "give before or after, not both")
	}
	if to.Before == id || to.After == id {
		return Idea{}, refuse(Invalid, "an idea cannot be placed relative to itself")
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Idea{}, err
	}
	if _, err := idea(g, id); err != nil {
		return Idea{}, err
	}
	for range placeAttempts {
		at, err := findGap(g, rankedIdea, to, id)
		if err != nil {
			return Idea{}, err
		}
		res, err := g.Query(`
			MATCH (i:Idea {id: $id})
			OPTIONAL MATCH (anchor:Idea {id: $anchor})
			OPTIONAL MATCH (o:Idea) WHERE o.id <> $id AND ($lo = '' OR o.rank > $lo) AND ($hi = '' OR o.rank < $hi)
			WITH i, anchor, count(o) AS between
			WITH i, CASE
				WHEN i.state = 'closed' THEN 'closed'
				WHEN between > 0 OR ($anchor <> '' AND (anchor IS NULL OR anchor.rank <> $anchorRank)) THEN 'gap'
			END AS violation
			FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
				SET i.rank = $rank, i.moved_by = $actor, i.moved_at = $now)
			RETURN violation`,
			map[string]any{
				"id": id, "anchor": at.anchor, "anchorRank": at.anchorRank, "lo": at.lo, "hi": at.hi,
				"rank": Between(at.lo, at.hi), "actor": actor, "now": now(),
			}, nil)
		if err != nil {
			return Idea{}, fmt.Errorf("move idea: %w", err)
		}
		res.Next()
		switch str(res.Record(), "violation") {
		case "":
			return idea(g, id)
		case "closed":
			return Idea{}, refuse(Conflict, "%s is closed", id)
		}
	}
	return Idea{}, refuse(Conflict, "the ideas list kept changing while %s was being moved; try again", id)
}

// UpdateIdea edits an idea's title and notes while it is not closed.
func (b *Board) UpdateIdea(ws, actor, id string, patch IdeaPatch) (Idea, error) {
	if patch.Title != nil && *patch.Title == "" {
		return Idea{}, refuse(Invalid, "title cannot be empty")
	}
	if err := plainText(deref(patch.Title), deref(patch.Notes)); err != nil {
		return Idea{}, err
	}
	return b.changeIdea(ws, id, `
		SET i.title = coalesce($title, i.title), i.notes = coalesce($notes, i.notes),
			i.updated_by = $actor, i.updated_at = $now`,
		map[string]any{"title": opt(patch.Title), "notes": opt(patch.Notes), "actor": actor, "now": now()},
		"open", "proposed")
}

// ApproveIdea accepts a suggested idea at its current spot.
func (b *Board) ApproveIdea(ws, actor, id string) (Idea, error) {
	return b.changeIdea(ws, id, `SET i.state = 'open', i.approved_by = $actor, i.approved_at = $now`,
		map[string]any{"actor": actor, "now": now()}, "proposed")
}

// DropIdea closes an idea that became nothing, with the reason.
func (b *Board) DropIdea(ws, actor, id, reason string) (Idea, error) {
	if reason == "" {
		return Idea{}, refuse(Invalid, "a reason is required to drop an idea")
	}
	if err := plainText(reason); err != nil {
		return Idea{}, err
	}
	return b.changeIdea(ws, id, `
		SET i.state = 'closed', i.drop_reason = $reason, i.closed_by = $actor, i.closed_at = $now`,
		map[string]any{"reason": reason, "actor": actor, "now": now()}, "open", "proposed")
}

// changeIdea applies set to idea id when its state is one of from.
func (b *Board) changeIdea(ws, id, set string, params map[string]any, from ...string) (Idea, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return Idea{}, err
	}
	params["id"], params["from"] = id, from
	res, err := g.Query(`
		MATCH (i:Idea {id: $id})
		WITH i, CASE WHEN NOT i.state IN $from THEN 'state' END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END | `+set+`)
		RETURN violation, i.state AS state`, params, nil)
	if err != nil {
		return Idea{}, fmt.Errorf("change idea: %w", err)
	}
	if !res.Next() {
		return Idea{}, refuse(NotFound, "no idea %q", id)
	}
	if r := res.Record(); str(r, "violation") != "" {
		return Idea{}, refuse(Conflict, "%s is %s", id, str(r, "state"))
	}
	return idea(g, id)
}

// RefineIdea closes an open idea, linking it to the stories and phase it
// became. They are created first, the usual way; none may be closed.
func (b *Board) RefineIdea(ws, actor, id string, to Refinement) (Idea, error) {
	stories := slices.Compact(slices.Sorted(slices.Values(to.Stories)))
	if len(stories) == 0 && to.Phase == 0 {
		return Idea{}, refuse(Invalid, "name the stories or the phase the idea became")
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Idea{}, err
	}
	res, err := g.Query(`
		MATCH (i:Idea {id: $id})
		OPTIONAL MATCH (s:Story) WHERE s.id IN $stories
		WITH i, collect(s) AS found
		OPTIONAL MATCH (p:Phase {number: $phase})
		WITH i, found, p, [x IN $stories WHERE NOT x IN [s IN found | s.id]] AS missing,
			[s IN found WHERE s.status = 'closed' | s.id] AS closed
		WITH i, found, p, missing, closed, CASE
			WHEN size(missing) > 0 THEN 'missing'
			WHEN $phase <> 0 AND p IS NULL THEN 'phase'
			WHEN i.state <> 'open' THEN 'state'
			WHEN size(closed) > 0 THEN 'closed'
		END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
			SET i.state = 'closed', i.closed_by = $actor, i.closed_at = $now
			FOREACH (s IN found | CREATE (i)-[:BECAME]->(s))
			FOREACH (q IN CASE WHEN p IS NULL THEN [] ELSE [p] END | CREATE (i)-[:BECAME]->(q)))
		RETURN violation, i.state AS state, missing, closed`,
		map[string]any{"id": id, "stories": stories, "phase": to.Phase, "actor": actor, "now": now()}, nil)
	if err != nil {
		return Idea{}, fmt.Errorf("refine idea: %w", err)
	}
	if !res.Next() {
		return Idea{}, refuse(NotFound, "no idea %q", id)
	}
	r := res.Record()
	switch str(r, "violation") {
	case "missing":
		return Idea{}, refuse(NotFound, "no story %s", list(strs(r, "missing")))
	case "phase":
		return Idea{}, refuse(NotFound, "no phase %d", to.Phase)
	case "state":
		return Idea{}, refuse(Conflict, "%s is %s; only an open idea is refined", id, str(r, "state"))
	case "closed":
		return Idea{}, refuse(Conflict, "%s closed; an idea cannot become a closed story", list(strs(r, "closed")))
	}
	return idea(g, id)
}

// Idea reads one idea, closed or not.
func (b *Board) Idea(ws, id string) (Idea, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return Idea{}, err
	}
	return idea(g, id)
}

// Ideas lists the ideas that are not closed, in order, top first.
func (b *Board) Ideas(ws string) ([]Idea, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	return ideas(g, "")
}

func idea(g *falkordb.Graph, id string) (Idea, error) {
	is, err := ideas(g, id)
	if err != nil {
		return Idea{}, err
	}
	if len(is) == 0 {
		return Idea{}, refuse(NotFound, "no idea %q", id)
	}
	return is[0], nil
}

// ideas reads every idea that is not closed in rank order, or only idea id
// when id is set.
func ideas(g *falkordb.Graph, id string) ([]Idea, error) {
	res, err := g.ROQuery(`
		MATCH (i:Idea) WHERE ($id = '' AND i.state <> 'closed') OR i.id = $id
		OPTIONAL MATCH (i)-[:BECAME]->(s:Story)
		WITH i, collect(s.id) AS stories
		OPTIONAL MATCH (i)-[:BECAME]->(p:Phase)
		RETURN i.id AS id, i.title AS title, i.notes AS notes, i.state AS state, i.reasoning AS reasoning,
			stories, p.number AS phase, i.drop_reason AS drop_reason,
			i.created_by AS created_by, i.created_at AS created_at,
			i.approved_by AS approved_by, i.approved_at AS approved_at,
			i.moved_by AS moved_by, i.moved_at AS moved_at,
			i.updated_by AS updated_by, i.updated_at AS updated_at,
			i.closed_by AS closed_by, i.closed_at AS closed_at
		ORDER BY i.rank`,
		map[string]any{"id": id}, nil)
	if err != nil {
		return nil, fmt.Errorf("read ideas: %w", err)
	}
	out := []Idea{}
	for res.Next() {
		r := res.Record()
		out = append(out, Idea{
			ID: str(r, "id"), Title: str(r, "title"), Notes: str(r, "notes"), State: str(r, "state"),
			Reasoning: str(r, "reasoning"), BecameStories: byNumber(strs(r, "stories")),
			BecamePhase: num(r, "phase"), DropReason: str(r, "drop_reason"),
			CreatedBy: str(r, "created_by"), CreatedAt: str(r, "created_at"),
			ApprovedBy: str(r, "approved_by"), ApprovedAt: str(r, "approved_at"),
			MovedBy: str(r, "moved_by"), MovedAt: str(r, "moved_at"),
			UpdatedBy: str(r, "updated_by"), UpdatedAt: str(r, "updated_at"),
			ClosedBy: str(r, "closed_by"), ClosedAt: str(r, "closed_at"),
		})
	}
	return out, nil
}
