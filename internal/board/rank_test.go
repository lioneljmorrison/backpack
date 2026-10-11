package board

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func TestBetweenFallsStrictlyBetweenItsNeighbours(t *testing.T) {
	cases := []struct{ a, b string }{
		{"", ""},
		{"", "V"},
		{"V", ""},
		{"V", "W"},
		{"V", "V1"},
		{"Vz", "W"},
		{"1", "2"},
		{"", "1"},
		{"", "01"},
		{"z", ""},
		{"zz", ""},
		{"a0V", "a1"},
	}
	for _, c := range cases {
		k := Between(c.a, c.b)
		if c.a != "" && k <= c.a {
			t.Errorf("Between(%q, %q) = %q, not above %q", c.a, c.b, k, c.a)
		}
		if c.b != "" && k >= c.b {
			t.Errorf("Between(%q, %q) = %q, not below %q", c.a, c.b, k, c.b)
		}
	}
}

func TestBetweenNeverEndsInZeroSoThereIsAlwaysRoomBelow(t *testing.T) {
	for _, c := range []struct{ a, b string }{{"", "1"}, {"", "01"}, {"0V", "1"}, {"", ""}} {
		if k := Between(c.a, c.b); k == "" || strings.HasSuffix(k, "0") {
			t.Errorf("Between(%q, %q) = %q", c.a, c.b, k)
		}
	}
}

// Many random inserts, as a busy backlog would see, keep a strict order.
func TestRandomInsertsKeepAStrictOrder(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	keys := []string{}
	for range 2000 {
		i := r.IntN(len(keys) + 1)
		var a, b string
		if i > 0 {
			a = keys[i-1]
		}
		if i < len(keys) {
			b = keys[i]
		}
		keys = slices.Insert(keys, i, Between(a, b))
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("keys %d and %d out of order: %q >= %q", i-1, i, keys[i-1], keys[i])
		}
	}
}

func TestRepeatedInsertsAtTheTopStayShort(t *testing.T) {
	top := ""
	for range 200 {
		top = Between("", top)
	}
	if len(top) > 40 {
		t.Fatalf("key grew to %d characters", len(top))
	}
}
