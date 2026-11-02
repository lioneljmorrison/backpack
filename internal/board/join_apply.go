package board

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/FalkorDB/falkordb-go/v2"
	"github.com/lioneljmorrison/backpack/internal/falkor"
)

// Applying a join (Phase 6). Moving work between graphs cannot be one query,
// so the join builds the new workspace's graph where nobody can reach it (it
// has no registry entry), reads it back and checks it, and only then makes it
// visible with one registry query that also marks the sources joined. Every
// change anyone can see is one query: the freeze, the switch and the unfreeze
// after a failure. Until the switch the sources are exactly as they were, apart
// from the freeze mark that makes them refuse writes.

// ErrJoinInterrupted is what a JoinHooks.After returns to stop a join dead, as
// a crash would: nothing is cleaned up. It exists for tests.
var ErrJoinInterrupted = errors.New("join interrupted")

// JoinHooks lets a test look into a join. After is called when a stage is
// done: "frozen", "prepared", "written" and "verified". An error from it
// fails the join (and is cleaned up after) unless it is ErrJoinInterrupted.
type JoinHooks struct {
	After func(stage string) error
}

func (h JoinHooks) stage(name string) error {
	if h.After == nil {
		return nil
	}
	return h.After(name)
}

// ApplyJoin joins the request's sources into the new workspace. Any failure
// before the switch leaves the sources as they were; a join interrupted
// mid-way (the process killed) is finished by running it again, or undone by
// AbortJoin. It answers with the plan it applied.
func (b *Board) ApplyJoin(req JoinRequest, hooks JoinHooks) (_ JoinPlan, err error) {
	defer func() { b.changed(err, Change{Workspace: req.Into, Kind: KindWorkspace}) }()
	// A preview first: it holds every refusal that needs no write.
	if _, err := b.PreviewJoin(req); err != nil {
		return JoinPlan{}, err
	}
	entries, err := b.joinEntries()
	if err != nil {
		return JoinPlan{}, err
	}
	resuming := true
	for _, s := range req.From {
		if entries[s].joining != req.Into {
			resuming = false
		}
	}

	ok, err := b.freeze(req)
	if err != nil {
		return JoinPlan{}, err
	}
	if !ok {
		return JoinPlan{}, refuse(Conflict, "the workspaces changed while the join was starting; nothing was changed, try again")
	}
	built := false
	defer func() {
		if err == nil || errors.Is(err, ErrJoinInterrupted) {
			return
		}
		err = b.failJoin(req.Into, built, err)
	}()
	if err := hooks.stage("frozen"); err != nil {
		return JoinPlan{}, err
	}

	// Frozen, the sources cannot change: read them for good.
	plan, err := b.PreviewJoin(req)
	if err != nil {
		return JoinPlan{}, err
	}

	g := b.graph(req.Into)
	exists, err := b.graphExists(req.Into)
	if err != nil {
		return JoinPlan{}, err
	}
	if exists {
		if !resuming {
			return JoinPlan{}, refuse(Conflict, "a graph for %q already exists and no join of these workspaces left it", req.Into)
		}
		// Left by this join, interrupted: unregistered, so nobody reached it.
		if err := g.Delete(); err != nil {
			return JoinPlan{}, fmt.Errorf("delete the half-built workspace: %w", err)
		}
	}
	built = true
	if err := falkor.Migrate(g, migrations, repoParams(req.Into, "")); err != nil {
		return JoinPlan{}, err
	}
	if err := hooks.stage("prepared"); err != nil {
		return JoinPlan{}, err
	}

	at := now()
	bld := buildJoin(plan, at)
	ids, err := bld.write(g)
	if err != nil {
		return JoinPlan{}, err
	}
	if err := hooks.stage("written"); err != nil {
		return JoinPlan{}, err
	}
	if err := b.verifyJoin(g, plan, bld, ids); err != nil {
		return JoinPlan{}, err
	}
	if err := hooks.stage("verified"); err != nil {
		return JoinPlan{}, err
	}

	release, err := hasReleaseStage(g)
	if err != nil {
		return JoinPlan{}, err
	}
	switched, err := b.switchOver(plan, release, at)
	if err != nil {
		return JoinPlan{}, err
	}
	if !switched {
		return JoinPlan{}, refuse(Conflict, "the workspaces changed during the join; nothing was changed, try again")
	}
	built = false // the new workspace is the board's now
	return plan, nil
}

