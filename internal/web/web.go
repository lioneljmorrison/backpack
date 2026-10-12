// Package web serves Backpack's pages for people. A browser signs in once by
// pasting a token, which is kept in a cookie; the pages then read the board
// through the same operations as the API and the MCP server.
package web

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/live"
	"github.com/lioneljmorrison/backpack/internal/search"
)

// cookieName holds the browser's token.
const cookieName = "backpack_token"

// cookieMaxAge keeps a browser signed in for a year, so a token is pasted
// once. Revoking the token still signs the browser out at its next request.
const cookieMaxAge = 365 * 24 * 60 * 60

//go:embed templates static
var files embed.FS

type site struct {
	lookup auth.LookupFunc
	board  *board.Board
	search *search.Index
	live   *live.Hub
	pages  map[string]*template.Template
}

// page is what every template is given.
type page struct {
	Title string
	Actor string
	Error string
	// WS is the workspace a page is about, and Nav which of its pages.
	WS  *board.Workspace
	Nav string
	// Live names the kinds of change, such as "story phase", after which
	// the page fetches itself again; empty, the page doesn't watch.
	Live string
	Data any
}

// New returns the pages. Only /sign-in and /static/ are open without a token.
// Pages that watch the board hear of its changes from hub.
func New(lookup auth.LookupFunc, b *board.Board, x *search.Index, hub *live.Hub) http.Handler {
	s := &site{lookup: lookup, board: b, search: x, live: hub, pages: parse()}

	signedIn := http.NewServeMux()
	signedIn.HandleFunc("GET /{$}", s.workspaces)
	signedIn.HandleFunc("POST /sign-out", s.signOut)
	s.routePages(signedIn)
	s.routeActions(signedIn)

	static, _ := fs.Sub(files, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /sign-in", s.signInPage)
	mux.HandleFunc("POST /sign-in", s.signIn)
	mux.Handle("/", auth.RequireCookie(lookup, cookieName, "/sign-in")(signedIn))
	return http.NewCrossOriginProtection().Handler(mux)
}

func parse() map[string]*template.Template {
	names := []string{"signin", "workspaces", "board", "story", "phases", "ideas", "search", "newstory"}
	pages := make(map[string]*template.Template, len(names))
	for _, name := range names {
		pages[name] = template.Must(template.New("").Funcs(funcs).ParseFS(files, "templates/layout.html", "templates/"+name+".html"))
	}
	return pages
}

var funcs = template.FuncMap{"label": label, "when": when, "percent": percent, "cardOf": cardOf,
	"list": func(v ...string) []string { return v }}

// card is what the card template is given: a story and its workspace, and
// for a backlog story the stories either side, to move it past.
type card struct {
	WS       string
	S        board.Story
	Up, Down string
	Movable  bool
}

func cardOf(ws string, s board.Story) card { return card{WS: ws, S: s} }

// render writes the named page, or a plain error if it can't be built, so a
// half-written page is never sent.
func (s *site) render(w http.ResponseWriter, r *http.Request, status int, name string, p page) {
	p.Actor, _ = auth.ActorFrom(r.Context())
	if msg := s.takeFlash(w, r); msg != "" && p.Error == "" {
		p.Error = msg
	}
	var buf bytes.Buffer
	if err := s.pages[name].ExecuteTemplate(&buf, "layout", p); err != nil {
		log.Printf("web: render %s: %v", name, err)
		http.Error(w, "the page could not be shown", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (s *site) signInPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "signin", page{Title: "Sign in"})
}

func (s *site) signIn(w http.ResponseWriter, r *http.Request) {
	tok := r.PostFormValue("token")
	actor, ok, err := s.lookup(tok)
	if err != nil {
		log.Printf("web: sign-in: %v", err)
		s.render(w, r, http.StatusServiceUnavailable, "signin", page{Title: "Sign in", Error: "Signing in is unavailable right now. Try again shortly."})
		return
	}
	if tok == "" || !ok {
		s.render(w, r, http.StatusUnauthorized, "signin", page{Title: "Sign in", Error: "That token was not recognised. Check it, or issue a new one with make token."})
		return
	}
	log.Printf("web: %s signed in", actor)
	http.SetCookie(w, s.cookie(r, tok, cookieMaxAge))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *site) signOut(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.cookie(r, "", -1))
	http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
}

// cookie keeps tok where scripts can't read it and other sites can't send
// it. It is Secure everywhere but a loopback host, which is reached over
// plain HTTP.
func (s *site) cookie(r *http.Request, tok string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    tok,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   !loopback(r.Host),
		SameSite: http.SameSiteStrictMode,
	}
}

func loopback(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *site) workspaces(w http.ResponseWriter, r *http.Request) {
	all, err := s.board.Workspaces()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "workspaces", page{Title: "Workspaces", Data: all})
}

// fail shows a board refusal with its reason, and hides anything else.
func (s *site) fail(w http.ResponseWriter, r *http.Request, err error) {
	if e := (*board.Error)(nil); errors.As(err, &e) && e.Kind == board.NotFound {
		http.Error(w, e.Msg, http.StatusNotFound)
		return
	}
	log.Printf("web: %s %s: %v", r.Method, r.URL.Path, err)
	http.Error(w, "the board is unavailable", http.StatusServiceUnavailable)
}
