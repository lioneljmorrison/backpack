package board

import (
	"fmt"
	"slices"
	"strings"
)

// The columns a story moves through, then the two it can be pulled into.
// Proposed holds an agent's new story until a person approves it.
const (
	Proposed   = "proposed"
	Backlog    = "backlog"
	InProgress = "in_progress"
	InReview   = "in_review"
	Merged     = "merged"
	Released   = "released"
	Deployed   = "deployed"
	OnHold     = "on_hold"
	Closed     = "closed"
)

// statuses is every status, in board order.
var statuses = []string{Proposed, OnHold, Backlog, InProgress, InReview, Merged, Released, Deployed, Closed}

// complete is every status in which a story's work has landed.
var complete = []string{Merged, Released, Deployed}

// moves is every allowed change of status but two, which depend on the story
// or its workspace: a story on hold resumes in the column it left, and
// Merged goes to Released or Deployed by the workspace's release stage.
var moves = []string{
	Proposed + ">" + Backlog, Proposed + ">" + OnHold, Proposed + ">" + Closed,
	Backlog + ">" + InProgress, InProgress + ">" + InReview, InReview + ">" + Merged,
	Merged + ">" + Released, Merged + ">" + Deployed, Released + ">" + Deployed,
	InProgress + ">" + Backlog, InReview + ">" + InProgress,
	Backlog + ">" + OnHold, InProgress + ">" + OnHold, InReview + ">" + OnHold,
	Backlog + ">" + Closed, InProgress + ">" + Closed, InReview + ">" + Closed, OnHold + ">" + Closed,
}

// Moves lists the statuses s can change to, in board order, for a workspace
// with or without a release stage. SetStatus still checks the rest.
func Moves(s Story, release bool) []string {
	var to []string
	for _, st := range statuses {
		ok := slices.Contains(moves, s.Status+">"+st) || (s.Status == OnHold && st == s.HeldFrom)
		if s.Status == Merged && (st == Released && !release || st == Deployed && release) {
			ok = false
		}
		if ok {
			to = append(to, st)
		}
	}
	return to
}

