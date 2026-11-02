package board

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/falkor"
)

// Joining workspaces (Phase 6). PreviewJoin is the first half: it reads the
// source workspaces and works out, without writing anything, exactly what a
// join into one new workspace would do. Its JoinPlan is also what the join
// itself applies: the snapshot of each source, the offsets that renumber it,
// and Remap, the rule that turns a snapshot node into the node the target
// gets.

// joinLabels and joinEdges are everything a workspace graph holds. A source
// with anything else is refused, so a join never silently drops data.
var joinLabels = []string{"Workspace", "SchemaVersion", "Repo", "Branch", "Phase", "Goal", "Story", "Release", "Idea", "LogEntry"}
var joinEdges = []string{"HAS_GOAL", "CONTAINS", "DEPENDS_ON", "FOUND_IN", "LOGGED", "CORRECTS", "BECAME", "IN_RELEASE", "HAS_BRANCH", "HOLDS", "HAS_RELEASE"}

// maxJoinSources is how many workspaces one join takes: each source's ranks
// get one digit of rank.go's alphabet as a prefix, and the first is kept
// for the first source's own.
const maxJoinSources = len(digits) - 1

// JoinRequest says which workspaces become which new workspace.
type JoinRequest struct {
	Into  string   `json:"into"`
	Name  string   `json:"name"`
	Actor string   `json:"actor"`
	From  []string `json:"from"`
}

// JoinOffsets is what a source's ids move up by: the number of ids the
// sources before it used. The first source's are all zero.
type JoinOffsets struct {
	Story, Log, Idea, Goal, Release, Phase int
}

// JoinCounters are the next ids of a workspace: its Workspace node's next_*
// fields, a missing one reading as 1.
type JoinCounters struct {
	Story, Log, Idea, Goal, Release int
}

// JoinNotes is the block of log ids the target reserves, one per story, for
// the note each story gets saying where it came from: First is the number of
// the first id.
type JoinNotes struct {
	First, Count int
}

// SnapNode is one node of a source graph. ID is the graph's internal id,
// which SnapEdge points at; it means nothing in the target.
type SnapNode struct {
	ID     uint64
	Labels []string
	Props  map[string]any
}

// SnapEdge is one edge of a source graph, between the internal ids of two
// nodes of its snapshot.
type SnapEdge struct {
	Type     string
	From, To uint64
	Props    map[string]any
}

// JoinSource is one source workspace as the join sees it.
type JoinSource struct {
	Slug string
	// Index is its place in the join, from 0; its ranks are prefixed with
	// the Index+1'th digit.
	Index      int
	RankPrefix string
	Repos      []Repo
	// Branches are "repo/branch", sorted.
	Branches []string
	// Counts are the nodes of each label.
	Counts   map[string]int
	Counters JoinCounters
	// Phases is the highest phase number, 0 for none.
	Phases  int
	Offsets JoinOffsets
	// Mentions is how many text fields (a story's description, goal and
	// plan, an idea's context, a phase's description, a goal's text, a
	// release's notes) name an id such as S-3. A join copies text exactly,
	// so these keep naming the old ids.
	Mentions int
	Nodes    []SnapNode
	Edges    []SnapEdge
}

// JoinTop is the story that would top the merged backlog.
type JoinTop struct {
	Source, OldID, NewID, Title string
}

// JoinPlan is everything a join would do.
type JoinPlan struct {
	Request JoinRequest
	Sources []JoinSource
	// Target is the next ids the new workspace starts with.
	Target JoinCounters
	Notes  JoinNotes
	// Top is nil when no source has a backlog story.
	Top      *JoinTop
	Warnings []string
}

var idPattern = regexp.MustCompile(`\b[SILGR]-\d+\b`)

