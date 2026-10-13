package board

import (
	"fmt"
	"slices"
	"strings"
)

// The columns a story moves through, then the two it can be pulled into.
// Proposed holds an agent's new story until a person approves it. A merged
// story is in the column of the branch it reached (Story.Into), so there is
// one Merged column per branch. Released is the last step and is reached only
// by cutting a release (CutRelease), never by a status change.
const (
	Proposed   = "proposed"
	Backlog    = "backlog"
	InProgress = "in_progress"
	InReview   = "in_review"
	Merged     = "merged"
	Released   = "released"
	OnHold     = "on_hold"
	Closed     = "closed"
)

// statuses is every status, in board order.
var statuses = []string{Proposed, OnHold, Backlog, InProgress, InReview, Merged, Released, Closed}

// complete is every status in which a story's work has landed.
var complete = []string{Merged, Released}

// moves is every allowed change of status but those that depend on the story
// or its branches: a story on hold resumes in the column it left, a merge
// goes to a particular branch. Nothing moves into Released here: a release
// takes the stories.
var moves = []string{
	Proposed + ">" + Backlog, Proposed + ">" + OnHold, Proposed + ">" + Closed,
	Backlog + ">" + InProgress, InProgress + ">" + InReview, InReview + ">" + Merged,
	Merged + ">" + Merged,
	InProgress + ">" + Backlog, InReview + ">" + InProgress,
	Backlog + ">" + OnHold, InProgress + ">" + OnHold, InReview + ">" + OnHold,
	Backlog + ">" + Closed, InProgress + ">" + Closed, InReview + ">" + Closed, OnHold + ">" + Closed,
}

// Move is a change of status a story may make. Into names the branch when it
// is a merge.
type Move struct {
	To   string
	Into string
}

// String reads "merged:main" for a merge and the status alone otherwise.
func (m Move) String() string {
	if m.Into == "" {
		return m.To
	}
	return m.To + ":" + m.Into
}

// ParseMove reads what Move.String wrote.
func ParseMove(s string) Move {
	to, into, _ := strings.Cut(s, ":")
	return Move{To: to, Into: into}
}

// startingBranches are the branches nothing merges into, in merge order: where
// a story from review is merged first. A workspace has one until a branch is
// added beside it (staging into main while dev still merges into main); then a
// merge from review has to name which.
func startingBranches(bs []Branch) []string {
	var out []string
	for _, b := range bs {
		if !slices.ContainsFunc(bs, func(o Branch) bool { return o.Into == b.Name }) {
			out = append(out, b.Name)
		}
	}
	return out
}

// Moves lists the changes s can make, in board order, for a workspace with
// the given branches. A merge names its branch: from review there is one per
// starting branch. SetStatus still checks the rest.
func Moves(s Story, bs []Branch) []Move {
	var to []Move
	for _, st := range statuses {
		ok := slices.Contains(moves, s.Status+">"+st) || (s.Status == OnHold && st == s.HeldFrom)
		m := Move{To: st}
		if st == Merged && ok {
			var into []string
			switch s.Status {
			case InReview:
				into = startingBranches(bs)
			case Merged:
				i := slices.IndexFunc(bs, func(b Branch) bool { return b.Name == s.Into })
				if i >= 0 && bs[i].Into != "" {
					into = []string{bs[i].Into}
				}
			}
			for _, name := range into {
				to = append(to, Move{To: Merged, Into: name})
			}
			continue
		}
		if ok {
			to = append(to, m)
		}
	}
	return to
}

// StatusChange moves a story to another column. Branch is given only when
// starting, PRURL when opening review, Commit and Into when merging (Into
// names the branch merged into; with none, the only starting branch, and
// with several the merge is refused until it names one), Workaround when
// starting a blocked story, and Reason when holding or closing.
type StatusChange struct {
	To         string `json:"to"`
	Branch     string `json:"branch,omitempty"`
	PRURL      string `json:"pr_url,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Into       string `json:"into,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Workaround string `json:"workaround,omitempty"`
}

