package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/board"
)

const maxBody = 1 << 20

func routeBoard(mux *http.ServeMux, b *board.Board) {
	a := boardAPI{b}
	mux.HandleFunc("GET /v1/workspaces", a.listWorkspaces)
	mux.HandleFunc("POST /v1/workspaces", a.createWorkspace)
	mux.HandleFunc("GET /v1/workspaces/{ws}/next", a.next)
	mux.HandleFunc("GET /v1/workspaces/{ws}/status", a.where)
	mux.HandleFunc("GET /v1/workspaces/{ws}/board", a.board)
	mux.HandleFunc("GET /v1/workspaces/{ws}/phases", a.listPhases)
	mux.HandleFunc("POST /v1/workspaces/{ws}/phases", a.addPhase)
	mux.HandleFunc("PATCH /v1/workspaces/{ws}/phases/{n}", a.updatePhase)
	mux.HandleFunc("POST /v1/workspaces/{ws}/phases/{n}/goals", a.addGoal)
	mux.HandleFunc("PATCH /v1/workspaces/{ws}/goals/{id}", a.setGoalMet)
	mux.HandleFunc("GET /v1/workspaces/{ws}/stories", a.listStories)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories", a.addStory)
	mux.HandleFunc("GET /v1/workspaces/{ws}/stories/{id}", a.getStory)
	mux.HandleFunc("PATCH /v1/workspaces/{ws}/stories/{id}", a.updateStory)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/move", a.moveStory)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/status", a.setStatus)
	mux.HandleFunc("GET /v1/workspaces/{ws}/stories/{id}/log", a.readLog)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/log", a.addLogEntry)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/dependencies", a.addDependency)
	mux.HandleFunc("DELETE /v1/workspaces/{ws}/stories/{id}/dependencies/{on}", a.removeDependency)
}

type boardAPI struct{ b *board.Board }

func (a boardAPI) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	ws, err := a.b.Workspaces()
	respond(w, http.StatusOK, map[string]any{"workspaces": ws}, err)
}

func (a boardAPI) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slug            string `json:"slug"`
		Name            string `json:"name"`
		RepoURL         string `json:"repo_url"`
		HasReleaseStage bool   `json:"has_release_stage"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	ws, err := a.b.CreateWorkspace(actor(r), board.Workspace{Slug: in.Slug, Name: in.Name, RepoURL: in.RepoURL,
		HasReleaseStage: in.HasReleaseStage})
	respond(w, http.StatusCreated, ws, err)
}

func (a boardAPI) next(w http.ResponseWriter, r *http.Request) {
	s, err := a.b.Next(r.PathValue("ws"))
	respond(w, http.StatusOK, map[string]any{"story": s}, err)
}

// where answers as JSON, or as plain text when the caller asks for it, as
// the session hook does.
func (a boardAPI) where(w http.ResponseWriter, r *http.Request) {
	where, err := a.b.Where(r.PathValue("ws"))
	if err != nil || !strings.Contains(r.Header.Get("Accept"), "text/plain") {
		respond(w, http.StatusOK, where, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, where.Text())
}

func (a boardAPI) board(w http.ResponseWriter, r *http.Request) {
	cols, err := a.b.Board(r.PathValue("ws"))
	respond(w, http.StatusOK, map[string]any{"columns": cols}, err)
}

func (a boardAPI) listPhases(w http.ResponseWriter, r *http.Request) {
	ps, err := a.b.Phases(r.PathValue("ws"))
	respond(w, http.StatusOK, map[string]any{"phases": ps}, err)
}

func (a boardAPI) addPhase(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	p, err := a.b.AddPhase(r.PathValue("ws"), actor(r), in.Title, in.Description)
	respond(w, http.StatusCreated, p, err)
}

func (a boardAPI) updatePhase(w http.ResponseWriter, r *http.Request) {
	var in board.PhasePatch
	n, ok := phaseNumber(w, r)
	if !ok || !decodeBody(w, r, &in) {
		return
	}
	p, err := a.b.UpdatePhase(r.PathValue("ws"), actor(r), n, in)
	respond(w, http.StatusOK, p, err)
}

func (a boardAPI) addGoal(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	n, ok := phaseNumber(w, r)
	if !ok || !decodeBody(w, r, &in) {
		return
	}
	g, err := a.b.AddGoal(r.PathValue("ws"), actor(r), n, in.Text)
	respond(w, http.StatusCreated, g, err)
}

func (a boardAPI) setGoalMet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Met *bool `json:"met"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if in.Met == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "met is required"})
		return
	}
	g, err := a.b.SetGoalMet(r.PathValue("ws"), actor(r), r.PathValue("id"), *in.Met)
	respond(w, http.StatusOK, g, err)
}

