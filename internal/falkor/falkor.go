// Package falkor connects to FalkorDB and keeps a graph's indexes and
// constraints at the version the code expects.
package falkor

import (
	"context"
	"fmt"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
)

// Connect parses a falkor:// (or redis://) URL. It does not contact the
// server, so the service can start while FalkorDB is still coming up.
func Connect(url string) (*falkordb.FalkorDB, error) {
	db, err := falkordb.FromURL(url)
	if err != nil {
		return nil, fmt.Errorf("falkordb url: %w", err)
	}
	return db, nil
}

// Ping reports whether the server answers.
func Ping(db *falkordb.FalkorDB) error {
	return db.Conn.Ping(context.Background()).Err()
}

// Version is the number of migrations already applied to the graph.
func Version(g *falkordb.Graph) (int, error) {
	res, err := g.Query("MATCH (s:SchemaVersion) RETURN s.v", nil, nil)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	if !res.Next() {
		return 0, nil
	}
	v, _ := res.Record().GetByIndex(0)
	n, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("schema version is %T, want int64", v)
	}
	return int(n), nil
}

// Migrate applies, in order, every migration the graph has not yet had.
// Migration n (1-based) is ms[n-1]. Index and constraint changes cannot share
// a query with a write, so each migration is followed by its own version
// update; a failure leaves the version at the last migration that succeeded.
// An index that already exists counts as applied: it is what a crash between
// the two queries leaves behind.
func Migrate(g *falkordb.Graph, ms []string) error {
	v, err := Version(g)
	if err != nil {
		return err
	}
	for i := v; i < len(ms); i++ {
		if _, err := g.Query(ms[i], nil, nil); err != nil && !strings.Contains(err.Error(), "already indexed") {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := g.Query("MERGE (s:SchemaVersion) SET s.v = $v", map[string]any{"v": i + 1}, nil); err != nil {
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
	}
	return nil
}
