package board

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
)

// Phase is a numbered body of work with its own goals.
type Phase struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Goals       []Goal `json:"goals"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedBy   string `json:"updated_by,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// Goal is one outcome a phase commits to.
type Goal struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

// PhasePatch changes the fields that are set and leaves the rest.
type PhasePatch struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

// AddPhase appends a phase, numbered after the last one.
func (b *Board) AddPhase(ws, actor, title, description string) (Phase, error) {
	if title == "" {
		return Phase{}, refuse(Invalid, "title is required")
	}
	if err := plainText(title, description); err != nil {
		return Phase{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Phase{}, err
	}
	p := Phase{Title: title, Description: description, Goals: []Goal{}, CreatedBy: actor, CreatedAt: now()}
	res, err := g.Query(`
		OPTIONAL MATCH (last:Phase)
		WITH coalesce(max(last.number), 0) + 1 AS n
		CREATE (:Phase {number: n, title: $title, description: $description,
			created_by: $actor, created_at: $now})
		RETURN n`,
		map[string]any{"title": title, "description": description, "actor": actor, "now": p.CreatedAt}, nil)
	if err != nil {
		return Phase{}, fmt.Errorf("add phase: %w", err)
	}
	res.Next()
	p.Number = num(res.Record(), "n")
	return p, nil
}

// Phases lists a workspace's phases in order, each with its goals.
func (b *Board) Phases(ws string) ([]Phase, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	return phases(g, 0)
}

// UpdatePhase edits a phase's title or description.
func (b *Board) UpdatePhase(ws, actor string, number int, patch PhasePatch) (Phase, error) {
	if patch.Title != nil && *patch.Title == "" {
		return Phase{}, refuse(Invalid, "title cannot be empty")
	}
	if err := plainText(deref(patch.Title), deref(patch.Description)); err != nil {
		return Phase{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Phase{}, err
	}
	res, err := g.Query(`
		MATCH (p:Phase {number: $n})
		SET p.title = coalesce($title, p.title),
			p.description = coalesce($description, p.description),
			p.updated_by = $actor, p.updated_at = $now
		RETURN p.number`,
		map[string]any{"n": number, "title": opt(patch.Title), "description": opt(patch.Description),
			"actor": actor, "now": now()}, nil)
	if err != nil {
		return Phase{}, fmt.Errorf("update phase: %w", err)
	}
	if !res.Next() {
		return Phase{}, refuse(NotFound, "no phase %d", number)
	}
	ps, err := phases(g, number)
	if err != nil {
		return Phase{}, err
	}
	return ps[0], nil
}

// AddGoal adds a goal to a phase. Goals are numbered across the workspace.
func (b *Board) AddGoal(ws, actor string, phase int, text string) (Goal, error) {
	if text == "" {
		return Goal{}, refuse(Invalid, "text is required")
	}
	if err := plainText(text); err != nil {
		return Goal{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Goal{}, err
	}
	goal := Goal{Text: text, CreatedBy: actor, CreatedAt: now()}
	res, err := g.Query(`
		MATCH (w:Workspace), (p:Phase {number: $n})
		WITH w, p, 'G-' + toString(w.next_goal) AS id
		SET w.next_goal = w.next_goal + 1
		CREATE (p)-[:HAS_GOAL]->(:Goal {id: id, text: $text, created_by: $actor, created_at: $now})
		RETURN id`,
		map[string]any{"n": phase, "text": text, "actor": actor, "now": goal.CreatedAt}, nil)
	if err != nil {
		return Goal{}, fmt.Errorf("add goal: %w", err)
	}
	if !res.Next() {
		return Goal{}, refuse(NotFound, "no phase %d", phase)
	}
	goal.ID = str(res.Record(), "id")
	return goal, nil
}

// phases reads every phase, or only phase n when n is not 0.
func phases(g *falkordb.Graph, n int) ([]Phase, error) {
	res, err := g.ROQuery(`
		MATCH (p:Phase) WHERE $n = 0 OR p.number = $n
		OPTIONAL MATCH (p)-[:HAS_GOAL]->(goal:Goal)
		WITH p, collect({id: goal.id, text: goal.text, by: goal.created_by, at: goal.created_at}) AS goals
		RETURN p.number AS number, p.title AS title, p.description AS description,
			p.created_by AS by, p.created_at AS at,
			p.updated_by AS updated_by, p.updated_at AS updated_at, goals
		ORDER BY p.number`,
		map[string]any{"n": n}, nil)
	if err != nil {
		return nil, fmt.Errorf("read phases: %w", err)
	}
	out := []Phase{}
	for res.Next() {
		r := res.Record()
		p := Phase{Number: num(r, "number"), Title: str(r, "title"), Description: str(r, "description"),
			CreatedBy: str(r, "by"), CreatedAt: str(r, "at"),
			UpdatedBy: str(r, "updated_by"), UpdatedAt: str(r, "updated_at"), Goals: []Goal{}}
		raw, _ := r.Get("goals")
		list, _ := raw.([]any)
		for _, x := range list {
			m, _ := x.(map[string]any)
			if id, _ := m["id"].(string); id != "" {
				text, _ := m["text"].(string)
				by, _ := m["by"].(string)
				at, _ := m["at"].(string)
				p.Goals = append(p.Goals, Goal{ID: id, Text: text, CreatedBy: by, CreatedAt: at})
			}
		}
		slices.SortFunc(p.Goals, func(a, b Goal) int { return seq(a.ID) - seq(b.ID) })
		out = append(out, p)
	}
	return out, nil
}

// seq is the number in an ID such as S-12 or G-3.
func seq(id string) int {
	_, n, _ := strings.Cut(id, "-")
	i, _ := strconv.Atoi(n)
	return i
}

// deref reads an optional field; unset reads as "".
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// opt turns an unset optional field into a Cypher null.
func opt(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
