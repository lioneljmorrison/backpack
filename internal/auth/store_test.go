package auth_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

func newStore(t *testing.T) *auth.Store {
	t.Helper()
	s, err := auth.NewStore(falkortest.Graph(t))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

func TestACreatedTokenIdentifiesItsActor(t *testing.T) {
	s := newStore(t)
	tok, err := s.CreateToken("lionel")
	if err != nil {
		t.Fatal(err)
	}
	actor, ok, err := s.ActorFor(tok)
	if err != nil || !ok || actor != "lionel" {
		t.Fatalf("ActorFor = %q, %v, %v; want lionel, true, nil", actor, ok, err)
	}
}

func TestAnUnknownTokenIdentifiesNobody(t *testing.T) {
	s := newStore(t)
	_, ok, err := s.ActorFor("bp_never-issued")
	if err != nil || ok {
		t.Fatalf("ActorFor = %v, %v; want false, nil", ok, err)
	}
}

func TestTheStoreKeepsOnlyTheHash(t *testing.T) {
	g := falkortest.Graph(t)
	s, err := auth.NewStore(g)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := s.CreateToken("lionel")
	res, err := g.Query("MATCH (t:Token) WHERE t.hash = $raw RETURN count(t)", map[string]any{"raw": tok}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Next()
	if n, _ := res.Record().GetByIndex(0); n.(int64) != 0 {
		t.Fatal("the raw token was stored")
	}
}

func TestRevokingAnActorDisablesAllItsTokensAndNoOneElses(t *testing.T) {
	s := newStore(t)
	first, _ := s.CreateToken("agent")
	second, _ := s.CreateToken("agent")
	other, _ := s.CreateToken("lionel")

	n, err := s.Revoke("agent")
	if err != nil || n != 2 {
		t.Fatalf("Revoke = %d, %v; want 2, nil", n, err)
	}
	for _, tok := range []string{first, second} {
		if _, ok, _ := s.ActorFor(tok); ok {
			t.Fatal("a revoked token still identifies its actor")
		}
	}
	if _, ok, _ := s.ActorFor(other); !ok {
		t.Fatal("revoking agent disabled lionel's token")
	}
}

func TestRevokingAnActorWithNoTokensRevokesNothing(t *testing.T) {
	s := newStore(t)
	if n, err := s.Revoke("nobody"); err != nil || n != 0 {
		t.Fatalf("Revoke = %d, %v; want 0, nil", n, err)
	}
}

func TestCreateTokenRejectsAnInvalidActorName(t *testing.T) {
	s := newStore(t)
	if _, err := s.CreateToken("has space"); err == nil {
		t.Fatal("want an error")
	}
}