// failJoin undoes a failed join: it deletes the graph the join built, then
// unfreezes the sources. When the graph cannot be deleted the sources stay
// frozen, so that running the join again or aborting it can finish the clean-up.
func (b *Board) failJoin(into string, built bool, cause error) error {
	if built {
		if err := b.graph(into).Delete(); err != nil {
			return fmt.Errorf("%w (and the half-built workspace could not be deleted: %v; the sources stay frozen, run the join again or abort it)", cause, err)
		}
	}
	if _, err := b.unfreeze(into); err != nil {
		return fmt.Errorf("%w (and the sources could not be unfrozen: %v; abort the join)", cause, err)
	}
	return cause
}

// AbortJoin undoes an interrupted join into the workspace: it deletes the
// half-built graph and unfreezes the sources. It refuses a workspace that is
// registered, and one that no join froze sources for, so it never deletes a
// graph it cannot show to be a join's own.
func (b *Board) AbortJoin(into string) error {
	if len(into) > 64 || !slugPattern.MatchString(into) {
		return refuse(Invalid, "slug must be 1-64 lowercase letters, digits and single hyphens")
	}
	entries, err := b.joinEntries()
	if err != nil {
		return err
	}
	if _, taken := entries[into]; taken {
		return refuse(Conflict, "workspace %q already exists: its join is finished, or the name is taken; there is nothing to abort", into)
	}
	frozen := false
	for _, e := range entries {
		if e.joining == into {
			frozen = true
		}
	}
	if !frozen {
		return refuse(NotFound, "no join into %q is under way", into)
	}
	// While the sources are frozen nobody can register the name, so the graph
	// is deleted first and the unfreeze, which lets anyone use the name, last.
	exists, err := b.graphExists(into)
	if err != nil {
		return err
	}
	if exists {
		if err := b.graph(into).Delete(); err != nil {
			return fmt.Errorf("delete the half-built workspace: %w", err)
		}
	}
	_, err = b.unfreeze(into)
	return err
}

func (b *Board) graphExists(slug string) (bool, error) {
	names, err := b.db.ListGraphs()
	if err != nil {
		return false, fmt.Errorf("list graphs: %w", err)
	}
	return slices.Contains(names, b.prefix+slug), nil
}

// freeze marks every source as being joined into the target, in one query
// that checks its rules first: every source exists and was not joined, none is
// being joined into another target, the target has no entry, and no other
// workspace is being joined into it. It answers false, having written nothing,
// when a rule does not hold. Freezing again to the same target is the resume.
func (b *Board) freeze(req JoinRequest) (bool, error) {
	res, err := b.system.Query(`
		OPTIONAL MATCH (t:WorkspaceEntry) WHERE t.slug = $target
		WITH count(t) AS taken
		MATCH (e:WorkspaceEntry) WHERE e.slug IN $from
		WITH taken, collect(e) AS es
		WITH taken, es, size([e IN es WHERE e.joined_into IS NULL AND (e.joining IS NULL OR e.joining = $target)]) AS free
		OPTIONAL MATCH (o:WorkspaceEntry) WHERE o.joining = $target AND NOT o.slug IN $from
		WITH taken, es, free, count(o) AS others
		WITH es, taken = 0 AND others = 0 AND size(es) = size($from) AND free = size($from) AS go
		FOREACH (e IN CASE WHEN go THEN es ELSE [] END | SET e.joining = $target)
		RETURN go`,
		map[string]any{"target": req.Into, "from": toStrings(req.From)}, nil)
	if err != nil {
		return false, fmt.Errorf("freeze the sources: %w", err)
	}
	return answeredTrue(res), nil
}

// unfreeze lifts the freeze of a join that failed or was aborted, in one query
// that refuses to touch anything once the target has an entry. It answers how
// many sources it unfroze.
func (b *Board) unfreeze(into string) (int, error) {
	res, err := b.system.Query(`
		OPTIONAL MATCH (t:WorkspaceEntry) WHERE t.slug = $target
		WITH count(t) AS taken
		MATCH (e:WorkspaceEntry) WHERE e.joining = $target AND taken = 0
		SET e.joining = NULL
		RETURN count(e)`, map[string]any{"target": into}, nil)
	if err != nil {
		return 0, fmt.Errorf("unfreeze the sources: %w", err)
	}
	if !res.Next() {
		return 0, nil
	}
	v, _ := res.Record().GetByIndex(0)
	n, _ := v.(int64)
	return int(n), nil
}

