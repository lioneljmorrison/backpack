package board_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// mergedColumns lists the Merged columns as repo/branch(story ids), in order.
func mergedColumns(cols []board.Column) string {
	var out []string
	for _, c := range cols {
		if c.Status != board.Merged {
			continue
		}
		ids := []string{}
		for _, s := range c.Stories {
			ids = append(ids, s.ID)
		}
		out = append(out, fmt.Sprintf("%s/%s(%s)", c.Repo, c.Branch, strings.Join(ids, ",")))
	}
	return strings.Join(out, " ")
}

func TestEachMergedColumnNamesItsRepositoryAndBranch(t *testing.T) {
	b, g := twoRepos(t)
	addWeb(t, g)
	webStory(t, g, "W-1", "merged", "dev")
	cols, err := b.Board("demo")
	if err != nil {
		t.Fatal(err)
	}
	// Repositories in name order, each one's branches in merge order, and a
	// story only in its own repository's column.
	if got, want := mergedColumns(cols), "api/dev() api/main() web/dev(W-1) web/main()"; got != want {
		t.Fatalf("merged columns %q, want %q", got, want)
	}
	// The shared columns name no repository.
	for _, c := range cols {
		if c.Status != board.Merged && (c.Repo != "" || c.Branch != "") {
			t.Errorf("shared column %+v names a repository or branch", c)
		}
	}
}

func TestAOneRepositoryWorkspacesMergedColumnsNameItToo(t *testing.T) {
	b, _ := twoRepos(t)
	cols, err := b.Board("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mergedColumns(cols), "api/dev() api/main()"; got != want {
		t.Fatalf("merged columns %q, want %q", got, want)
	}
}
