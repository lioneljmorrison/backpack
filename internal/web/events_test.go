package web

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// watch opens the workspace's events as a browser signed in with token, and
// returns its lines as they arrive.
func watch(t *testing.T, srv *httptest.Server, path, token string) <-chan string {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	return lines
}

// nextData waits for the next event's data line.
func nextData(t *testing.T, lines <-chan string) string {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("the stream ended")
			}
			if d, ok := strings.CutPrefix(l, "data: "); ok {
				return d
			}
		case <-deadline:
			t.Fatal("no event arrived")
		}
	}
}

func TestEventsNeedSigningIn(t *testing.T) {
	h, _ := withDemo(t)
	w := get(h, "/w/demo/events", "")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/sign-in" {
		t.Fatalf("status %d, location %q", w.Code, w.Header().Get("Location"))
	}
}

func TestEventsForAnUnknownWorkspaceAreNotFound(t *testing.T) {
	h, _ := withDemo(t)
	if w := get(h, "/w/nowhere/events", "bp_good"); w.Code != http.StatusNotFound {
		t.Fatalf("status %d", w.Code)
	}
}

func TestAChangeArrivesAsAnEventNamingOnlyWhatChanged(t *testing.T) {
	h, b := withDemo(t)
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "other", Name: "Other"}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close) // after the stream is closed: cleanups run last first
	lines := watch(t, srv, "/w/demo/events", "bp_good")

	must[board.Story](t)(b.AddStory("other", "tester", board.NewStory{Title: "Elsewhere"}))
	must[board.Story](t)(b.AddStory("demo", "tester", board.NewStory{Title: "Secret plans"}))
	if got, want := nextData(t, lines), `{"kind":"story","id":"S-1"}`; got != want {
		t.Fatalf("data %s, want %s", got, want)
	}
	must[board.Phase](t)(b.AddPhase("demo", "tester", "First", ""))
	if got, want := nextData(t, lines), `{"kind":"phase","id":"1"}`; got != want {
		t.Fatalf("data %s, want %s", got, want)
	}
}

func TestRevokingTheTokenEndsTheStream(t *testing.T) {
	defer func(d time.Duration) { pingEvery = d }(pingEvery)
	pingEvery = 20 * time.Millisecond
	var revoked atomic.Bool
	lookup := func(tok string) (string, bool, error) { return "tester", tok == "bp_good" && !revoked.Load(), nil }
	h, b := newSiteWith(t, lookup)
	must[board.Workspace](t)(b.CreateWorkspace("tester", board.Workspace{Slug: "demo", Name: "Demo"}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close) // after the stream is closed: cleanups run last first
	lines := watch(t, srv, "/w/demo/events", "bp_good")

	for l := range lines {
		if l == ": ping" {
			break
		}
	}
	revoked.Store(true)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-lines:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the stream outlived its token")
		}
	}
}

func TestTheBoardAndStoryPagesWatchForChangesAndOthersDont(t *testing.T) {
	h, b := withDemo(t)
	s := story(t, b, board.NewStory{Title: "Watched"})
	watching := `data-live="/w/demo/events"`
	wantPage(t, get(h, "/w/demo", "bp_good"), watching, `data-live-kinds="story phase idea"`, `src="/static/live.js?v=`)
	wantPage(t, get(h, "/w/demo/stories/"+s.ID, "bp_good"), watching, `data-live-kinds="story"`, `src="/static/live.js?v=`)
	for _, path := range []string{"/w/demo/phases", "/w/demo/ideas", "/"} {
		if page := get(h, path, "bp_good").Body.String(); strings.Contains(page, "data-live") || strings.Contains(page, "live.js") {
			t.Errorf("%s watches for changes", path)
		}
	}
}
