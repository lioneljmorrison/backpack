package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/board"
)

type tools struct{ b *board.Board }

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

func addTools(s *mcp.Server, t tools) {
	tool(s, "whats_next", "The next story to deliver: the highest-ranked backlog story whose dependencies are all complete, with its description, goal and plan. The story is null when nothing is ready.", t.whatsNext)
	tool(s, "where_are_we", "The current phase and how far along it is, what is in progress and what each story is doing, what is in review, what is on hold and why, and the next three in the backlog.", t.whereAreWe)
	tool(s, "get_story", "One story, with its status, what blocks it and what it blocks.", t.getStory)
	tool(s, "add_story", "Add a story to the backlog, directly before or after another story, or at the bottom when neither is given. It must rank below everything it will depend on.", t.addStory)
	tool(s, "update_story", "Change a story's fields; fields not given are left as they are. Activity is what an in-progress story is doing.", t.updateStory)
	tool(s, "move_story", "Move a story directly before or after another, or to the bottom of the backlog. Refused if it would rank above something it depends on, or below something that depends on it.", t.moveStory)
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
		Goal: a.Goal, Plan: a.Plan, Phase: a.Phase, Position: board.Position(a.position)})
}

func (t tools) updateStory(ctx context.Context, a updateStoryIn) (any, error) {
	return t.b.UpdateStory(a.Workspace, actor(ctx), a.ID, board.StoryPatch{Title: a.Title,
		Description: a.Description, Goal: a.Goal, Plan: a.Plan, Activity: a.Activity, WaitingOn: a.WaitingOn,
		Branch: a.Branch, PRURL: a.PRURL})
}

func (t tools) moveStory(ctx context.Context, a moveStoryIn) (any, error) {
	return t.b.MoveStory(a.Workspace, actor(ctx), a.ID, board.Position(a.position))
}
