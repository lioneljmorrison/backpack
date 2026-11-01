package web

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/board"
)

// flashName holds the message an action leaves for the page it sends the
// browser back to. Only Backpack can set it, so a link can't put words on a
// page.
const flashName = "backpack_flash"

func (s *site) routeActions(mux *http.ServeMux) {
	mux.HandleFunc("POST /w/{ws}/approve", s.approve)
	mux.HandleFunc("GET /w/{ws}/stories/new", s.newStoryPage)
	mux.HandleFunc("POST /w/{ws}/stories", s.addStory)
	mux.HandleFunc("POST /w/{ws}/stories/{id}/move", s.moveStory)
	mux.HandleFunc("POST /w/{ws}/stories/{id}/status", s.setStatus)
	mux.HandleFunc("POST /w/{ws}/stories/{id}/activity", s.setActivity)
	mux.HandleFunc("POST /w/{ws}/stories/{id}/kind", s.setKind)
	mux.HandleFunc("POST /w/{ws}/stories/{id}/log", s.addLog)
	mux.HandleFunc("POST /w/{ws}/ideas", s.addIdea)
	mux.HandleFunc("POST /w/{ws}/ideas/{id}/refine", s.refineIdea)
	mux.HandleFunc("POST /w/{ws}/ideas/{id}/drop", s.dropIdea)
}

func actor(r *http.Request) string {
	a, _ := auth.ActorFrom(r.Context())
	return a
}

// done sends the browser back, with the board's reason when it refused.
func (s *site) done(w http.ResponseWriter, r *http.Request, back string, err error) {
	if err != nil {
		msg := "The board is unavailable. Try again shortly."
		if e := (*board.Error)(nil); errors.As(err, &e) {
			msg = e.Msg
		} else {
			log.Printf("web: %s %s: %v", r.Method, r.URL.Path, err)
		}
		http.SetCookie(w, s.flash(r, url.QueryEscape(msg), 60))
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (s *site) flash(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: flashName, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: !loopback(r.Host), SameSite: http.SameSiteStrictMode}
}

// takeFlash returns the message an action left, and clears it.
func (s *site) takeFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(flashName)
	if err != nil {
		return ""
	}
	http.SetCookie(w, s.flash(r, "", -1))
	msg, _ := url.QueryUnescape(c.Value)
	return msg
}

func boardURL(r *http.Request) string { return "/w/" + url.PathEscape(r.PathValue("ws")) }

func storyURL(r *http.Request, id string) string {
	return boardURL(r) + "/stories/" + url.PathEscape(id)
}

// approve approves or declines every chosen proposed story. Each is answered
// on its own, so one refusal doesn't stop the rest.
func (s *site) approve(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ws, ids := r.PathValue("ws"), r.PostForm["id"]
	c := board.StatusChange{To: board.Backlog}
	if r.PostFormValue("decision") == "decline" {
		c = board.StatusChange{To: board.Closed, Reason: strings.TrimSpace(r.PostFormValue("reason"))}
	}
	if len(ids) == 0 {
		s.done(w, r, boardURL(r), &board.Error{Kind: board.Invalid, Msg: "Choose at least one story."})
		return
	}
	var refused []string
	for _, id := range ids {
		if _, err := s.board.SetStatus(ws, actor(r), id, c); err != nil {
			if e := (*board.Error)(nil); !errors.As(err, &e) {
				s.done(w, r, boardURL(r), err)
				return
			}
			refused = append(refused, id+": "+err.Error())
		}
	}
	var err error
	if len(refused) > 0 {
		err = &board.Error{Kind: board.Conflict, Msg: strings.Join(refused, "; ")}
	}
	s.done(w, r, boardURL(r), err)
}

func (s *site) moveStory(w http.ResponseWriter, r *http.Request) {
	to := board.Position{Before: r.PostFormValue("before"), After: r.PostFormValue("after")}
	_, err := s.board.MoveStory(r.PathValue("ws"), actor(r), r.PathValue("id"), to)
	s.done(w, r, boardURL(r), err)
}

func (s *site) setStatus(w http.ResponseWriter, r *http.Request) {
	c := board.StatusChange{
		To:         r.PostFormValue("to"),
		Branch:     strings.TrimSpace(r.PostFormValue("branch")),
		PRURL:      strings.TrimSpace(r.PostFormValue("pr_url")),
		Commit:     strings.TrimSpace(r.PostFormValue("commit")),
		Into:       strings.TrimSpace(r.PostFormValue("into")),
		Reason:     strings.TrimSpace(r.PostFormValue("reason")),
		Workaround: strings.TrimSpace(r.PostFormValue("workaround")),
	}
	// A move from a story's page names a merge as "merged:branch".
	if m := board.ParseMove(c.To); m.Into != "" {
		c.To, c.Into = m.To, m.Into
	}
	ws, id := r.PathValue("ws"), r.PathValue("id")
	// A drop on the board comes back to the board. Only this one fixed value
	// is understood: back is never an address.
	back := storyURL(r, id)
	if r.PostFormValue("back") == "board" {
		back = boardURL(r)
	}
	// A story dropped into the backlog lands where it was dropped: placed
	// first, then its status changes. A drop the status flow would refuse is
	// left alone, so it never moves a story it won't then change.
	at := board.Position{Before: r.PostFormValue("before"), After: r.PostFormValue("after")}
	if c.To == board.Backlog && (at.Before != "" || at.After != "") {
		st, err := s.board.Story(ws, id)
		if err == nil && slices.ContainsFunc(board.Moves(st, nil), func(m board.Move) bool { return m.To == board.Backlog }) {
			_, err = s.board.MoveStory(ws, actor(r), id, at)
		}
		if err != nil {
			s.done(w, r, back, err)
			return
		}
	}
	_, err := s.board.SetStatus(ws, actor(r), id, c)
	s.done(w, r, back, err)
}

