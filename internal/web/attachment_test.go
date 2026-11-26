package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	store "github.com/lioneljmorrison/backpack/internal/files"
)

// Invented contents: only the leading bytes matter to type detection.
var (
	testPNG = []byte("\x89PNG\r\n\x1a\n invented pixels")
	testMD  = []byte("# Invented notes\n")
)

// withFiles is withDemo with somewhere to keep files, and a story, an idea
// and a phase to attach them to.
func withFiles(t *testing.T) (http.Handler, *board.Board) {
	t.Helper()
	h, b := withDemo(t)
	s, err := store.New(filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	b.SetFiles(s)
	story(t, b, board.NewStory{Title: "Lay the slab"})
	must[board.Idea](t)(b.AddIdea("demo", "tester", board.NewIdea{Title: "Add a porch"}))
	must[board.Phase](t)(b.AddPhase("demo", "tester", "Foundations", ""))
	return h, b
}

// upload posts a multipart form whose "file" part is name, as a signed-in
// browser on the same origin.
func upload(h http.Handler, path, name string, data []byte, site string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	r := httptest.NewRequest("POST", "http://"+host+path, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Sec-Fetch-Site", site)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "bp_good"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// followed fetches back as the browser would after w, with its flash cookie.
func followed(h http.Handler, w *httptest.ResponseRecorder, back string) string {
	r := httptest.NewRequest("GET", back, nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "bp_good"})
	for _, c := range w.Result().Cookies() {
		if c.Name != cookieName && c.MaxAge >= 0 {
			r.AddCookie(c)
		}
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, r)
	return page.Body.String()
}

func TestFilesAreAttachedFromTheStoryIdeaAndPhasePages(t *testing.T) {
	h, b := withFiles(t)
	for _, c := range []struct{ path, back string }{
		{"/w/demo/stories/S-1/attachments", "/w/demo/stories/S-1"},
		{"/w/demo/ideas/I-1/attachments", "/w/demo/ideas#I-1"},
		{"/w/demo/phases/1/attachments", "/w/demo/phases/1"},
	} {
		w := upload(h, c.path, "shot.png", testPNG, "same-origin")
		wantRedirect(t, w, c.back)
		page := followed(h, w, strings.SplitN(c.back, "#", 2)[0])
		for _, s := range []string{"Attachments", "shot.png", `src="/w/demo/attachments/A-`, `enctype="multipart/form-data"`} {
			if !strings.Contains(page, s) {
				t.Errorf("%s does not show %q", c.back, s)
			}
		}
	}
	if st := must[board.Story](t)(b.Story("demo", "S-1")); len(st.Attachments) != 1 || st.Attachments[0].Name != "shot.png" {
		t.Fatalf("story has %+v", st.Attachments)
	}
}

func TestAMarkdownFileIsListedWithADownloadLinkNotAThumbnail(t *testing.T) {
	h, _ := withFiles(t)
	upload(h, "/w/demo/stories/S-1/attachments", "notes.md", testMD, "same-origin")
	page := get(h, "/w/demo/stories/S-1", "bp_good").Body.String()
	if !strings.Contains(page, `href="/w/demo/attachments/A-1"`) || !strings.Contains(page, "notes.md") {
		t.Error("no download link")
	}
	if strings.Contains(page, "<img") {
		t.Error("a markdown file is shown as an image")
	}
}

func TestAnAttachmentIsServedToTheCookieWithLockedDownHeaders(t *testing.T) {
	h, _ := withFiles(t)
	upload(h, "/w/demo/stories/S-1/attachments", "shot.png", testPNG, "same-origin")
	upload(h, "/w/demo/stories/S-1/attachments", "notes.md", testMD, "same-origin")
	w := get(h, "/w/demo/attachments/A-1", "bp_good")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), testPNG) || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("image: %d %q %s", w.Code, w.Body, w.Header().Get("Content-Type"))
	}
	for _, id := range []string{"A-1", "A-2"} {
		h2 := get(h, "/w/demo/attachments/"+id, "bp_good").Header()
		if h2.Get("X-Content-Type-Options") != "nosniff" || h2.Get("Content-Security-Policy") != "default-src 'none'; sandbox" ||
			h2.Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s headers %v", id, h2)
		}
	}
	if d := get(h, "/w/demo/attachments/A-2", "bp_good").Header().Get("Content-Disposition"); !strings.HasPrefix(d, "attachment") || !strings.Contains(d, "notes.md") {
		t.Errorf("markdown disposition %q", d)
	}
	if d := get(h, "/w/demo/attachments/A-1", "bp_good").Header().Get("Content-Disposition"); d != "" {
		t.Errorf("image disposition %q", d)
	}
}