func (c StatusChange) check() error {
	if c.To == Released || c.To == "deployed" {
		return refuse(Invalid, "a story is released by cutting a release, not by a status change; use cut_release (deployed is no longer a status)")
	}
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
		only("into", c.Into, Merged, "merging"),
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
	return plainText(c.Branch, c.PRURL, c.Commit, c.Into, c.Reason, c.Workaround)
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
		OPTIONAL MATCH (br:Branch)
		WITH w, s, waiting, dependents, from, n, collect(br.name) AS names, collect(br.into) AS intos,
			collect(CASE WHEN br.name = s.into THEN br.into END) AS afters
		WITH w, s, waiting, dependents, from, n, names, intos, afters[0] AS after,
			[x IN names WHERE NOT x IN intos] AS starts
		WITH w, s, waiting, dependents, from, n, names, after, starts,
			CASE WHEN from = 'merged' THEN $into
				ELSE coalesce($into, CASE WHEN size(starts) = 1 THEN starts[0] END) END AS target
		WITH w, s, waiting, dependents, from, n, target, starts, after, CASE
			WHEN NOT ((from + '>' + $to) IN $moves OR (from = 'on_hold' AND $to = s.held_from)) THEN 'move'
			WHEN $to = 'merged' AND $into IS NOT NULL AND NOT $into IN names THEN 'no branch'
			WHEN from = 'in_review' AND $to = 'merged' AND $into IS NULL AND size(starts) > 1 THEN 'name'
			WHEN from = 'in_review' AND $to = 'merged' AND (target IS NULL OR NOT target IN starts) THEN 'skip'
			WHEN from = 'merged' AND $to = 'merged' AND after IS NULL THEN 'last'
			WHEN from = 'merged' AND $to = 'merged' AND target IS NULL THEN 'next'
			WHEN from = 'merged' AND $to = 'merged' AND target <> after THEN 'next'
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
				s.into = CASE WHEN $to = 'merged' THEN target ELSE s.into END,
				s.pr_url = coalesce($pr, s.pr_url),
				s.workaround = CASE WHEN $to = 'backlog' THEN null WHEN $workaround = '' THEN s.workaround ELSE $workaround END,
				w.next_log = n + CASE WHEN $workaround = '' THEN 1 ELSE 2 END
			CREATE (s)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n), at: $now, actor: $actor, kind: 'status',
				from: from, to: $to, reason: $reason, branch: $branch, pr_url: $pr, commit: $commit,
				into: CASE WHEN $to = 'merged' THEN target END})
			FOREACH (__ IN CASE WHEN $workaround = '' THEN [] ELSE [1] END |
				CREATE (s)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n + 1), at: $now, actor: $actor,
					kind: 'decision', reason: $workaround, text: 'Started while blocked by ' +
					reduce(t = '', x IN waiting | t + CASE WHEN t = '' THEN '' ELSE ', ' END + x)})))
		RETURN violation, from, s.held_from AS held_from, waiting, dependents, starts, after, s.into AS into`,
		map[string]any{
			"id": id, "to": c.To, "moves": moves, "complete": complete, "actor": actor, "now": now(),
			"reason": orNull(c.Reason), "branch": orNull(c.Branch), "pr": orNull(c.PRURL), "commit": orNull(c.Commit), "into": orNull(c.Into),
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
	case "no branch":
		return Story{}, refuse(Conflict, "%s has no branch %q", ws, c.Into)
	case "name":
		return Story{}, refuse(Conflict, "%s is in review and several branches start the line; name the branch: %s", id, orList(strs(r, "starts")))
	case "skip":
		return Story{}, refuse(Conflict, "%s is in review; merge it into %s first, then on from there", id, orList(strs(r, "starts")))
	case "last":
		return Story{}, refuse(Conflict, "%s is already in %s, which merges nowhere", id, str(r, "into"))
	case "next":
		return Story{}, refuse(Conflict, "%s is merged into %s; its next merge is into %s", id, str(r, "into"), str(r, "after"))
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

// orList joins names for a choice: "dev or staging", "a, b or c".
func orList(names []string) string {
	names = slices.Clone(names)
	slices.Sort(names)
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
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
