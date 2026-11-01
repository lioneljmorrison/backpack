package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/search"
)

type tools struct {
	b *board.Board
	x *search.Index
}

// in is what every tool needs: the workspace it acts on.
type in struct {
	Workspace string `json:"workspace"`
}

// storyIn names a story.
type storyIn struct {
	in
	ID string `json:"id"`
}

// position places a story directly before or after another; with neither,
// at the bottom of the backlog.
type releaseIn struct {
	in
	ID string `json:"id"`
}

type position struct {
	Before string `json:"before,omitempty" jsonschema:"before this story"`
	After  string `json:"after,omitempty" jsonschema:"after this story"`
}

type addStoryIn struct {
	in
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Goal        string `json:"goal,omitempty"`
	Plan        string `json:"plan,omitempty"`
	Kind        string `json:"kind,omitempty" jsonschema:"bug"`
	FoundIn     string `json:"found_in,omitempty" jsonschema:"story it was found in"`
	Phase       int    `json:"phase,omitempty" jsonschema:"fixed once set"`
	Reasoning   string `json:"reasoning" jsonschema:"why this spot"`
	position
}

type updateStoryIn struct {
	storyIn
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Goal        *string `json:"goal,omitempty"`
	Plan        *string `json:"plan,omitempty"`
	Activity    *string `json:"activity,omitempty" jsonschema:"planning, building, testing or waiting"`
	WaitingOn   *string `json:"waiting_on,omitempty" jsonschema:"what it waits on"`
	Branch      *string `json:"branch,omitempty"`
	PRURL       *string `json:"pr_url,omitempty"`
	Kind        *string `json:"kind,omitempty" jsonschema:"bug; empty clears"`
}

type markReviewedIn struct {
	storyIn
	Reason string `json:"reason" jsonschema:"what was checked or changed"`
}

type moveStoryIn struct {
	storyIn
	position
}

type mergeBranchIn struct {
	in
	Branch string `json:"branch" jsonschema:"the branch that was merged, such as dev"`
	Commit string `json:"commit" jsonschema:"the merge commit"`
}

type cutReleaseIn struct {
	in
	Version string `json:"version" jsonschema:"version, such as v1.0.0"`
	Tag     string `json:"tag" jsonschema:"git tag"`
	Commit  string `json:"commit,omitempty" jsonschema:"commit the tag points at"`
	Notes   string `json:"notes,omitempty" jsonschema:"release notes"`
}

type addBranchIn struct {
	in
	Name  string `json:"name" jsonschema:"no whitespace, max 100 characters"`
	Into  string `json:"into" jsonschema:"existing branch it merges into"`
	Alias string `json:"alias,omitempty" jsonschema:"web column heading, max 100 characters"`
}

type removeBranchIn struct {
	in
	Branch string `json:"branch" jsonschema:"branch name"`
}

type updateBranchIn struct {
	in
	Branch string  `json:"branch" jsonschema:"current branch name"`
	Name   *string `json:"name,omitempty" jsonschema:"new name: no whitespace, max 100 characters"`
	Alias  *string `json:"alias,omitempty" jsonschema:"column heading on the web board, max 100 characters; empty clears"`
	Into   *string `json:"into,omitempty" jsonschema:"existing branch it merges into from now on"`
}

type setStatusIn struct {
	storyIn
	To         string `json:"to" jsonschema:"proposed, backlog, in_progress, in_review, merged, on_hold or closed"`
	Branch     string `json:"branch,omitempty" jsonschema:"task branch, when starting"`
	PRURL      string `json:"pr_url,omitempty" jsonschema:"PR link, when opening review"`
	Commit     string `json:"commit,omitempty" jsonschema:"merge commit, when merging"`
	Into       string `json:"into,omitempty" jsonschema:"branch merged into, when merging; default: the first (such as dev)"`
	Reason     string `json:"reason,omitempty" jsonschema:"required for on_hold and closed"`
	Workaround string `json:"workaround,omitempty" jsonschema:"how a blocked story goes ahead"`
}

