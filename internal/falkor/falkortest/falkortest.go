// Package falkortest gives tests throwaway graphs on a real FalkorDB.
package falkortest

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/falkor"
)

// URL returns BACKPACK_TEST_FALKORDB, skipping the test when it is unset.
func URL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("BACKPACK_TEST_FALKORDB")
	if url == "" {
		t.Skip("BACKPACK_TEST_FALKORDB not set")
	}
	return url
}

// Graph returns a uniquely named graph, deleted when the test ends.
func Graph(t *testing.T) *falkordb.Graph {
	t.Helper()
	db, prefix := Prefix(t)
	return db.SelectGraph(prefix + "graph")
}

// Prefix returns a connection and a unique graph-name prefix. Every graph
// whose name starts with the prefix is deleted when the test ends.
func Prefix(t *testing.T) (*falkordb.FalkorDB, string) {
	t.Helper()
	db, err := falkor.Connect(URL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	prefix := "test_" + hex.EncodeToString(b) + "_"
	t.Cleanup(func() {
		names, _ := db.ListGraphs()
		for _, name := range names {
			if strings.HasPrefix(name, prefix) {
				_ = db.SelectGraph(name).Delete()
			}
		}
		_ = db.Conn.Close()
	})
	return db, prefix
}

// MarkEntry sets properties on a workspace's registry entry in the system
// graph, as a join would: tests use it to mark a workspace joined or being
// joined without running one.
func MarkEntry(t *testing.T, db *falkordb.FalkorDB, systemGraph, slug string, props map[string]any) {
	t.Helper()
	if _, err := db.SelectGraph(systemGraph).Query("MATCH (e:WorkspaceEntry {slug: $slug}) SET e += $props",
		map[string]any{"slug": slug, "props": props}, nil); err != nil {
		t.Fatalf("mark %s: %v", slug, err)
	}
}
