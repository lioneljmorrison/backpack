// Package falkortest gives tests a throwaway graph on a real FalkorDB.
package falkortest

import (
	"crypto/rand"
	"encoding/hex"
	"os"
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
	db, err := falkor.Connect(URL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	g := db.SelectGraph("test_" + hex.EncodeToString(b))
	t.Cleanup(func() {
		_ = g.Delete()
		_ = db.Conn.Close()
	})
	return g
}
