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
	Workspace string `json:"workspace" jsonschema:"the workspace's slug, from the project's .backpack.json"`
}

// storyIn names a story.
type storyIn struct {
	in
	ID string `json:"id" jsonschema:"the story's id, such as S-4"`
}

// position places a story directly before or after another; with neither,
// at the bottom of the backlog.
type releaseIn struct {
	in
	ID string `json:"id" jsonschema:"the release's id, such as R-2"`
}

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

type markReviewedIn struct {
	storyIn
	Reason string `json:"reason" jsonschema:"required: what was checked or changed, in plain text; it is logged on the story"`
}

type moveStoryIn struct {
	storyIn
	position
}

type mergeBranchIn struct {
	in
	Branch string `json:"branch" jsonschema:"the branch that was merged, such as dev; it must merge into another branch"`
	Commit string `json:"commit" jsonschema:"the merge commit"`
}

type cutReleaseIn struct {
	in
	Version string `json:"version" jsonschema:"the release's version, such as v1.0.0; required, and not one already released"`
	Tag     string `json:"tag" jsonschema:"the git tag that marks the release; required"`
	Commit  string `json:"commit,omitempty" jsonschema:"the commit the tag points at"`
	Notes   string `json:"notes,omitempty" jsonschema:"release notes"`
}

type addBranchIn struct {
	in
	Name  string `json:"name" jsonschema:"the new branch's name: plain text, no whitespace, at most 100 characters, not already a branch's"`
	Into  string `json:"into" jsonschema:"the existing branch it merges into; required, because a workspace has one release branch and it is already there"`
	Alias string `json:"alias,omitempty" jsonschema:"the name the web board's column heading shows instead of the branch name: plain text, at most 100 characters"`
}

type removeBranchIn struct {
	in
	Branch string `json:"branch" jsonschema:"the branch to remove, by name"`
}

type updateBranchIn struct {
	in
	Branch string  `json:"branch" jsonschema:"the branch to change, by its current name"`
	Name   *string `json:"name,omitempty" jsonschema:"the new name: plain text, no whitespace, at most 100 characters, not already a branch's"`
	Alias  *string `json:"alias,omitempty" jsonschema:"the name the web board's column heading shows instead of the branch name (nothing else uses it): plain text, at most 100 characters; an empty string clears it"`
	Into   *string `json:"into,omitempty" jsonschema:"the existing branch it merges into from now on; not itself or one that leads back to it, and not given for the release branch"`
}

