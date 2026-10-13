package board

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/FalkorDB/falkordb-go/v2"
)

// Branch is a branch a workspace delivers through. Into names the branch it
// merges into; the release branch merges nowhere.
type Branch struct {
	Name      string `json:"name"`
	Into      string `json:"into,omitempty"`
	Alias     string `json:"alias,omitempty"`
	Release   bool   `json:"release"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

// Branches lists a workspace's branches in merge order: each comes before the
// one it merges into, the release branch last.
func (b *Board) Branches(ws string) ([]Branch, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	return branches(g)
}

func branches(g *falkordb.Graph) ([]Branch, error) {
	res, err := g.ROQuery(`MATCH (b:Branch)
		RETURN b.name AS name, b.into AS into, b.alias AS alias, b.created_by AS by, b.created_at AS at`, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("read branches: %w", err)
	}
	out := []Branch{}
	for res.Next() {
		r := res.Record()
		into := str(r, "into")
		out = append(out, Branch{Name: str(r, "name"), Into: into, Alias: str(r, "alias"), Release: into == "",
			CreatedBy: str(r, "by"), CreatedAt: str(r, "at")})
	}
	into := map[string]string{}
	for _, br := range out {
		into[br.Name] = br.Into
	}
	// Steps to the release branch; the bound keeps a cycle from looping.
	depth := func(n string) (d int) {
		for ; into[n] != "" && d <= len(out); n = into[n] {
			d++
		}
		return d
	}
	sort.SliceStable(out, func(i, j int) bool {
		if di, dj := depth(out[i].Name), depth(out[j].Name); di != dj {
			return di > dj
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ReleaseStage reports whether the workspace has a branch that merges into
// another, which is what puts a second Merged column on the board.
func (b *Board) ReleaseStage(ws string) (bool, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return false, err
	}
	return hasReleaseStage(g)
}

// MergeBranch records that a branch was merged into the one it merges into,
// at the given commit. Every story merged into the branch moves to the next
// branch and gets the same log entry a single merge writes; stories still in
// progress or in review stay put. It answers with the stories it moved, none
// when the branch holds nothing. The release branch merges nowhere, so
// merging it is refused. All of it is one query, so every story moves or none
// does.
func (b *Board) MergeBranch(ws, actor, branch, commit string) (_ []Story, err error) {
	if commit == "" {
		return nil, refuse(Invalid, "a commit is required to record a merge")
	}
	if branch == "" {
		return nil, refuse(Invalid, "a branch is required")
	}
	if err := plainText(branch, commit); err != nil {
		return nil, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	res, err := g.Query(`
		MATCH (w:Workspace)
		OPTIONAL MATCH (b:Branch {name: $branch})
		WITH w, b, coalesce(w.next_log, 1) AS n,
			CASE WHEN b IS NULL THEN 'no branch' WHEN b.into IS NULL THEN 'release' END AS violation
		OPTIONAL MATCH (s:Story {status: 'merged', into: $branch}) WHERE violation IS NULL
		WITH w, b, n, violation, s ORDER BY s.rank, s.id
		WITH w, b, n, violation, collect(s) AS ss
		FOREACH (_ IN CASE WHEN violation IS NULL AND size(ss) > 0 THEN [1] ELSE [] END |
			SET w.next_log = n + size(ss))
		FOREACH (i IN range(0, size(ss) - 1) | FOREACH (s IN [ss[i]] |
			SET s.into = b.into
			CREATE (s)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(n + i), at: $now, actor: $actor, kind: 'status',
				from: 'merged', to: 'merged', commit: $commit, into: b.into})))
		RETURN violation, b.into AS into, [s IN ss | s.id] AS ids`,
		map[string]any{"branch": branch, "commit": commit, "actor": actor, "now": now()}, nil)
	if err != nil {
		return nil, fmt.Errorf("merge branch: %w", err)
	}
	if !res.Next() {
		return nil, refuse(NotFound, "no workspace %q", ws)
	}
	r := res.Record()
	switch str(r, "violation") {
	case "":
	case "no branch":
		return nil, refuse(NotFound, "%s has no branch %q", ws, branch)
	default: // release
		return nil, refuse(Conflict, "%s is the release branch and merges nowhere", branch)
	}
	moved := []Story{}
	for _, id := range strs(r, "ids") {
		s, err := story(g, id)
		if err != nil {
			return nil, err
		}
		moved = append(moved, s)
	}
	for _, s := range moved {
		b.changed(nil, Change{ws, KindStory, s.ID})
	}
	return moved, nil
}

// maxBranchName bounds a branch's name, in characters.
const maxBranchName = 100

// BranchPatch changes a branch; a field left nil is left as it is. Only the
// name can change so far.
type BranchPatch struct {
	Name *string `json:"name"`
}

// UpdateBranch changes a branch. Renaming it changes its name everywhere the
// board refers to it: the branches that merge into it and the stories that
// reached it. No story moves, and past log entries keep the old name, because
// a log is never edited. A name already in use, or one that is empty, has
// whitespace or runs past 100 characters, is refused; renaming a branch to its
// own name, like an empty patch, is accepted and changes nothing. It is one
// query, so every reference follows or none does. It answers with the branch
// as it is now, and tells the board's watchers about each story it renamed.
func (b *Board) UpdateBranch(ws, actor, branch string, patch BranchPatch) (_ Branch, err error) {
	if patch.Name != nil {
		n := *patch.Name
		switch {
		case n == "":
			return Branch{}, refuse(Invalid, "a branch's name cannot be empty")
		case utf8.RuneCountInString(n) > maxBranchName:
			return Branch{}, refuse(Invalid, "a branch's name is at most %d characters", maxBranchName)
		case strings.ContainsFunc(n, unicode.IsSpace):
			return Branch{}, refuse(Invalid, "a branch's name cannot contain whitespace")
		}
		if err := plainText(n); err != nil {
			return Branch{}, err
		}
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Branch{}, err
	}
	res, err := g.Query(`
		MATCH (w:Workspace)
		OPTIONAL MATCH (b:Branch {name: $branch})
		OPTIONAL MATCH (x:Branch) WHERE x.name = $name
		WITH b, x,
			CASE WHEN b IS NULL THEN 'no branch'
				WHEN x IS NOT NULL AND x.name <> b.name THEN 'taken' END AS violation,
			b IS NOT NULL AND $name IS NOT NULL AND $name <> b.name AS renames
		OPTIONAL MATCH (s:Story {into: $branch}) WHERE violation IS NULL AND renames
		WITH b, violation, renames, collect(s) AS ss
		OPTIONAL MATCH (c:Branch {into: $branch}) WHERE violation IS NULL AND renames
		WITH b, violation, renames, ss, collect(c) AS cs
		FOREACH (_ IN CASE WHEN violation IS NULL AND renames THEN [1] ELSE [] END |
			SET b.name = $name
			FOREACH (s IN ss | SET s.into = $name)
			FOREACH (c IN cs | SET c.into = $name))
		RETURN violation, [s IN ss | s.id] AS ids`,
		map[string]any{"branch": branch, "name": opt(patch.Name)}, nil)
	if err != nil {
		return Branch{}, fmt.Errorf("update branch: %w", err)
	}
	if !res.Next() {
		return Branch{}, refuse(NotFound, "no workspace %q", ws)
	}
	r := res.Record()
	switch str(r, "violation") {
	case "":
	case "no branch":
		return Branch{}, refuse(NotFound, "%s has no branch %q", ws, branch)
	default: // taken
		return Branch{}, refuse(Conflict, "%s already has a branch named %q", ws, deref(patch.Name))
	}
	ids := strs(r, "ids")
	sort.Strings(ids)
	name := branch
	if patch.Name != nil {
		name = *patch.Name
	}
	for _, id := range ids {
		b.changed(nil, Change{ws, KindStory, id})
	}
	bs, err := branches(g)
	if err != nil {
		return Branch{}, err
	}
	for _, br := range bs {
		if br.Name == name {
			return br, nil
		}
	}
	return Branch{}, fmt.Errorf("update branch: %q is gone after the change", name)
}
