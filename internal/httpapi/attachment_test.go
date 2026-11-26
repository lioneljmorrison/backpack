package httpapi

import (
	"bytes"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
	"github.com/lioneljmorrison/backpack/internal/files"
	"github.com/lioneljmorrison/backpack/internal/health"
)

// Invented contents: only the leading bytes matter to type detection.
var (
	testPNG = []byte("\x89PNG\r\n\x1a\n invented pixels")
	testMD  = []byte("# Invented notes\n")
)

// withFilesDemo is withDemo with somewhere to keep files and a story, an idea
// and a phase to attach them to.
func withFilesDemo(t *testing.T) http.Handler {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	s, err := files.New(filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	b.SetFiles(s)
	tester := func(tok string) (string, bool, error) { return "tester", tok == "bp_good", nil }
	h := New(map[string]health.Check{}, tester, b, nil, nil)
	want(t, call(t, h, "POST", "/v1/workspaces", `{"slug": "demo", "name": "Demo"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "S"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/ideas", `{"title": "I"}`), http.StatusCreated)
	want(t, call(t, h, "POST", "/v1/workspaces/demo/phases", `{"title": "P"}`), http.StatusCreated)
	return h
}

// upload posts a multipart form with one part named field.
func upload(t *testing.T, h http.Handler, path, field, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile(field, name)
	part.Write(data)
	mw.Close()
	r := httptest.NewRequest("POST", path, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Authorization", "Bearer bp_good")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestFilesAreUploadedAndFetchedOverHTTP(t *testing.T) {
	h := withFilesDemo(t)
	for _, c := range []struct{ path, id string }{
		{"/v1/workspaces/demo/stories/S-1/attachments", "A-1"},
		{"/v1/workspaces/demo/ideas/I-1/attachments", "A-2"},
		{"/v1/workspaces/demo/phases/1/attachments", "A-3"},
	} {
		a := want(t, upload(t, h, c.path, "file", "shot.png", testPNG), http.StatusCreated)
		if a["id"] != c.id || a["name"] != "shot.png" || a["type"] != "image/png" || a["size"] != float64(len(testPNG)) ||
			a["actor"] != "tester" || a["at"] == "" || a["key"] != nil {
			t.Fatalf("uploaded %v", a)
		}
	}
	w := call(t, h, "GET", "/v1/workspaces/demo/attachments/A-1", "")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), testPNG) {
		t.Fatalf("fetched %d %q", w.Code, w.Body)
	}
	story := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-1", ""), http.StatusOK)
	got, _ := story["attachments"].([]any)
	if len(got) != 1 || got[0].(map[string]any)["id"] != "A-1" || got[0].(map[string]any)["name"] != "shot.png" ||
		got[0].(map[string]any)["key"] != nil {
		t.Fatalf("story %v", story)
	}
	// Left out when there are none.
	want(t, call(t, h, "POST", "/v1/workspaces/demo/stories", `{"title": "T"}`), http.StatusCreated)
	if s := want(t, call(t, h, "GET", "/v1/workspaces/demo/stories/S-2", ""), http.StatusOK); s["attachments"] != nil {
		t.Fatalf("empty list sent: %v", s)
	}
}

// Every file response must keep a browser from running it and a cache from
// keeping it, whatever the file claims to be.
func TestEveryFileResponseIsLockedDown(t *testing.T) {
	h := withFilesDemo(t)
	want(t, upload(t, h, "/v1/workspaces/demo/stories/S-1/attachments", "file", "shot.png", testPNG), http.StatusCreated)
	want(t, upload(t, h, "/v1/workspaces/demo/stories/S-1/attachments", "file", `we"ird name.md`, testMD), http.StatusCreated)
	for _, id := range []string{"A-1", "A-2"} {
		w := call(t, h, "GET", "/v1/workspaces/demo/attachments/"+id, "")
		for k, v := range map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"Content-Security-Policy": "default-src 'none'; sandbox",
			"Cache-Control":           "private, no-store",
		} {
			if got := w.Header().Get(k); got != v {
				t.Errorf("%s: %s = %q, want %q", id, k, got, v)
			}
		}
	}
	if w := call(t, h, "GET", "/v1/workspaces/demo/attachments/A-1", ""); w.Header().Get("Content-Type") != "image/png" ||
		w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("image headers %v", w.Header())
	}
	w := call(t, h, "GET", "/v1/workspaces/demo/attachments/A-2", "")
	disp, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
	if w.Header().Get("Content-Type") != "text/markdown; charset=utf-8" || err != nil || disp != "attachment" ||
		params["filename"] != `we"ird name.md` {
		t.Fatalf("markdown headers %v (%v %v)", w.Header(), disp, params)
	}
}

func TestFilesNeedAToken(t *testing.T) {
	h := withFilesDemo(t)
	want(t, upload(t, h, "/v1/workspaces/demo/stories/S-1/attachments", "file", "a.png", testPNG), http.StatusCreated)
	for _, c := range [][2]string{
		{"POST", "/v1/workspaces/demo/stories/S-1/attachments"},
		{"GET", "/v1/workspaces/demo/attachments/A-1"},
		{"DELETE", "/v1/workspaces/demo/attachments/A-1"},
	} {
		if w := serve(h, c[0], c[1], ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token: %d", c[0], c[1], w.Code)
		}
		if w := serve(h, c[0], c[1], "bp_bad"); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a bad token: %d", c[0], c[1], w.Code)
		}
	}
}

