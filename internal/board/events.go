package board

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Event is one thing that happened on the board, as a short line that starts
// with its own position (ID): L-n for a log entry, S-n for a story added, I-n
// for an idea added and R-n for a release cut. Kind is the log entry's kind
// (decision, correction or status), or story, idea or release; Story is the
// story a log entry belongs to; To is the status a status entry moved to.
type Event struct {
	ID    string `json:"id"`
	At    string `json:"at"`
	Kind  string `json:"kind"`
	Story string `json:"story,omitempty"`
	To    string `json:"to,omitempty"`
	Line  string `json:"line"`
}

// EventsQuery picks which events to read; give at most one. With none, it is
// the latest page. Before is a position: the page ending just before it.
// After is a position: every event from its second on, except itself (the
// rest of its own second repeats, so none is missed). Since is an RFC 3339
// time, or a bare date taken as midnight UTC: every event from then on.
type EventsQuery struct {
	Before, After, Since string
}

const (
	eventsPage = 20
	// eventTextMax cuts a decision's text, its reason and a title in a line.
	eventTextMax = 100
)

var position = regexp.MustCompile(`^([SIRL])-([0-9]+)$`)

// kindRank orders the events of one second: stories, ideas, releases, logs.
var kindRank = map[string]int{"S": 1, "I": 2, "R": 3, "L": 4}

// eventsQuery collects the four kinds of event in one read-only query. The
// anchor is the event, or any log entry, story, idea or release, a position
// names. Rows are dropped inside each collect, never before it, so an empty
// kind cannot lose the anchor. The same release's entries are not repeated:
// a story it released has a released entry at the release's own time. A
// proposal's placement decision is written at the story's own time and is
// folded into the story's added line.
const eventsQuery = `
	OPTIONAL MATCH (al:LogEntry {id: $pos})
	OPTIONAL MATCH (ast:Story {id: $pos})
	OPTIONAL MATCH (ai:Idea {id: $pos})
	OPTIONAL MATCH (ar:Release {id: $pos})
	WITH coalesce(al.at, ast.created_at, ai.created_at, ar.created_at) AS pat
	OPTIONAL MATCH (s:Story)-[:LOGGED]->(e:LogEntry)
	OPTIONAL MATCH (e)-[:CORRECTS]->(ce:LogEntry)
	OPTIONAL MATCH (s)-[:IN_RELEASE]->(rr:Release)
	WITH pat, collect(CASE WHEN e.at IS NOT NULL AND (
			(e.kind IN ['decision', 'correction'] AND NOT (e.kind = 'decision' AND e.at = s.created_at AND
				(coalesce(e.text, '') STARTS WITH 'Proposed' OR coalesce(e.text, '') STARTS WITH 'Straight into the backlog')))
			OR (e.kind = 'status' AND (e.to IN ['merged', 'on_hold', 'closed', 'deployed'] OR
				(e.to = 'released' AND NOT (rr IS NOT NULL AND rr.id <> 'R-0' AND rr.created_at = e.at)))))
		THEN {id: e.id, at: e.at, t: 4, n: toInteger(substring(e.id, 2)), kind: e.kind, story: s.id, to: e.to,
			text: e.text, reason: e.reason, commit: e.commit, into: e.into, other: ce.id} END) AS logs
	OPTIONAL MATCH (st:Story)
	OPTIONAL MATCH (st)-[:FOUND_IN]->(f:Story)
	WITH pat, logs, collect(CASE WHEN st.created_at IS NOT NULL
		THEN {id: st.id, at: st.created_at, t: 1, n: toInteger(substring(st.id, 2)), kind: 'story', text: st.title, other: f.id} END) AS adds
	OPTIONAL MATCH (i:Idea)
	WITH pat, logs, adds, collect(CASE WHEN i.created_at IS NOT NULL
		THEN {id: i.id, at: i.created_at, t: 2, n: toInteger(substring(i.id, 2)), kind: 'idea', text: i.title} END) AS ideas
	OPTIONAL MATCH (r:Release) WHERE r.id <> 'R-0'
	OPTIONAL MATCH (r)<-[:IN_RELEASE]-(m:Story)
	WITH pat, logs, adds, ideas, r, count(m) AS c
	WITH pat, logs, adds, ideas, collect(CASE WHEN r.created_at IS NOT NULL
		THEN {id: r.id, at: r.created_at, t: 3, n: toInteger(substring(r.id, 2)), kind: 'release', text: r.version, other: r.tag, count: c} END) AS rels
	UNWIND CASE WHEN $pos <> '' AND pat IS NULL THEN [{missing: true}] ELSE logs + adds + ideas + rels END AS ev
	WITH ev, pat
	WHERE ev.missing = true OR (ev.id IS NOT NULL AND (%s))
	RETURN ev.missing AS missing, ev.id AS id, ev.at AS at, ev.t AS t, ev.n AS n, ev.kind AS kind, ev.story AS story, ev.to AS to,
		ev.text AS text, ev.reason AS reason, ev.commit AS commit, ev.into AS into, ev.other AS other, ev.count AS count
	ORDER BY at DESC, t DESC, n DESC %s`

