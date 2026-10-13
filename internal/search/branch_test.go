package search_test

import (
	"context"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/search"
)

// A branch notice names nothing to index: Sync answers without reading the
// board or reaching the index (neither exists here).
func TestABranchNoticeIsIgnoredBySearch(t *testing.T) {
	x := search.New("http://127.0.0.1:1", "invented-key", "invented", nil)
	if err := x.Sync(context.Background(), board.Change{Workspace: "demo", Kind: board.KindBranch}); err != nil {
		t.Fatalf("sync: %v", err)
	}
}