type setStatusIn struct {
	storyIn
	To         string `json:"to" jsonschema:"proposed (only to resume a held proposal), backlog, in_progress, in_review, merged, on_hold or closed (released is reached only by cut_release)"`
	Branch     string `json:"branch,omitempty" jsonschema:"only when starting: the task branch"`
	PRURL      string `json:"pr_url,omitempty" jsonschema:"only when opening review: the pull request's link"`
	Commit     string `json:"commit,omitempty" jsonschema:"only when merging: the merge commit"`
	Into       string `json:"into,omitempty" jsonschema:"only when merging: the branch merged into; with none, the only branch nothing merges into (such as dev); with several, name it. A story moves on to the branch its current one merges into, one step at a time"`
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

type updatePhaseIn struct {
	in
	Number      int     `json:"number" jsonschema:"the phase's number"`
	Title       *string `json:"title,omitempty" jsonschema:"the new title; it can't be empty"`
	Description *string `json:"description,omitempty" jsonschema:"the new description; an empty string clears it"`
}

type updateGoalIn struct {
	in
	ID   string `json:"id" jsonschema:"the goal's id, such as G-2"`
	Text string `json:"text" jsonschema:"the goal's new text; it can't be empty"`
}

// readLogIn names what to read the log of: a story or a goal by id, or a
// phase by number.
type readLogIn struct {
	in
	ID    string `json:"id,omitempty" jsonschema:"a story's id, such as S-4, or a goal's, such as G-2"`
	Phase int    `json:"phase,omitempty" jsonschema:"a phase's number; give this or id, not both"`
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
	Title   string `json:"title"`
	Context string `json:"context,omitempty" jsonschema:"anything that helps understand the idea and turn it into work: notes, links, documents, background. Optional, but an idea without context is only as good as its title"`
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
	steps := map[string]step{}
	tool(s, "list_workspaces", "Every workspace, by slug, with its name and repository. Use it to find a workspace's slug when the project's .backpack.json isn't at hand.", t.listWorkspaces)
	tool(s, "list_branches", "A workspace's branches in merge order, each with the branch it merges into. The release branch merges nowhere and is marked release.", t.listBranches)
	tool(s, "whats_next", "The next story to deliver: the highest-ranked backlog story whose dependencies are all complete, with its description, goal and plan. The story is null when nothing is ready.", t.whatsNext)
	tool(s, "where_are_we", "Where the project stands, as a few lines of plain text (the same as the session hook): the current phase and how far along it is, what is in progress and what each story is doing, what is in review, what is on hold and why, the stories whose upstream story changed its goal or plan, how many stories merged into the release branch wait for a release (with the latest three), the next three in the backlog, and how many are waiting for approval. For a story's detail, use get_story.", t.whereAreWe)
	tool(s, "get_story", "One story, with its status, what blocks it and what it blocks.", t.getStory)
	write(s, steps, "add_story", "Propose a story: it goes in as proposed, directly before or after another story or at the bottom when neither is given, with the reasoning for that spot logged as a decision. It waits until a person approves it by setting its status to backlog. It must rank below everything it will depend on. Answers with the story's id and status.", t.addStory)
	write(s, steps, "update_story", "Change a story's fields; fields not given are left as they are. Activity is what an in-progress story is doing. Changing the goal or plan to a different value marks every open story that depends on it, directly or further down, as upstream changed (shown by get_story and where_are_we) and logs a note on each; it also clears this story's own mark. Other fields mark nothing. Answers with the id, the names of the fields that actually changed, and the ids of the stories it marked (when any).", t.updateStory)
	write(s, steps, "mark_reviewed", "Clear a story's upstream-changed mark: a story it depends on, directly or further down, had its goal or plan changed, and you have checked what that means for this one. The reason (required) is logged on the story as a note. Updating the story's own goal or plan also clears the mark. A story that is not marked is refused. Answers with the story's id.", t.markReviewed)
	write(s, steps, "move_story", "Move a story directly before or after another, or to the bottom of the backlog. Refused if it would rank above something it depends on, or below something that depends on it. Answers with the story's id.", t.moveStory)
	write(s, steps, "set_status", "Move a story across the board: forward one column, or back one before merged. A proposed story is approved to backlog, declined by closing it, or parked with on_hold. Merging records the branch reached (into; none means the only branch nothing merges into, and with several the merge must name one) and moves on one branch at a time, so merged into the release branch is a second merge. Released is not set here: cut_release releases the stories merged into the release branch. Merged and released are complete. on_hold resumes to the column it left (a held proposal resumes to proposed); closed is final. Every change is logged. Answers with the id and the new status (and, for a merge, the branch reached as into).", t.setStatus)
	write(s, steps, "merge_branch", "Record that a branch was merged into the one it merges into, such as dev into main, at the merge commit. Every story merged into that branch moves to the next one at once, each logged with the commit; stories still in progress or in review stay put. Answers with the ids of the stories it moved, none when the branch holds nothing. The release branch merges nowhere, so it is refused.", t.mergeBranch)
	write(s, steps, "cut_release", "Record an official release: a version and the git tag that marks it, with the commit and notes if you have them. Every story merged into the release branch that is not in a release yet becomes Released, is linked to the new release (R-n) and gets a log entry naming it; stories merged into other branches, in review or in progress stay put. Answers with the release and the ids of its stories. A version already released is refused, and so is a cut with nothing to release. Backpack does not talk to GitHub: create the tag yourself and record it here.", t.cutRelease)
	tool(s, "list_releases", "Every release, newest first, each with its version, tag, commit, notes, who cut it and when, and the ids of its stories in delivery order. get_release reads one with its stories in full.", t.listReleases)
	tool(s, "get_release", "One release by id (R-n): its version, tag, commit, notes, who cut it and when, and its stories in delivery order with title, goal and plan, as a changelog. An unknown id is refused.", t.getRelease)
	write(s, steps, "add_branch", "Add a branch that merges into an existing one, such as staging into main. No story moves: stories name their branch. A workspace has one release branch, so into is required. A name already in use, or one that is empty, has whitespace or is over 100 characters, is refused, as is an unknown branch to merge into. Several branches nothing merges into (dev and staging, both into main) are all places a story from review can be merged first; a merge then has to name which. Answers with the branch.", t.addBranch)
	write(s, steps, "update_branch", "Change a branch; fields not given are left as they are. Giving into changes the branch it merges into, such as dev into staging once staging is added: no story moves, and merges from the branch go to the new one. It is refused for the branch itself, one that leads back to it, an unknown branch, the release branch (it merges nowhere) and an empty into on any other branch (that would make a second release branch). Giving a new name renames it everywhere the board refers to it: the branches that merge into it and the stories that reached it. No story moves; past log entries keep the old name. A name already in use, or one that is empty, has whitespace or is over 100 characters, is refused. Giving an alias sets the name the web board's column heading shows for the branch (an empty one clears it); the API, tools and logs keep using the branch name. Answers with the branch.", t.updateBranch)
	write(s, steps, "log", "Add an entry to a story's log. Entries are never edited or deleted: fix a mistake with a correction that points to the entry. Answers with the new entry's id.", t.log)
	tool(s, "read_log", "The log of a story or goal (give id) or of a phase (give phase), oldest first. A phase's or goal's log holds one edit entry for each field an update changed, with the field and its old (from) and new (to) value.", t.readLog)
	write(s, steps, "add_dependency", "Record that a story can't be done until another is complete. The other must rank above it, be in the same or an earlier phase, and not close a circle. Answers with the story's id and whether it is now blocked.", t.addDependency)
	write(s, steps, "remove_branch", "Remove a branch, only when nothing refers to it: no story is merged into it (whatever its status) and no branch merges into it. The refusal names the stories and branches that still do. The release branch is never removed, nor the first branch (the only branch nothing merges into); with several branches nothing merges into, an empty one can go. An unknown branch is refused. Past log entries keep the removed name.", t.removeBranch)
	write(s, steps, "remove_dependency", "Remove a story's dependency on another.", t.removeDependency)
	tool(s, "list_phases", "Every phase in order, with its goals, how many stories are complete, whether it is complete, and whether its goals were all met while stories are still open.", t.listPhases)
	write(s, steps, "add_phase", "Add a phase, numbered after the last one. Answers with the phase's number.", t.addPhase)
	write(s, steps, "add_goal", "Add a goal to a phase. Answers with the goal's id and whether it is met.", t.addGoal)
	write(s, steps, "update_phase", "Change a phase's title or description; fields not given are left as they are, and the title can't be empty. Each field that changes is logged on the phase with its old and new value (read_log with phase). Answers with the phase's number.", t.updatePhase)
	write(s, steps, "update_goal", "Change a goal's text, which can't be empty. The change is logged on the goal with the old and new text (read_log with the goal's id). Answers with the goal's id.", t.updateGoal)
	write(s, steps, "mark_goal_met", "Mark a goal met, or not met. Answers with the goal's id and whether it is met.", t.markGoalMet)
	write(s, steps, "add_idea", "Add an idea: a rough thought, not yet work. It goes into the ideas list, apart from the backlog, directly before or after another idea or at the bottom, and is open at once; no approval is needed. Give it context: anything that helps understand the idea and turn it into work, such as notes, links, documents or background. An idea without context is only as good as its title. Answers with the idea's id and state.", t.addIdea)
	tool(s, "list_ideas", "The ideas that are not closed, in order, top first.", t.listIdeas)
	tool(s, "get_idea", "One idea, closed or not, with what it became or why it was dropped.", t.getIdea)
	write(s, steps, "update_idea", "Change an idea's title or context while it is not closed; fields not given are left as they are. Answers with the idea's id and state.", t.updateIdea)
	write(s, steps, "move_idea", "Move an idea that is not closed directly before or after another, or to the bottom of the ideas list. Answers with the idea's id and state.", t.moveIdea)
	write(s, steps, "refine_idea", "Close an open idea, linking it to the stories and phase it became. Build those from the idea's context first, the usual way; none may be closed. Answers with the idea's id and state; what it became is in get_idea.", t.refineIdea)
	write(s, steps, "drop_idea", "Close an idea that became nothing, with the reason. Answers with the idea's id and state.", t.dropIdea)
	tool(s, "search", "Check whether a story, idea or phase already covers something, before adding one. Matches by keyword and by meaning, best first, up to ten; closed stories and dropped ideas are included, with their state.", t.search)
	tool(s, "batch", "Make many board changes in one call. Each action names a tool that changes the board and gives that tool's arguments without the workspace, which is given once for the batch. Actions run in order, each as if called alone; one that fails is answered with its reason and doesn't stop the rest. An action can name only items that already exist, not one an earlier action in the batch adds. Up to 100 actions; reads can't be batched.", batch(steps))
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
	return map[string]any{"workspaces": ws}, err
}

func (t tools) listBranches(_ context.Context, a in) (any, error) {
	bs, err := t.b.Branches(a.Workspace)
	return map[string]any{"branches": bs}, err
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
		Goal: a.Goal, Plan: a.Plan, Phase: a.Phase, Proposed: true, Reasoning: a.Reasoning,
		Position: board.Position(a.position)})
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": s.ID, "status": s.Status}, nil
}