// switchOver makes the new workspace visible: in one query that checks its
// rules first (the target has no entry and every source is frozen to it) it
// registers the target and marks each source joined into it, with the
// repositories that went there, and lifts the freeze. It answers false,
// having written nothing, when a rule does not hold.
func (b *Board) switchOver(plan JoinPlan, release bool, at string) (bool, error) {
	req := plan.Request
	var sources []any
	for _, s := range plan.Sources {
		repos := make([]any, len(s.Repos))
		for i, r := range s.Repos {
			repos[i] = r.Name
		}
		sources = append(sources, map[string]any{"slug": s.Slug, "repos": repos})
	}
	res, err := b.system.Query(`
		OPTIONAL MATCH (t:WorkspaceEntry) WHERE t.slug = $target
		WITH count(t) AS taken
		MATCH (e:WorkspaceEntry) WHERE e.slug IN $from
		WITH taken, collect(e) AS es
		WITH taken, es, size([e IN es WHERE e.joining = $target AND e.joined_into IS NULL]) AS ready
		WITH es, taken = 0 AND size(es) = size($from) AND ready = size($from) AS go
		FOREACH (_ IN CASE WHEN go THEN [1] ELSE [] END |
			CREATE (:WorkspaceEntry {slug: $target, name: $name, repo_url: '',
				has_release_stage: $release, created_by: $actor, created_at: $at}))
		FOREACH (e IN CASE WHEN go THEN es ELSE [] END |
			SET e.joined_into = $target, e.joined_repos = [x IN $sources WHERE x.slug = e.slug][0].repos,
				e.joined_at = $at, e.joined_by = $actor, e.joining = NULL)
		RETURN go`,
		map[string]any{"target": req.Into, "from": toStrings(req.From), "name": req.Name, "release": release,
			"actor": req.Actor, "at": at, "sources": sources}, nil)
	if err != nil {
		return false, fmt.Errorf("switch to the new workspace: %w", err)
	}
	return answeredTrue(res), nil
}

func answeredTrue(res *falkordb.QueryResult) bool {
	if !res.Next() {
		return false
	}
	v, _ := res.Record().GetByIndex(0)
	return v == true
}

