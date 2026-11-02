package board

import (
	"fmt"

	"github.com/FalkorDB/falkordb-go/v2"
)

// joinChunk is how many rows one write query carries.
const joinChunk = 500

// writeNodes creates nodes carrying the given labels (such as "Story", or
// "A:B"), one per row. A row is {"k": <int64 key>, "p": <properties>}; the
// answer maps each key to the new node's internal id.
func writeNodes(g *falkordb.Graph, labels string, rows []map[string]any) (map[int64]int64, error) {
	out := make(map[int64]int64, len(rows))
	for from := 0; from < len(rows); from += joinChunk {
		chunk := rows[from:min(from+joinChunk, len(rows))]
		res, err := g.Query("UNWIND $rows AS r CREATE (n:"+labels+") SET n += r.p RETURN r.k, id(n)",
			map[string]any{"rows": toAny(chunk)}, nil)
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", labels, err)
		}
		for res.Next() {
			k, _ := res.Record().GetByIndex(0)
			id, _ := res.Record().GetByIndex(1)
			ki, kok := k.(int64)
			ii, iok := id.(int64)
			if !kok || !iok {
				return nil, fmt.Errorf("write %s: an answer came back as %T, %T", labels, k, id)
			}
			out[ki] = ii
		}
	}
	if len(out) != len(rows) {
		return nil, fmt.Errorf("write %s: %d nodes written for %d rows", labels, len(out), len(rows))
	}
	return out, nil
}

// writeEdges creates edges of one type, one per row: {"a": <from id>, "b":
// <to id>, "p": <properties>}, the ids being internal ones.
func writeEdges(g *falkordb.Graph, typ string, rows []map[string]any) error {
	for from := 0; from < len(rows); from += joinChunk {
		chunk := rows[from:min(from+joinChunk, len(rows))]
		res, err := g.Query(`UNWIND $rows AS r MATCH (a) WHERE id(a) = r.a MATCH (b) WHERE id(b) = r.b
			CREATE (a)-[e:`+typ+`]->(b) SET e += r.p RETURN count(e)`, map[string]any{"rows": toAny(chunk)}, nil)
		if err != nil {
			return fmt.Errorf("write %s: %w", typ, err)
		}
		if !res.Next() {
			return fmt.Errorf("write %s: no answer", typ)
		}
		v, _ := res.Record().GetByIndex(0)
		if n, _ := v.(int64); int(n) != len(chunk) {
			return fmt.Errorf("write %s: %d links written for %d rows", typ, n, len(chunk))
		}
	}
	return nil
}

func toAny(rows []map[string]any) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out
}