// Events reads the latest events, however old, oldest first, one short line
// each, in one query. Nothing has happened on an empty board, and only then
// is the answer empty (or when an option leaves nothing). After and Since are
// not limited, but a result is capped by the store at 10,000 rows.
func (b *Board) Events(ws string, q EventsQuery) ([]Event, error) {
	given := 0
	for _, o := range []string{q.Before, q.After, q.Since} {
		if o != "" {
			given++
		}
	}
	if given > 1 {
		return nil, refuse(Invalid, "give one of before, after or since")
	}
	filter, limit, pos, since := "true", "LIMIT $lim", "", ""
	switch {
	case q.Before != "":
		pos, filter = q.Before, "ev.at < pat OR (ev.at = pat AND (ev.t < $pt OR (ev.t = $pt AND ev.n < $pn)))"
	case q.After != "":
		pos, filter, limit = q.After, "ev.at >= pat AND ev.id <> $pos", ""
	case q.Since != "":
		t, err := parseSince(q.Since)
		if err != nil {
			return nil, err
		}
		since, filter, limit = t.UTC().Format(time.RFC3339), "ev.at >= $since", ""
	}
	pt, pn := 0, 0
	if pos != "" {
		m := position.FindStringSubmatch(pos)
		if m == nil {
			return nil, refuse(Invalid, "%q is not a position: use an id such as L-41, S-3, I-2 or R-1", pos)
		}
		pt = kindRank[m[1]]
		pn, _ = strconv.Atoi(m[2])
	}
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	res, err := g.ROQuery(fmt.Sprintf(eventsQuery, filter, limit),
		map[string]any{"pos": pos, "pt": pt, "pn": pn, "since": since, "lim": eventsPage}, nil)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}
	evs := []Event{}
	for res.Next() {
		r := res.Record()
		if flag(r, "missing") {
			return nil, refuse(NotFound, "no event, story, idea or release %q", pos)
		}
		e := Event{ID: str(r, "id"), At: str(r, "at"), Kind: str(r, "kind"), Story: str(r, "story"), To: str(r, "to")}
		e.Line = eventLine(e, r.Get)
		evs = append(evs, e)
	}
	slices.Reverse(evs) // read newest first for the limit; given oldest first
	return evs, nil
}

// parseSince reads an RFC 3339 time, or a bare date as midnight UTC.
func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	return time.Time{}, refuse(Invalid, "since %q is not a time: use RFC 3339 such as 2026-10-31T09:00:00Z, or a date such as 2026-10-31", s)
}

// eventLine writes an event as one line: its position, its time to the
// minute, then what happened. get reads the rest of the row.
func eventLine(e Event, get func(string) (any, bool)) string {
	field := func(k string) string {
		v, _ := get(k)
		s, _ := v.(string)
		return s
	}
	when := e.At
	if t, err := time.Parse(time.RFC3339, e.At); err == nil {
		when = t.UTC().Format("2006-01-02 15:04")
	}
	head := e.ID + " " + when + " "
	text, reason, other := field("text"), field("reason"), field("other")
	switch e.Kind {
	case "story":
		line := head + "story added: " + cut(text)
		if other != "" {
			line += " (found in " + other + ")"
		}
		return line
	case "idea":
		return head + "idea added: " + cut(text)
	case "release":
		v, _ := get("count")
		n, _ := v.(int64)
		stories := "stories"
		if n == 1 {
			stories = "story"
		}
		return fmt.Sprintf("%srelease %s (tag %s): %d %s", head, cut(text), cut(other), n, stories)
	}
	head += e.Story + " "
	switch {
	case e.Kind == "decision":
		return head + "decision: " + cut(text) + joinReason(reason)
	case e.Kind == "correction" && other != "":
		return head + "correction of " + other + ": " + cut(text)
	case e.Kind == "correction":
		return head + "correction: " + cut(text)
	}
	switch e.To {
	case "merged":
		line := head + "merged"
		if into := field("into"); into != "" {
			line += " into " + into
		}
		if c := []rune(field("commit")); len(c) > 0 {
			line += " (" + string(c[:min(len(c), 7)]) + ")"
		}
		return line
	case "on_hold":
		return head + "on hold" + colonReason(reason)
	case "closed":
		return head + "closed" + colonReason(reason)
	case "released": // only an old one remains: a cut's own entries are not events
		return head + "reached the release branch (recorded before releases)"
	default: // deployed
		return head + "released (recorded before releases)"
	}
}

func joinReason(reason string) string {
	if reason == "" {
		return ""
	}
	return " — " + cut(reason)
}

func colonReason(reason string) string {
	if reason == "" {
		return ""
	}
	return ": " + cut(reason)
}

// cut keeps the first eventTextMax characters of s, ending in an ellipsis
// when it cuts, and puts a long line's text on one line.
func cut(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > eventTextMax {
		return string(r[:eventTextMax-1]) + "…"
	}
	return s
}
