package board

import (
	"fmt"
)

// Release is an official release: what shipped together, under a version and
// the git tag that marks it. Commit and Notes are optional. Stories are the
// ids of the stories it holds, in delivery order. Backpack does not talk to
// GitHub; the caller records the tag as it records commits.
type Release struct {
	ID        string   `json:"id"`
	Version   string   `json:"version"`
	Tag       string   `json:"tag"`
	Commit    string   `json:"commit,omitempty"`
	Notes     string   `json:"notes,omitempty"`
	CreatedBy string   `json:"created_by"`
	CreatedAt string   `json:"created_at"`
	Stories   []string `json:"stories"`
}

// NewRelease is what CutRelease needs: a version and its git tag, and
// optionally the commit the tag points at and notes.
type NewRelease struct {
	Version string `json:"version"`
	Tag     string `json:"tag"`
	Commit  string `json:"commit"`
	Notes   string `json:"notes"`
}

// CutRelease records an official release. Every story merged into the release
// branch that is not in a release yet becomes Released, is linked to the new
// release and gets a status entry from merged to released that names it, with
// the release commit when one is given. Stories merged into other branches,
// and those still in progress or in review, stay put. A version already
// released, or nothing to release, is a refusal (Conflict). All of it is one
// query, so the release and every story change together or not at all.
func (b *Board) CutRelease(ws, actor string, n NewRelease) (_ Release, err error) {
	if n.Version == "" {
		return Release{}, refuse(Invalid, "a version is required to cut a release")
	}
	if n.Tag == "" {
		return Release{}, refuse(Invalid, "a tag is required to cut a release")
	}
	if err := plainText(n.Version, n.Tag, n.Commit, n.Notes); err != nil {
		return Release{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Release{}, err
	}
	at := now()
	res, err := g.Query(`
		MATCH (w:Workspace)
		OPTIONAL MATCH (b:Branch) WHERE b.into IS NULL
		WITH w, collect(b.name)[0] AS branch, coalesce(w.next_log, 1) AS n, coalesce(w.next_release, 1) AS rn
		OPTIONAL MATCH (d:Release {version: $version})
		WITH w, branch, n, rn, count(d) AS dups
		OPTIONAL MATCH (s:Story {status: 'merged'}) WHERE s.into = branch
		WITH w, branch, n, rn, dups, s ORDER BY s.rank, s.id
		WITH w, branch, n, rn, dups, collect(s) AS ss
		WITH w, branch, n, rn, ss, 'R-' + toString(rn) AS id,
			CASE WHEN dups > 0 THEN 'duplicate' WHEN size(ss) = 0 THEN 'nothing' END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
			SET w.next_release = rn + 1, w.next_log = n + size(ss)
			CREATE (r:Release {id: id, version: $version, tag: $tag, commit: $commit, notes: $notes,
				created_by: $actor, created_at: $at})
			FOREACH (i IN range(0, size(ss) - 1) | FOREACH (s IN [ss[i]] |
				SET s.status = 'released'
				CREATE (s)-[:IN_RELEASE]->(r)
				CREATE (s)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n + i), at: $at, actor: $actor, kind: 'status',
					from: 'merged', to: 'released', commit: $commit, into: branch,
					text: 'Released in ' + id + ' (' + $version + ')'}))))
		RETURN violation, id, branch, [s IN ss | s.id] AS ids`,
		map[string]any{"version": n.Version, "tag": n.Tag, "commit": orNull(n.Commit), "notes": orNull(n.Notes),
			"actor": actor, "at": at}, nil)
	if err != nil {
		return Release{}, fmt.Errorf("cut release: %w", err)
	}
	if !res.Next() {
		return Release{}, refuse(NotFound, "no workspace %q", ws)
	}
	r := res.Record()
	switch str(r, "violation") {
	case "":
	case "duplicate":
		return Release{}, refuse(Conflict, "%s already has a release %s", ws, n.Version)
	default: // nothing
		return Release{}, refuse(Conflict, "nothing to release: no story is merged into %s that is not already in a release", str(r, "branch"))
	}
	ids := strs(r, "ids")
	for _, id := range ids {
		b.changed(nil, Change{ws, KindStory, id})
	}
	return Release{ID: str(r, "id"), Version: n.Version, Tag: n.Tag, Commit: n.Commit, Notes: n.Notes,
		CreatedBy: actor, CreatedAt: at, Stories: ids}, nil
}