// PreviewJoin reads the sources and reports what joining them would do. It
// writes nothing: sources are only read, and the target's graph is not
// touched.
func (b *Board) PreviewJoin(req JoinRequest) (JoinPlan, error) {
	if err := checkJoinRequest(req); err != nil {
		return JoinPlan{}, err
	}
	entries, err := b.joinEntries()
	if err != nil {
		return JoinPlan{}, err
	}
	graphs, err := b.db.ListGraphs()
	if err != nil {
		return JoinPlan{}, fmt.Errorf("list graphs: %w", err)
	}
	if err := checkJoinRegistry(req, entries, func(slug string) bool { return slices.Contains(graphs, b.prefix+slug) }); err != nil {
		return JoinPlan{}, err
	}

	plan := JoinPlan{Request: req}
	var off JoinOffsets
	stories := 0
	repoIn := map[string]string{}
	r0 := ""
	for k, slug := range req.From {
		src, err := b.readJoinSource(slug, k)
		if err != nil {
			return JoinPlan{}, err
		}
		for _, r := range src.Repos {
			if other, ok := repoIn[r.Name]; ok {
				return JoinPlan{}, refuse(Conflict, "repository %q is in %s and %s; rename one before joining", r.Name, other, slug)
			}
			repoIn[r.Name] = slug
		}
		if src.hasR0() {
			if r0 != "" {
				return JoinPlan{}, refuse(Conflict, "R-0 (Before releases) is in %s and %s; only one is allowed", r0, slug)
			}
			r0 = slug
		}
		src.Offsets = off
		off.Story += src.Counters.Story - 1
		off.Log += src.Counters.Log - 1
		off.Idea += src.Counters.Idea - 1
		off.Goal += src.Counters.Goal - 1
		off.Release += src.Counters.Release - 1
		off.Phase += src.Phases
		stories += src.Counts["Story"]
		plan.Sources = append(plan.Sources, src)
	}
	plan.Target = JoinCounters{Story: 1 + off.Story, Log: 1 + off.Log + stories, Idea: 1 + off.Idea,
		Goal: 1 + off.Goal, Release: 1 + off.Release}
	plan.Notes = JoinNotes{First: 1 + off.Log, Count: stories}
	plan.Top, plan.Warnings = plan.review()
	return plan, nil
}

func checkJoinRequest(req JoinRequest) error {
	switch n := len(req.From); {
	case n < 2:
		return refuse(Invalid, "a join takes at least two workspaces")
	case n > maxJoinSources:
		return refuse(Invalid, "a join takes at most %d workspaces", maxJoinSources)
	}
	seen := map[string]bool{}
	for _, s := range req.From {
		if seen[s] {
			return refuse(Invalid, "%s is named twice", s)
		}
		seen[s] = true
	}
	if len(req.Into) > 64 || !slugPattern.MatchString(req.Into) {
		return refuse(Invalid, "slug must be 1-64 lowercase letters, digits and single hyphens")
	}
	if req.Name == "" {
		return refuse(Invalid, "name is required")
	}
	if plainText(req.Name) != nil {
		return refuse(Invalid, "name cannot contain control or invisible formatting characters")
	}
	if req.Actor == "" {
		return refuse(Invalid, "actor is required")
	}
	if plainText(req.Actor) != nil {
		return refuse(Invalid, "actor cannot contain control or invisible formatting characters")
	}
	return nil
}

type joinEntry struct{ joinedInto, joining string }

