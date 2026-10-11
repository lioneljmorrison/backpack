package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/falkor"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestUsageErrorsExitWithTwo(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"bogus"},
		{"token"},
		{"token", "create"},
		{"token", "revoke"},
		{"token", "rotate", "--actor", "x"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, env(nil), &out, &errOut); code != 2 {
			t.Errorf("run(%q) = %d, want 2", args, code)
		}
		if errOut.Len() == 0 {
			t.Errorf("run(%q) printed no usage", args)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	c := loadConfig(env(nil))
	if c.Addr != ":8780" || c.FalkorURL != "falkor://localhost:6379" ||
		c.TypesenseURL != "http://localhost:8108" || c.SystemGraph != "backpack" {
		t.Fatalf("defaults = %+v", c)
	}
}

func TestConfigReadsTheEnvironment(t *testing.T) {
	c := loadConfig(env(map[string]string{
		"BACKPACK_ADDR":          ":9000",
		"BACKPACK_FALKORDB_URL":  "falkor://db:6379",
		"BACKPACK_TYPESENSE_URL": "http://ts:8108",
		"BACKPACK_SYSTEM_GRAPH":  "other",
	}))
	if c.Addr != ":9000" || c.FalkorURL != "falkor://db:6379" ||
		c.TypesenseURL != "http://ts:8108" || c.SystemGraph != "other" {
		t.Fatalf("config = %+v", c)
	}
}

func TestTokenCreatePrintsAWorkingTokenAndRevokeDisablesIt(t *testing.T) {
	url := falkortest.URL(t)
	g := falkortest.Graph(t) // only for its unique name and cleanup
	e := env(map[string]string{"BACKPACK_FALKORDB_URL": url, "BACKPACK_SYSTEM_GRAPH": g.Id})

	var out, errOut bytes.Buffer
	if code := run([]string{"token", "create", "--actor", "lionel"}, e, &out, &errOut); code != 0 {
		t.Fatalf("create exit %d: %s", code, errOut.String())
	}
	tok := strings.TrimSpace(out.String())

	db, _ := falkor.Connect(url)
	defer db.Conn.Close()
	store, err := auth.NewStore(db.SelectGraph(g.Id))
	if err != nil {
		t.Fatal(err)
	}
	if actor, ok, _ := store.ActorFor(tok); !ok || actor != "lionel" {
		t.Fatalf("printed token %q does not identify lionel", tok)
	}

	out.Reset()
	if code := run([]string{"token", "revoke", "--actor", "lionel"}, e, &out, &errOut); code != 0 {
		t.Fatalf("revoke exit %d: %s", code, errOut.String())
	}
	if _, ok, _ := store.ActorFor(tok); ok {
		t.Fatal("token still works after revoke")
	}
	if !strings.Contains(out.String(), "1") {
		t.Fatalf("revoke output %q does not say how many tokens", out.String())
	}
}
