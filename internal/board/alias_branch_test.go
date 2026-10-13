package board_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func alias(b *board.Board, branch, to string) (board.Branch, error) {
	return b.UpdateBranch("demo", actor, branch, board.BranchPatch{Alias: &to})
}

func aliasOf(t *testing.T, b *board.Board, name string) string {
	t.Helper()
	bs, err := b.Branches("demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, br := range bs {
		if br.Name == name {
			return br.Alias
		}
	}
	t.Fatalf("no branch %q", name)
	return ""
}

func TestABranchTakesAnAliasAndLosesItWhenItIsCleared(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	got, err := alias(b, "main", "Production")
	if err != nil || got.Name != "main" || got.Alias != "Production" {
		t.Fatalf("%+v, %v", got, err)
	}
	if a := aliasOf(t, b, "main"); a != "Production" {
		t.Fatalf("alias %q", a)
	}
	if a := aliasOf(t, b, "dev"); a != "" {
		t.Fatalf("dev gained alias %q", a)
	}
	// Nothing but the alias changes: stories and the log stay as they were.
	if s := wantStatus(t, b, "S-2", board.Merged); s.Into != "dev" {
		t.Fatalf("S-2 into %q", s.Into)
	}
	// An empty patch leaves the alias alone; "" clears it.
	if br, err := b.UpdateBranch("demo", actor, "main", board.BranchPatch{}); err != nil || br.Alias != "Production" {
		t.Fatalf("empty patch: %+v, %v", br, err)
	}
	if br, err := alias(b, "main", ""); err != nil || br.Alias != "" {
		t.Fatalf("clearing: %+v, %v", br, err)
	}
	if a := aliasOf(t, b, "main"); a != "" {
		t.Fatalf("alias %q after clearing", a)
	}
}

func TestRenamingAndAliasingInOnePatchAppliesBoth(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	name, al := "develop", "Staging"
	got, err := b.UpdateBranch("demo", actor, "dev", board.BranchPatch{Name: &name, Alias: &al})
	if err != nil || got.Name != "develop" || got.Alias != "Staging" {
		t.Fatalf("%+v, %v", got, err)
	}
	// A rename alone keeps the alias.
	other := "dev"
	if got, err := rename(b, "develop", other); err != nil || got.Alias != "Staging" {
		t.Fatalf("rename kept alias: %+v, %v", got, err)
	}
}

func TestAnAliasIsRefusedWhenBadAndNothingChanges(t *testing.T) {
	b := threeMergedAndOneInReview(t)
	for _, c := range []struct {
		branch, to string
		kind       board.Kind
	}{
		{"main", strings.Repeat("a", 101), board.Invalid},
		{"main", "bad​alias", board.Invalid},
		{"nope", "x", board.NotFound},
	} {
		_, err := alias(b, c.branch, c.to)
		var e *board.Error
		if !errors.As(err, &e) || e.Kind != c.kind {
			t.Fatalf("alias %q on %q: %v, want kind %v", c.to, c.branch, err, c.kind)
		}
	}
	if _, err := alias(b, "main", strings.Repeat("é", 100)); err != nil {
		t.Fatalf("100 characters is allowed: %v", err)
	}
	// A taken name refuses the whole patch, alias included.
	name, al := "main", "Nope"
	if _, err := b.UpdateBranch("demo", actor, "dev", board.BranchPatch{Name: &name, Alias: &al}); err == nil {
		t.Fatal("a taken name is refused")
	}
	if a := aliasOf(t, b, "dev"); a != "" {
		t.Fatalf("a refused patch set alias %q", a)
	}
}
