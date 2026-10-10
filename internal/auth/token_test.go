package auth

import (
	"strings"
	"testing"
)

func TestNewTokenHasThePrefixAndEnoughRandomness(t *testing.T) {
	tok, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "bp_") {
		t.Fatalf("token %q lacks the bp_ prefix", tok)
	}
	// 32 random bytes, base64url without padding, is 43 characters.
	if got := len(tok) - len("bp_"); got != 43 {
		t.Fatalf("token body is %d characters, want 43", got)
	}
}

func TestNewTokenNeverRepeats(t *testing.T) {
	a, _ := NewToken()
	b, _ := NewToken()
	if a == b {
		t.Fatal("two tokens were identical")
	}
}

func TestHashIsStableHexSHA256(t *testing.T) {
	h := Hash("bp_example")
	if h != Hash("bp_example") {
		t.Fatal("hash is not deterministic")
	}
	if len(h) != 64 || strings.ToLower(h) != h {
		t.Fatalf("hash %q is not lowercase hex SHA-256", h)
	}
	if h == Hash("bp_other") {
		t.Fatal("different tokens hashed the same")
	}
}

func TestValidActorName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"lionel", true},
		{"claude-backpack", true},
		{"agent_2.dev", true},
		{"", false},
		{"has space", false},
		{"semi;colon", false},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
	}
	for _, c := range cases {
		err := ValidActorName(c.name)
		if (err == nil) != c.ok {
			t.Errorf("ValidActorName(%q) error = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
