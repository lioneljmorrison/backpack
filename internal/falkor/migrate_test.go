package falkor_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/falkor"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

func TestMigrateAppliesEveryMigrationToAnEmptyGraph(t *testing.T) {
	g := falkortest.Graph(t)
	ms := []string{
		"CREATE INDEX FOR (a:Alpha) ON (a.name)",
		"CREATE INDEX FOR (b:Beta) ON (b.name)",
	}
	if err := falkor.Migrate(g, ms); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if v, err := falkor.Version(g); err != nil || v != 2 {
		t.Fatalf("version = %d, %v; want 2", v, err)
	}
}

func TestMigrateTwiceIsANoOp(t *testing.T) {
	g := falkortest.Graph(t)
	ms := []string{"CREATE INDEX FOR (a:Alpha) ON (a.name)"}
	if err := falkor.Migrate(g, ms); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := falkor.Migrate(g, ms); err != nil {
		t.Fatalf("second run must not re-apply: %v", err)
	}
}

func TestMigrateAppliesOnlyNewMigrations(t *testing.T) {
	g := falkortest.Graph(t)
	ms := []string{"CREATE INDEX FOR (a:Alpha) ON (a.name)"}
	if err := falkor.Migrate(g, ms); err != nil {
		t.Fatal(err)
	}
	ms = append(ms, "CREATE INDEX FOR (b:Beta) ON (b.name)")
	if err := falkor.Migrate(g, ms); err != nil {
		t.Fatalf("migrate with one new: %v", err)
	}
	if v, _ := falkor.Version(g); v != 2 {
		t.Fatalf("version = %d, want 2", v)
	}
}

func TestMigrateStopsAtAFailingMigration(t *testing.T) {
	g := falkortest.Graph(t)
	ms := []string{
		"CREATE INDEX FOR (a:Alpha) ON (a.name)",
		"THIS IS NOT CYPHER",
	}
	if err := falkor.Migrate(g, ms); err == nil {
		t.Fatal("want an error from the broken migration")
	}
	if v, _ := falkor.Version(g); v != 1 {
		t.Fatalf("version = %d, want 1 (the good migration only)", v)
	}
}

// A crash between a migration and its version update leaves the index in
// place with the version behind. The next start must finish the job, not fail.
func TestMigrateCompletesWhenAnIndexAlreadyExists(t *testing.T) {
	g := falkortest.Graph(t)
	if _, err := g.Query("CREATE INDEX FOR (a:Alpha) ON (a.name)", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := falkor.Migrate(g, []string{"CREATE INDEX FOR (a:Alpha) ON (a.name)"}); err != nil {
		t.Fatalf("migrate over an existing index: %v", err)
	}
	if v, _ := falkor.Version(g); v != 1 {
		t.Fatalf("version = %d, want 1", v)
	}
}
