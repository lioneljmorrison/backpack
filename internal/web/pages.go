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
	Cards  []card
}

func (s *site) boardPage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var d boardData
	var cols []board.Column
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
	for _, c := range cols {
		col := column{Status: c.Status, Cards: make([]card, len(c.Stories))}
		for i, st := range c.Stories {
			col.Cards[i] = cardOf(ws.Slug, st)
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
	s.render(w, r, http.StatusOK, "board", page{Title: ws.Name, WS: ws, Nav: "board", Live: "story phase idea", Data: d})
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
	d.Moves = board.Moves(d.Story, ws.HasReleaseStage)
	d.Titles = make(map[string]string, len(all))
	for _, st := range all {
		d.Titles[st.ID] = st.Title
	}
	s.render(w, r, http.StatusOK, "story", page{Title: d.Story.ID + " " + d.Story.Title, WS: ws, Nav: "board", Live: "story", Data: d})
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
	board.InReview: "In review", board.Merged: "Merged", board.Released: "Released",
	board.Deployed: "Deployed", board.OnHold: "On hold", board.Closed: "Closed",
	"open": "Open", "refined": "Refined", "dropped": "Dropped",
	"planning": "Planning", "building": "Building", "testing": "Testing", "waiting": "Waiting",
	"note": "Note", "decision": "Decision", "commit": "Commit", "pr": "Pull request",
	"status": "Status", "correction": "Correction",
}

func label(s string) string {
	if l, ok := labels[s]; ok {
		return l
	}
	return s
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
