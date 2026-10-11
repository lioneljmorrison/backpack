package board

import (
	"fmt"
	"slices"

	"github.com/FalkorDB/falkordb-go/v2"
)

// Story is one deliverable piece of work.
type Story struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Goal        string   `json:"goal"`
	Plan        string   `json:"plan"`
	Phase       int      `json:"phase,omitempty"`
	Status      string   `json:"status"`
	BlockedBy   []string `json:"blocked_by"`
	Blocks      []string `json:"blocks"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	MovedBy     string   `json:"moved_by,omitempty"`
	MovedAt     string   `json:"moved_at,omitempty"`
}

// Position places a story directly before or directly after another one.
// With neither set, the story goes to the bottom of the backlog.
type Position struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// NewStory is what AddStory needs. Phase 0 means the story has no phase.
type NewStory struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Goal        string `json:"goal"`
	Plan        string `json:"plan"`
	Phase       int    `json:"phase"`
	Position
}

// StoryPatch changes the fields that are set and leaves the rest.
type StoryPatch struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Goal        *string `json:"goal"`
	Plan        *string `json:"plan"`
}

// placeAttempts bounds the retries when another write takes the same gap.
const placeAttempts = 3

// AddStory creates a story in the backlog at the given position.
func (b *Board) AddStory(ws, actor string, s NewStory) (Story, error) {
	if s.Title == "" {
		return Story{}, refuse(Invalid, "title is required")
	}
	if s.Before != "" && s.After != "" {
		return Story{}, refuse(Invalid, "give before or after, not both")
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Story{}, err
	}
	for range placeAttempts {
		at, err := findGap(g, s.Position, "")
		if err != nil {
			return Story{}, err
		}
		res, err := g.Query(`
			MATCH (w:Workspace)
			OPTIONAL MATCH (p:Phase {number: $phase})
			OPTIONAL MATCH (anchor:Story {id: $anchor})
			OPTIONAL MATCH (o:Story) WHERE ($lo = '' OR o.rank > $lo) AND ($hi = '' OR o.rank < $hi)
			WITH w, p, anchor, count(o) AS between
			WITH w, p, 'S-' + toString(w.next_story) AS id, CASE
				WHEN $phase <> 0 AND p IS NULL THEN 'phase'
				WHEN between > 0 OR ($anchor <> '' AND (anchor IS NULL OR anchor.rank <> $anchorRank)) THEN 'gap'
			END AS violation
			FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
				SET w.next_story = w.next_story + 1
				CREATE (s:Story {id: id, title: $title, description: $description, goal: $goal,
					plan: $plan, rank: $rank, status: 'backlog', created_by: $actor, created_at: $now})
				FOREACH (q IN CASE WHEN p IS NULL THEN [] ELSE [p] END | CREATE (q)-[:CONTAINS]->(s)))
			RETURN violation, id`,
			map[string]any{
				"phase": s.Phase, "anchor": at.anchor, "anchorRank": at.anchorRank, "lo": at.lo, "hi": at.hi,
				"rank": Between(at.lo, at.hi), "title": s.Title, "description": s.Description,
				"goal": s.Goal, "plan": s.Plan, "actor": actor, "now": now(),
			}, nil)
		if err != nil {
			return Story{}, fmt.Errorf("add story: %w", err)
		}
		res.Next()
		r := res.Record()
		switch str(r, "violation") {
		case "":
			return story(g, str(r, "id"))
		case "phase":
			return Story{}, refuse(NotFound, "no phase %d", s.Phase)
		}
	}
	return Story{}, refuse(Conflict, "the backlog kept changing while the story was being placed; try again")
}

// MoveStory moves a story to a new position, keeping it below everything it
// depends on and above everything that depends on it.
func (b *Board) MoveStory(ws, actor, id string, to Position) (Story, error) {
	if to.Before != "" && to.After != "" {
		return Story{}, refuse(Invalid, "give before or after, not both")
	}
	if to.Before == id || to.After == id {
		return Story{}, refuse(Invalid, "a story cannot be placed relative to itself")
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Story{}, err
	}
	if _, err := story(g, id); err != nil {
		return Story{}, err
	}
	for range placeAttempts {
		at, err := findGap(g, to, id)
		if err != nil {
			return Story{}, err
		}
		res, err := g.Query(`
			MATCH (s:Story {id: $id})
			OPTIONAL MATCH (anchor:Story {id: $anchor})
			OPTIONAL MATCH (o:Story) WHERE o.id <> $id AND ($lo = '' OR o.rank > $lo) AND ($hi = '' OR o.rank < $hi)
			WITH s, anchor, count(o) AS between
			OPTIONAL MATCH (s)-[:DEPENDS_ON]->(t:Story) WHERE t.rank >= $rank
			WITH s, anchor, between, collect(t.id) AS above
			OPTIONAL MATCH (d:Story)-[:DEPENDS_ON]->(s) WHERE d.rank <= $rank
			WITH s, anchor, between, above, collect(d.id) AS below
			WITH s, above, below, CASE
				WHEN between > 0 OR ($anchor <> '' AND (anchor IS NULL OR anchor.rank <> $anchorRank)) THEN 'gap'
				WHEN size(above) > 0 THEN 'above'
				WHEN size(below) > 0 THEN 'below'
			END AS violation
			FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
				SET s.rank = $rank, s.moved_by = $actor, s.moved_at = $now)
			RETURN violation, above, below`,
			map[string]any{
				"id": id, "anchor": at.anchor, "anchorRank": at.anchorRank, "lo": at.lo, "hi": at.hi,
				"rank": Between(at.lo, at.hi), "actor": actor, "now": now(),
			}, nil)
		if err != nil {
			return Story{}, fmt.Errorf("move story: %w", err)
		}
		if !res.Next() {
			return Story{}, refuse(NotFound, "no story %q", id)
		}
		r := res.Record()
		switch str(r, "violation") {
		case "":
			return story(g, id)
		case "above":
			return Story{}, refuse(Conflict, "%s must stay below %s, which it depends on", id, list(strs(r, "above")))
		case "below":
			return Story{}, refuse(Conflict, "%s must stay above %s, which depends on it", id, list(strs(r, "below")))
		}
	}
	return Story{}, refuse(Conflict, "the backlog kept changing while %s was being moved; try again", id)
}

// gap is where a story is about to go: strictly between the ranks lo and hi
// ("" is an open end), next to the anchor story when one was named.
type gap struct {
	lo, hi, anchor, anchorRank string
}

// findGap reads the ranks around a position. moving is the story being
// moved, which is not its own neighbour; "" for a new story.
func findGap(g *falkordb.Graph, to Position, moving string) (gap, error) {
	var q string
	at := gap{anchor: to.Before + to.After}
	switch {
	case to.After != "":
		q = `MATCH (a:Story {id: $anchor})
			OPTIONAL MATCH (n:Story) WHERE n.rank > a.rank AND n.id <> $moving
			RETURN a.rank AS anchor, a.rank AS lo, min(n.rank) AS hi`
	case to.Before != "":
		q = `MATCH (a:Story {id: $anchor})
			OPTIONAL MATCH (n:Story) WHERE n.rank < a.rank AND n.id <> $moving
			RETURN a.rank AS anchor, max(n.rank) AS lo, a.rank AS hi`
	default:
		q = `OPTIONAL MATCH (n:Story) WHERE n.id <> $moving RETURN max(n.rank) AS lo`
	}
	res, err := g.ROQuery(q, map[string]any{"anchor": at.anchor, "moving": moving}, nil)
	if err != nil {
		return gap{}, fmt.Errorf("read backlog: %w", err)
	}
	if !res.Next() {
		return gap{}, refuse(NotFound, "no story %q", at.anchor)
	}
	r := res.Record()
	at.lo, at.hi, at.anchorRank = str(r, "lo"), str(r, "hi"), str(r, "anchor")
	return at, nil
}

// Story reads one story with its dependencies in both directions.
func (b *Board) Story(ws, id string) (Story, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return Story{}, err
	}
	return story(g, id)
}

// Stories lists the backlog in delivery order, top first.
func (b *Board) Stories(ws string) ([]Story, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	return stories(g, "")
}

// UpdateStory edits a story's text.
func (b *Board) UpdateStory(ws, id string, patch StoryPatch) (Story, error) {
	if patch.Title != nil && *patch.Title == "" {
		return Story{}, refuse(Invalid, "title cannot be empty")
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Story{}, err
	}
	res, err := g.Query(`
		MATCH (s:Story {id: $id})
		SET s.title = coalesce($title, s.title),
			s.description = coalesce($description, s.description),
			s.goal = coalesce($goal, s.goal),
			s.plan = coalesce($plan, s.plan)
		RETURN s.id`,
		map[string]any{"id": id, "title": opt(patch.Title), "description": opt(patch.Description),
			"goal": opt(patch.Goal), "plan": opt(patch.Plan)}, nil)
	if err != nil {
		return Story{}, fmt.Errorf("update story: %w", err)
	}
	if !res.Next() {
		return Story{}, refuse(NotFound, "no story %q", id)
	}
	return story(g, id)
}

func story(g *falkordb.Graph, id string) (Story, error) {
	ss, err := stories(g, id)
	if err != nil {
		return Story{}, err
	}
	if len(ss) == 0 {
		return Story{}, refuse(NotFound, "no story %q", id)
	}
	return ss[0], nil
}

// stories reads every story in rank order, or only story id when id is set.
func stories(g *falkordb.Graph, id string) ([]Story, error) {
	res, err := g.ROQuery(`
		MATCH (s:Story) WHERE $id = '' OR s.id = $id
		OPTIONAL MATCH (p:Phase)-[:CONTAINS]->(s)
		OPTIONAL MATCH (s)-[:DEPENDS_ON]->(t:Story)
		WITH s, p, collect(t.id) AS blocked_by
		OPTIONAL MATCH (d:Story)-[:DEPENDS_ON]->(s)
		WITH s, p, blocked_by, collect(d.id) AS blocks
		RETURN s.id AS id, s.title AS title, s.description AS description, s.goal AS goal,
			s.plan AS plan, s.status AS status, p.number AS phase, blocked_by, blocks,
			s.created_by AS created_by, s.created_at AS created_at,
			s.moved_by AS moved_by, s.moved_at AS moved_at
		ORDER BY s.rank`,
		map[string]any{"id": id}, nil)
	if err != nil {
		return nil, fmt.Errorf("read stories: %w", err)
	}
	out := []Story{}
	for res.Next() {
		r := res.Record()
		out = append(out, Story{
			ID: str(r, "id"), Title: str(r, "title"), Description: str(r, "description"),
			Goal: str(r, "goal"), Plan: str(r, "plan"), Status: str(r, "status"), Phase: num(r, "phase"),
			BlockedBy: byNumber(strs(r, "blocked_by")), Blocks: byNumber(strs(r, "blocks")),
			CreatedBy: str(r, "created_by"), CreatedAt: str(r, "created_at"),
			MovedBy: str(r, "moved_by"), MovedAt: str(r, "moved_at"),
		})
	}
	return out, nil
}

func byNumber(ids []string) []string {
	slices.SortFunc(ids, func(a, b string) int { return seq(a) - seq(b) })
	return ids
}

// list joins IDs for a message: "S-1", "S-1 and S-2", "S-1, S-2 and S-3".
func list(ids []string) string {
	ids = byNumber(ids)
	if len(ids) < 2 {
		return fmt.Sprint(ids[0])
	}
	out := ids[0]
	for _, id := range ids[1 : len(ids)-1] {
		out += ", " + id
	}
	return out + " and " + ids[len(ids)-1]
}
