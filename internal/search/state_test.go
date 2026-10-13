package search

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// A merged story's state names the branch it reached; a released story's is
// just "released", though it keeps the branch it was released from.
func TestAStoryDocsStateReadsMergedWithItsBranchAndReleasedAlone(t *testing.T) {
	for _, c := range []struct {
		s    board.Story
		want string
	}{
		{board.Story{ID: "S-1", Status: board.Merged, Into: "main"}, "merged:main"},
		{board.Story{ID: "S-2", Status: board.Released, Into: "main"}, "released"},
		{board.Story{ID: "S-3", Status: board.InReview}, "in_review"},
	} {
		if got := storyDoc(c.s, nil).State; got != c.want {
			t.Errorf("%s: state %q, want %q", c.s.Status, got, c.want)
		}
	}
}
