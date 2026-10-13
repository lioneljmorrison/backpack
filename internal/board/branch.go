package board

import (
	"fmt"
	"sort"

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