func (s *site) setActivity(w http.ResponseWriter, r *http.Request) {
	activity := r.PostFormValue("activity")
	patch := board.StoryPatch{Activity: &activity}
	// The form keeps what the story was waiting on; it counts only while
	// the story is still waiting.
	if waiting := strings.TrimSpace(r.PostFormValue("waiting_on")); waiting != "" && activity == "waiting" {
		patch.WaitingOn = &waiting
	}
	_, err := s.board.UpdateStory(r.PathValue("ws"), actor(r), r.PathValue("id"), patch)
	s.done(w, r, storyURL(r, r.PathValue("id")), err)
}

// setKind marks a story as a bug, or clears the mark. The form is one
// checkbox, so an unticked box sends nothing and means "clear".
func (s *site) setKind(w http.ResponseWriter, r *http.Request) {
	kind := r.PostFormValue("kind")
	_, err := s.board.UpdateStory(r.PathValue("ws"), actor(r), r.PathValue("id"), board.StoryPatch{Kind: &kind})
	s.done(w, r, storyURL(r, r.PathValue("id")), err)
}

func (s *site) addLog(w http.ResponseWriter, r *http.Request) {
	e := board.NewLogEntry{
		Kind:     r.PostFormValue("kind"),
		Text:     strings.TrimSpace(r.PostFormValue("text")),
		Reason:   strings.TrimSpace(r.PostFormValue("reason")),
		Commit:   strings.TrimSpace(r.PostFormValue("commit")),
		PRURL:    strings.TrimSpace(r.PostFormValue("pr_url")),
		Corrects: strings.TrimSpace(r.PostFormValue("corrects")),
	}
	_, err := s.board.AddLogEntry(r.PathValue("ws"), actor(r), r.PathValue("id"), e)
	s.done(w, r, storyURL(r, r.PathValue("id")), err)
}

type newStoryData struct {
	Phases  []board.Phase
	Stories []board.Story
}

func (s *site) newStoryPage(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var d newStoryData
	if d.Phases, err = s.board.Phases(ws.Slug); err == nil {
		d.Stories, err = s.board.Stories(ws.Slug)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "newstory", page{Title: "New story · " + ws.Name, WS: ws, Nav: "board", Data: d})
}

// addStory puts a person's story straight into the backlog: it needs no
// approval.
func (s *site) addStory(w http.ResponseWriter, r *http.Request) {
	n := board.NewStory{
		Title:       strings.TrimSpace(r.PostFormValue("title")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Goal:        strings.TrimSpace(r.PostFormValue("goal")),
		Plan:        strings.TrimSpace(r.PostFormValue("plan")),
		Kind:        r.PostFormValue("kind"),
		Position:    board.Position{After: r.PostFormValue("after")},
	}
	if p := r.PostFormValue("phase"); p != "" {
		var err error
		if n.Phase, err = strconv.Atoi(p); err != nil {
			s.done(w, r, boardURL(r)+"/stories/new", &board.Error{Kind: board.Invalid, Msg: "Choose a phase from the list."})
			return
		}
	}
	st, err := s.board.AddStory(r.PathValue("ws"), actor(r), n)
	if err != nil {
		s.done(w, r, boardURL(r)+"/stories/new", err)
		return
	}
	s.done(w, r, storyURL(r, st.ID), nil)
}

func ideasURL(r *http.Request) string { return boardURL(r) + "/ideas" }

func (s *site) addIdea(w http.ResponseWriter, r *http.Request) {
	i := board.NewIdea{Title: strings.TrimSpace(r.PostFormValue("title")), Context: strings.TrimSpace(r.PostFormValue("context"))}
	_, err := s.board.AddIdea(r.PathValue("ws"), actor(r), i)
	s.done(w, r, ideasURL(r), err)
}

// refineIdea links an idea to the stories it became, or to a phase.
func (s *site) refineIdea(w http.ResponseWriter, r *http.Request) {
	var to board.Refinement
	for _, id := range strings.FieldsFunc(r.PostFormValue("stories"), func(c rune) bool { return c == ',' || c == ' ' }) {
		to.Stories = append(to.Stories, strings.ToUpper(id))
	}
	if p := strings.TrimSpace(r.PostFormValue("phase")); p != "" {
		var err error
		if to.Phase, err = strconv.Atoi(p); err != nil {
			s.done(w, r, ideasURL(r), &board.Error{Kind: board.Invalid, Msg: "A phase is given by its number."})
			return
		}
	}
	_, err := s.board.RefineIdea(r.PathValue("ws"), actor(r), r.PathValue("id"), to)
	s.done(w, r, ideasURL(r), err)
}

func (s *site) dropIdea(w http.ResponseWriter, r *http.Request) {
	_, err := s.board.DropIdea(r.PathValue("ws"), actor(r), r.PathValue("id"), strings.TrimSpace(r.PostFormValue("reason")))
	s.done(w, r, ideasURL(r), err)
}
