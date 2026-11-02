package board

import "fmt"

// AddRepo adds a repository to a workspace. It starts with main alone, its
// release branch; the branch actions shape the rest. The name follows the
// slug rule and is unique in the workspace (Conflict when taken); the URL is
// optional plain text. It is one query, so the repository and its branch
// exist together or not at all. It tells the board's watchers the branches
// changed, which is what redraws an open page.
func (b *Board) AddRepo(ws, actor string, r Repo) (Repo, error) {
	if len(r.Name) > 64 || !slugPattern.MatchString(r.Name) {
		return Repo{}, refuse(Invalid, "repo name must be 1-64 lowercase letters, digits and single hyphens")
	}
	if err := plainText(r.URL); err != nil {
		return Repo{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Repo{}, err
	}
	at := now()
	res, err := g.Query(`
		MATCH (w:Workspace)
		OPTIONAL MATCH (x:Repo {name: $name})
		WITH w, x IS NOT NULL AS taken
		FOREACH (_ IN CASE WHEN taken THEN [] ELSE [1] END |
			CREATE (r:Repo {name: $name, url: $url, created_by: $actor, created_at: $at}),
				(r)-[:HAS_BRANCH]->(:Branch {name: 'main', created_by: $actor, created_at: $at}))
		RETURN taken`,
		map[string]any{"name": r.Name, "url": orNull(r.URL), "actor": actor, "at": at}, nil)
	if err != nil {
		return Repo{}, fmt.Errorf("add repo: %w", err)
	}
	if !res.Next() {
		return Repo{}, refuse(NotFound, "no workspace %q", ws)
	}
	if v, _ := res.Record().Get("taken"); v == true {
		return Repo{}, refuse(Conflict, "%s already has a repo named %q", ws, r.Name)
	}
	b.changed(nil, Change{Workspace: ws, Kind: KindBranch})
	return r, nil
}
