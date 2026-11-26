package board

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
)

// upcoming is how many backlog stories "where are we" shows.
const upcoming = 3

// Next is the top backlog story that is not blocked in the current phase (the
// first that is neither complete nor closed), or, when that phase has none or
// there is no current phase, the highest-ranked one in the whole backlog; nil
// when there is none. Rank orders the stories inside a phase. With a
// repository it is that repository's: a story is still blocked by unfinished
// work in any repository. An unknown repository is NotFound.
func (b *Board) Next(ws, repo string) (*Story, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	if repo != "" {
		rs, err := repos(g)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(rs, func(r Repo) bool { return r.Name == repo }) {
			return nil, noRepo(ws, repo)
		}
	}
	ss, err := stories(g, "")
	if err != nil {
		return nil, err
	}
	ps, err := phases(g, 0)
	if err != nil {
		return nil, err
	}
	ready := func(s Story) bool { return s.Status == Backlog && !s.Blocked && (repo == "" || s.Repo == repo) }
	i := -1
	if cur := currentPhase(ps); cur != nil {
		i = slices.IndexFunc(ss, func(s Story) bool { return ready(s) && s.Phase == cur.Number })
	}
	if i < 0 {
		i = slices.IndexFunc(ss, ready)
	}
	if i < 0 {
		return nil, nil
	}
	return &ss[i], nil
}

// currentPhase is the first phase that is neither complete nor closed; nil
// when there is none.
func currentPhase(ps []Phase) *Phase {
	if i := slices.IndexFunc(ps, func(p Phase) bool { return !p.Complete && !p.Closed }); i >= 0 {
		return &ps[i]
	}
	return nil
}

// upNext is the stories "where are we" lists as next: the current phase's
// ready backlog stories first, in rank order, then the rest of the backlog in
// rank order (blocked included), up to upcoming.
func upNext(ss []Story, cur *Phase, repo string) []Story {
	out := []Story{}
	inBacklog := func(s Story) bool { return s.Status == Backlog && (repo == "" || s.Repo == repo) }
	if cur != nil {
		for _, s := range ss {
			if len(out) < upcoming && inBacklog(s) && !s.Blocked && s.Phase == cur.Number {
				out = append(out, s)
			}
		}
	}
	for _, s := range ss {
		if len(out) < upcoming && inBacklog(s) && !slices.ContainsFunc(out, func(o Story) bool { return o.ID == s.ID }) {
			out = append(out, s)
		}
	}
	return out
}

// Where is the short answer to "where are we".
type Where struct {
	// Phase is the first phase that is neither complete nor closed; nil when
	// there is none.
	Phase *Phase `json:"phase"`
	// Proposed is what is waiting for a person to approve it.
	Proposed   []Story `json:"proposed"`
	InProgress []Story `json:"in_progress"`
	InReview   []Story `json:"in_review"`
	OnHold     []Story `json:"on_hold"`
	// Next is up to three backlog stories: the current phase's ready ones first,
	// then the rest by rank, blocked stories included.
	Next []Story `json:"next"`
	// StraightIn is the bugs that skipped approval and are not yet started.
	StraightIn []Story `json:"straight_in"`
	// Unreleased is what is merged into the release branch and not yet in a
	// release, in delivery order.
	Unreleased []Story `json:"unreleased"`
	// UpstreamChanged is the stories not yet complete whose upstream story
	// had its goal or plan changed, in delivery order.
	UpstreamChanged []Story `json:"upstream_changed"`

	// several is true when the workspace has more than one repository, which
	// is when the text says which repository each story is in.
	several bool
}

// Where reads the current phase, the work under way and what comes next.
func (b *Board) Where(ws string) (Where, error) { return b.WhereFor(ws, "") }

// WhereFor is Where with Next narrowed to one repository's stories, chosen
// by the same rule as Next. Everything else stays the whole workspace's. An
// unknown repository is NotFound.
func (b *Board) WhereFor(ws, repo string) (Where, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return Where{}, err
	}
	rs, err := repos(g)
	if err != nil {
		return Where{}, err
	}
	if repo != "" && !slices.ContainsFunc(rs, func(r Repo) bool { return r.Name == repo }) {
		return Where{}, noRepo(ws, repo)
	}
	ps, err := phases(g, 0)
	if err != nil {
		return Where{}, err
	}
	ss, err := stories(g, "")
	if err != nil {
		return Where{}, err
	}
	bs, err := branches(g, "")
	if err != nil {
		return Where{}, err
	}
	// Each repository waits on its own release branch.
	release := map[string]string{}
	for _, br := range bs {
		if _, ok := release[br.Repo]; br.Release && !ok {
			release[br.Repo] = br.Name
		}
	}
	w := Where{Proposed: []Story{}, InProgress: []Story{}, InReview: []Story{}, OnHold: []Story{}, Next: []Story{}, StraightIn: []Story{}, Unreleased: []Story{}, UpstreamChanged: []Story{}, several: len(rs) > 1}
	w.Phase = currentPhase(ps)
	for _, s := range ss {
		if s.UpstreamChanged != nil && !slices.Contains(complete, s.Status) && s.Status != Closed {
			w.UpstreamChanged = append(w.UpstreamChanged, s)
		}
		switch s.Status {
		case Proposed:
			w.Proposed = append(w.Proposed, s)
		case InProgress:
			w.InProgress = append(w.InProgress, s)
		case InReview:
			w.InReview = append(w.InReview, s)
		case OnHold:
			w.OnHold = append(w.OnHold, s)
		case Merged:
			if rb := release[s.Repo]; rb != "" && s.Into == rb {
				w.Unreleased = append(w.Unreleased, s)
			}
		case Backlog:
			if s.StraightIn {
				w.StraightIn = append(w.StraightIn, s)
			}
		}
	}
	w.Next = upNext(ss, w.Phase, repo)
	return w, nil
}