func toStrings(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// joinBuild is the new workspace's graph as the join means to write it: its
// nodes and edges, the edges naming nodes by their place in Nodes.
type joinBuild struct {
	Nodes []SnapNode
	Edges []SnapEdge
}

// buildJoin works out, in memory, every node and edge the new workspace gets:
// each source's nodes remapped (the Workspace nodes collapsed into one, the
// SchemaVersion nodes left to the migrations), its edges, and one join note per
// story.
func buildJoin(plan JoinPlan, at string) joinBuild {
	var bld joinBuild
	into := plan.Request.Into
	ws := map[string]any{}
	for _, n := range plan.Sources[0].Nodes {
		if n.Labels[0] == "Workspace" {
			for k, v := range n.Props {
				ws[k] = v
			}
		}
	}
	t := plan.Target
	ws["slug"] = into
	ws["next_story"], ws["next_log"], ws["next_idea"] = int64(t.Story), int64(t.Log), int64(t.Idea)
	ws["next_goal"], ws["next_release"] = int64(t.Goal), int64(t.Release)
	bld.Nodes = append(bld.Nodes, SnapNode{ID: 0, Labels: []string{"Workspace"}, Props: ws})

	type story struct {
		index  int
		number int
		src    string
		oldID  string
	}
	var stories []story
	for _, s := range plan.Sources {
		place := map[uint64]uint64{}
		for _, n := range s.Nodes {
			switch n.Labels[0] {
			case "Workspace":
				place[n.ID] = 0
				continue
			case "SchemaVersion":
				continue
			}
			m := s.Remap(n)
			idx := uint64(len(bld.Nodes))
			place[n.ID] = idx
			m.ID = idx
			bld.Nodes = append(bld.Nodes, m)
			if m.Labels[0] == "Story" {
				id, _ := m.Props["id"].(string)
				num, _ := strconv.Atoi(strings.TrimPrefix(id, "S-"))
				old, _ := m.Props["old_id"].(string)
				stories = append(stories, story{int(idx), num, s.Slug, old})
			}
		}
		for _, e := range s.Edges {
			from, fok := place[e.From]
			to, tok := place[e.To]
			if fok && tok {
				bld.Edges = append(bld.Edges, SnapEdge{Type: e.Type, From: from, To: to, Props: e.Props})
			}
		}
	}
	// One note per story, ids from the block the target reserved, in the order
	// of the new story ids.
	sort.SliceStable(stories, func(i, j int) bool { return stories[i].number < stories[j].number })
	for k, s := range stories {
		idx := uint64(len(bld.Nodes))
		bld.Nodes = append(bld.Nodes, SnapNode{ID: idx, Labels: []string{"LogEntry"}, Props: map[string]any{
			"id": "L-" + strconv.Itoa(plan.Notes.First+k), "at": at, "actor": plan.Request.Actor, "kind": "note",
			"text": fmt.Sprintf("Joined into %s from %s, where this was %s. Ids in earlier entries are %s's.", into, s.src, s.oldID, s.src),
		}})
		bld.Edges = append(bld.Edges, SnapEdge{Type: "LOGGED", From: uint64(s.index), To: idx})
	}
	return bld
}

// write creates the build in the target graph, nodes first then edges, and
// answers the internal id each node got, by its place in Nodes.
func (bld joinBuild) write(g *falkordb.Graph) (ids map[uint64]int64, err error) {
	// The client panics on a value it cannot send; that is an error here.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("write the new workspace: %v", r)
		}
	}()
	var order []string
	byLabels := map[string][]map[string]any{}
	for i, n := range bld.Nodes {
		key := strings.Join(n.Labels, ":")
		if _, seen := byLabels[key]; !seen {
			order = append(order, key)
		}
		byLabels[key] = append(byLabels[key], map[string]any{"k": int64(i), "p": propsOrEmpty(n.Props)})
	}
	ids = make(map[uint64]int64, len(bld.Nodes))
	for _, key := range order {
		got, err := writeNodes(g, key, byLabels[key])
		if err != nil {
			return nil, err
		}
		for k, id := range got {
			ids[uint64(k)] = id
		}
	}
	var types []string
	byType := map[string][]map[string]any{}
	for _, e := range bld.Edges {
		if _, seen := byType[e.Type]; !seen {
			types = append(types, e.Type)
		}
		byType[e.Type] = append(byType[e.Type], map[string]any{"a": ids[e.From], "b": ids[e.To], "p": propsOrEmpty(e.Props)})
	}
	for _, typ := range types {
		if err := writeEdges(g, typ, byType[typ]); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func propsOrEmpty(p map[string]any) map[string]any {
	if p == nil {
		return map[string]any{}
	}
	return p
}

// verifyJoin reads the new graph back and holds it against what the join meant
// to write, then checks the board's rules and that no source changed since it
// was read.
func (b *Board) verifyJoin(g *falkordb.Graph, plan JoinPlan, bld joinBuild, ids map[uint64]int64) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("verification failed: "+format, args...)
	}
	nodes, edges, err := readSnapshot(g)
	if err != nil {
		return fmt.Errorf("verification failed: read back: %w", err)
	}
	got := map[uint64]SnapNode{}
	schema := 0
	for _, n := range nodes {
		if n.Labels[0] == "SchemaVersion" {
			schema++
			if v, _ := n.Props["v"].(int64); int(v) != len(migrations) {
				return fail("the schema version is %d, want %d", v, len(migrations))
			}
			continue
		}
		got[n.ID] = n
	}
	if schema != 1 || len(got) != len(bld.Nodes) {
		return fail("the new workspace has %d nodes and %d schema marks, want %d and 1", len(got), schema, len(bld.Nodes))
	}
	for i, want := range bld.Nodes {
		n, ok := got[uint64(ids[uint64(i)])]
		if !ok {
			return fail("%s node %d is missing", want.Labels[0], i)
		}
		wl, gl := slices.Clone(want.Labels), slices.Clone(n.Labels)
		slices.Sort(wl)
		slices.Sort(gl)
		if !slices.Equal(wl, gl) || !reflect.DeepEqual(propsOrEmpty(want.Props), propsOrEmpty(n.Props)) {
			return fail("%s node %v read back as %v", want.Labels[0], want.Props["id"], n.Props)
		}
	}
	key := func(e SnapEdge, from, to uint64) string {
		return fmt.Sprintf("%s|%d|%d|%#v", e.Type, from, to, propsOrEmpty(e.Props))
	}
	left := map[string]int{}
	for _, e := range bld.Edges {
		left[key(e, uint64(ids[e.From]), uint64(ids[e.To]))]++
	}
	if len(edges) != len(bld.Edges) {
		return fail("the new workspace has %d links, want %d", len(edges), len(bld.Edges))
	}
	for _, e := range edges {
		k := key(e, e.From, e.To)
		if left[k] == 0 {
			return fail("a %s link was read back that the join did not write", e.Type)
		}
		left[k]--
	}

	if err := checkJoinRules(g); err != nil {
		return err
	}
	if err := checkCounters(nodes); err != nil {
		return err
	}
	for _, s := range plan.Sources {
		sn, se, err := readSnapshot(b.graph(s.Slug))
		if err != nil {
			return fmt.Errorf("verification failed: read %s again: %w", s.Slug, err)
		}
		if !reflect.DeepEqual(sn, s.Nodes) || !reflect.DeepEqual(se, s.Edges) {
			return fail("%s changed while it was being joined", s.Slug)
		}
	}
	return nil
}

