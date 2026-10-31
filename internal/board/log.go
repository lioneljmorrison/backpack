package board

import (
	"fmt"
	"slices"
)

// LogEntry is one append-only line in the log of a story, a phase or a goal.
// An "edit" entry (written by a phase or goal edit, never by a caller) names
// the Field changed, with its old value in From and its new one in To.
type LogEntry struct {
	ID          string   `json:"id"`
	At          string   `json:"at"`
	Actor       string   `json:"actor"`
	Kind        string   `json:"kind"`
	Field       string   `json:"field,omitempty"`
	Text        string   `json:"text,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Commit      string   `json:"commit,omitempty"`
	PRURL       string   `json:"pr_url,omitempty"`
	Branch      string   `json:"branch,omitempty"`
	Into        string   `json:"into,omitempty"`
	From        string   `json:"from,omitempty"`
	To          string   `json:"to,omitempty"`
	Corrects    string   `json:"corrects,omitempty"`
	CorrectedBy []string `json:"corrected_by,omitempty"`
}

// NewLogEntry is what AddLogEntry needs. A note needs text; a decision,
// text and its reason; a commit, its id; a pull request, its link; and a
// correction, text and the entry it corrects.
type NewLogEntry struct {
	Kind     string `json:"kind"`
	Text     string `json:"text"`
	Reason   string `json:"reason"`
	Commit   string `json:"commit"`
	PRURL    string `json:"pr_url"`
	Corrects string `json:"corrects"`
}

// needs is what each kind of entry a caller may write must carry.
var needs = map[string]string{
	"note": "text", "decision": "text and reason", "commit": "commit", "pr": "pr_url",
	"correction": "text and corrects",
}

func (e NewLogEntry) check() error {
	if e.Kind == "status" {
		return refuse(Invalid, "status entries are written only by status changes")
	}
	need, ok := needs[e.Kind]
	if !ok {
		return refuse(Invalid, "kind is one of note, decision, commit, pr, correction")
	}
	missing := map[string]bool{
		"note":       e.Text == "",
		"decision":   e.Text == "" || e.Reason == "",
		"commit":     e.Commit == "",
		"pr":         e.PRURL == "",
		"correction": e.Text == "" || e.Corrects == "",
	}[e.Kind]
	if missing {
		return refuse(Invalid, "a %s entry needs %s", e.Kind, need)
	}
	only := func(field, value, kind string) error {
		if value != "" && e.Kind != kind {
			return refuse(Invalid, "%s belongs only to a %s entry", field, kind)
		}
		return nil
	}
	for _, err := range []error{
		only("reason", e.Reason, "decision"),
		only("commit", e.Commit, "commit"),
		only("pr_url", e.PRURL, "pr"),
		only("corrects", e.Corrects, "correction"),
	} {
		if err != nil {
			return err
		}
	}
	return plainText(e.Text, e.Reason, e.Commit, e.PRURL, e.Corrects)
}

// AddLogEntry appends an entry to a story's log. Entries are never edited
// or deleted; a correction points to an earlier entry of the same story.
func (b *Board) AddLogEntry(ws, actor, id string, e NewLogEntry) (_ LogEntry, err error) {
	defer func() { b.changed(err, Change{ws, KindStory, id}) }()
	if err := e.check(); err != nil {
		return LogEntry{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return LogEntry{}, err
	}
	at := now()
	res, err := g.Query(`
		MATCH (w:Workspace), (s:Story {id: $id})
		OPTIONAL MATCH (s)-[:LOGGED]->(c:LogEntry {id: $corrects})
		WITH w, s, c, coalesce(w.next_log, 1) AS n,
			CASE WHEN $corrects <> '' AND c IS NULL THEN 'corrects' END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
			SET w.next_log = n + 1
			CREATE (s)-[:LOGGED]->(e:LogEntry {id: 'L-' + toString(n), at: $at, actor: $actor, kind: $kind,
				text: $text, reason: $reason, commit: $commit, pr_url: $pr})
			FOREACH (x IN CASE WHEN c IS NULL THEN [] ELSE [c] END | CREATE (e)-[:CORRECTS]->(x)))
		RETURN violation, 'L-' + toString(n) AS id`,
		map[string]any{"id": id, "corrects": e.Corrects, "at": at, "actor": actor, "kind": e.Kind,
			"text": orNull(e.Text), "reason": orNull(e.Reason), "commit": orNull(e.Commit), "pr": orNull(e.PRURL)}, nil)
	if err != nil {
		return LogEntry{}, fmt.Errorf("add log entry: %w", err)
	}
	if !res.Next() {
		return LogEntry{}, refuse(NotFound, "no story %q", id)
	}
	r := res.Record()
	if str(r, "violation") != "" {
		return LogEntry{}, refuse(NotFound, "%s has no log entry %q", id, e.Corrects)
	}
	return LogEntry{ID: str(r, "id"), At: at, Actor: actor, Kind: e.Kind, Text: e.Text, Reason: e.Reason,
		Commit: e.Commit, PRURL: e.PRURL, Corrects: e.Corrects}, nil
}

// Log reads a story's log, oldest entry first.
func (b *Board) Log(ws, id string) ([]LogEntry, error) {
	return b.readLog(ws, "(s:Story {id: $id})", id, refuse(NotFound, "no story %q", id))
}

// PhaseLog reads a phase's log, oldest entry first.
func (b *Board) PhaseLog(ws string, number int) ([]LogEntry, error) {
	return b.readLog(ws, "(s:Phase {number: $id})", number, refuse(NotFound, "no phase %d", number))
}

// GoalLog reads a goal's log, oldest entry first.
func (b *Board) GoalLog(ws, id string) ([]LogEntry, error) {
	return b.readLog(ws, "(s:Goal {id: $id})", id, refuse(NotFound, "no goal %q", id))
}

// readLog reads the log of the one node the pattern names (as s), or
// returns missing when there is none.
func (b *Board) readLog(ws, pattern string, id any, missing error) ([]LogEntry, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	res, err := g.ROQuery(`
		MATCH `+pattern+`
		OPTIONAL MATCH (s)-[:LOGGED]->(e:LogEntry)
		OPTIONAL MATCH (e)-[:CORRECTS]->(c:LogEntry)
		OPTIONAL MATCH (by:LogEntry)-[:CORRECTS]->(e)
		RETURN e.id AS id, e.at AS at, e.actor AS actor, e.kind AS kind, e.field AS field, e.text AS text,
			e.reason AS reason, e.commit AS commit, e.pr_url AS pr_url, e.branch AS branch, e.into AS into,
			e.from AS from, e.to AS to, c.id AS corrects, collect(by.id) AS corrected_by`,
		map[string]any{"id": id}, nil)
	if err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}
	out, found := []LogEntry{}, false
	for res.Next() {
		r := res.Record()
		found = true
		if str(r, "id") == "" {
			continue // nothing logged yet
		}
		out = append(out, LogEntry{
			ID: str(r, "id"), At: str(r, "at"), Actor: str(r, "actor"), Kind: str(r, "kind"), Field: str(r, "field"),
			Text: str(r, "text"), Reason: str(r, "reason"), Commit: str(r, "commit"), PRURL: str(r, "pr_url"),
			Branch: str(r, "branch"), Into: str(r, "into"), From: str(r, "from"), To: str(r, "to"), Corrects: str(r, "corrects"),
			CorrectedBy: byNumber(strs(r, "corrected_by")),
		})
	}
	if !found {
		return nil, missing
	}
	slices.SortFunc(out, func(a, b LogEntry) int { return seq(a.ID) - seq(b.ID) })
	return out, nil
}