func TestUploadsAreRefusedByTypeSizeAndShape(t *testing.T) {
	h := withFilesDemo(t)
	path := "/v1/workspaces/demo/stories/S-1/attachments"
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	want(t, upload(t, h, path, "file", "logo.svg", svg), http.StatusUnsupportedMediaType)
	want(t, upload(t, h, path, "file", "page.png", []byte("<html><script>x</script></html>")), http.StatusUnsupportedMediaType)
	want(t, upload(t, h, path, "other", "a.png", testPNG), http.StatusBadRequest)
	want(t, upload(t, h, path, "file", "", testPNG), http.StatusBadRequest)
	want(t, upload(t, h, "/v1/workspaces/demo/stories/S-9/attachments", "file", "a.png", testPNG), http.StatusNotFound)
	want(t, upload(t, h, "/v1/workspaces/demo/phases/x/attachments", "file", "a.png", testPNG), http.StatusBadRequest)
	want(t, upload(t, h, "/v1/workspaces/nope/stories/S-1/attachments", "file", "a.png", testPNG), http.StatusNotFound)
	if w := call(t, h, "POST", path, `{"json": true}`); w.Code != http.StatusBadRequest {
		t.Fatalf("not multipart: %d", w.Code)
	}

	over := append(append([]byte{}, testPNG...), make([]byte, board.MaxAttachment)...)
	want(t, upload(t, h, path, "file", "big.png", over), http.StatusRequestEntityTooLarge)
	exact := append(append([]byte{}, testPNG...), make([]byte, board.MaxAttachment-len(testPNG))...)
	want(t, upload(t, h, path, "file", "exact.png", exact), http.StatusCreated)
	// A body far over the limit is cut off, not read to the end.
	huge := make([]byte, board.MaxAttachment+1<<20)
	want(t, upload(t, h, path, "file", "huge.png", huge), http.StatusRequestEntityTooLarge)
}

func TestTheJSONBodyLimitStaysSmall(t *testing.T) {
	h := withFilesDemo(t)
	big := `{"title": "` + strings.Repeat("x", maxBody) + `"}`
	if w := call(t, h, "POST", "/v1/workspaces/demo/stories", big); w.Code != http.StatusBadRequest {
		t.Fatalf("a JSON body over the limit: %d", w.Code)
	}
}

func TestAnAttachmentIsRemovedOverHTTP(t *testing.T) {
	h := withFilesDemo(t)
	want(t, upload(t, h, "/v1/workspaces/demo/ideas/I-1/attachments", "file", "a.png", testPNG), http.StatusCreated)
	want(t, call(t, h, "DELETE", "/v1/workspaces/demo/attachments/A-1", ""), http.StatusNoContent)
	want(t, call(t, h, "GET", "/v1/workspaces/demo/attachments/A-1", ""), http.StatusNotFound)
	want(t, call(t, h, "DELETE", "/v1/workspaces/demo/attachments/A-1", ""), http.StatusNotFound)
}

// deadlineRecorder notes the deadlines a handler sets for its own request.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	read, write time.Time
}

func (d *deadlineRecorder) SetReadDeadline(t time.Time) error  { d.read = t; return nil }
func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error { d.write = t; return nil }

// The server's own timeouts stay short; a transfer asks for more for itself.
func TestUploadsAndDownloadsExtendTheirOwnDeadlines(t *testing.T) {
	h := withFilesDemo(t)
	want(t, upload(t, h, "/v1/workspaces/demo/stories/S-1/attachments", "file", "a.png", testPNG), http.StatusCreated)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	p, _ := mw.CreateFormFile("file", "b.png")
	p.Write(testPNG)
	mw.Close()
	up := httptest.NewRequest("POST", "/v1/workspaces/demo/stories/S-1/attachments", &body)
	up.Header.Set("Content-Type", mw.FormDataContentType())
	down := httptest.NewRequest("GET", "/v1/workspaces/demo/attachments/A-1", nil)
	for _, r := range []*http.Request{up, down} {
		r.Header.Set("Authorization", "Bearer bp_good")
		d := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
		h.ServeHTTP(d, r)
		if d.Code/100 != 2 {
			t.Fatalf("%s: %d", r.Method, d.Code)
		}
		for _, dl := range []time.Time{d.read, d.write} {
			if time.Until(dl) < time.Minute {
				t.Errorf("%s: deadline %v is not extended", r.Method, dl)
			}
		}
	}
	// Other routes leave the server's deadlines alone.
	d := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	r := httptest.NewRequest("GET", "/v1/workspaces/demo/stories/S-1", nil)
	r.Header.Set("Authorization", "Bearer bp_good")
	h.ServeHTTP(d, r)
	if !d.read.IsZero() || !d.write.IsZero() {
		t.Fatal("a plain read changed the deadlines")
	}
}
