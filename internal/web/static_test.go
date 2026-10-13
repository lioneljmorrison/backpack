package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func TestAStaticAddressChangesWithTheFilesContents(t *testing.T) {
	a := newAssets(fstest.MapFS{"x.css": {Data: []byte("one")}, "y.js": {Data: []byte("same")}})
	b := newAssets(fstest.MapFS{"x.css": {Data: []byte("two")}, "y.js": {Data: []byte("same")}})
	if a.url("x.css") == b.url("x.css") {
		t.Fatalf("changed file kept its address %q", a.url("x.css"))
	}
	if a.url("y.js") != b.url("y.js") {
		t.Fatalf("unchanged file moved: %q vs %q", a.url("y.js"), b.url("y.js"))
	}
	if !regexp.MustCompile(`^/static/x\.css\?v=[0-9a-f]{12}$`).MatchString(a.url("x.css")) {
		t.Fatalf("address %q, want /static/x.css?v=<12 hex>", a.url("x.css"))
	}
}

func TestPagesLoadVersionedStaticFiles(t *testing.T) {
	h, b := newSite(t)
	if _, err := b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/sign-in", "/w/demo"} {
		body := get(h, path, "bp_good").Body.String()
		for _, f := range []string{`href="/static/style.css?v=`, `src="/static/live.js?v=`} {
			if path == "/sign-in" && strings.Contains(f, "live.js") {
				continue
			}
			if !strings.Contains(body, f) {
				t.Errorf("%s: missing %s", path, f)
			}
		}
	}
	if body := get(h, "/w/demo", "bp_good").Body.String(); !strings.Contains(body, `src="/static/board.js?v=`) {
		t.Error("board page: board.js is not versioned")
	}
}

func TestOnlyAVersionedStaticAddressIsCachedForLong(t *testing.T) {
	h, _ := newSite(t)
	body := get(h, "/sign-in", "").Body.String()
	m := regexp.MustCompile(`/static/style\.css\?v=[0-9a-f]+`).FindString(body)
	if m == "" {
		t.Fatal("no versioned stylesheet address on the sign-in page")
	}
	if cc := get(h, m, "").Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("versioned: Cache-Control %q, want immutable", cc)
	}
	for _, p := range []string{"/static/style.css", "/static/style.css?v=stale"} {
		w := get(h, p, "")
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "" {
			t.Errorf("%s: %d, Cache-Control %q, want 200 and none", p, w.Code, w.Header().Get("Cache-Control"))
		}
	}
}
