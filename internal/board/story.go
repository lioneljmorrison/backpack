package board

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
)

// Story is one deliverable piece of work.
type Story struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Goal        string `json:"goal"`
	Plan        string `json:"plan"`
	// Kind is "bug" for a bug; none for any other story.
	Kind string `json:"kind,omitempty"`
	// FoundIn is the story this one turned up while building; none if it
	// was not found that way. Found is the reverse: the stories found
	// while building this one.
	FoundIn string   `json:"found_in,omitempty"`
	Found   []string `json:"found"`
	Phase   int      `json:"phase,omitempty"`
	Status  string   `json:"status"`
	// Blocked is worked out, never set: a dependency is not yet complete.
	Blocked    bool     `json:"blocked"`
	BlockedBy  []string `json:"blocked_by"`
	Blocks     []string `json:"blocks"`
	HeldFrom   string   `json:"held_from,omitempty"`
	HoldReason string   `json:"hold_reason,omitempty"`
	Activity   string   `json:"activity,omitempty"`
	WaitingOn  string   `json:"waiting_on,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	// Into is the furthest branch a merged story reached, by name; none
	// before it is merged. Branch is the task branch it is built on.
	Into       string `json:"into,omitempty"`
	PRURL      string `json:"pr_url,omitempty"`
	Workaround string `json:"workaround,omitempty"`
	// Release is the release a released story shipped in; none before that,
	// and none for a story released before releases were recorded.
	Release   *ReleaseRef `json:"release,omitempty"`
	CreatedBy string      `json:"created_by"`
	CreatedAt string      `json:"created_at"`
	MovedBy   string      `json:"moved_by,omitempty"`
	MovedAt   string      `json:"moved_at,omitempty"`
	UpdatedBy string      `json:"updated_by,omitempty"`
	UpdatedAt string      `json:"updated_at,omitempty"`

	// UpstreamChanged is set when a story this one depends on, directly or
	// further down, had its goal or plan changed since this story was last
	// reviewed; none otherwise. It records the most recent such change.
	UpstreamChanged *UpstreamRef `json:"upstream_changed,omitempty"`

	// waiting is the dependencies that are not yet complete.
	waiting []string
}

// ReleaseRef names a release from a story: enough to show and to link.
type ReleaseRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// UpstreamRef says which story's goal or plan changed, and when.
type UpstreamRef struct {
	Story string `json:"story"`
	At    string `json:"at"`
}

// Position places a story directly before or directly after another one.
// With neither set, the story goes to the bottom of the backlog.
type Position struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// NewStory is what AddStory needs. Phase 0 means the story has no phase. A
// proposed story waits in Proposed, with the reasoning for its spot logged as
// a decision, until a person moves it to the backlog.
type NewStory struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Goal        string `json:"goal"`
	Plan        string `json:"plan"`
	Kind        string `json:"kind"`
	// FoundIn names the story this one was found while building.
	FoundIn   string `json:"found_in"`
	Phase     int    `json:"phase"`
	Proposed  bool   `json:"-"`
	Reasoning string `json:"-"`
	Position
}

// StoryPatch changes the fields that are set and leaves the rest. Activity
// and WaitingOn apply only while the story is in progress, and WaitingOn
// only while it is waiting.
type StoryPatch struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Goal        *string `json:"goal"`
	Plan        *string `json:"plan"`
	Activity    *string `json:"activity"`
	WaitingOn   *string `json:"waiting_on"`
	Branch      *string `json:"branch"`
	PRURL       *string `json:"pr_url"`
	// Kind "bug" marks the story as a bug; "" clears the mark.
	Kind *string `json:"kind"`
}

// kindBug is the one story kind there is. A story of no kind is just a story.
const kindBug = "bug"

// checkKind refuses any kind but bug, or none ("").
func checkKind(k string) error {
	if k != "" && k != kindBug {
		return refuse(Invalid, "kind is %q, or empty for none", kindBug)
	}
	return nil
}

// activities are what an in-progress story can be doing.
var activities = []string{"planning", "building", "testing", "waiting"}

// placeAttempts bounds the retries when another write takes the same gap.
const placeAttempts = 3

// AddStory creates a story in the backlog at the given position.
func (b *Board) AddStory(ws, actor string, s NewStory) (out Story, err error) {
	defer func() { b.changed(err, Change{ws, KindStory, out.ID}) }()
	if s.Title == "" {
		return Story{}, refuse(Invalid, "title is required")
	}
	if s.Before != "" && s.After != "" {
		return Story{}, refuse(Invalid, "give before or after, not both")
	}
	if s.Proposed && s.Reasoning == "" {
		return Story{}, refuse(Invalid, "reasoning is required to propose a story")
	}
	if !s.Proposed && s.Reasoning != "" {
		return Story{}, refuse(Invalid, "reasoning is given only when proposing a story")
	}
	if err := checkKind(s.Kind); err != nil {
		return Story{}, err
	}
	if err := plainText(s.Title, s.Description, s.Goal, s.Plan, s.Reasoning); err != nil {
		return Story{}, err
	}
	status, placed := Backlog, "Proposed at the bottom of the backlog"
	switch {
	case s.After != "":
		placed = "Proposed after " + s.After
	case s.Before != "":
		placed = "Proposed before " + s.Before
	}
	if s.Proposed {
		status = Proposed
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Story{}, err
	}
	for range placeAttempts {
		at, err := findGap(g, rankedStory, s.Position, "")
		if err != nil {
			return Story{}, err
		}
		res, err := g.Query(`
			MATCH (w:Workspace)
			OPTIONAL MATCH (p:Phase {number: $phase})
			OPTIONAL MATCH (anchor:Story {id: $anchor})
			OPTIONAL MATCH (f:Story {id: $found})
			OPTIONAL MATCH (o:Story) WHERE ($lo = '' OR o.rank > $lo) AND ($hi = '' OR o.rank < $hi)
			WITH w, p, anchor, f, count(o) AS between
			WITH w, p, f, 'S-' + toString(w.next_story) AS id, coalesce(w.next_log, 1) AS n, CASE
				WHEN $phase <> 0 AND p IS NULL THEN 'phase'
				WHEN $found <> '' AND f IS NULL THEN 'found_in'
				WHEN f.status = 'closed' THEN 'found_closed'
				WHEN between > 0 OR ($anchor <> '' AND (anchor IS NULL OR anchor.rank <> $anchorRank)) THEN 'gap'
			END AS violation
			FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
				SET w.next_story = w.next_story + 1
				CREATE (s:Story {id: id, title: $title, description: $description, goal: $goal,
					plan: $plan, kind: $kind, rank: $rank, status: $status, created_by: $actor, created_at: $now})
				FOREACH (q IN CASE WHEN p IS NULL THEN [] ELSE [p] END | CREATE (q)-[:CONTAINS]->(s))
				FOREACH (q IN CASE WHEN f IS NULL THEN [] ELSE [f] END | CREATE (s)-[:FOUND_IN]->(q))
				FOREACH (__ IN CASE WHEN $reasoning IS NULL THEN [] ELSE [1] END |
					SET w.next_log = n + 1
					CREATE (s)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n), at: $now, actor: $actor,
						kind: 'decision', text: $placed, reason: $reasoning})))
			RETURN violation, id`,
			map[string]any{
				"phase": s.Phase, "anchor": at.anchor, "anchorRank": at.anchorRank, "lo": at.lo, "hi": at.hi,
				"rank": Between(at.lo, at.hi), "title": s.Title, "description": s.Description,
				"goal": s.Goal, "plan": s.Plan, "kind": orNull(s.Kind), "found": s.FoundIn, "status": status, "placed": placed, "reasoning": orNull(s.Reasoning),
				"actor": actor, "now": now(),
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
		case "found_in":
			return Story{}, refuse(NotFound, "no story %q to have been found in", s.FoundIn)
		case "found_closed":
			return Story{}, refuse(Conflict, "%s is closed; a story cannot be found in a closed story", s.FoundIn)
		}
	}
	return Story{}, refuse(Conflict, "the backlog kept changing while the story was being placed; try again")
}

// MoveStory moves a story to a new position, keeping it below everything it
// depends on and above everything that depends on it.
func (b *Board) MoveStory(ws, actor, id string, to Position) (_ Story, err error) {
	defer func() { b.changed(err, Change{ws, KindStory, id}) }()
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
		at, err := findGap(g, rankedStory, to, id)
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

// gap is where a story or idea is about to go: strictly between the ranks lo and hi
// ("" is an open end), next to the anchor story when one was named.
type gap struct {
	lo, hi, anchor, anchorRank string
}

// ranked is a kind of node kept in its own order: stories in the backlog,
// ideas in the ideas list. Its label goes into the query text, so it is only
// ever one of these two, never caller input.
type ranked struct{ label, noun string }

var (
	rankedStory = ranked{"Story", "story"}
	rankedIdea  = ranked{"Idea", "idea"}
)

// findGap reads the ranks around a position. moving is the node being
// moved, which is not its own neighbour; "" for a new one.
func findGap(g *falkordb.Graph, k ranked, to Position, moving string) (gap, error) {
	var q string
	at := gap{anchor: to.Before + to.After}
	switch {
	case to.After != "":
		q = `MATCH (a:` + k.label + ` {id: $anchor})
			OPTIONAL MATCH (n:` + k.label + `) WHERE n.rank > a.rank AND n.id <> $moving
			RETURN a.rank AS anchor, a.rank AS lo, min(n.rank) AS hi`
	case to.Before != "":
		q = `MATCH (a:` + k.label + ` {id: $anchor})
			OPTIONAL MATCH (n:` + k.label + `) WHERE n.rank < a.rank AND n.id <> $moving
			RETURN a.rank AS anchor, max(n.rank) AS lo, a.rank AS hi`
	default:
		q = `OPTIONAL MATCH (n:` + k.label + `) WHERE n.id <> $moving RETURN max(n.rank) AS lo`
	}
	res, err := g.ROQuery(q, map[string]any{"anchor": at.anchor, "moving": moving}, nil)
	if err != nil {
		return gap{}, fmt.Errorf("read %s order: %w", k.noun, err)
	}
	if !res.Next() {
		return gap{}, refuse(NotFound, "no %s %q", k.noun, at.anchor)
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

// UpdateStory edits a story's text, branch and pull request, and what it
// is doing while in progress.
func (b *Board) UpdateStory(ws, actor, id string, patch StoryPatch) (Story, error) {
	r, err := b.UpdateStoryDetail(ws, actor, id, patch)
	return r.Story, err
}

// StoryUpdate is what an update did: the story, the names of the fields
// whose value actually changed, and the dependents it marked as upstream
// changed.
type StoryUpdate struct {
	Story   Story
	Changed []string
	Marked  []string
}

// UpdateStoryDetail is UpdateStory, also answering which fields changed and
// which stories were marked, from the same single query.
func (b *Board) UpdateStoryDetail(ws, actor, id string, patch StoryPatch) (_ StoryUpdate, err error) {
	defer func() { b.changed(err, Change{ws, KindStory, id}) }()
	if patch.Title != nil && *patch.Title == "" {
		return StoryUpdate{}, refuse(Invalid, "title cannot be empty")
	}
	if patch.Activity != nil && !slices.Contains(activities, *patch.Activity) {
		return StoryUpdate{}, refuse(Invalid, "activity is one of %s", strings.Join(activities, ", "))
	}
	if err := checkKind(deref(patch.Kind)); err != nil {
		return StoryUpdate{}, err
	}
	if err := plainText(deref(patch.Title), deref(patch.Description), deref(patch.Goal), deref(patch.Plan),
		deref(patch.WaitingOn), deref(patch.Branch), deref(patch.PRURL)); err != nil {
		return StoryUpdate{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return StoryUpdate{}, err
	}
	res, err := g.Query(`
		MATCH (w:Workspace), (s:Story {id: $id})
		WITH w, s, coalesce($activity, s.activity) AS activity, coalesce(w.next_log, 1) AS n,
			[coalesce(s.title, ''), coalesce(s.description, ''), coalesce(s.goal, ''), coalesce(s.plan, ''),
				coalesce(s.activity, ''), coalesce(s.waiting_on, ''), coalesce(s.branch, ''), coalesce(s.pr_url, ''), coalesce(s.kind, '')] AS old
		WITH w, s, activity, n, old, CASE
			WHEN $activity IS NOT NULL AND s.status <> 'in_progress' THEN 'not started'
			WHEN $waiting IS NOT NULL AND coalesce(activity, '') <> 'waiting' THEN 'not waiting'
		END AS violation,
			$goal IS NOT NULL AND $goal <> coalesce(s.goal, '') AS goalChanged,
			$plan IS NOT NULL AND $plan <> coalesce(s.plan, '') AS planChanged
		OPTIONAL MATCH p = (d:Story)-[:DEPENDS_ON*1..]->(s)
			WHERE violation IS NULL AND (goalChanged OR planChanged)
				AND all(x IN nodes(p) WHERE x = s OR NOT x.status IN $done)
		WITH w, s, activity, n, old, violation, goalChanged, planChanged, collect(DISTINCT d) AS ds,
			CASE WHEN goalChanged AND planChanged THEN 'goal and plan' WHEN goalChanged THEN 'goal' ELSE 'plan' END AS what
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
			SET s.title = coalesce($title, s.title),
				s.description = coalesce($description, s.description),
				s.goal = coalesce($goal, s.goal),
				s.plan = coalesce($plan, s.plan),
				s.activity = activity,
				s.waiting_on = CASE WHEN activity = 'waiting' THEN coalesce($waiting, s.waiting_on) END,
				s.branch = coalesce($branch, s.branch),
				s.pr_url = coalesce($pr, s.pr_url),
				s.kind = CASE WHEN $kind IS NULL THEN s.kind WHEN $kind = '' THEN null ELSE $kind END,
				s.updated_by = $actor, s.updated_at = $now
			FOREACH (__ IN CASE WHEN goalChanged OR planChanged THEN [1] ELSE [] END |
				SET s.upstream_id = null, s.upstream_at = null, w.next_log = n + size(ds))
			FOREACH (i IN range(0, size(ds) - 1) | FOREACH (d IN [ds[i]] |
				SET d.upstream_id = $id, d.upstream_at = $now
				CREATE (d)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n + i), at: $now, actor: $actor, kind: 'note',
					text: 'The ' + what + ' of ' + $id + ' changed; review this story'}))))
		RETURN violation, s.status AS status, [d IN ds | d.id] AS marked, old`,
		map[string]any{"id": id, "title": opt(patch.Title), "description": opt(patch.Description),
			"goal": opt(patch.Goal), "plan": opt(patch.Plan), "activity": opt(patch.Activity),
			"waiting": opt(patch.WaitingOn), "branch": opt(patch.Branch), "pr": opt(patch.PRURL), "kind": opt(patch.Kind),
			"done": append(slices.Clone(complete), Closed), "actor": actor, "now": now()}, nil)
	if err != nil {
		return StoryUpdate{}, fmt.Errorf("update story: %w", err)
	}
	if !res.Next() {
		return StoryUpdate{}, refuse(NotFound, "no story %q", id)
	}
	var marked, old []string
	switch r := res.Record(); str(r, "violation") {
	case "not started":
		return StoryUpdate{}, refuse(Conflict, "%s is %s; activity is set only while a story is in progress", id, label(str(r, "status")))
	case "not waiting":
		return StoryUpdate{}, refuse(Invalid, "waiting_on is set only while a story's activity is waiting")
	default:
		marked = byNumber(strs(r, "marked"))
		for _, m := range marked {
			b.changed(nil, Change{ws, KindStory, m})
		}
		old = strs(r, "old")
	}
	s, err := story(g, id)
	if err != nil {
		return StoryUpdate{}, err
	}
	given := []struct {
		name string
		to   *string
	}{{"title", patch.Title}, {"description", patch.Description}, {"goal", patch.Goal}, {"plan", patch.Plan},
		{"activity", patch.Activity}, {"waiting_on", patch.WaitingOn}, {"branch", patch.Branch}, {"pr_url", patch.PRURL}, {"kind", patch.Kind}}
	changed := []string{}
	for i, f := range given {
		if f.to != nil && i < len(old) && *f.to != old[i] {
			changed = append(changed, f.name)
		}
	}
	return StoryUpdate{Story: s, Changed: changed, Marked: marked}, nil
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
		OPTIONAL MATCH (rel:Release)<-[:IN_RELEASE]-(s)
		OPTIONAL MATCH (s)-[:DEPENDS_ON]->(t:Story)
		WITH s, p, rel, collect(t.id) AS blocked_by,
			collect(CASE WHEN NOT t.status IN $complete THEN t.id END) AS waiting
		OPTIONAL MATCH (d:Story)-[:DEPENDS_ON]->(s)
		WITH s, p, rel, blocked_by, waiting, collect(d.id) AS blocks
		OPTIONAL MATCH (s)-[:FOUND_IN]->(fi:Story)
		WITH s, p, rel, blocked_by, waiting, blocks, fi
		OPTIONAL MATCH (fd:Story)-[:FOUND_IN]->(s)
		WITH s, p, rel, blocked_by, waiting, blocks, fi, collect(fd.id) AS found
		RETURN s.id AS id, s.title AS title, s.description AS description, s.goal AS goal,
			s.plan AS plan, s.kind AS kind, fi.id AS found_in, found, s.status AS status, p.number AS phase, blocked_by, blocks, waiting,
			rel.id AS release_id, rel.version AS release_version,
			s.held_from AS held_from, s.hold_reason AS hold_reason, s.activity AS activity,
			s.waiting_on AS waiting_on, s.branch AS branch, s.into AS into, s.pr_url AS pr_url, s.workaround AS workaround,
			s.created_by AS created_by, s.created_at AS created_at,
			s.moved_by AS moved_by, s.moved_at AS moved_at,
			s.updated_by AS updated_by, s.updated_at AS updated_at,
			s.upstream_id AS upstream_id, s.upstream_at AS upstream_at
		ORDER BY s.rank`,
		map[string]any{"id": id, "complete": complete}, nil)
	if err != nil {
		return nil, fmt.Errorf("read stories: %w", err)
	}
	out := []Story{}
	for res.Next() {
		r := res.Record()
		waiting := byNumber(strs(r, "waiting"))
		var rel *ReleaseRef
		if id := str(r, "release_id"); id != "" {
			rel = &ReleaseRef{ID: id, Version: str(r, "release_version")}
		}
		var up *UpstreamRef
		if id := str(r, "upstream_id"); id != "" {
			up = &UpstreamRef{Story: id, At: str(r, "upstream_at")}
		}
		out = append(out, Story{
			ID: str(r, "id"), Title: str(r, "title"), Description: str(r, "description"),
			Goal: str(r, "goal"), Plan: str(r, "plan"), Kind: str(r, "kind"), FoundIn: str(r, "found_in"), Found: byNumber(strs(r, "found")), Status: str(r, "status"), Phase: num(r, "phase"),
			BlockedBy: byNumber(strs(r, "blocked_by")), Blocks: byNumber(strs(r, "blocks")),
			Blocked: len(waiting) > 0, waiting: waiting,
			HeldFrom: str(r, "held_from"), HoldReason: str(r, "hold_reason"), Activity: str(r, "activity"),
			WaitingOn: str(r, "waiting_on"), Branch: str(r, "branch"), Into: str(r, "into"), PRURL: str(r, "pr_url"),
			Workaround: str(r, "workaround"), Release: rel,
			CreatedBy: str(r, "created_by"), CreatedAt: str(r, "created_at"),
			MovedBy: str(r, "moved_by"), MovedAt: str(r, "moved_at"),
			UpdatedBy: str(r, "updated_by"), UpdatedAt: str(r, "updated_at"), UpstreamChanged: up,
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

// MarkReviewed clears a story's upstream-changed mark, logging a note with the
// reason: the story was looked at and the changed plan or goal needs nothing
// from it. A story that is not marked is refused.
func (b *Board) MarkReviewed(ws, actor, id, reason string) (_ Story, err error) {
	defer func() { b.changed(err, Change{ws, KindStory, id}) }()
	if reason == "" {
		return Story{}, refuse(Invalid, "a reason is required: say what was checked or changed")
	}
	if err := plainText(reason); err != nil {
		return Story{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Story{}, err
	}
	res, err := g.Query(`
		MATCH (w:Workspace), (s:Story {id: $id})
		WITH w, s, coalesce(w.next_log, 1) AS n, s.upstream_id AS up,
			CASE WHEN s.upstream_id IS NULL THEN 'unmarked' END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
			SET w.next_log = n + 1, s.upstream_id = null, s.upstream_at = null
			CREATE (s)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n), at: $now, actor: $actor, kind: 'note',
				text: 'Reviewed after ' + up + ' changed: ' + $reason}))
		RETURN violation`,
		map[string]any{"id": id, "reason": reason, "actor": actor, "now": now()}, nil)
	if err != nil {
		return Story{}, fmt.Errorf("mark reviewed: %w", err)
	}
	if !res.Next() {
		return Story{}, refuse(NotFound, "no story %q", id)
	}
	if str(res.Record(), "violation") != "" {
		return Story{}, refuse(Conflict, "%s is not marked as upstream changed; there is nothing to review", id)
	}
	return story(g, id)
}
