package auth

import (
	"fmt"
	"time"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/falkor"
)

// migrations for the system graph's actors and tokens. Append only.
var migrations = []string{
	"CREATE INDEX FOR (t:Token) ON (t.hash)",
	"CREATE INDEX FOR (a:Actor) ON (a.name)",
}

// Store keeps actors and their token hashes in the system graph.
type Store struct {
	g *falkordb.Graph
}

// NewStore brings the graph's indexes up to date and returns the store.
func NewStore(g *falkordb.Graph) (*Store, error) {
	if err := falkor.Migrate(g, migrations); err != nil {
		return nil, err
	}
	return &Store{g: g}, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// CreateToken issues a new token for the actor, creating the actor if new.
// The token is returned once and never stored.
func (s *Store) CreateToken(actor string) (string, error) {
	if err := ValidActorName(actor); err != nil {
		return "", err
	}
	tok, err := NewToken()
	if err != nil {
		return "", err
	}
	_, err = s.g.Query(`
		MERGE (a:Actor {name: $actor})
		ON CREATE SET a.created_at = $now
		CREATE (a)-[:HOLDS]->(:Token {hash: $hash, created_at: $now})`,
		map[string]any{"actor": actor, "hash": Hash(tok), "now": now()}, nil)
	if err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}
	return tok, nil
}

// ActorFor returns the actor holding an unrevoked token.
func (s *Store) ActorFor(token string) (string, bool, error) {
	res, err := s.g.ROQuery(`
		MATCH (a:Actor)-[:HOLDS]->(t:Token {hash: $hash})
		WHERE t.revoked_at IS NULL
		RETURN a.name`,
		map[string]any{"hash": Hash(token)}, nil)
	if err != nil {
		return "", false, fmt.Errorf("look up token: %w", err)
	}
	if !res.Next() {
		return "", false, nil
	}
	v, _ := res.Record().GetByIndex(0)
	name, _ := v.(string)
	return name, true, nil
}

// Revoke disables every live token the actor holds and returns how many.
func (s *Store) Revoke(actor string) (int, error) {
	res, err := s.g.Query(`
		MATCH (:Actor {name: $actor})-[:HOLDS]->(t:Token)
		WHERE t.revoked_at IS NULL
		SET t.revoked_at = $now
		RETURN count(t)`,
		map[string]any{"actor": actor, "now": now()}, nil)
	if err != nil {
		return 0, fmt.Errorf("revoke: %w", err)
	}
	if !res.Next() {
		return 0, nil
	}
	v, _ := res.Record().GetByIndex(0)
	n, _ := v.(int64)
	return int(n), nil
}