// checkJoinRules holds the board's rules against the new graph, in one read
// query: no dependency ranked above its dependent, and every story, branch and
// release held by exactly one repository.
func checkJoinRules(g *falkordb.Graph) error {
	res, err := g.ROQuery(`
		OPTIONAL MATCH (d:Story)-[:DEPENDS_ON]->(t:Story) WHERE t.rank >= d.rank
		WITH count(d) AS deps
		OPTIONAL MATCH (s:Story) OPTIONAL MATCH (:Repo)-[h:HOLDS]->(s)
		WITH deps, s, count(h) AS c
		WITH deps, count(CASE WHEN s IS NOT NULL AND c <> 1 THEN 1 END) AS stories
		OPTIONAL MATCH (x:Branch) OPTIONAL MATCH (:Repo)-[h:HAS_BRANCH]->(x)
		WITH deps, stories, x, count(h) AS c
		WITH deps, stories, count(CASE WHEN x IS NOT NULL AND c <> 1 THEN 1 END) AS branches
		OPTIONAL MATCH (y:Release) OPTIONAL MATCH (:Repo)-[h:HAS_RELEASE]->(y)
		WITH deps, stories, branches, y, count(h) AS c
		RETURN deps, stories, branches, count(CASE WHEN y IS NOT NULL AND c <> 1 THEN 1 END) AS releases`, nil, nil)
	if err != nil {
		return fmt.Errorf("verification failed: check the rules: %w", err)
	}
	if !res.Next() {
		return errors.New("verification failed: the rule check answered nothing")
	}
	r := res.Record()
	if n := num(r, "deps"); n > 0 {
		return fmt.Errorf("verification failed: %d dependencies are ranked above their dependents", n)
	}
	if n := num(r, "stories"); n > 0 {
		return fmt.Errorf("verification failed: %d stories are not held by exactly one repository", n)
	}
	if n := num(r, "branches"); n > 0 {
		return fmt.Errorf("verification failed: %d branches are not in exactly one repository", n)
	}
	if n := num(r, "releases"); n > 0 {
		return fmt.Errorf("verification failed: %d releases are not in exactly one repository", n)
	}
	return nil
}

// checkCounters holds that each of the new workspace's counters is above the
// highest id of its kind.
func checkCounters(nodes []SnapNode) error {
	high := map[string]int{}
	var ws SnapNode
	for _, n := range nodes {
		l := n.Labels[0]
		if l == "Workspace" {
			ws = n
			continue
		}
		if id, ok := n.Props["id"].(string); ok && l != "Repo" && l != "Branch" {
			if _, num, found := strings.Cut(id, "-"); found {
				if v, err := strconv.Atoi(num); err == nil {
					high[l] = max(high[l], v)
				}
			}
		}
	}
	for label, counter := range map[string]string{"Story": "next_story", "LogEntry": "next_log", "Idea": "next_idea",
		"Goal": "next_goal", "Release": "next_release"} {
		if c := counterOf(ws, counter); c <= high[label] {
			return fmt.Errorf("verification failed: %s is %d but a %s has id %d", counter, c, label, high[label])
		}
	}
	return nil
}
