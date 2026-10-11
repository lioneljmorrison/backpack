package board

import (
	"fmt"
	"regexp"

	"github.com/lioneljmorrison/backpack/internal/falkor"
)

// Workspace is one project's board.
type Workspace struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	RepoURL string `json:"repo_url,omitempty"`
	// HasReleaseStage puts Released between Merged and Deployed.
	HasReleaseStage bool   `json:"has_release_stage"`
	CreatedBy       string `json:"created_by"`
	CreatedAt       string `json:"created_at"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// CreateWorkspace registers a new workspace. It writes two graphs, so it
// cannot be one query: the workspace's own graph is prepared first and the
// registry entry, written last, is what makes the workspace exist. A create
// interrupted between the two is safe to repeat.
func (b *Board) CreateWorkspace(actor string, w Workspace) (Workspace, error) {
	if len(w.Slug) > 64 || !slugPattern.MatchString(w.Slug) {
		return Workspace{}, refuse(Invalid, "slug must be 1-64 lowercase letters, digits and single hyphens")
	}
	if w.Name == "" {
		return Workspace{}, refuse(Invalid, "name is required")
	}
	if err := plainText(w.Name, w.RepoURL); err != nil {
		return Workspace{}, err
	}
	if _, err := b.workspace(w.Slug); err == nil {
		return Workspace{}, refuse(Conflict, "workspace %q already exists", w.Slug)
	}

	g := b.graph(w.Slug)
	if err := falkor.Migrate(g, migrations); err != nil {
		return Workspace{}, err
	}
	// A graph that already exists keeps its release stage, and the registry
	// records the one the graph has, which is the one status changes read.
	res, err := g.Query(`MERGE (w:Workspace {slug: $slug})
		ON CREATE SET w.next_story = 1, w.next_goal = 1, w.next_log = 1, w.has_release_stage = $release
		RETURN coalesce(w.has_release_stage, false) AS release`,
		map[string]any{"slug": w.Slug, "release": w.HasReleaseStage}, nil)
	if err != nil {
		return Workspace{}, fmt.Errorf("create workspace graph: %w", err)
	}
	if res.Next() {
		w.HasReleaseStage = flag(res.Record(), "release")
	}

	w.CreatedBy, w.CreatedAt = actor, now()
	res, err = b.system.Query(`
		OPTIONAL MATCH (taken:WorkspaceEntry {slug: $slug})
		WITH taken
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

// Workspaces lists every registered workspace by slug.
func (b *Board) Workspaces() ([]Workspace, error) {
	res, err := b.system.ROQuery(`
		MATCH (e:WorkspaceEntry)
		RETURN e.slug AS slug, e.name AS name, e.repo_url AS repo,
			coalesce(e.has_release_stage, false) AS release, e.created_by AS by, e.created_at AS at
		ORDER BY e.slug`, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	out := []Workspace{}
	for res.Next() {
		r := res.Record()
		out = append(out, Workspace{Slug: str(r, "slug"), Name: str(r, "name"), RepoURL: str(r, "repo"),
			HasReleaseStage: flag(r, "release"), CreatedBy: str(r, "by"), CreatedAt: str(r, "at")})
	}
	return out, nil
}
