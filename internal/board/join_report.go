package board

import (
	"fmt"
	"sort"
	"strings"
)

// review finds the top of the merged backlog and what a person should know
// before agreeing to the join.
func (p JoinPlan) review() (*JoinTop, []string) {
	var top *JoinTop
	topRank := ""
	var warns []string
	for _, s := range p.Sources {
		changed := s.Offsets != (JoinOffsets{})
		for _, n := range s.Nodes {
			if n.Labels[0] != "Story" {
				continue
			}
			m := s.Remap(n)
			status, _ := m.Props["status"].(string)
			rank, _ := m.Props["rank"].(string)
			if status == "backlog" && (top == nil || rank < topRank) {
				title, _ := m.Props["title"].(string)
				top = &JoinTop{Source: s.Slug, OldID: m.Props["old_id"].(string), NewID: m.Props["id"].(string), Title: title}
				topRank = rank
			}
			if changed && s.Offsets.Story > 0 && (status == "in_progress" || status == "in_review") {
				w := fmt.Sprintf("%s (was %s %s) is %s and gets a new id", m.Props["id"], s.Slug, m.Props["old_id"], status)
				if br, _ := m.Props["branch"].(string); br != "" {
					w += "; its branch is " + br
				}
				warns = append(warns, w)
			}
		}
		if changed && s.Mentions > 0 {
			warns = append(warns, fmt.Sprintf("%s: %d text field(s) name ids that change; text is copied exactly, so they keep naming the old ids",
				s.Slug, s.Mentions))
		}
	}
	return top, warns
}

// span writes the ids first..last, or the one id.
func span(prefix string, first, last int) string {
	if last < first {
		return "none"
	}
	if first == last {
		return fmt.Sprintf("%s%d", prefix, first)
	}
	return fmt.Sprintf("%s%d..%s%d", prefix, first, prefix, last)
}

// Report is the plan in words for a person: what each source brings, where
// its ids and ranks land, and what to look at before agreeing.
func (p JoinPlan) Report() string {
	return p.report(false)
}

// AppliedReport is the same, for a join that was done: what moved where, and
// what is left to do.
func (p JoinPlan) AppliedReport() string {
	return p.report(true)
}

func (p JoinPlan) report(applied bool) string {
	var b strings.Builder
	r := p.Request
	if applied {
		fmt.Fprintf(&b, "joined %s into %s (%s) as %s\n", strings.Join(r.From, ", "), r.Into, r.Name, r.Actor)
	} else {
		fmt.Fprintf(&b, "join %s into %s (%s) as %s\n", strings.Join(r.From, ", "), r.Into, r.Name, r.Actor)
		b.WriteString("preview only: nothing was written\n")
	}
	for _, s := range p.Sources {
		note := "its ids move up"
		if s.Offsets == (JoinOffsets{}) {
			note = "keeps its ids"
		}
		fmt.Fprintf(&b, "\n%s (%d of %d; %s; ranks start with %s)\n", s.Slug, s.Index+1, len(p.Sources), note, s.RankPrefix)
		names := make([]string, len(s.Repos))
		for i, rp := range s.Repos {
			names[i] = rp.Name
		}
		fmt.Fprintf(&b, "  repositories: %s\n", strings.Join(names, ", "))
		fmt.Fprintf(&b, "  branches: %s\n", strings.Join(s.Branches, ", "))
		fmt.Fprintf(&b, "  holds: %s\n", s.holds())
		o, c := s.Offsets, s.Counters
		row := func(kind, prefix string, count, off int) {
			fmt.Fprintf(&b, "  %s %s -> %s\n", kind, span(prefix, 1, count), span(prefix, 1+off, count+off))
		}
		row("stories", "S-", c.Story-1, o.Story)
		row("ideas", "I-", c.Idea-1, o.Idea)
		row("goals", "G-", c.Goal-1, o.Goal)
		row("releases", "R-", c.Release-1, o.Release)
		row("log entries", "L-", c.Log-1, o.Log)
		row("phases", "", s.Phases, o.Phase)
	}
	t := p.Target
	fmt.Fprintf(&b, "\nthe new workspace %s starts at next_story %d, next_log %d, next_idea %d, next_goal %d, next_release %d\n",
		r.Into, t.Story, t.Log, t.Idea, t.Goal, t.Release)
	fmt.Fprintf(&b, "join notes, one per story, take %s\n", span("L-", p.Notes.First, p.Notes.First+p.Notes.Count-1))
	if p.Top != nil {
		fmt.Fprintf(&b, "top of the merged backlog: %s (was %s %s) %s\n", p.Top.NewID, p.Top.Source, p.Top.OldID, p.Top.Title)
	} else {
		b.WriteString("top of the merged backlog: none, no source has a backlog story\n")
	}
	if len(p.Warnings) > 0 {
		if applied {
			b.WriteString("\nlook at after joining:\n")
		} else {
			b.WriteString("\nlook at before joining:\n")
		}
		for _, w := range p.Warnings {
			fmt.Fprintf(&b, "  %s\n", w)
		}
	}
	if applied {
		fmt.Fprintf(&b, "\nthe sources are marked joined into %s and keep their graphs\n", r.Into)
	}
	return b.String()
}

// holds lists what a source holds, label by label, for the report.
func (s JoinSource) holds() string {
	var ls []string
	for l := range s.Counts {
		ls = append(ls, l)
	}
	sort.Strings(ls)
	parts := make([]string, len(ls))
	for i, l := range ls {
		parts[i] = fmt.Sprintf("%d %s", s.Counts[l], l)
	}
	return strings.Join(parts, ", ")
}
