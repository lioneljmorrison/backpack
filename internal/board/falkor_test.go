package board_test

import (
	"slices"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// Ranks are compared inside queries, so FalkorDB must order rank keys exactly
// as Go does.
func TestFalkorDBOrdersRankKeysAsGoDoes(t *testing.T) {
	g := falkortest.Graph(t)
	keys := []string{board.Between("", "")}
	for i := range 60 {
		keys = append(keys, board.Between("", keys[i]), board.Between(keys[i], ""))
	}
	for _, k := range keys {
		if _, err := g.Query("CREATE (:K {k: $k})", map[string]any{"k": k}, nil); err != nil {
			t.Fatal(err)
		}
	}
	res, err := g.ROQuery("MATCH (n:K) RETURN n.k ORDER BY n.k", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for res.Next() {
		v, _ := res.Record().GetByIndex(0)
		got = append(got, v.(string))
	}
	slices.Sort(keys)
	if !slices.Equal(got, keys) {
		t.Fatalf("FalkorDB order differs from Go's:\n got %q\nwant %q", got, keys)
	}
}
