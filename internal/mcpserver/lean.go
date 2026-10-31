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
	Phase       int    `json:"phase,omitempty"`
	Status      string `json:"status"`
	// Blocked is sent only when true.
	Blocked         bool               `json:"blocked,omitempty"`
	BlockedBy       []string           `json:"blocked_by,omitempty"`
	Blocks          []string           `json:"blocks,omitempty"`
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
		ID: s.ID, Title: s.Title, Description: s.Description, Goal: s.Goal, Plan: s.Plan,
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
