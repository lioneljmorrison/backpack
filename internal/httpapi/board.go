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
	"github.com/lioneljmorrison/backpack/internal/search"
)

const maxBody = 1 << 20

func routeBoard(mux *http.ServeMux, b *board.Board) {
	a := boardAPI{b}
	mux.HandleFunc("GET /v1/workspaces", a.listWorkspaces)
	mux.HandleFunc("POST /v1/workspaces", a.createWorkspace)
	mux.HandleFunc("GET /v1/workspaces/{ws}/branches", a.listBranches)
	mux.HandleFunc("POST /v1/workspaces/{ws}/branches", a.addBranch)
	mux.HandleFunc("PATCH /v1/workspaces/{ws}/branches/{name}", a.updateBranch)
	mux.HandleFunc("DELETE /v1/workspaces/{ws}/branches/{name}", a.removeBranch)
	mux.HandleFunc("POST /v1/workspaces/{ws}/branches/{name}/merged", a.mergeBranch)
	mux.HandleFunc("GET /v1/workspaces/{ws}/releases", a.listReleases)
	mux.HandleFunc("GET /v1/workspaces/{ws}/releases/{id}", a.getRelease)
	mux.HandleFunc("POST /v1/workspaces/{ws}/releases", a.cutRelease)
	mux.HandleFunc("GET /v1/workspaces/{ws}/next", a.next)
	mux.HandleFunc("GET /v1/workspaces/{ws}/status", a.where)
	mux.HandleFunc("GET /v1/workspaces/{ws}/board", a.board)
	mux.HandleFunc("GET /v1/workspaces/{ws}/phases", a.listPhases)
	mux.HandleFunc("POST /v1/workspaces/{ws}/phases", a.addPhase)
	mux.HandleFunc("PATCH /v1/workspaces/{ws}/phases/{n}", a.updatePhase)
	mux.HandleFunc("GET /v1/workspaces/{ws}/phases/{n}/log", a.readPhaseLog)
	mux.HandleFunc("POST /v1/workspaces/{ws}/phases/{n}/goals", a.addGoal)
	mux.HandleFunc("PATCH /v1/workspaces/{ws}/goals/{id}", a.updateGoal)
	mux.HandleFunc("GET /v1/workspaces/{ws}/goals/{id}/log", a.readGoalLog)
	mux.HandleFunc("GET /v1/workspaces/{ws}/stories", a.listStories)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories", a.addStory)
	mux.HandleFunc("GET /v1/workspaces/{ws}/stories/{id}", a.getStory)
	mux.HandleFunc("GET /v1/workspaces/{ws}/stories/{id}/context", a.getContext)
	mux.HandleFunc("PATCH /v1/workspaces/{ws}/stories/{id}", a.updateStory)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/move", a.moveStory)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/status", a.setStatus)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/reviewed", a.markReviewed)
	mux.HandleFunc("GET /v1/workspaces/{ws}/stories/{id}/log", a.readLog)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/log", a.addLogEntry)
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/dependencies", a.addDependency)
	mux.HandleFunc("DELETE /v1/workspaces/{ws}/stories/{id}/dependencies/{on}", a.removeDependency)
	mux.HandleFunc("GET /v1/workspaces/{ws}/ideas", a.listIdeas)
	mux.HandleFunc("POST /v1/workspaces/{ws}/ideas", a.addIdea)
	mux.HandleFunc("GET /v1/workspaces/{ws}/ideas/{id}", a.getIdea)
	mux.HandleFunc("PATCH /v1/workspaces/{ws}/ideas/{id}", a.updateIdea)
	mux.HandleFunc("POST /v1/workspaces/{ws}/ideas/{id}/move", a.moveIdea)
	mux.HandleFunc("POST /v1/workspaces/{ws}/ideas/{id}/refine", a.refineIdea)
	mux.HandleFunc("POST /v1/workspaces/{ws}/ideas/{id}/drop", a.dropIdea)
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

func (a boardAPI) listBranches(w http.ResponseWriter, r *http.Request) {
	bs, err := a.b.Branches(r.PathValue("ws"))
	respond(w, http.StatusOK, map[string]any{"branches": bs}, err)
}

func (a boardAPI) addBranch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Into  string `json:"into"`
		Alias string `json:"alias"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	br, err := a.b.AddBranch(r.PathValue("ws"), actor(r), board.Branch{Name: in.Name, Into: in.Into, Alias: in.Alias})
	respond(w, http.StatusCreated, br, err)
}

func (a boardAPI) updateBranch(w http.ResponseWriter, r *http.Request) {
	var patch board.BranchPatch
	if !decodeBody(w, r, &patch) {
		return
	}
	br, err := a.b.UpdateBranch(r.PathValue("ws"), actor(r), r.PathValue("name"), patch)
	respond(w, http.StatusOK, br, err)
}

func (a boardAPI) removeBranch(w http.ResponseWriter, r *http.Request) {
	if err := a.b.RemoveBranch(r.PathValue("ws"), actor(r), r.PathValue("name")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a boardAPI) mergeBranch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Commit string `json:"commit"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	moved, err := a.b.MergeBranch(r.PathValue("ws"), actor(r), r.PathValue("name"), in.Commit)
	respond(w, http.StatusOK, map[string]any{"stories": moved}, err)
}

