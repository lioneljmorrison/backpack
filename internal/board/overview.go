package board

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
)

// upcoming is how many backlog stories "where are we" shows.
const upcoming = 3

// Next is the highest-ranked backlog story that is not blocked, or nil when
// there is none.
func (b *Board) Next(ws string) (*Story, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	ss, err := stories(g, "")
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(ss, func(s Story) bool { return s.Status == Backlog && !s.Blocked })
	if i < 0 {
		return nil, nil
	}
	return &ss[i], nil
}

// Where is the short answer to "where are we".
type Where struct {
	// Phase is the first phase that is not complete; nil when there is none.
	Phase *Phase `json:"phase"`
	// Proposed is what is waiting for a person to approve it.
	Proposed   []Story `json:"proposed"`
	InProgress []Story `json:"in_progress"`
	InReview   []Story `json:"in_review"`
	OnHold     []Story `json:"on_hold"`
	// Next is the top of the backlog, blocked stories included.
	Next []Story `json:"next"`
}

// Where reads the current phase, the work under way and what comes next.
func (b *Board) Where(ws string) (Where, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return Where{}, err
	}
	ps, err := phases(g, 0)
	if err != nil {
		return Where{}, err
	}
	ss, err := stories(g, "")
	if err != nil {
		return Where{}, err
	}
	w := Where{Proposed: []Story{}, InProgress: []Story{}, InReview: []Story{}, OnHold: []Story{}, Next: []Story{}}
	if i := slices.IndexFunc(ps, func(p Phase) bool { return !p.Complete }); i >= 0 {
		w.Phase = &ps[i]
	}
	for _, s := range ss {
		switch s.Status {
		case Proposed:
			w.Proposed = append(w.Proposed, s)
		case InProgress:
			w.InProgress = append(w.InProgress, s)
		case InReview:
			w.InReview = append(w.InReview, s)
		case OnHold:
			w.OnHold = append(w.OnHold, s)
		case Backlog:
			if len(w.Next) < upcoming {
				w.Next = append(w.Next, s)
			}
		}
	}
	return w, nil
}

// Text is "where are we" as a few lines for a session hook.
func (w Where) Text() string {
	if w.Phase == nil && len(w.Proposed)+len(w.InProgress)+len(w.InReview)+len(w.OnHold)+len(w.Next) == 0 {
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
				blocked := "blocked by " + list(s.waiting)
				if s.Workaround != "" {
					blocked += ", working around it: " + s.Workaround
				}
				parts = append(parts, blocked)
			}
			fmt.Fprintf(&b, "  %s %s", s.ID, s.Title)
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
	section("Next", w.Next, none)
	section("Waiting for approval", w.Proposed, none)
	return b.String()
}

// Column is one column of the board, its stories in delivery order.
type Column struct {
	Status  string  `json:"status"`
	Stories []Story `json:"stories"`
}

// Board reads every story grouped by column. Released appears only when the
// workspace has a release stage.
func (b *Board) Board(ws string) ([]Column, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return nil, err
	}
	release, err := hasReleaseStage(g)
	if err != nil {
		return nil, err
	}
	ss, err := stories(g, "")
	if err != nil {
		return nil, err
	}
	in := map[string][]Story{}
	for _, s := range ss {
		in[s.Status] = append(in[s.Status], s)
	}
	cols := []Column{}
	for _, status := range statuses {
		if status == Released && !release {
			continue
		}
		cols = append(cols, Column{Status: status, Stories: append([]Story{}, in[status]...)})
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