func TestAnAttachmentNeedsASignIn(t *testing.T) {
	h, _ := withFiles(t)
	upload(h, "/w/demo/stories/S-1/attachments", "shot.png", testPNG, "same-origin")
	wantRedirect(t, get(h, "/w/demo/attachments/A-1", ""), "/sign-in")
	if w := get(h, "/w/demo/attachments/A-9", "bp_good"); w.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", w.Code)
	}
}

func TestAnUploadRefusalShowsOnThePage(t *testing.T) {
	h, _ := withFiles(t)
	w := upload(h, "/w/demo/stories/S-1/attachments", "notes.txt", []byte("plain"), "same-origin")
	wantRedirect(t, w, "/w/demo/stories/S-1")
	if page := followed(h, w, "/w/demo/stories/S-1"); !strings.Contains(page, "only PNG, JPEG, GIF and WebP images and .md files") {
		t.Error("the refusal is not shown")
	}
	big := append(append([]byte{}, testPNG...), make([]byte, board.MaxAttachment)...)
	w = upload(h, "/w/demo/stories/S-1/attachments", "big.png", big, "same-origin")
	wantRedirect(t, w, "/w/demo/stories/S-1")
	if page := followed(h, w, "/w/demo/stories/S-1"); !strings.Contains(page, "at most 10 MiB") {
		t.Error("the size refusal is not shown")
	}
}

func TestACrossSiteUploadIsRefused(t *testing.T) {
	h, b := withFiles(t)
	if w := upload(h, "/w/demo/stories/S-1/attachments", "shot.png", testPNG, "cross-site"); w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", w.Code)
	}
	if st := must[board.Story](t)(b.Story("demo", "S-1")); len(st.Attachments) != 0 {
		t.Fatal("a cross-site upload attached a file")
	}
}

func TestAnAttachmentIsRemovedFromItsPageAndTheLogSaysSo(t *testing.T) {
	h, b := withFiles(t)
	upload(h, "/w/demo/stories/S-1/attachments", "shot.png", testPNG, "same-origin")
	page := get(h, "/w/demo/stories/S-1", "bp_good").Body.String()
	if !strings.Contains(page, `action="/w/demo/stories/S-1/attachments/A-1/remove"`) || !strings.Contains(page, `class="secondary"`) {
		t.Fatal("no destructive Remove form")
	}
	page = act(t, h, "/w/demo/stories/S-1/attachments/A-1/remove", nil, "/w/demo/stories/S-1")
	if !strings.Contains(page, "Removed shot.png") {
		t.Error("the log does not say it was removed")
	}
	if st := must[board.Story](t)(b.Story("demo", "S-1")); len(st.Attachments) != 0 {
		t.Fatal("still attached")
	}
	if w := get(h, "/w/demo/attachments/A-1", "bp_good"); w.Code != http.StatusNotFound {
		t.Fatalf("a removed file is still served: %d", w.Code)
	}
	// ideas and phases come back to their own pages
	upload(h, "/w/demo/ideas/I-1/attachments", "shot.png", testPNG, "same-origin")
	act(t, h, "/w/demo/ideas/I-1/attachments/A-2/remove", nil, "/w/demo/ideas#I-1")
	upload(h, "/w/demo/phases/1/attachments", "shot.png", testPNG, "same-origin")
	act(t, h, "/w/demo/phases/1/attachments/A-3/remove", nil, "/w/demo/phases/1")
}