func (a boardAPI) listStories(w http.ResponseWriter, r *http.Request) {
	ss, err := a.b.Stories(r.PathValue("ws"))
	respond(w, http.StatusOK, map[string]any{"stories": ss}, err)
}

func (a boardAPI) addStory(w http.ResponseWriter, r *http.Request) {
	var in board.NewStory
	if !decodeBody(w, r, &in) {
		return
	}
	s, err := a.b.AddStory(r.PathValue("ws"), actor(r), in)
	respond(w, http.StatusCreated, s, err)
}

func (a boardAPI) getStory(w http.ResponseWriter, r *http.Request) {
	s, err := a.b.Story(r.PathValue("ws"), r.PathValue("id"))
	respond(w, http.StatusOK, s, err)
}

func (a boardAPI) updateStory(w http.ResponseWriter, r *http.Request) {
	var in board.StoryPatch
	if !decodeBody(w, r, &in) {
		return
	}
	s, err := a.b.UpdateStory(r.PathValue("ws"), actor(r), r.PathValue("id"), in)
	respond(w, http.StatusOK, s, err)
}

func (a boardAPI) moveStory(w http.ResponseWriter, r *http.Request) {
	var in board.Position
	if !decodeBody(w, r, &in) {
		return
	}
	s, err := a.b.MoveStory(r.PathValue("ws"), actor(r), r.PathValue("id"), in)
	respond(w, http.StatusOK, s, err)
}

func (a boardAPI) setStatus(w http.ResponseWriter, r *http.Request) {
	var in board.StatusChange
	if !decodeBody(w, r, &in) {
		return
	}
	s, err := a.b.SetStatus(r.PathValue("ws"), actor(r), r.PathValue("id"), in)
	respond(w, http.StatusOK, s, err)
}

func (a boardAPI) readLog(w http.ResponseWriter, r *http.Request) {
	l, err := a.b.Log(r.PathValue("ws"), r.PathValue("id"))
	respond(w, http.StatusOK, map[string]any{"entries": l}, err)
}

func (a boardAPI) addLogEntry(w http.ResponseWriter, r *http.Request) {
	var in board.NewLogEntry
	if !decodeBody(w, r, &in) {
		return
	}
	e, err := a.b.AddLogEntry(r.PathValue("ws"), actor(r), r.PathValue("id"), in)
	respond(w, http.StatusCreated, e, err)
}

func (a boardAPI) addDependency(w http.ResponseWriter, r *http.Request) {
	var in struct {
		On string `json:"on"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	s, err := a.b.AddDependency(r.PathValue("ws"), actor(r), r.PathValue("id"), in.On)
	respond(w, http.StatusOK, s, err)
}

func (a boardAPI) removeDependency(w http.ResponseWriter, r *http.Request) {
	err := a.b.RemoveDependency(r.PathValue("ws"), actor(r), r.PathValue("id"), r.PathValue("on"))
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func actor(r *http.Request) string {
	a, _ := auth.ActorFrom(r.Context())
	return a
}

func phaseNumber(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a phase is referred to by its number"})
		return 0, false
	}
	return n, true
}

// decodeBody reads a JSON object, refusing unknown fields.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}

func respond(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, status, v)
}

// fail reports a board refusal with its reason. Anything else is logged and
// reported without detail.
func fail(w http.ResponseWriter, err error) {
	var be *board.Error
	if !errors.As(err, &be) {
		log.Printf("board: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	status := map[board.Kind]int{
		board.Invalid:  http.StatusBadRequest,
		board.NotFound: http.StatusNotFound,
		board.Conflict: http.StatusConflict,
	}[be.Kind]
	writeJSON(w, status, map[string]string{"error": be.Msg})
}
