package mcpserver

import "github.com/lioneljmorrison/backpack/internal/board"

// leanStory is a story as an agent reads it: only the fields that say
// something. Who made or moved it, and when, stay in the log (read_log), and
// nothing empty is sent. The HTTP routes keep answering with the full
// board.Story.
type leanStory struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Goal        string `json:"goal,omitempty"`
	Plan        string `json:"plan,omitempty"`
	Kind        string `json:"kind,omitempty"`
	FoundIn     string `json:"found_in,omitempty"`
	StraightIn  bool   `json:"straight_in,omitempty"`
	Phase       int    `json:"phase,omitempty"`
	Status      string `json:"status"`
	// Blocked is sent only when true.
	Blocked         bool               `json:"blocked,omitempty"`
	BlockedBy       []string           `json:"blocked_by,omitempty"`
	Blocks          []string           `json:"blocks,omitempty"`
	Found           []string           `json:"found,omitempty"`
	HeldFrom        string             `json:"held_from,omitempty"`
	HoldReason      string             `json:"hold_reason,omitempty"`
	Activity        string             `json:"activity,omitempty"`
	WaitingOn       string             `json:"waiting_on,omitempty"`
	Branch          string             `json:"branch,omitempty"`
	Into            string             `json:"into,omitempty"`
	PRURL           string             `json:"pr_url,omitempty"`
	Workaround      string             `json:"workaround,omitempty"`
	Release         *board.ReleaseRef  `json:"release,omitempty"`
	UpstreamChanged *board.UpstreamRef `json:"upstream_changed,omitempty"`
}

// lean is the agent's view of s.
func lean(s board.Story) leanStory {
	return leanStory{
		ID: s.ID, Title: s.Title, Description: s.Description, Goal: s.Goal, Plan: s.Plan, Kind: s.Kind, FoundIn: s.FoundIn, StraightIn: s.StraightIn, Found: s.Found,
		Phase: s.Phase, Status: s.Status, Blocked: s.Blocked, BlockedBy: s.BlockedBy, Blocks: s.Blocks,
		HeldFrom: s.HeldFrom, HoldReason: s.HoldReason, Activity: s.Activity, WaitingOn: s.WaitingOn,
		Branch: s.Branch, Into: s.Into, PRURL: s.PRURL, Workaround: s.Workaround,
		Release: s.Release, UpstreamChanged: s.UpstreamChanged,
	}
}

// leanResult is a handler's story result in its lean form, keeping its error.
func leanResult(s board.Story, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return lean(s), nil
}

// Lists answer summaries: an id, a name, the state and one useful detail.
// The single-item read (get_idea, get_release, get_story) has the rest, and
// the HTTP routes keep every field.

type ideaSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

func ideaSummaries(is []board.Idea) []ideaSummary {
	out := make([]ideaSummary, 0, len(is))
	for _, i := range is {
		out = append(out, ideaSummary{ID: i.ID, Title: i.Title, State: i.State})
	}
	return out
}

type goalSummary struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Met  bool   `json:"met"`
}

// phaseSummary leaves out who and when. The description stays, when there is
// one: there is no single-phase read over MCP, and it carries decisions.
type phaseSummary struct {
	Number          int           `json:"number"`
	Title           string        `json:"title"`
	Description     string        `json:"description,omitempty"`
	Complete        bool          `json:"complete"`
	StoriesComplete int           `json:"stories_complete"`
	StoriesTotal    int           `json:"stories_total"`
	Goals           []goalSummary `json:"goals"`
	// GoalsMetEarly is sent only when true.
	GoalsMetEarly bool `json:"goals_met_early,omitempty"`
}

func phaseSummaries(ps []board.Phase) []phaseSummary {
	out := make([]phaseSummary, 0, len(ps))
	for _, p := range ps {
		gs := make([]goalSummary, 0, len(p.Goals))
		for _, g := range p.Goals {
			gs = append(gs, goalSummary{ID: g.ID, Text: g.Text, Met: g.Met})
		}
		out = append(out, phaseSummary{Number: p.Number, Title: p.Title, Description: p.Description, Complete: p.Complete,
			StoriesComplete: p.StoriesComplete, StoriesTotal: p.StoriesTotal, Goals: gs, GoalsMetEarly: p.GoalsMetEarly})
	}
	return out
}

// releaseSummary carries how many stories shipped, not which.
type releaseSummary struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Tag     string `json:"tag"`
	Stories int    `json:"stories"`
}

func releaseSummaries(rs []board.Release) []releaseSummary {
	out := make([]releaseSummary, 0, len(rs))
	for _, r := range rs {
		out = append(out, releaseSummary{ID: r.ID, Version: r.Version, Tag: r.Tag, Stories: len(r.Stories)})
	}
	return out
}

// leanRelease is a release read as a changelog: its stories in the lean form
// and no who-and-when, like the story itself.
type leanRelease struct {
	ID      string      `json:"id"`
	Version string      `json:"version"`
	Tag     string      `json:"tag"`
	Commit  string      `json:"commit,omitempty"`
	Notes   string      `json:"notes,omitempty"`
	Stories []leanStory `json:"stories"`
}

func leanReleaseOf(d board.ReleaseDetail) leanRelease {
	ss := make([]leanStory, 0, len(d.Entries))
	for _, s := range d.Entries {
		ss = append(ss, lean(s))
	}
	return leanRelease{ID: d.ID, Version: d.Version, Tag: d.Tag, Commit: d.Commit, Notes: d.Notes, Stories: ss}
}

type workspaceSummary struct {
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	RepoURL         string `json:"repo_url,omitempty"`
	HasReleaseStage bool   `json:"has_release_stage"`
}

func workspaceSummaries(ws []board.Workspace) []workspaceSummary {
	out := make([]workspaceSummary, 0, len(ws))
	for _, w := range ws {
		out = append(out, workspaceSummary{Slug: w.Slug, Name: w.Name, RepoURL: w.RepoURL, HasReleaseStage: w.HasReleaseStage})
	}
	return out
}

type branchSummary struct {
	Name    string `json:"name"`
	Into    string `json:"into,omitempty"`
	Alias   string `json:"alias,omitempty"`
	Release bool   `json:"release"`
}

func branchSummaries(bs []board.Branch) []branchSummary {
	out := make([]branchSummary, 0, len(bs))
	for _, b := range bs {
		out = append(out, branchSummary{Name: b.Name, Into: b.Into, Alias: b.Alias, Release: b.Release})
	}
	return out
}
