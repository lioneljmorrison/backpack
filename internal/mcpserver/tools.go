package mcpserver

import (
	"context"

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
	Workspace string `json:"workspace" jsonschema:"the workspace's slug, from the project's .backpack.json"`
}

// storyIn names a story.
type storyIn struct {
	in
	ID string `json:"id" jsonschema:"the story's id, such as S-4"`
}

// position places a story directly before or after another; with neither,
// at the bottom of the backlog.
type position struct {
	Before string `json:"before,omitempty" jsonschema:"place directly before this story"`
	After  string `json:"after,omitempty" jsonschema:"place directly after this story"`
}

type addStoryIn struct {
	in
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Goal        string `json:"goal,omitempty"`
	Plan        string `json:"plan,omitempty"`
	Phase       int    `json:"phase,omitempty" jsonschema:"the number of the phase the story belongs to; it can't be changed later"`
	Reasoning   string `json:"reasoning" jsonschema:"why the story belongs at this spot, for the person who approves it"`
	position
}

type updateStoryIn struct {
	storyIn
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Goal        *string `json:"goal,omitempty"`
	Plan        *string `json:"plan,omitempty"`
	Activity    *string `json:"activity,omitempty" jsonschema:"only while in progress: planning, building, testing or waiting"`
	WaitingOn   *string `json:"waiting_on,omitempty" jsonschema:"only while waiting: what the story is waiting on"`
	Branch      *string `json:"branch,omitempty"`
	PRURL       *string `json:"pr_url,omitempty"`
}

type moveStoryIn struct {
	storyIn
	position
}

type setStatusIn struct {
	storyIn
	To         string `json:"to" jsonschema:"backlog, in_progress, in_review, merged, released, deployed, on_hold or closed"`
	Branch     string `json:"branch,omitempty" jsonschema:"only when starting: the task branch"`
	PRURL      string `json:"pr_url,omitempty" jsonschema:"only when opening review: the pull request's link"`
	Commit     string `json:"commit,omitempty" jsonschema:"only when merging: the merge commit"`
	Reason     string `json:"reason,omitempty" jsonschema:"required when putting on hold or closing, and only then"`
	Workaround string `json:"workaround,omitempty" jsonschema:"only when starting a blocked story: how it goes ahead anyway"`
}