// StatusChange moves a story to another column. Branch is given only when
// starting, PRURL when opening review, Commit when merging, Workaround when
// starting a blocked story, and Reason when holding or closing.
type StatusChange struct {
	To         string `json:"to"`
	Branch     string `json:"branch,omitempty"`
	PRURL      string `json:"pr_url,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Workaround string `json:"workaround,omitempty"`
}

func (c StatusChange) check() error {
	if !slices.Contains(statuses, c.To) {
		return refuse(Invalid, "no status %q", c.To)
	}
	only := func(field, value, to, when string) error {
		if value != "" && c.To != to {
			return refuse(Invalid, "%s is given only when %s", field, when)
		}
		return nil
	}
	for _, err := range []error{
		only("branch", c.Branch, InProgress, "starting a story"),
		only("workaround", c.Workaround, InProgress, "starting a blocked story"),
		only("pr_url", c.PRURL, InReview, "opening review"),
		only("commit", c.Commit, Merged, "merging"),
	} {
		if err != nil {
			return err
		}
	}
	needsReason := c.To == OnHold || c.To == Closed
	if needsReason && c.Reason == "" {
		return refuse(Invalid, "a reason is required to put a story %s", label(c.To))
	}
	if !needsReason && c.Reason != "" {
		return refuse(Invalid, "reason is given only when holding or closing a story")
	}
	return plainText(c.Branch, c.PRURL, c.Commit, c.Reason, c.Workaround)
}

// SetStatus moves a story to another column and logs the change. A blocked
// story starts only with a workaround, which is logged as a decision, and is
// never merged. A story is closed only when nothing depends on it.
func (b *Board) SetStatus(ws, actor, id string, c StatusChange) (_ Story, err error) {
	defer func() { b.changed(err, Change{ws, KindStory, id}) }()
	if err := c.check(); err != nil {
		return Story{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Story{}, err
	}
	res, err := g.Query(`
		MATCH (w:Workspace), (s:Story {id: $id})
		OPTIONAL MATCH (s)-[:DEPENDS_ON]->(t:Story) WHERE NOT t.status IN $complete
		WITH w, s, collect(t.id) AS waiting
		OPTIONAL MATCH (d:Story)-[:DEPENDS_ON]->(s) WHERE d.status <> 'closed'
		WITH w, s, waiting, collect(d.id) AS dependents, s.status AS from, coalesce(w.next_log, 1) AS n
		OPTIONAL MATCH (br:Branch) WHERE br.into IS NOT NULL
		WITH w, s, waiting, dependents, from, n, count(br) > 0 AS release
		WITH w, s, waiting, dependents, from, n, CASE
			WHEN NOT ((from + '>' + $to) IN $moves OR (from = 'on_hold' AND $to = s.held_from)) THEN 'move'
			WHEN from = 'merged' AND $to = 'released' AND NOT release THEN 'no release'
			WHEN from = 'merged' AND $to = 'deployed' AND release THEN 'release'
			WHEN $to = 'closed' AND size(dependents) > 0 THEN 'dependents'
			WHEN $to = 'merged' AND size(waiting) > 0 THEN 'blocked merge'
			WHEN $workaround <> '' AND from <> 'backlog' THEN 'not starting'
			WHEN $workaround <> '' AND size(waiting) = 0 THEN 'not blocked'
			WHEN from = 'backlog' AND $to = 'in_progress' AND size(waiting) > 0 AND $workaround = '' THEN 'blocked'
		END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
			SET s.status = $to,
				s.held_from = CASE WHEN $to = 'on_hold' THEN from END,
				s.hold_reason = CASE WHEN $to = 'on_hold' THEN $reason END,
				s.activity = CASE WHEN $to = 'in_progress' THEN 'planning' END,
				s.waiting_on = null,
				s.branch = coalesce($branch, s.branch),
				s.pr_url = coalesce($pr, s.pr_url),
				s.workaround = CASE WHEN $to = 'backlog' THEN null WHEN $workaround = '' THEN s.workaround ELSE $workaround END,
				w.next_log = n + CASE WHEN $workaround = '' THEN 1 ELSE 2 END
			CREATE (s)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n), at: $now, actor: $actor, kind: 'status',
				from: from, to: $to, reason: $reason, branch: $branch, pr_url: $pr, commit: $commit})
			FOREACH (__ IN CASE WHEN $workaround = '' THEN [] ELSE [1] END |
				CREATE (s)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n + 1), at: $now, actor: $actor,
					kind: 'decision', reason: $workaround, text: 'Started while blocked by ' +
					reduce(t = '', x IN waiting | t + CASE WHEN t = '' THEN '' ELSE ', ' END + x)})))
		RETURN violation, from, s.held_from AS held_from, waiting, dependents`,
		map[string]any{
			"id": id, "to": c.To, "moves": moves, "complete": complete, "actor": actor, "now": now(),
			"reason": orNull(c.Reason), "branch": orNull(c.Branch), "pr": orNull(c.PRURL), "commit": orNull(c.Commit),
			"workaround": c.Workaround,
		}, nil)
	if err != nil {
		return Story{}, fmt.Errorf("set status: %w", err)
	}
	if !res.Next() {
		return Story{}, refuse(NotFound, "no story %q", id)
	}
	r := res.Record()
	from := str(r, "from")
	switch str(r, "violation") {
	case "":
		return story(g, id)
	case "move":
		if from == OnHold {
			return Story{}, refuse(Conflict, "%s is on hold from %s and resumes there", id, label(str(r, "held_from")))
		}
		return Story{}, refuse(Conflict, "%s cannot go from %s to %s", id, label(from), label(c.To))
	case "no release":
		return Story{}, refuse(Conflict, "%s has no release stage; a merged story goes to deployed", ws)
	case "release":
		return Story{}, refuse(Conflict, "%s has a release stage; a merged story is released first", ws)
	case "dependents":
		return Story{}, refuse(Conflict, "%s cannot be closed while %s depend on it; remove those dependencies first",
			id, list(strs(r, "dependents")))
	case "blocked merge":
		return Story{}, refuse(Conflict, "%s is blocked by %s and cannot be merged until it is complete",
			id, list(strs(r, "waiting")))
	case "not starting":
		return Story{}, refuse(Conflict, "a workaround is given only when starting %s from the backlog", id)
	case "not blocked":
		return Story{}, refuse(Conflict, "%s is not blocked; a workaround is only for starting blocked work", id)
	default: // blocked
		return Story{}, refuse(Conflict, "%s is blocked by %s; give a workaround to start it anyway",
			id, list(strs(r, "waiting")))
	}
}

// label is a status as people write it: "in_review" reads "in review".
func label(status string) string { return strings.ReplaceAll(status, "_", " ") }

// orNull turns an empty optional value into a Cypher null, which is not stored.
func orNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}