func (t tools) updateStory(ctx context.Context, a updateStoryIn) (any, error) {
	u, err := t.b.UpdateStoryDetail(a.Workspace, actor(ctx), a.ID, board.StoryPatch{Title: a.Title,
		Description: a.Description, Goal: a.Goal, Plan: a.Plan, Activity: a.Activity, WaitingOn: a.WaitingOn,
		Branch: a.Branch, PRURL: a.PRURL})
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
	return t.b.CutRelease(a.Workspace, actor(ctx), board.NewRelease{Version: a.Version, Tag: a.Tag, Commit: a.Commit, Notes: a.Notes})
}

func (t tools) listReleases(_ context.Context, a in) (any, error) {
	rs, err := t.b.ListReleases(a.Workspace)
	return map[string]any{"releases": rs}, err
}

func (t tools) getRelease(_ context.Context, a releaseIn) (any, error) {
	return t.b.GetRelease(a.Workspace, a.ID)
}

func (t tools) addBranch(ctx context.Context, a addBranchIn) (any, error) {
	return t.b.AddBranch(a.Workspace, actor(ctx), board.Branch{Name: a.Name, Into: a.Into, Alias: a.Alias})
}

func (t tools) removeBranch(ctx context.Context, a removeBranchIn) (any, error) {
	err := t.b.RemoveBranch(a.Workspace, actor(ctx), a.Branch)
	return map[string]any{"removed": true}, err
}

func (t tools) updateBranch(ctx context.Context, a updateBranchIn) (any, error) {
	return t.b.UpdateBranch(a.Workspace, actor(ctx), a.Branch, board.BranchPatch{Name: a.Name, Alias: a.Alias, Into: a.Into})
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
	return map[string]any{"phases": ps}, err
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
	return map[string]any{"ideas": is}, err
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
