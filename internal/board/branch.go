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

// checkBranchName holds a new branch's name, or a renamed one's, to the rules:
// plain text, no whitespace, at most 100 characters.
func checkBranchName(n string) error {
	switch {
	case n == "":
		return refuse(Invalid, "a branch's name cannot be empty")
	case utf8.RuneCountInString(n) > maxBranchName:
		return refuse(Invalid, "a branch's name is at most %d characters", maxBranchName)
	case strings.ContainsFunc(n, unicode.IsSpace):
		return refuse(Invalid, "a branch's name cannot contain whitespace")
	}
	return plainText(n)
}

func checkBranchAlias(a string) error {
	if utf8.RuneCountInString(a) > maxBranchName {
		return refuse(Invalid, "a branch's alias is at most %d characters", maxBranchName)
	}
	return plainText(a)
}

// branchNamed reads one branch as it is now.
func branchNamed(g *falkordb.Graph, op, name string) (Branch, error) {
	bs, err := branches(g)
	if err != nil {
		return Branch{}, err
	}
	for _, br := range bs {
		if br.Name == name {
			return br, nil
		}
	}
	return Branch{}, fmt.Errorf("%s: %q is gone after the change", op, name)
}

// AddBranch adds a branch that merges into an existing one. Into is required:
// a workspace has one release branch, the one that merges nowhere, so a second
// branch that merges nowhere is refused. A name already in use, or one that is
// empty, has whitespace or runs past 100 characters, is a refusal, and so is an
// unknown branch to merge into (NotFound, as update_branch answers for a branch
// it does not know). No story moves, because stories name their branch, not a
// place in the order. Alias is optional. It is one query, so the branch exists
// with everything or not at all. It answers with the branch.
func (b *Board) AddBranch(ws, actor string, br Branch) (Branch, error) {
	if err := checkBranchName(br.Name); err != nil {
		return Branch{}, err
	}
	if err := checkBranchAlias(br.Alias); err != nil {
		return Branch{}, err
	}
	if br.Into == "" {
		return Branch{}, refuse(Invalid, "a branch must merge into another; a workspace has one release branch, and it is already there")
	}
	if err := plainText(br.Into); err != nil {
		return Branch{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Branch{}, err
	}
	res, err := g.Query(`
		MATCH (w:Workspace)
		OPTIONAL MATCH (x:Branch) WHERE x.name = $name
		OPTIONAL MATCH (y:Branch) WHERE y.name = $into
		WITH x, y, CASE WHEN x IS NOT NULL THEN 'taken' WHEN y IS NULL THEN 'no target' END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END |
			CREATE (:Branch {name: $name, into: $into, alias: $alias, created_by: $actor, created_at: $now}))
		RETURN violation`,
		map[string]any{"name": br.Name, "into": br.Into, "alias": orNull(br.Alias), "actor": actor, "now": now()}, nil)
	if err != nil {
		return Branch{}, fmt.Errorf("add branch: %w", err)
	}
	if !res.Next() {
		return Branch{}, refuse(NotFound, "no workspace %q", ws)
	}
	switch str(res.Record(), "violation") {
	case "":
	case "taken":
		return Branch{}, refuse(Conflict, "%s already has a branch named %q", ws, br.Name)
	default: // no target
		return Branch{}, refuse(NotFound, "%s has no branch %q to merge into", ws, br.Into)
	}
	return branchNamed(g, "add branch", br.Name)
}

// BranchPatch changes a branch; a field left nil is left as it is. An empty
// Alias clears the alias. Into names the branch it merges into.
type BranchPatch struct {
	Name  *string `json:"name"`
	Alias *string `json:"alias"`
	Into  *string `json:"into"`
}

// UpdateBranch changes a branch. Renaming it changes its name everywhere the
// board refers to it: the branches that merge into it and the stories that
// reached it. No story moves, and past log entries keep the old name, because
// a log is never edited. A name already in use, or one that is empty, has
// whitespace or runs past 100 characters, is refused; renaming a branch to its
// own name, like an empty patch, is accepted and changes nothing. An alias is
// the name its column heading shows on the web, and nothing else uses it; it
// is plain text of at most 100 characters, and an empty one clears it. Into
// changes where the branch merges: nothing moves, and a story merged into it
// follows on to the new branch at its next merge. It is refused for an unknown
// branch (NotFound), for the branch itself or one that leads back to it (a
// cycle), for the release branch (it would leave none), and when empty for any
// other branch (it would make a second release branch); merging into where it
// already merges, or clearing the release branch's, changes nothing. It is one
// query, so every reference follows or none does. It answers with the branch as
// it is now, and tells the board's watchers about each story it renamed.
func (b *Board) UpdateBranch(ws, actor, branch string, patch BranchPatch) (_ Branch, err error) {
	if patch.Name != nil {
		if err := checkBranchName(*patch.Name); err != nil {
			return Branch{}, err
		}
	}
	if patch.Alias != nil {
		if err := checkBranchAlias(*patch.Alias); err != nil {
			return Branch{}, err
		}
	}
	if patch.Into != nil {
		if err := plainText(*patch.Into); err != nil {
			return Branch{}, err
		}
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Branch{}, err
	}
	// The cycle check walks the branches' into names from the one to merge
	// into, one step per branch at most (the branches never loop, so that is
	// enough), and asks whether the walk meets this branch. Into is a property,
	// not an edge, so a variable-length path match is not available; the walk
	// is a reduce over the branches collected in the same query.
	res, err := g.Query(`
		MATCH (w:Workspace)
		OPTIONAL MATCH (b:Branch {name: $branch})
		OPTIONAL MATCH (x:Branch) WHERE x.name = $name
		OPTIONAL MATCH (y:Branch) WHERE y.name = $into
		OPTIONAL MATCH (a:Branch)
		WITH b, x, y, collect(a) AS every
		WITH b, x, y, reduce(p = [$into], i IN range(1, size(every)) |
			p + [head([a IN every WHERE a.name = p[size(p) - 1] | a.into])]) AS walk
		WITH b, x, y,
			CASE WHEN b IS NULL THEN 'no branch'
				WHEN x IS NOT NULL AND x.name <> b.name THEN 'taken'
				WHEN $into IS NOT NULL AND $into <> '' AND y IS NULL THEN 'no target'
				WHEN $into IS NOT NULL AND $into <> '' AND b.into IS NULL THEN 'release'
				WHEN $into = '' AND b.into IS NOT NULL THEN 'second release'
				WHEN $into IS NOT NULL AND $into <> '' AND $branch IN walk THEN 'cycle' END AS violation,
			b IS NOT NULL AND $name IS NOT NULL AND $name <> b.name AS renames
		OPTIONAL MATCH (s:Story {into: $branch}) WHERE violation IS NULL AND renames
		WITH b, violation, renames, collect(s) AS ss
		OPTIONAL MATCH (c:Branch {into: $branch}) WHERE violation IS NULL AND renames
		WITH b, violation, renames, ss, collect(c) AS cs
		FOREACH (_ IN CASE WHEN violation IS NULL AND renames THEN [1] ELSE [] END |
			SET b.name = $name
			FOREACH (s IN ss | SET s.into = $name)
			FOREACH (c IN cs | SET c.into = $name))
		FOREACH (_ IN CASE WHEN violation IS NULL AND $alias IS NOT NULL THEN [1] ELSE [] END |
			SET b.alias = CASE WHEN $alias = '' THEN null ELSE $alias END)
		FOREACH (_ IN CASE WHEN violation IS NULL AND $into IS NOT NULL AND $into <> '' THEN [1] ELSE [] END |
			SET b.into = $into)
		RETURN violation, [s IN ss | s.id] AS ids`,
		map[string]any{"branch": branch, "name": opt(patch.Name), "alias": opt(patch.Alias), "into": opt(patch.Into)}, nil)
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
	case "taken":
		return Branch{}, refuse(Conflict, "%s already has a branch named %q", ws, deref(patch.Name))
	case "no target":
		return Branch{}, refuse(NotFound, "%s has no branch %q to merge into", ws, deref(patch.Into))
	case "release":
		return Branch{}, refuse(Conflict, "%s is the release branch and merges nowhere; it cannot merge into %s, which would leave no release branch", branch, deref(patch.Into))
	case "second release":
		return Branch{}, refuse(Conflict, "%s must merge into another branch; clearing it would make a second release branch", branch)
	default: // cycle
		if deref(patch.Into) == branch {
			return Branch{}, refuse(Conflict, "%s cannot merge into itself", branch)
		}
		return Branch{}, refuse(Conflict, "%s cannot merge into %s: %s leads back to %s", branch, deref(patch.Into), deref(patch.Into), branch)
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
	return branchNamed(g, "update branch", name)
}

// RemoveBranch takes a branch off the board, only when nothing refers to it:
// no story is merged into it (whatever its status) and no branch merges into
// it. The refusal names what still does. The release branch is never removed,
// nor the first branch: the only one nothing merges into, where a story from
// review is merged first. With several such starting branches an empty one can
// go. An unknown branch is NotFound. Past log entries keep the removed name,
// because a log is never edited. It is one query, so the branch is removed with
// every check made or not at all.
func (b *Board) RemoveBranch(ws, actor, branch string) error {
	if branch == "" {
		return refuse(Invalid, "a branch is required")
	}
	if err := plainText(branch); err != nil {
		return err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return err
	}
	res, err := g.Query(`
		MATCH (w:Workspace)
		OPTIONAL MATCH (b:Branch {name: $branch})
		OPTIONAL MATCH (a:Branch)
		WITH b, collect(a) AS every
		WITH b,
			[a IN every WHERE size([d IN every WHERE d.into = a.name]) = 0 | a.name] AS starting,
			[a IN every WHERE a.into = $branch | a.name] AS merging
		OPTIONAL MATCH (s:Story {into: $branch})
		WITH b, starting, merging, s ORDER BY s.rank, s.id
		WITH b, starting, merging, collect(s.id) AS stories
		WITH b, merging, stories,
			CASE WHEN b IS NULL THEN 'no branch'
				WHEN b.into IS NULL THEN 'release'
				WHEN b.name IN starting AND size(starting) = 1 THEN 'first'
				WHEN size(stories) > 0 OR size(merging) > 0 THEN 'in use' END AS violation
		FOREACH (_ IN CASE WHEN violation IS NULL THEN [1] ELSE [] END | DELETE b)
		RETURN violation, merging, stories`,
		map[string]any{"branch": branch}, nil)
	if err != nil {
		return fmt.Errorf("remove branch: %w", err)
	}
	if !res.Next() {
		return refuse(NotFound, "no workspace %q", ws)
	}
	r := res.Record()
	switch str(r, "violation") {
	case "":
		return nil
	case "no branch":
		return refuse(NotFound, "%s has no branch %q", ws, branch)
	case "release":
		return refuse(Conflict, "%s is the release branch and is never removed", branch)
	case "first":
		return refuse(Conflict, "%s is the first branch, the only one stories are merged into first, and is never removed", branch)
	}
	var refs []string
	if ids := strs(r, "stories"); len(ids) > 0 {
		refs = append(refs, "stories merged into it: "+strings.Join(ids, ", "))
	}
	if bs := strs(r, "merging"); len(bs) > 0 {
		sort.Strings(bs)
		refs = append(refs, "branches merging into it: "+strings.Join(bs, ", "))
	}
	return refuse(Conflict, "%s is still in use (%s)", branch, strings.Join(refs, "; "))
}