func (a boardAPI) cutRelease(w http.ResponseWriter, r *http.Request) {
	var in board.NewRelease
	if !decodeBody(w, r, &in) {
		return
	}
	rel, err := a.b.CutRelease(r.PathValue("ws"), actor(r), in)
	respond(w, http.StatusCreated, rel, err)
}

func (a boardAPI) listReleases(w http.ResponseWriter, r *http.Request) {
	rs, err := a.b.ListReleases(r.PathValue("ws"))
	respond(w, http.StatusOK, map[string]any{"releases": rs}, err)
}

func (a boardAPI) getRelease(w http.ResponseWriter, r *http.Request) {
	rel, err := a.b.GetRelease(r.PathValue("ws"), r.PathValue("id"))
	respond(w, http.StatusOK, rel, err)
}

func (a boardAPI) next(w http.ResponseWriter, r *http.Request) {
	s, err := a.b.Next(r.PathValue("ws"))
	respond(w, http.StatusOK, map[string]any{"story": s}, err)
}

// where answers as JSON, or as plain text when the caller asks for text and
// not JSON, as the session hook does.
func (a boardAPI) where(w http.ResponseWriter, r *http.Request) {
	where, err := a.b.Where(r.PathValue("ws"))
	accept := r.Header.Get("Accept")
	if err != nil || !strings.Contains(accept, "text/plain") || strings.Contains(accept, "application/json") {
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

// updateGoal marks a goal met or unmarks it (met), or changes its text.
// One request does one of them.
func (a boardAPI) updateGoal(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Met  *bool   `json:"met"`
		Text *string `json:"text"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	if (in.Met == nil) == (in.Text == nil) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "give met or text, one of them"})
		return
	}
	var g board.Goal
	var err error
	if in.Met != nil {
		g, err = a.b.SetGoalMet(r.PathValue("ws"), actor(r), r.PathValue("id"), *in.Met)
	} else {
		g, err = a.b.UpdateGoal(r.PathValue("ws"), actor(r), r.PathValue("id"), *in.Text)
	}
	respond(w, http.StatusOK, g, err)
}

func (a boardAPI) readPhaseLog(w http.ResponseWriter, r *http.Request) {
	n, ok := phaseNumber(w, r)
	if !ok {
		return
	}
	l, err := a.b.PhaseLog(r.PathValue("ws"), n)
	respond(w, http.StatusOK, map[string]any{"entries": l}, err)
}

func (a boardAPI) readGoalLog(w http.ResponseWriter, r *http.Request) {
	l, err := a.b.GoalLog(r.PathValue("ws"), r.PathValue("id"))
	respond(w, http.StatusOK, map[string]any{"entries": l}, err)
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

func (a boardAPI) getContext(w http.ResponseWriter, r *http.Request) {
	c, err := a.b.Context(r.PathValue("ws"), r.PathValue("id"))
	respond(w, http.StatusOK, c, err)
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

func (a boardAPI) markReviewed(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	s, err := a.b.MarkReviewed(r.PathValue("ws"), actor(r), r.PathValue("id"), in.Reason)
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

func (a boardAPI) listIdeas(w http.ResponseWriter, r *http.Request) {
	is, err := a.b.Ideas(r.PathValue("ws"))
	respond(w, http.StatusOK, map[string]any{"ideas": is}, err)
}

// addIdea adds an idea, open from the start: only stories wait for approval.
func (a boardAPI) addIdea(w http.ResponseWriter, r *http.Request) {
	var in board.NewIdea
	if !decodeBody(w, r, &in) {
		return
	}
	i, err := a.b.AddIdea(r.PathValue("ws"), actor(r), in)
	respond(w, http.StatusCreated, i, err)
}

func (a boardAPI) getIdea(w http.ResponseWriter, r *http.Request) {
	i, err := a.b.Idea(r.PathValue("ws"), r.PathValue("id"))
	respond(w, http.StatusOK, i, err)
}

func (a boardAPI) updateIdea(w http.ResponseWriter, r *http.Request) {
	var in board.IdeaPatch
	if !decodeBody(w, r, &in) {
		return
	}
	i, err := a.b.UpdateIdea(r.PathValue("ws"), actor(r), r.PathValue("id"), in)
	respond(w, http.StatusOK, i, err)
}

func (a boardAPI) moveIdea(w http.ResponseWriter, r *http.Request) {
	var in board.Position
	if !decodeBody(w, r, &in) {
		return
	}
	i, err := a.b.MoveIdea(r.PathValue("ws"), actor(r), r.PathValue("id"), in)
	respond(w, http.StatusOK, i, err)
}

func (a boardAPI) refineIdea(w http.ResponseWriter, r *http.Request) {
	var in board.Refinement
	if !decodeBody(w, r, &in) {
		return
	}
	i, err := a.b.RefineIdea(r.PathValue("ws"), actor(r), r.PathValue("id"), in)
	respond(w, http.StatusOK, i, err)
}

func (a boardAPI) dropIdea(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	i, err := a.b.DropIdea(r.PathValue("ws"), actor(r), r.PathValue("id"), in.Reason)
	respond(w, http.StatusOK, i, err)
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

// fail reports a board refusal with its reason, and search being down as
// such. Anything else is logged and reported without detail.
func fail(w http.ResponseWriter, err error) {
	var be *board.Error
	if errors.Is(err, search.ErrUnavailable) {
		log.Printf("search: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "search unavailable"})
		return
	}
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