// joinEntries reads every registry entry's join marks, joined ones too.
func (b *Board) joinEntries() (map[string]joinEntry, error) {
	res, err := b.system.ROQuery("MATCH (e:WorkspaceEntry) RETURN e.slug AS slug, e.joined_into AS into, e.joining AS joining", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	out := map[string]joinEntry{}
	for res.Next() {
		r := res.Record()
		out[str(r, "slug")] = joinEntry{str(r, "into"), str(r, "joining")}
	}
	return out, nil
}

// checkJoinRegistry holds the rules the registry and the graph names decide.
// A target graph with no entry is left over from an interrupted join, and is
// only acceptable when every source is frozen to this target (a resume).
func checkJoinRegistry(req JoinRequest, entries map[string]joinEntry, hasGraph func(string) bool) error {
	if _, taken := entries[req.Into]; taken {
		return refuse(Conflict, "workspace %q already exists", req.Into)
	}
	resume := true
	for _, s := range req.From {
		e, ok := entries[s]
		switch {
		case !ok:
			return refuse(NotFound, "no workspace %q", s)
		case e.joinedInto != "":
			return refuse(Conflict, "%s was joined into %s; it cannot be joined again", s, e.joinedInto)
		case e.joining != "" && e.joining != req.Into:
			return refuse(Conflict, "%s is being joined into %s", s, e.joining)
		}
		if e.joining != req.Into {
			resume = false
		}
		if !hasGraph(s) {
			return refuse(Conflict, "%s has no graph", s)
		}
	}
	for slug, e := range entries {
		if e.joining == req.Into && !slices.Contains(req.From, slug) {
			return refuse(Conflict, "%s is already being joined into %s", slug, req.Into)
		}
	}
	if hasGraph(req.Into) && !resume {
		return refuse(Conflict, "a graph for %q already exists and no join of these workspaces left it", req.Into)
	}
	return nil
}

// readJoinSource reads one source workspace, only reading, and refuses a
// graph the join cannot move whole.
func (b *Board) readJoinSource(slug string, k int) (JoinSource, error) {
	g := b.graph(slug)
	v, err := falkor.Version(g)
	if err != nil {
		return JoinSource{}, err
	}
	if v != len(migrations) {
		return JoinSource{}, refuse(Conflict, "%s is not on the current schema (version %d, want %d); start Backpack to migrate it", slug, v, len(migrations))
	}
	nodes, edges, err := readSnapshot(g)
	if err != nil {
		return JoinSource{}, fmt.Errorf("read %s: %w", slug, err)
	}
	for _, n := range nodes {
		for _, l := range n.Labels {
			if !slices.Contains(joinLabels, l) {
				return JoinSource{}, refuse(Conflict, "%s has nodes labelled %s, which a join does not know how to move", slug, l)
			}
		}
		if len(n.Labels) == 0 {
			return JoinSource{}, refuse(Conflict, "%s has a node with no label, which a join does not know how to move", slug)
		}
	}
	for _, e := range edges {
		if !slices.Contains(joinEdges, e.Type) {
			return JoinSource{}, refuse(Conflict, "%s has %s links, which a join does not know how to move", slug, e.Type)
		}
	}
	src := JoinSource{Slug: slug, Index: k, RankPrefix: string(digits[k+1]), Counts: map[string]int{}, Nodes: nodes, Edges: edges}
	byID := map[uint64]SnapNode{}
	for _, n := range nodes {
		byID[n.ID] = n
		label := n.Labels[0]
		src.Counts[label]++
		switch label {
		case "Workspace":
			src.Counters = JoinCounters{Story: counterOf(n, "next_story"), Log: counterOf(n, "next_log"),
				Idea: counterOf(n, "next_idea"), Goal: counterOf(n, "next_goal"), Release: counterOf(n, "next_release")}
		case "Repo":
			name, _ := n.Props["name"].(string)
			url, _ := n.Props["url"].(string)
			src.Repos = append(src.Repos, Repo{Name: name, URL: url})
		case "Phase":
			if num, _ := n.Props["number"].(int64); int(num) > src.Phases {
				src.Phases = int(num)
			}
		}
		src.Mentions += mentions(n)
	}
	if src.Counts["Workspace"] == 0 {
		src.Counters = JoinCounters{1, 1, 1, 1, 1}
	}
	slices.SortFunc(src.Repos, func(a, c Repo) int { return strings.Compare(a.Name, c.Name) })
	for _, e := range edges {
		if e.Type == "HAS_BRANCH" {
			repo, _ := byID[e.From].Props["name"].(string)
			branch, _ := byID[e.To].Props["name"].(string)
			src.Branches = append(src.Branches, repo+"/"+branch)
		}
	}
	slices.Sort(src.Branches)
	return src, nil
}

func counterOf(n SnapNode, key string) int {
	if v, ok := n.Props[key].(int64); ok {
		return int(v)
	}
	return 1
}

// textFields are the properties whose text can name another item by id.
var textFields = map[string][]string{
	"Story":   {"description", "goal", "plan"},
	"Idea":    {"context"},
	"Phase":   {"description"},
	"Goal":    {"text"},
	"Release": {"notes"},
}

// mentions counts the text fields of a node that name an id.
func mentions(n SnapNode) int {
	c := 0
	for _, f := range textFields[n.Labels[0]] {
		if s, _ := n.Props[f].(string); idPattern.MatchString(s) {
			c++
		}
	}
	return c
}

func (s JoinSource) hasR0() bool {
	return slices.ContainsFunc(s.Nodes, func(n SnapNode) bool {
		return n.Labels[0] == "Release" && n.Props["id"] == "R-0"
	})
}

// readSnapshot reads a whole graph generically, so a join moves anything the
// graph holds. Nodes come back in internal-id order and edges in a fixed one.
func readSnapshot(g *falkordb.Graph) ([]SnapNode, []SnapEdge, error) {
	res, err := g.ROQuery("MATCH (n) RETURN n", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var nodes []SnapNode
	for res.Next() {
		v, _ := res.Record().GetByIndex(0)
		n, ok := v.(*falkordb.Node)
		if !ok {
			return nil, nil, fmt.Errorf("a node came back as %T", v)
		}
		nodes = append(nodes, SnapNode{ID: n.ID, Labels: n.Labels, Props: n.Properties})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	res, err = g.ROQuery("MATCH (a)-[e]->(c) RETURN id(a), e, id(c)", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var edges []SnapEdge
	for res.Next() {
		r := res.Record()
		a, _ := r.GetByIndex(0)
		v, _ := r.GetByIndex(1)
		c, _ := r.GetByIndex(2)
		e, ok := v.(*falkordb.Edge)
		af, aok := a.(int64)
		cf, cok := c.(int64)
		if !ok || !aok || !cok {
			return nil, nil, fmt.Errorf("an edge came back as %T", v)
		}
		edges = append(edges, SnapEdge{Type: e.Relation, From: uint64(af), To: uint64(cf), Props: e.Properties})
	}
	sort.Slice(edges, func(i, j int) bool {
		x, y := edges[i], edges[j]
		if x.From != y.From {
			return x.From < y.From
		}
		if x.To != y.To {
			return x.To < y.To
		}
		return x.Type < y.Type
	})
	return nodes, edges, nil
}

// Remap returns the node the target gets for a node of this source. Ids
// that count up (stories, ideas, goals, log entries, releases but R-0) and
// phase numbers move up by the source's offsets; Story.upstream_id follows
// its story; Story and Idea ranks get the source's prefix; every item that
// is renumbered keeps its old id and workspace as old_id and old_workspace.
// Everything else, text included, is copied as it is. The node it is given
// is not changed.
func (s JoinSource) Remap(n SnapNode) SnapNode {
	out := SnapNode{ID: n.ID, Labels: n.Labels, Props: make(map[string]any, len(n.Props)+2)}
	for k, v := range n.Props {
		out.Props[k] = v
	}
	p := out.Props
	renumber := func(prefix string, off int) {
		old, _ := p["id"].(string)
		p["old_id"], p["old_workspace"] = old, s.Slug
		p["id"] = shiftID(old, prefix, off)
	}
	switch n.Labels[0] {
	case "Story":
		renumber("S-", s.Offsets.Story)
		if up, ok := p["upstream_id"].(string); ok {
			p["upstream_id"] = shiftID(up, "S-", s.Offsets.Story)
		}
		s.prefixRank(p)
	case "Idea":
		renumber("I-", s.Offsets.Idea)
		s.prefixRank(p)
	case "Goal":
		renumber("G-", s.Offsets.Goal)
	case "Release":
		renumber("R-", s.Offsets.Release)
	case "LogEntry":
		old, _ := p["id"].(string)
		p["id"] = shiftID(old, "L-", s.Offsets.Log)
	case "Phase":
		num, _ := p["number"].(int64)
		p["old_id"], p["old_workspace"] = strconv.FormatInt(num, 10), s.Slug
		p["number"] = num + int64(s.Offsets.Phase)
	}
	return out
}

func (s JoinSource) prefixRank(p map[string]any) {
	if r, ok := p["rank"].(string); ok {
		p["rank"] = s.RankPrefix + r
	}
}

// shiftID moves an id such as S-4 up by off. R-0, an id of another shape and
// an id without a number are left alone.
func shiftID(id, prefix string, off int) string {
	n, ok := strings.CutPrefix(id, prefix)
	if !ok {
		return id
	}
	v, err := strconv.Atoi(n)
	if err != nil || v == 0 {
		return id
	}
	return prefix + strconv.Itoa(v+off)
}