// Text is "where are we" as a few lines for a session hook.
func (w Where) Text() string {
	if w.Phase == nil && len(w.Proposed)+len(w.InProgress)+len(w.InReview)+len(w.OnHold)+len(w.Next)+len(w.StraightIn)+len(w.Unreleased)+len(w.UpstreamChanged) == 0 {
		return "Nothing in progress, in review or in the backlog.\n"
	}
	var b strings.Builder
	if p := w.Phase; p != nil {
		fmt.Fprintf(&b, "Phase %d: %s — %d of %d stories complete", p.Number, p.Title, p.StoriesComplete, p.StoriesTotal)
		if p.GoalsMetEarly {
			b.WriteString(" (every goal is met but stories are still open)")
		}
		b.WriteString("\n")
	}
	section := func(title string, ss []Story, detail func(Story) []string) {
		if len(ss) == 0 {
			return
		}
		b.WriteString(title + ":\n")
		for _, s := range ss {
			parts := detail(s)
			if s.Blocked {
				blocked := "blocked by " + list(s.BlockedBy)
				if s.Workaround != "" {
					blocked += ", working around it: " + s.Workaround
				}
				parts = append(parts, blocked)
			}
			if w.several {
				fmt.Fprintf(&b, "  %s [%s] %s", s.ID, s.Repo, s.Title)
			} else {
				fmt.Fprintf(&b, "  %s %s", s.ID, s.Title)
			}
			if len(parts) > 0 {
				b.WriteString(" — " + strings.Join(parts, "; "))
			}
			b.WriteString("\n")
		}
	}
	none := func(Story) []string { return nil }
	section("In progress", w.InProgress, func(s Story) []string {
		if s.Activity == "waiting" && s.WaitingOn != "" {
			return []string{"waiting on " + s.WaitingOn}
		}
		return []string{s.Activity}
	})
	section("In review", w.InReview, func(s Story) []string {
		if s.PRURL == "" {
			return nil
		}
		return []string{s.PRURL}
	})
	section("On hold", w.OnHold, func(s Story) []string {
		return []string{fmt.Sprintf("%s (was in %s)", s.HoldReason, label(s.HeldFrom))}
	})
	// One line per repository (or the one line, with a single repository): a
	// count and the latest few, so a line stays one line however long the wait
	// for a release gets. The full list is in the JSON.
	var waiting []string
	byRepo := map[string][]Story{}
	for _, s := range w.Unreleased {
		if _, ok := byRepo[s.Repo]; !ok {
			waiting = append(waiting, s.Repo)
		}
		byRepo[s.Repo] = append(byRepo[s.Repo], s)
	}
	slices.Sort(waiting)
	for _, repo := range waiting {
		ss := byRepo[repo]
		n := len(ss)
		latest := make([]string, 0, upcoming)
		for _, s := range ss[max(0, n-upcoming):] {
			latest = append(latest, s.ID+" "+s.Title)
		}
		in := ""
		if w.several {
			in = " in " + repo
		}
		fmt.Fprintf(&b, "Merged, waiting for a release%s: %d (latest: %s)\n", in, n, strings.Join(latest, ", "))
	}
	section("Upstream changed", w.UpstreamChanged, func(s Story) []string {
		return []string{"after " + s.UpstreamChanged.Story}
	})
	section("Straight in, not started", w.StraightIn, func(s Story) []string {
		parts := []string{"found in " + s.FoundIn}
		if f := s.StraightFlag(); f != "" {
			parts = append(parts, "flagged: "+f)
		}
		return parts
	})
	section("Next", w.Next, none)
	section("Waiting for approval", w.Proposed, none)
	return b.String()
}

// Column is one column of the board, its stories in delivery order. A merged
// column names its repository and branch.
type Column struct {
	Status  string  `json:"status"`
	Repo    string  `json:"repo,omitempty"`
	Branch  string  `json:"branch,omitempty"`
	Stories []Story `json:"stories"`
}

// Board reads every story grouped by column. Merged has one column per
// branch of every repository, repositories in name order and each one's
// branches in merge order, holding the stories of that repository whose
// furthest branch it is.
func (b *Board) Board(ws string) ([]Column, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	bs, err := branches(g, "")
	if err != nil {
		return nil, err
	}
	ss, err := stories(g, "")
	if err != nil {
		return nil, err
	}
	in := map[string][]Story{}
	for _, s := range ss {
		k := s.Status
		if k == Merged {
			k += ":" + s.Repo + ":" + s.Into
		}
		in[k] = append(in[k], s)
	}
	cols := []Column{}
	for _, status := range statuses {
		if status != Merged {
			cols = append(cols, Column{Status: status, Stories: append([]Story{}, in[status]...)})
			continue
		}
		for _, br := range bs {
			cols = append(cols, Column{Status: status, Repo: br.Repo, Branch: br.Name, Stories: append([]Story{}, in[status+":"+br.Repo+":"+br.Name]...)})
		}
	}
	return cols, nil
}

// hasReleaseStage is true when some branch merges into another.
func hasReleaseStage(g *falkordb.Graph) (bool, error) {
	res, err := g.ROQuery("MATCH (b:Branch) WHERE b.into IS NOT NULL RETURN count(b) > 0 AS release", nil, nil)
	if err != nil {
		return false, fmt.Errorf("read branches: %w", err)
	}
	return res.Next() && flag(res.Record(), "release"), nil
}
