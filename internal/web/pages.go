package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/search"
)

func (s *site) routePages(mux *http.ServeMux) {
	mux.HandleFunc("GET /w/{ws}", s.boardPage)
	mux.HandleFunc("GET /w/{ws}/stories/{id}", s.storyPage)
	mux.HandleFunc("GET /w/{ws}/phases", s.phasesPage)
	mux.HandleFunc("GET /w/{ws}/releases", s.releasesPage)
	mux.HandleFunc("GET /w/{ws}/releases/{id}", s.releasePage)
	mux.HandleFunc("GET /w/{ws}/ideas", s.ideasPage)
	mux.HandleFunc("GET /w/{ws}/search", s.searchPage)
	mux.HandleFunc("GET /w/{ws}/events", s.events)
}

// workspace finds the workspace a page is about.
func (s *site) workspace(slug string) (*board.Workspace, error) {
	all, err := s.board.Workspaces()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Slug == slug {
			return &all[i], nil
		}
	}
	return nil, &board.Error{Kind: board.NotFound, Msg: "no workspace " + slug}
}

type boardData struct {
	Where board.Where
	Next  *board.Story
	// Ideas are the open ideas, shown as the first column.
	Ideas   []board.Idea
	Columns []column
}

type column struct {
	Status string
	// Into names the branch of a merged column.
	Into  string
	Title string
	Cards []card
}

func (s *site) boardPage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var d boardData
	var cols []board.Column
	bs, err := s.board.Branches(ws.Slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Where, err = s.board.Where(ws.Slug); err == nil {
		if d.Next, err = s.board.Next(ws.Slug); err == nil {
			if cols, err = s.board.Board(ws.Slug); err == nil {
				d.Ideas, err = s.board.Ideas(ws.Slug)
			}
		}
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A column's heading shows its branch's alias; everything else keeps the name.
	aliases := map[string]string{}
	for _, br := range bs {
		if br.Alias != "" {
			aliases[br.Name] = br.Alias
		}
	}
	for _, c := range cols {
		heading := c.Branch
		if a, ok := aliases[heading]; ok {
			heading = a
		}
		col := column{Status: c.Status, Into: c.Branch, Title: statusLabel(c.Status, heading), Cards: make([]card, len(c.Stories))}
		for i, st := range c.Stories {
			col.Cards[i] = cardOf(ws.Slug, st)
			col.Cards[i].Moves = moveKeys(board.Moves(st, bs))
			// The backlog is where order is chosen; a card moves past its
			// neighbours there.
			if c.Status == board.Backlog {
				col.Cards[i].Movable = true
				if i > 0 {
					col.Cards[i].Up = c.Stories[i-1].ID
				}
				if i < len(c.Stories)-1 {
					col.Cards[i].Down = c.Stories[i+1].ID
				}
			}
		}
		d.Columns = append(d.Columns, col)
	}
	s.render(w, r, http.StatusOK, "board", page{Title: ws.Name, WS: ws, Nav: "board", Live: "story phase idea branch", Data: d})
}

type storyData struct {
	Story board.Story
	// Moves are the statuses the story can change to.
	Moves []string
	Log   []board.LogEntry
	// Titles names every story in the workspace, for its dependencies.
	Titles map[string]string
}

func (s *site) storyPage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var d storyData
	var all []board.Story
	if d.Story, err = s.board.Story(ws.Slug, r.PathValue("id")); err == nil {
		if d.Log, err = s.board.Log(ws.Slug, d.Story.ID); err == nil {
			all, err = s.board.Stories(ws.Slug)
		}
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	bs, err := s.board.Branches(ws.Slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d.Moves = moveKeys(board.Moves(d.Story, bs))
	d.Titles = make(map[string]string, len(all))
	for _, st := range all {
		d.Titles[st.ID] = st.Title
	}
	s.render(w, r, http.StatusOK, "story", page{Title: d.Story.ID + " " + d.Story.Title, WS: ws, Nav: "board", Live: "story branch", Data: d})
}

type phaseData struct {
	Phase   board.Phase
	Stories []board.Story
}

func (s *site) phasesPage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var ps []board.Phase
	var all []board.Story
	if ps, err = s.board.Phases(ws.Slug); err == nil {
		all, err = s.board.Stories(ws.Slug)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := make([]phaseData, len(ps))
	for i, p := range ps {
		d[i].Phase = p
		for _, st := range all {
			if st.Phase == p.Number {
				d[i].Stories = append(d[i].Stories, st)
			}
		}
	}
	s.render(w, r, http.StatusOK, "phases", page{Title: "Phases · " + ws.Name, WS: ws, Nav: "phases", Data: d})
}

func (s *site) releasesPage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rs, err := s.board.ListReleases(ws.Slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "releases", page{Title: "Releases · " + ws.Name, WS: ws, Nav: "releases", Live: "story", Data: rs})
}

func (s *site) releasePage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rel, err := s.board.GetRelease(ws.Slug, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "release", page{Title: rel.ID + " " + rel.Version, WS: ws, Nav: "releases", Live: "story", Data: rel})
}

func (s *site) ideasPage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ideas, err := s.board.Ideas(ws.Slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "ideas", page{Title: "Ideas · " + ws.Name, WS: ws, Nav: "ideas", Data: ideas})
}

type searchData struct {
	Query   string
	Results []search.Result
}

func (s *site) searchPage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := searchData{Query: strings.TrimSpace(r.FormValue("q"))}
	p := page{Title: "Search · " + ws.Name, WS: ws, Nav: "search", Data: &d}
	switch {
	case d.Query == "":
	case s.search == nil:
		p.Error = "Search is unavailable."
	default:
		d.Results, err = s.search.Search(r.Context(), ws.Slug, d.Query)
		if errors.Is(err, search.ErrUnavailable) {
			p.Error = "Search is unavailable until the index is rebuilt (make reindex)."
		} else if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.render(w, r, http.StatusOK, "search", p)
}

// labels are how statuses, activities and log kinds read on a page.
var labels = map[string]string{
	board.Proposed: "Proposed", board.Backlog: "Backlog", board.InProgress: "In progress",
	board.InReview: "In review", board.Merged: "Merged",
	board.Released: "Released", board.OnHold: "On hold", board.Closed: "Closed",
	"open": "Open", "refined": "Refined", "dropped": "Dropped",
	"planning": "Planning", "building": "Building", "testing": "Testing", "waiting": "Waiting",
	"note": "Note", "decision": "Decision", "commit": "Commit", "pr": "Pull request",
	"status": "Status", "correction": "Correction",
	"deployed": "Deployed", // no longer a status; old log entries and search states may say it
}

func label(s string) string {
	// A merge move reads "merged:main"; a release entry reads "released:main".
	if to, into, ok := strings.Cut(s, ":"); ok {
		return statusLabel(to, into)
	}
	if l, ok := labels[s]; ok {
		return l
	}
	return s
}

// statusLabel is a status as a column or tag names it: a merged story is in
// the branch it reached, as in "Merged (main)".
func statusLabel(status, into string) string {
	if into != "" && status == board.Merged {
		return label(status) + " (" + into + ")"
	}
	return label(status)
}

// storyLabel is where a story stands, by its status and the branch it reached.
func storyLabel(s board.Story) string { return statusLabel(s.Status, s.Into) }

// moveKeys are moves as the page writes them, "merged:main" for a merge.
func moveKeys(ms []board.Move) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.String()
	}
	return out
}

// when shows a recorded time to the minute, in UTC like the record.
func when(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return at
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// percent is how far done of total is, for a progress bar.
func percent(done, total int) int {
	if total == 0 {
		return 0
	}
	return done * 100 / total
}
