package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

// known holds one live token, bp_good, belonging to "tester".
func known(tok string) (string, bool, error) { return "tester", tok == "bp_good", nil }

// newSite serves the pages over a real, empty board.
func newSite(t *testing.T) (http.Handler, *board.Board) {
	t.Helper()
	db, prefix := falkortest.Prefix(t)
	b, err := board.New(db, prefix+"system", prefix+"ws:")
	if err != nil {
		t.Fatal(err)
	}
	return New(known, b, nil), b
}

// get fetches path as a browser signed in with token; no token, signed out.
func get(h http.Handler, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	if token != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// post submits form to path from host, as a browser signed in with token.
func post(h http.Handler, host, path, token string, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://"+host+path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func setCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			return c
		}
	}
	return nil
}

func wantRedirect(t *testing.T, w *httptest.ResponseRecorder, to string) {
	t.Helper()
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != to {
		t.Fatalf("got %d to %q, want 303 to %q; body %s", w.Code, w.Header().Get("Location"), to, w.Body)
	}
}

func wantPage(t *testing.T, w *httptest.ResponseRecorder, shows ...string) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body %s", w.Code, w.Body)
	}
	for _, s := range shows {
		if !strings.Contains(w.Body.String(), s) {
			t.Errorf("page does not show %q", s)
		}
	}
}

func TestASignedOutBrowserIsSentToSignIn(t *testing.T) {
	h, _ := newSite(t)
	wantRedirect(t, get(h, "/", ""), "/sign-in")
}

func TestARevokedTokenIsSentToSignIn(t *testing.T) {
	h, _ := newSite(t)
	wantRedirect(t, get(h, "/", "bp_revoked"), "/sign-in")
}

func TestTheSignInPageAsksForAToken(t *testing.T) {
	h, _ := newSite(t)
	wantPage(t, get(h, "/sign-in", ""), `name="token"`)
}

func TestSigningInWithALiveTokenKeepsItInASecureCookie(t *testing.T) {
	h, _ := newSite(t)
	w := post(h, "backpack.example.com", "/sign-in", "", url.Values{"token": {"bp_good"}})
	wantRedirect(t, w, "/")
	c := setCookie(w)
	if c == nil || c.Value != "bp_good" {
		t.Fatalf("cookie %v, want the token", c)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie %+v is not HttpOnly, Secure, SameSite=Strict on /", c)
	}
}

func TestSigningInOnLocalhostKeepsTheCookieWithoutHTTPS(t *testing.T) {
	h, _ := newSite(t)
	for _, host := range []string{"localhost:8780", "127.0.0.1:8780"} {
		c := setCookie(post(h, host, "/sign-in", "", url.Values{"token": {"bp_good"}}))
		if c == nil || c.Secure {
			t.Errorf("%s: cookie %+v, want one without Secure", host, c)
		}
	}
}

func TestSigningInWithAnUnknownTokenSaysSoAndSetsNoCookie(t *testing.T) {
	h, _ := newSite(t)
	w := post(h, "backpack.example.com", "/sign-in", "", url.Values{"token": {"bp_bad"}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not recognised") {
		t.Errorf("page does not say the token was not recognised: %s", w.Body)
	}
	if setCookie(w) != nil {
		t.Error("an unknown token set a cookie")
	}
}

func TestSigningOutClearsTheCookie(t *testing.T) {
	h, _ := newSite(t)
	w := post(h, "backpack.example.com", "/sign-out", "bp_good", nil)
	wantRedirect(t, w, "/sign-in")
	if c := setCookie(w); c == nil || c.MaxAge >= 0 {
		t.Fatalf("cookie %+v, want it cleared", c)
	}
}

func TestACrossSiteFormIsRefused(t *testing.T) {
	h, _ := newSite(t)
	r := httptest.NewRequest("POST", "/sign-in", strings.NewReader("token=bp_good"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || setCookie(w) != nil {
		t.Fatalf("status %d, want 403 and no cookie", w.Code)
	}
}

func TestTheStylesheetNeedsNoSignIn(t *testing.T) {
	h, _ := newSite(t)
	w := get(h, "/static/style.css", "")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("got %d %q, want 200 text/css", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestTheHomePageListsEveryWorkspace(t *testing.T) {
	h, b := newSite(t)
	for _, ws := range []board.Workspace{{Slug: "demo", Name: "Demo"}, {Slug: "other", Name: "Other project"}} {
		if _, err := b.CreateWorkspace("tester", ws); err != nil {
			t.Fatal(err)
		}
	}
	wantPage(t, get(h, "/", "bp_good"), "Demo", `href="/w/demo"`, "Other project", `href="/w/other"`, "tester")
}

func TestAnUnknownPageIsNotFound(t *testing.T) {
	h, _ := newSite(t)
	if w := get(h, "/nowhere", "bp_good"); w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", w.Code)
	}
}
