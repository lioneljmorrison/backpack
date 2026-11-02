package board

import (
	"fmt"
	"regexp"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/falkor"
)

// Workspace is one project's board.
type Workspace struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// RepoName and RepoURL are given when creating, to seed the workspace's
	// repository: the name defaults to the slug. Afterwards the repositories
	// are the truth, listed in Repos.
	RepoName string `json:"-"`
	RepoURL  string `json:"-"`
	Repos    []Repo `json:"repos"`
	// HasReleaseStage is true when a branch merges into another. It is given
	// when creating, to seed the branches; afterwards the branches are the truth.
	HasReleaseStage bool   `json:"has_release_stage"`
	CreatedBy       string `json:"created_by"`
	CreatedAt       string `json:"created_at"`
}

// Repo is a git repository a workspace's branches, stories and releases
// belong to.
type Repo struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// CreateWorkspace registers a new workspace. It writes two graphs, so it
// cannot be one query: the workspace's own graph is prepared first and the
// registry entry, written last, is what makes the workspace exist. A create
// interrupted between the two is safe to repeat.
func (b *Board) CreateWorkspace(actor string, w Workspace) (_ Workspace, err error) {
	defer func() { b.changed(err, Change{Workspace: w.Slug, Kind: KindWorkspace}) }()
	if len(w.Slug) > 64 || !slugPattern.MatchString(w.Slug) {
		return Workspace{}, refuse(Invalid, "slug must be 1-64 lowercase letters, digits and single hyphens")
	}
	if w.Name == "" {
		return Workspace{}, refuse(Invalid, "name is required")
	}
	if w.RepoName == "" {
		w.RepoName = w.Slug
	}
	if len(w.RepoName) > 64 || !slugPattern.MatchString(w.RepoName) {
		return Workspace{}, refuse(Invalid, "repo must be 1-64 lowercase letters, digits and single hyphens")
	}
	if err := plainText(w.Name, w.RepoURL); err != nil {
		return Workspace{}, err
	}
	// The registry is read directly: a joined workspace refuses lookups, but
	// its slug is still taken, and so is one a join is building.
	taken, err := b.system.ROQuery("MATCH (e:WorkspaceEntry) WHERE e.slug = $slug OR e.joining = $slug RETURN e.slug LIMIT 1",
		map[string]any{"slug": w.Slug}, nil)
	if err != nil {
		return Workspace{}, fmt.Errorf("look up workspace: %w", err)
	}
	if taken.Next() {
		return Workspace{}, refuse(Conflict, "workspace %q already exists", w.Slug)
	}

	g := b.graph(w.Slug)
	if err := falkor.Migrate(g, migrations, repoParams(w.RepoName, w.RepoURL)); err != nil {
		return Workspace{}, err
	}
	// A graph that already exists keeps its branches, and the registry
	// records the release stage they give, which is the one status changes
	// read. A new graph is seeded: main alone, or dev merging into main.
	at := now()
	if _, err := g.Query(`MERGE (w:Workspace {slug: $slug})
		ON CREATE SET w.next_story = 1, w.next_goal = 1, w.next_log = 1
		MERGE (r:Repo {name: $repo})
			ON CREATE SET r.url = $url, r.created_by = $actor, r.created_at = $at
		WITH w, r
		OPTIONAL MATCH (b:Branch)
		WITH w, r, count(b) AS have
		FOREACH (_ IN CASE WHEN have = 0 THEN [1] ELSE [] END |
			CREATE (r)-[:HAS_BRANCH]->(:Branch {name: 'main', created_by: $actor, created_at: $at}))
		FOREACH (_ IN CASE WHEN have = 0 AND $release THEN [1] ELSE [] END |
			CREATE (r)-[:HAS_BRANCH]->(:Branch {name: 'dev', into: 'main', created_by: $actor, created_at: $at}))`,
		map[string]any{"slug": w.Slug, "release": w.HasReleaseStage, "actor": actor, "at": at,
			"repo": w.RepoName, "url": orNull(w.RepoURL)}, nil); err != nil {
		return Workspace{}, fmt.Errorf("create workspace graph: %w", err)
	}
	if w.HasReleaseStage, err = hasReleaseStage(g); err != nil {
		return Workspace{}, err
	}

	if w.Repos, err = repos(g); err != nil {
		return Workspace{}, err
	}
	w.CreatedBy, w.CreatedAt = actor, at
	res, err := b.system.Query(`
		OPTIONAL MATCH (taken:WorkspaceEntry) WHERE taken.slug = $slug OR taken.joining = $slug
		WITH taken LIMIT 1
		FOREACH (_ IN CASE WHEN taken IS NULL THEN [1] ELSE [] END |
			CREATE (:WorkspaceEntry {slug: $slug, name: $name, repo_url: $repo,
				has_release_stage: $release, created_by: $actor, created_at: $now}))
		RETURN taken IS NOT NULL AS taken`,
		map[string]any{"slug": w.Slug, "name": w.Name, "repo": w.RepoURL, "release": w.HasReleaseStage, "actor": actor, "now": w.CreatedAt}, nil)
	if err != nil {
		return Workspace{}, fmt.Errorf("register workspace: %w", err)
	}
	if res.Next() {
		if v, _ := res.Record().Get("taken"); v == true {
			return Workspace{}, refuse(Conflict, "workspace %q already exists", w.Slug)
		}
	}
	return w, nil
}

// Workspaces lists every registered workspace by slug, each with its
// repositories.
func (b *Board) Workspaces() ([]Workspace, error) {
	out, err := b.entries()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Repos, err = repos(b.graph(out[i].Slug)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Repos lists a workspace's repositories by name; an unknown workspace is
// NotFound.
func (b *Board) Repos(ws string) ([]Repo, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	return repos(g)
}

// repos reads a workspace graph's repositories by name.
func repos(g *falkordb.Graph) ([]Repo, error) {
	res, err := g.ROQuery("MATCH (r:Repo) RETURN r.name AS name, r.url AS url ORDER BY r.name", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("read repositories: %w", err)
	}
	out := []Repo{}
	for res.Next() {
		out = append(out, Repo{Name: str(res.Record(), "name"), URL: str(res.Record(), "url")})
	}
	return out, nil
}

// entries lists the registry: every workspace that was not joined into
// another, without its repositories. A workspace being joined stays listed.
func (b *Board) entries() ([]Workspace, error) {
	res, err := b.system.ROQuery(`
		MATCH (e:WorkspaceEntry) WHERE e.joined_into IS NULL
		RETURN e.slug AS slug, e.name AS name, e.repo_url AS repo,
			coalesce(e.has_release_stage, false) AS release, e.created_by AS by, e.created_at AS at
		ORDER BY e.slug`, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	out := []Workspace{}
	for res.Next() {
		r := res.Record()
		out = append(out, Workspace{Slug: str(r, "slug"), Name: str(r, "name"), RepoURL: str(r, "repo"), Repos: []Repo{},
			HasReleaseStage: flag(r, "release"), CreatedBy: str(r, "by"), CreatedAt: str(r, "at")})
	}
	return out, nil
}