type logIn struct {
	storyIn
	Kind     string `json:"kind" jsonschema:"note (text), decision (text and reason), commit (commit), pr (pr_url) or correction (text and corrects)"`
	Text     string `json:"text,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Commit   string `json:"commit,omitempty"`
	PRURL    string `json:"pr_url,omitempty"`
	Corrects string `json:"corrects,omitempty" jsonschema:"the id of the entry a correction corrects, such as L-3"`
}

type dependencyIn struct {
	storyIn
	On string `json:"on" jsonschema:"the story this one can't be done without"`
}

type addPhaseIn struct {
	in
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type addGoalIn struct {
	in
	Phase int    `json:"phase" jsonschema:"the phase's number"`
	Text  string `json:"text"`
}

type markGoalMetIn struct {
	in
	ID  string `json:"id" jsonschema:"the goal's id, such as G-2"`
	Met bool   `json:"met" jsonschema:"true to mark the goal met, false to unmark it"`
}

// ideaIn names an idea.
type ideaIn struct {
	in
	ID string `json:"id" jsonschema:"the idea's id, such as I-3"`
}

// ideaPosition places an idea directly before or after another; with
// neither, at the bottom of the ideas list.
type ideaPosition struct {
	Before string `json:"before,omitempty" jsonschema:"place directly before this idea"`
	After  string `json:"after,omitempty" jsonschema:"place directly after this idea"`
}

type addIdeaIn struct {
	in
	Title     string `json:"title"`
	Notes     string `json:"notes,omitempty"`
	Reasoning string `json:"reasoning" jsonschema:"why the idea belongs at this spot, for the person who approves it"`
	ideaPosition
}

type updateIdeaIn struct {
	ideaIn
	Title *string `json:"title,omitempty"`
	Notes *string `json:"notes,omitempty"`
}

type moveIdeaIn struct {
	ideaIn
	ideaPosition
}

type searchIn struct {
	in
	Query string `json:"query" jsonschema:"words, or what the work is about in plain language"`
}

type refineIdeaIn struct {
	ideaIn
	Stories []string `json:"stories,omitempty" jsonschema:"the stories the idea became, already added"`
	Phase   int      `json:"phase,omitempty" jsonschema:"the number of the phase the idea became, already added"`
}

type dropIdeaIn struct {
	ideaIn
	Reason string `json:"reason" jsonschema:"why the idea is dropped"`
}

func addTools(s *mcp.Server, t tools) {
	tool(s, "whats_next", "The next story to deliver: the highest-ranked backlog story whose dependencies are all complete, with its description, goal and plan. The story is null when nothing is ready.", t.whatsNext)
	tool(s, "where_are_we", "The current phase and how far along it is, what is in progress and what each story is doing, what is in review, what is on hold and why, and the next three in the backlog.", t.whereAreWe)
	tool(s, "get_story", "One story, with its status, what blocks it and what it blocks.", t.getStory)
	tool(s, "add_story", "Propose a story: it goes in as proposed, directly before or after another story or at the bottom when neither is given, with the reasoning for that spot logged as a decision. It waits until a person approves it by setting its status to backlog. It must rank below everything it will depend on.", t.addStory)
	tool(s, "update_story", "Change a story's fields; fields not given are left as they are. Activity is what an in-progress story is doing.", t.updateStory)
	tool(s, "move_story", "Move a story directly before or after another, or to the bottom of the backlog. Refused if it would rank above something it depends on, or below something that depends on it.", t.moveStory)
	tool(s, "set_status", "Move a story across the board: forward one column, or back one before merged. A proposed story is approved to backlog or declined by closing it. Merged, released and deployed are complete. on_hold resumes to the column it left; closed is final. Every change is logged.", t.setStatus)
	tool(s, "log", "Add an entry to a story's log. Entries are never edited or deleted: fix a mistake with a correction that points to the entry.", t.log)
	tool(s, "read_log", "A story's log, oldest first.", t.readLog)
	tool(s, "add_dependency", "Record that a story can't be done until another is complete. The other must rank above it, be in the same or an earlier phase, and not close a circle.", t.addDependency)
	tool(s, "remove_dependency", "Remove a story's dependency on another.", t.removeDependency)
	tool(s, "list_phases", "Every phase in order, with its goals, how many stories are complete, whether it is complete, and whether its goals were all met while stories are still open.", t.listPhases)
	tool(s, "add_phase", "Add a phase, numbered after the last one.", t.addPhase)
	tool(s, "add_goal", "Add a goal to a phase.", t.addGoal)
	tool(s, "mark_goal_met", "Mark a goal met, or not met.", t.markGoalMet)
	tool(s, "add_idea", "Suggest an idea: a rough thought, not yet work. It goes into the ideas list, apart from the backlog, directly before or after another idea or at the bottom, and waits as proposed, with the reasoning for its spot, until a person approves it.", t.addIdea)
	tool(s, "list_ideas", "The ideas that are not closed, in order, top first.", t.listIdeas)
	tool(s, "get_idea", "One idea, closed or not, with what it became or why it was dropped.", t.getIdea)
	tool(s, "update_idea", "Change an idea's title or notes while it is not closed; fields not given are left as they are.", t.updateIdea)
	tool(s, "move_idea", "Move an idea that is not closed directly before or after another, or to the bottom of the ideas list.", t.moveIdea)
	tool(s, "approve_idea", "Approve a proposed idea at its spot, making it open.", t.approveIdea)
	tool(s, "refine_idea", "Close an open idea, linking it to the stories and phase it became. Add those first, the usual way; none may be closed.", t.refineIdea)
	tool(s, "drop_idea", "Close an idea that became nothing, with the reason.", t.dropIdea)
	tool(s, "search", "Check whether a story, idea or phase already covers something, before adding one. Matches by keyword and by meaning, best first, up to ten; closed stories and dropped ideas are included, with their state.", t.search)
}

func (t tools) search(ctx context.Context, a searchIn) (any, error) {
	rs, err := t.x.Search(ctx, a.Workspace, a.Query)
	return map[string]any{"results": rs}, err
}

func actor(ctx context.Context) string {
	a, _ := auth.ActorFrom(ctx)
	return a
}

func (t tools) whatsNext(_ context.Context, a in) (any, error) {
	s, err := t.b.Next(a.Workspace)
	return map[string]any{"story": s}, err
}

func (t tools) whereAreWe(_ context.Context, a in) (any, error) {
	return t.b.Where(a.Workspace)
}

func (t tools) getStory(_ context.Context, a storyIn) (any, error) {
	return t.b.Story(a.Workspace, a.ID)
}

func (t tools) addStory(ctx context.Context, a addStoryIn) (any, error) {
	return t.b.AddStory(a.Workspace, actor(ctx), board.NewStory{Title: a.Title, Description: a.Description,
		Goal: a.Goal, Plan: a.Plan, Phase: a.Phase, Proposed: true, Reasoning: a.Reasoning,
		Position: board.Position(a.position)})
}

func (t tools) updateStory(ctx context.Context, a updateStoryIn) (any, error) {
	return t.b.UpdateStory(a.Workspace, actor(ctx), a.ID, board.StoryPatch{Title: a.Title,
		Description: a.Description, Goal: a.Goal, Plan: a.Plan, Activity: a.Activity, WaitingOn: a.WaitingOn,
		Branch: a.Branch, PRURL: a.PRURL})
}

func (t tools) moveStory(ctx context.Context, a moveStoryIn) (any, error) {
	return t.b.MoveStory(a.Workspace, actor(ctx), a.ID, board.Position(a.position))
}

func (t tools) setStatus(ctx context.Context, a setStatusIn) (any, error) {
	return t.b.SetStatus(a.Workspace, actor(ctx), a.ID, board.StatusChange{To: a.To, Branch: a.Branch,
		PRURL: a.PRURL, Commit: a.Commit, Reason: a.Reason, Workaround: a.Workaround})
}

func (t tools) log(ctx context.Context, a logIn) (any, error) {
	return t.b.AddLogEntry(a.Workspace, actor(ctx), a.ID, board.NewLogEntry{Kind: a.Kind, Text: a.Text,
		Reason: a.Reason, Commit: a.Commit, PRURL: a.PRURL, Corrects: a.Corrects})
}

func (t tools) readLog(_ context.Context, a storyIn) (any, error) {
	l, err := t.b.Log(a.Workspace, a.ID)
	return map[string]any{"entries": l}, err
}

func (t tools) addDependency(ctx context.Context, a dependencyIn) (any, error) {
	return t.b.AddDependency(a.Workspace, actor(ctx), a.ID, a.On)
}

func (t tools) removeDependency(ctx context.Context, a dependencyIn) (any, error) {
	err := t.b.RemoveDependency(a.Workspace, actor(ctx), a.ID, a.On)
	return map[string]any{"removed": true}, err
}

func (t tools) listPhases(_ context.Context, a in) (any, error) {
	ps, err := t.b.Phases(a.Workspace)
	return map[string]any{"phases": ps}, err
}

func (t tools) addPhase(ctx context.Context, a addPhaseIn) (any, error) {
	return t.b.AddPhase(a.Workspace, actor(ctx), a.Title, a.Description)
}

func (t tools) addGoal(ctx context.Context, a addGoalIn) (any, error) {
	return t.b.AddGoal(a.Workspace, actor(ctx), a.Phase, a.Text)
}

func (t tools) markGoalMet(ctx context.Context, a markGoalMetIn) (any, error) {
	return t.b.SetGoalMet(a.Workspace, actor(ctx), a.ID, a.Met)
}

func (t tools) addIdea(ctx context.Context, a addIdeaIn) (any, error) {
	return t.b.AddIdea(a.Workspace, actor(ctx), board.NewIdea{Title: a.Title, Notes: a.Notes, Proposed: true,
		Reasoning: a.Reasoning, Position: board.Position(a.ideaPosition)})
}

func (t tools) listIdeas(_ context.Context, a in) (any, error) {
	is, err := t.b.Ideas(a.Workspace)
	return map[string]any{"ideas": is}, err
}

func (t tools) getIdea(_ context.Context, a ideaIn) (any, error) {
	return t.b.Idea(a.Workspace, a.ID)
}

func (t tools) updateIdea(ctx context.Context, a updateIdeaIn) (any, error) {
	return t.b.UpdateIdea(a.Workspace, actor(ctx), a.ID, board.IdeaPatch{Title: a.Title, Notes: a.Notes})
}

func (t tools) moveIdea(ctx context.Context, a moveIdeaIn) (any, error) {
	return t.b.MoveIdea(a.Workspace, actor(ctx), a.ID, board.Position(a.ideaPosition))
}

func (t tools) approveIdea(ctx context.Context, a ideaIn) (any, error) {
	return t.b.ApproveIdea(a.Workspace, actor(ctx), a.ID)
}

func (t tools) refineIdea(ctx context.Context, a refineIdeaIn) (any, error) {
	return t.b.RefineIdea(a.Workspace, actor(ctx), a.ID, board.Refinement{Stories: a.Stories, Phase: a.Phase})
}

func (t tools) dropIdea(ctx context.Context, a dropIdeaIn) (any, error) {
	return t.b.DropIdea(a.Workspace, actor(ctx), a.ID, a.Reason)
}
