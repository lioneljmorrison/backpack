package board_test

import (
	"context"
	"time"

	"github.com/FalkorDB/falkordb-go/v2"
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

// A query that hands out several log ids takes a block from a counter: it
// collects the stories, numbers them with UNWIND, and moves the counter past
// the block, all in one statement. An empty list must run cleanly and leave
// the counter alone.
func TestFalkorDBHandsOutABlockOfIdsInOneQuery(t *testing.T) {
	g := falkortest.Graph(t)
	if _, err := g.Query(`CREATE (:W {next: 5}), (:S {k: 'a'}), (:S {k: 'b'}), (:S {k: 'c'})`, nil, nil); err != nil {
		t.Fatal(err)
	}
	const q = `MATCH (w:W)
		OPTIONAL MATCH (s:S) WHERE s.k <> $skip
		WITH w, collect(s) AS ss, w.next AS n
		SET w.next = n + size(ss)
		WITH n, ss
		UNWIND range(0, size(ss) - 1) AS i
		WITH ss[i] AS s, n + i AS id
		CREATE (s)-[:L]->(:E {id: 'L-' + toString(id)})
		RETURN count(*)`
	if _, err := g.Query(q, map[string]any{"skip": "b"}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := g.ROQuery("MATCH (w:W) OPTIONAL MATCH (e:E) RETURN w.next, e.id ORDER BY e.id", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	var next int64
	for res.Next() {
		n, _ := res.Record().GetByIndex(0)
		e, _ := res.Record().GetByIndex(1)
		next = n.(int64)
		ids = append(ids, e.(string))
	}
	if next != 7 || !slices.Equal(ids, []string{"L-5", "L-6"}) {
		t.Fatalf("counter %d, ids %q; want 7 and L-5, L-6", next, ids)
	}
	// Nothing to number: no rows, no error, the counter stays.
	if _, err := g.Query(q, map[string]any{"skip": ""}, nil); err != nil {
		t.Fatal(err)
	}
}

// A unique constraint makes a write fail part way through a query, which is
// how the board's tests prove a query applies completely or not at all.
func TestFalkorDBRollsBackAQueryThatFailsPartWay(t *testing.T) {
	g := falkortest.Graph(t)
	if _, err := g.Query("CREATE INDEX FOR (e:E) ON (e.id)", nil, nil); err != nil {
		t.Fatal(err)
	}
	uniqueConstraint(t, g, "E", "id")
	if _, err := g.Query("CREATE (:E {id: 'taken'}), (:S {k: 'a'})", nil, nil); err != nil {
		t.Fatal(err)
	}
	_, err := g.Query(`MATCH (s:S) SET s.k = 'changed' CREATE (:E {id: 'fresh'}) CREATE (:E {id: 'taken'})`, nil, nil)
	if err == nil {
		t.Fatal("want the duplicate refused")
	}
	res, _ := g.ROQuery("MATCH (s:S) OPTIONAL MATCH (e:E {id: 'fresh'}) RETURN s.k, count(e)", nil, nil)
	res.Next()
	k, _ := res.Record().GetByIndex(0)
	c, _ := res.Record().GetByIndex(1)
	if k != "a" || c != int64(0) {
		t.Fatalf("after the failure s.k=%v, fresh entries=%v; want a and 0", k, c)
	}
}

// uniqueConstraint creates a unique constraint and waits until it is enforced.
func uniqueConstraint(t *testing.T, g *falkordb.Graph, label, prop string) {
	t.Helper()
	if err := g.Conn.Do(context.Background(), "GRAPH.CONSTRAINT", "CREATE", g.Id, "UNIQUE", "NODE", label,
		"PROPERTIES", "1", prop).Err(); err != nil {
		t.Fatalf("constraint: %v", err)
	}
	for range 100 {
		res, err := g.CallProcedure("db.constraints", []string{"status"})
		if err != nil {
			t.Fatal(err)
		}
		ready := res.Next()
		for ok := ready; ok; ok = res.Next() {
			v, _ := res.Record().GetByIndex(0)
			ready = ready && v == "OPERATIONAL"
		}
		if ready {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("constraint never became operational")
}
