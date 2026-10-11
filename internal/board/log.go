package board

import (
	"fmt"
	"slices"
)

// LogEntry is one append-only line in a story's log.
type LogEntry struct {
	ID          string   `json:"id"`
	At          string   `json:"at"`
	Actor       string   `json:"actor"`
	Kind        string   `json:"kind"`
	Text        string   `json:"text,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Commit      string   `json:"commit,omitempty"`
	PRURL       string   `json:"pr_url,omitempty"`
	Branch      string   `json:"branch,omitempty"`
	From        string   `json:"from,omitempty"`
	To          string   `json:"to,omitempty"`
	Corrects    string   `json:"corrects,omitempty"`
	CorrectedBy []string `json:"corrected_by,omitempty"`
}

// Log reads a story's log, oldest entry first.
func (b *Board) Log(ws, id string) ([]LogEntry, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	res, err := g.ROQuery(`
		MATCH (s:Story {id: $id})
		OPTIONAL MATCH (s)-[:LOGGED]->(e:LogEntry)
		OPTIONAL MATCH (e)-[:CORRECTS]->(c:LogEntry)
		OPTIONAL MATCH (by:LogEntry)-[:CORRECTS]->(e)
		RETURN e.id AS id, e.at AS at, e.actor AS actor, e.kind AS kind, e.text AS text,
			e.reason AS reason, e.commit AS commit, e.pr_url AS pr_url, e.branch AS branch,
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
			continue // the story has no entries yet
		}
		out = append(out, LogEntry{
			ID: str(r, "id"), At: str(r, "at"), Actor: str(r, "actor"), Kind: str(r, "kind"),
			Text: str(r, "text"), Reason: str(r, "reason"), Commit: str(r, "commit"), PRURL: str(r, "pr_url"),
			Branch: str(r, "branch"), From: str(r, "from"), To: str(r, "to"), Corrects: str(r, "corrects"),
			CorrectedBy: byNumber(strs(r, "corrected_by")),
		})
	}
	if !found {
		return nil, refuse(NotFound, "no story %q", id)
	}
	slices.SortFunc(out, func(a, b LogEntry) int { return seq(a.ID) - seq(b.ID) })
	return out, nil
}