type logIn struct {
	storyIn
	Kind     string `json:"kind" jsonschema:"note (text), decision (text, reason), commit (commit), pr (pr_url) or correction (text, corrects)"`
	Text     string `json:"text,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Commit   string `json:"commit,omitempty"`
	PRURL    string `json:"pr_url,omitempty"`
	Corrects string `json:"corrects,omitempty" jsonschema:"entry id corrected, such as L-3"`
}

type dependencyIn struct {
	storyIn
	On string `json:"on" jsonschema:"story it depends on"`
}

type addPhaseIn struct {
	in
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type addGoalIn struct {
	in
	Phase int    `json:"phase"`
	Text  string `json:"text"`
}

type markGoalMetIn struct {
	in
	ID  string `json:"id"`
	Met bool   `json:"met" jsonschema:"true: met; false: not met"`
}

type updatePhaseIn struct {
	in
	Number      int     `json:"number"`
	Title       *string `json:"title,omitempty" jsonschema:"new title; not empty"`
	Description *string `json:"description,omitempty" jsonschema:"new description; empty clears"`
}

type updateGoalIn struct {
	in
	ID   string `json:"id"`
	Text string `json:"text" jsonschema:"new text; not empty"`
}

// readLogIn names what to read the log of: a story or a goal by id, or a
// phase by number.
type readLogIn struct {
	in
	ID    string `json:"id,omitempty" jsonschema:"story id, or goal id such as G-2"`
	Phase int    `json:"phase,omitempty" jsonschema:"phase number; not with id"`
}

// ideaIn names an idea.
type ideaIn struct {
	in
	ID string `json:"id"`
}

// ideaPosition places an idea directly before or after another; with
// neither, at the bottom of the ideas list.
type ideaPosition struct {
	Before string `json:"before,omitempty" jsonschema:"before this idea"`
	After  string `json:"after,omitempty" jsonschema:"after this idea"`
}

type addIdeaIn struct {
	in
	Title   string `json:"title"`
	Context string `json:"context,omitempty" jsonschema:"notes, links, background"`
	ideaPosition
}

type updateIdeaIn struct {
	ideaIn
	Title   *string `json:"title,omitempty"`
	Context *string `json:"context,omitempty"`
}

type moveIdeaIn struct {
	ideaIn
	ideaPosition
}

type searchIn struct {
	in
	Query string `json:"query" jsonschema:"keywords or plain language"`
}

type refineIdeaIn struct {
	ideaIn
	Stories []string `json:"stories,omitempty" jsonschema:"story ids it became"`
	Phase   int      `json:"phase,omitempty" jsonschema:"phase number it became"`
}

type dropIdeaIn struct {
	ideaIn
	Reason string `json:"reason" jsonschema:"why it is dropped"`
}

func addTools(s *mcp.Server, t tools) {
	r := &registry{s: s, steps: map[string]stepFunc{}}
	tool(s, "list_workspaces", "Every workspace: slug, name, repository.", t.listWorkspaces)
	tool(s, "list_branches", "The branches in merge order, each with the branch it merges into.", t.listBranches)
	tool(s, "whats_next", "The highest-ranked backlog story whose dependencies are complete, in full. Null when none is ready.", t.whatsNext)
	tool(s, "where_are_we", "Where the project stands in brief: phase, in progress, in review, on hold, awaiting release, next up, proposals.", t.whereAreWe)
	tool(s, "get_story", "One story: status, what blocks it and what it blocks.", t.getStory)
	write(r, "add_story", "Propose a story, ranked before or after another (default: bottom). It stays proposed until a person approves it. Answers with id and status.", t.addStory)
	write(r, "update_story", "Change a story's fields; others stay. A new goal or plan marks dependents upstream changed. Answers with id, fields changed and stories marked.", t.updateStory)
	later(r, "mark_reviewed", "Clear a story's upstream-changed mark. Needs a reason. Answers with id.", t.markReviewed)
	later(r, "move_story", "Move a story before or after another. Answers with id.", t.moveStory)
	write(r, "set_status", "Move a story one column forward, or back before merged; or on_hold, or closed (final). Approve a proposal with backlog. Released comes only from cut_release. Answers with id and status.", t.setStatus)
	later(r, "merge_branch", "Record a branch merged at a commit; its stories move on. Answers with the story ids moved.", t.mergeBranch)
	later(r, "cut_release", "Record a release: merged stories become Released. Answers with id, version and story count.", t.cutRelease)
	tool(s, "list_releases", "Every release, newest first: id, version, tag, story count.", t.listReleases)
	tool(s, "get_release", "One release by id (R-n), with its stories in delivery order.", t.getRelease)
	later(r, "add_branch", "Add a branch merging into an existing one. Answers with name and target.", t.addBranch)
	later(r, "update_branch", "Change a branch's name, alias or merge target. Answers with name and fields changed.", t.updateBranch)
	write(r, "log", "Add an entry to a story's log. Entries are never edited; fix with a correction. Answers with entry id.", t.log)
	tool(s, "read_log", "The log of a story or goal (id) or a phase (phase), oldest first.", t.readLog)
	write(r, "add_dependency", "Record that a story can't be done until another is complete. Answers with id and whether it is blocked.", t.addDependency)
	later(r, "remove_branch", "Remove a branch nothing refers to. The refusal names what still does.", t.removeBranch)
	later(r, "remove_dependency", "Remove a story's dependency on another.", t.removeDependency)
	tool(s, "list_phases", "Every phase in order: title, description, completion and its goals.", t.listPhases)
	later(r, "add_phase", "Add a phase after the last. Answers with its number.", t.addPhase)
	later(r, "add_goal", "Add a goal to a phase. Answers with id and whether met.", t.addGoal)
	later(r, "update_phase", "Change a phase's title or description. Answers with its number.", t.updatePhase)
	later(r, "update_goal", "Change a goal's text. Answers with id.", t.updateGoal)
	later(r, "mark_goal_met", "Mark a goal met, or not met. Answers with id and whether met.", t.markGoalMet)
	write(r, "add_idea", "Add an idea: a rough thought, not yet work, outside the backlog. Give it context. Answers with id and state.", t.addIdea)
	tool(s, "list_ideas", "The open ideas, top first: id, title, state.", t.listIdeas)
	tool(s, "get_idea", "One idea, with its context and what it became or why it was dropped.", t.getIdea)
	later(r, "update_idea", "Change an open idea's title or context. Answers with id and state.", t.updateIdea)
	later(r, "move_idea", "Move an open idea before or after another. Answers with id and state.", t.moveIdea)
	later(r, "refine_idea", "Close an idea, linking what it became. Answers with id and state.", t.refineIdea)
	later(r, "drop_idea", "Close an idea that became nothing, with a reason. Answers with id and state.", t.dropIdea)
	tool(s, "search", "Check whether a story, idea or phase already covers something, by keyword and meaning, closed included. Up to ten, best first.", t.search)
	tool(s, "actions", "The rarer actions (branches, merging, releases, phases, goals, ideas, moving stories, dependencies), which run through batch. No name: one line each. With a name: its arguments. Never runs one.", actions(r))
	tool(s, "batch", "Many board changes in one call: each action names a changing tool and its arguments. It also runs the rarer changes actions lists (branches, releases, phases, goals, ideas). In order; a failure doesn't stop the rest. Up to 100; no reads.", batch(r.steps))
}

func (t tools) search(ctx context.Context, a searchIn) (any, error) {
	rs, err := t.x.Search(ctx, a.Workspace, a.Query)
	return map[string]any{"results": rs}, err
}

func actor(ctx context.Context) string {
	a, _ := auth.ActorFrom(ctx)
	return a
}

// listWorkspaces is the one tool that takes no workspace.
func (t tools) listWorkspaces(_ context.Context, _ struct{}) (any, error) {
	ws, err := t.b.Workspaces()
	return map[string]any{"workspaces": workspaceSummaries(ws)}, err
}

func (t tools) listBranches(_ context.Context, a in) (any, error) {
	bs, err := t.b.Branches(a.Workspace)
	return map[string]any{"branches": branchSummaries(bs)}, err
}

func (t tools) whatsNext(_ context.Context, a in) (any, error) {
	s, err := t.b.Next(a.Workspace)
	if err != nil || s == nil {
		return map[string]any{"story": nil}, err
	}
	return map[string]any{"story": lean(*s)}, nil
}

func (t tools) whereAreWe(_ context.Context, a in) (any, error) {
	w, err := t.b.Where(a.Workspace)
	if err != nil {
		return nil, err
	}
	return plain(w.Text()), nil
}

func (t tools) getStory(_ context.Context, a storyIn) (any, error) {
	return leanResult(t.b.Story(a.Workspace, a.ID))
}

func (t tools) addStory(ctx context.Context, a addStoryIn) (any, error) {
	s, err := t.b.AddStory(a.Workspace, actor(ctx), board.NewStory{Title: a.Title, Description: a.Description,
		Goal: a.Goal, Plan: a.Plan, Kind: a.Kind, FoundIn: a.FoundIn, Phase: a.Phase, Proposed: true, Reasoning: a.Reasoning,
		Position: board.Position(a.position)})
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": s.ID, "status": s.Status}, nil
}

func (t tools) updateStory(ctx context.Context, a updateStoryIn) (any, error) {
	u, err := t.b.UpdateStoryDetail(a.Workspace, actor(ctx), a.ID, board.StoryPatch{Title: a.Title,
		Description: a.Description, Goal: a.Goal, Plan: a.Plan, Activity: a.Activity, WaitingOn: a.WaitingOn,
		Branch: a.Branch, PRURL: a.PRURL, Kind: a.Kind})
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": u.Story.ID, "changed": u.Changed}
	if len(u.Marked) > 0 {
		out["marked"] = u.Marked
	}
	return out, nil
}

func (t tools) markReviewed(ctx context.Context, a markReviewedIn) (any, error) {
	s, err := t.b.MarkReviewed(a.Workspace, actor(ctx), a.ID, a.Reason)
	return map[string]any{"id": s.ID}, err
}

func (t tools) moveStory(ctx context.Context, a moveStoryIn) (any, error) {
	s, err := t.b.MoveStory(a.Workspace, actor(ctx), a.ID, board.Position(a.position))
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": s.ID}, nil
}

func (t tools) setStatus(ctx context.Context, a setStatusIn) (any, error) {
	s, err := t.b.SetStatus(a.Workspace, actor(ctx), a.ID, board.StatusChange{To: a.To, Branch: a.Branch,
		PRURL: a.PRURL, Commit: a.Commit, Into: a.Into, Reason: a.Reason, Workaround: a.Workaround})
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": s.ID, "status": s.Status}
	if a.To == board.Merged {
		out["into"] = s.Into
	}
	return out, nil
}

func (t tools) mergeBranch(ctx context.Context, a mergeBranchIn) (any, error) {
	moved, err := t.b.MergeBranch(a.Workspace, actor(ctx), a.Branch, a.Commit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(moved))
	for i, s := range moved {
		ids[i] = s.ID
	}
	return map[string]any{"stories": ids}, nil
}

func (t tools) cutRelease(ctx context.Context, a cutReleaseIn) (any, error) {
	r, err := t.b.CutRelease(a.Workspace, actor(ctx), board.NewRelease{Version: a.Version, Tag: a.Tag, Commit: a.Commit, Notes: a.Notes})
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": r.ID, "version": r.Version, "stories": len(r.Stories)}, nil
}

func (t tools) listReleases(_ context.Context, a in) (any, error) {
	rs, err := t.b.ListReleases(a.Workspace)
	return map[string]any{"releases": releaseSummaries(rs)}, err
}

func (t tools) getRelease(_ context.Context, a releaseIn) (any, error) {
	d, err := t.b.GetRelease(a.Workspace, a.ID)
	if err != nil {
		return nil, err
	}
	return leanReleaseOf(d), nil
}

func (t tools) addBranch(ctx context.Context, a addBranchIn) (any, error) {
	br, err := t.b.AddBranch(a.Workspace, actor(ctx), board.Branch{Name: a.Name, Into: a.Into, Alias: a.Alias})
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": br.Name, "into": br.Into}, nil
}

func (t tools) removeBranch(ctx context.Context, a removeBranchIn) (any, error) {
	err := t.b.RemoveBranch(a.Workspace, actor(ctx), a.Branch)
	return map[string]any{"removed": true}, err
}

func (t tools) updateBranch(ctx context.Context, a updateBranchIn) (any, error) {
	br, changed, err := t.b.UpdateBranchDetail(a.Workspace, actor(ctx), a.Branch, board.BranchPatch{Name: a.Name, Alias: a.Alias, Into: a.Into})
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": br.Name, "changed": changed}, nil
}

func (t tools) log(ctx context.Context, a logIn) (any, error) {
	e, err := t.b.AddLogEntry(a.Workspace, actor(ctx), a.ID, board.NewLogEntry{Kind: a.Kind, Text: a.Text,
		Reason: a.Reason, Commit: a.Commit, PRURL: a.PRURL, Corrects: a.Corrects})
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": e.ID}, nil
}

func (t tools) readLog(_ context.Context, a readLogIn) (any, error) {
	var l []board.LogEntry
	var err error
	switch {
	case a.ID != "" && a.Phase != 0:
		return nil, &board.Error{Kind: board.Invalid, Msg: "give id or phase, not both"}
	case a.Phase != 0:
		l, err = t.b.PhaseLog(a.Workspace, a.Phase)
	case strings.HasPrefix(a.ID, "G-"):
		l, err = t.b.GoalLog(a.Workspace, a.ID)
	case a.ID != "":
		l, err = t.b.Log(a.Workspace, a.ID)
	default:
		return nil, &board.Error{Kind: board.Invalid, Msg: "give the id of a story or goal, or the number of a phase"}
	}
	return map[string]any{"entries": l}, err
}

func (t tools) addDependency(ctx context.Context, a dependencyIn) (any, error) {
	s, err := t.b.AddDependency(a.Workspace, actor(ctx), a.ID, a.On)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": s.ID, "blocked": s.Blocked}, nil
}

func (t tools) removeDependency(ctx context.Context, a dependencyIn) (any, error) {
	err := t.b.RemoveDependency(a.Workspace, actor(ctx), a.ID, a.On)
	return map[string]any{"removed": true}, err
}

func (t tools) listPhases(_ context.Context, a in) (any, error) {
	ps, err := t.b.Phases(a.Workspace)
	return map[string]any{"phases": phaseSummaries(ps)}, err
}

func (t tools) addPhase(ctx context.Context, a addPhaseIn) (any, error) {
	p, err := t.b.AddPhase(a.Workspace, actor(ctx), a.Title, a.Description)
	if err != nil {
		return nil, err
	}
	return map[string]any{"number": p.Number}, nil
}

func (t tools) addGoal(ctx context.Context, a addGoalIn) (any, error) {
	g, err := t.b.AddGoal(a.Workspace, actor(ctx), a.Phase, a.Text)
	if err != nil {
		return nil, err
	}
	return goalAnswer(g), nil
}

func (t tools) updatePhase(ctx context.Context, a updatePhaseIn) (any, error) {
	p, err := t.b.UpdatePhase(a.Workspace, actor(ctx), a.Number, board.PhasePatch{Title: a.Title, Description: a.Description})
	return map[string]any{"number": p.Number}, err
}

func (t tools) updateGoal(ctx context.Context, a updateGoalIn) (any, error) {
	g, err := t.b.UpdateGoal(a.Workspace, actor(ctx), a.ID, a.Text)
	return map[string]any{"id": g.ID}, err
}

func (t tools) markGoalMet(ctx context.Context, a markGoalMetIn) (any, error) {
	g, err := t.b.SetGoalMet(a.Workspace, actor(ctx), a.ID, a.Met)
	if err != nil {
		return nil, err
	}
	return goalAnswer(g), nil
}

// goalAnswer is the short answer to a goal change: its id and whether it is met.
func goalAnswer(g board.Goal) map[string]any {
	return map[string]any{"id": g.ID, "met": g.Met}
}

// ideaAnswer is the short answer to an idea change: its id and state.
func ideaAnswer(i board.Idea, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": i.ID, "state": i.State}, nil
}

func (t tools) addIdea(ctx context.Context, a addIdeaIn) (any, error) {
	return ideaAnswer(t.b.AddIdea(a.Workspace, actor(ctx), board.NewIdea{Title: a.Title, Context: a.Context, Position: board.Position(a.ideaPosition)}))
}

func (t tools) listIdeas(_ context.Context, a in) (any, error) {
	is, err := t.b.Ideas(a.Workspace)
	return map[string]any{"ideas": ideaSummaries(is)}, err
}

func (t tools) getIdea(_ context.Context, a ideaIn) (any, error) {
	return t.b.Idea(a.Workspace, a.ID)
}

func (t tools) updateIdea(ctx context.Context, a updateIdeaIn) (any, error) {
	return ideaAnswer(t.b.UpdateIdea(a.Workspace, actor(ctx), a.ID, board.IdeaPatch{Title: a.Title, Context: a.Context}))
}

func (t tools) moveIdea(ctx context.Context, a moveIdeaIn) (any, error) {
	return ideaAnswer(t.b.MoveIdea(a.Workspace, actor(ctx), a.ID, board.Position(a.ideaPosition)))
}

func (t tools) refineIdea(ctx context.Context, a refineIdeaIn) (any, error) {
	return ideaAnswer(t.b.RefineIdea(a.Workspace, actor(ctx), a.ID, board.Refinement{Stories: a.Stories, Phase: a.Phase}))
}

func (t tools) dropIdea(ctx context.Context, a dropIdeaIn) (any, error) {
	return ideaAnswer(t.b.DropIdea(a.Workspace, actor(ctx), a.ID, a.Reason))
}
