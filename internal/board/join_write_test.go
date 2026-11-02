package board

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// The join writes with UNWIND over a list of maps and SET n += r. The
// FalkorDB client had never sent a list of maps, so this round trip, one node
// of every kind of value and one edge, comes first: if the query shape is
// refused or loses a value, the join falls back to one CREATE per node.
func TestAListOfMapsRoundTripsThroughUnwindAndSetPlus(t *testing.T) {
	g := falkortest.Graph(t)
	props := map[string]any{
		"id": "S-1", "title": "café \"quoted\" \\ back\nline\ttab", "n": int64(7), "f": 1.5, "ok": true,
		"list": []any{"a", "b"},
	}
	ids, err := writeNodes(g, "Story", []map[string]any{{"k": int64(0), "p": props}, {"k": int64(1), "p": map[string]any{"id": "S-2"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids %v", ids)
	}
	if err := writeEdges(g, "DEPENDS_ON", []map[string]any{{"a": ids[0], "b": ids[1], "p": map[string]any{"created_by": "x"}},
		{"a": ids[1], "b": ids[0], "p": map[string]any{}}}); err != nil {
		t.Fatal(err)
	}
	nodes, edges, err := readSnapshot(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || len(edges) != 2 {
		t.Fatalf("nodes %d edges %d", len(nodes), len(edges))
	}
	if !reflect.DeepEqual(nodes[0].Props, props) || nodes[0].Labels[0] != "Story" {
		t.Fatalf("round trip lost a value:\n got %#v\nwant %#v", nodes[0].Props, props)
	}
	if uint64(ids[0]) != nodes[0].ID || edges[0].Props["created_by"] != "x" && edges[1].Props["created_by"] != "x" {
		t.Fatalf("ids %v edges %+v", ids, edges)
	}
}

// The freeze, the switch and the unfreeze are each one system query, and
// each checks its rules inside it: a rule that does not hold changes nothing.
func TestTheFreezeMarksEverySourceOrNone(t *testing.T) {
	db, prefix := falkortest.Prefix(t)
	b, err := New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	sys := db.SelectGraph(prefix + "system")
	for _, s := range []string{"demo-api", "demo-web"} {
		if _, err := sys.Query("CREATE (:WorkspaceEntry {slug: $s, name: $s})", map[string]any{"s": s}, nil); err != nil {
			t.Fatal(err)
		}
	}
	req := JoinRequest{Into: "demo", Name: "Demo", Actor: "x", From: []string{"demo-api", "demo-web"}}
	joining := func() string {
		res, err := sys.ROQuery("MATCH (e:WorkspaceEntry) RETURN e.slug, e.joining ORDER BY e.slug", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		var out string
		for res.Next() {
			a, _ := res.Record().GetByIndex(0)
			c, _ := res.Record().GetByIndex(1)
			out += fmt.Sprintf("%v=%v ", a, c)
		}
		return out
	}
	// One source joined elsewhere: nothing is frozen.
	if _, err := sys.Query("MATCH (e:WorkspaceEntry {slug: 'demo-web'}) SET e.joined_into = 'other'", nil, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := b.freeze(req); err != nil || ok {
		t.Fatalf("freeze %v %v", ok, err)
	}
	if got := joining(); got != "demo-api=<nil> demo-web=<nil> " {
		t.Fatalf("a refused freeze marked something: %s", got)
	}
	// Free again: both are frozen, and freezing twice is the resume path.
	if _, err := sys.Query("MATCH (e:WorkspaceEntry {slug: 'demo-web'}) SET e.joined_into = NULL", nil, nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if ok, err := b.freeze(req); err != nil || !ok {
			t.Fatalf("freeze %v %v", ok, err)
		}
	}
	if got := joining(); got != "demo-api=demo demo-web=demo " {
		t.Fatalf("frozen: %s", got)
	}
	// Another target cannot take them, a registered target cannot be built,
	// and the switch needs every source frozen to this target.
	other := req
	other.Into = "other-target"
	if ok, _ := b.freeze(other); ok {
		t.Fatal("a frozen source was frozen to another target")
	}
	if _, err := sys.Query("MATCH (e:WorkspaceEntry {slug: 'demo-web'}) SET e.joining = NULL", nil, nil); err != nil {
		t.Fatal(err)
	}
	plan := JoinPlan{Request: req, Sources: []JoinSource{{Slug: "demo-api"}, {Slug: "demo-web"}}}
	if ok, err := b.switchOver(plan, false, "now"); err != nil || ok {
		t.Fatalf("switch with a source not frozen: %v %v", ok, err)
	}
	res, _ := sys.ROQuery("MATCH (e:WorkspaceEntry) WHERE e.slug = 'demo' OR e.joined_into = 'demo' RETURN count(e)", nil, nil)
	res.Next()
	if v, _ := res.Record().GetByIndex(0); v.(int64) != 0 {
		t.Fatal("a refused switch wrote something")
	}
}

func TestWritingMoreRowsThanOneChunkKeepsEveryRow(t *testing.T) {
	g := falkortest.Graph(t)
	var rows []map[string]any
	for i := range 2*joinChunk + 37 {
		rows = append(rows, map[string]any{"k": int64(i), "p": map[string]any{"n": int64(i)}})
	}
	ids, err := writeNodes(g, "Item", rows)
	if err != nil || len(ids) != len(rows) {
		t.Fatalf("%d ids, %v", len(ids), err)
	}
	var links []map[string]any
	for i := 1; i < len(rows); i++ {
		links = append(links, map[string]any{"a": ids[int64(i-1)], "b": ids[int64(i)], "p": map[string]any{}})
	}
	if err := writeEdges(g, "NEXT", links); err != nil {
		t.Fatal(err)
	}
	nodes, edges, err := readSnapshot(g)
	if err != nil || len(nodes) != len(rows) || len(edges) != len(links) {
		t.Fatalf("%d nodes %d edges %v", len(nodes), len(edges), err)
	}
}
