package board_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

func phaseLog(t *testing.T, b *board.Board, n int) []board.LogEntry {
	t.Helper()
	l, err := b.PhaseLog("demo", n)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func goalLog(t *testing.T, b *board.Board, id string) []board.LogEntry {
	t.Helper()
	l, err := b.GoalLog("demo", id)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestEditingAPhaseLogsEachChangedFieldWithItsOldAndNewValue(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "Old")
	if _, err := b.UpdatePhase("demo", "editor", 1, board.PhasePatch{Title: ptr("New")}); err != nil {
		t.Fatal(err)
	}
	l := phaseLog(t, b, 1)
	if len(l) != 1 {
		t.Fatalf("log %+v, want one entry", l)
	}
	e := l[0]
	if e.Kind != "edit" || e.Field != "title" || e.From != "Old" || e.To != "New" || e.Actor != "editor" || e.At == "" || e.ID == "" {
		t.Fatalf("entry %+v", e)
	}

	// Both fields at once: one entry for each field changed, in a row.
	if _, err := b.UpdatePhase("demo", "editor", 1, board.PhasePatch{Title: ptr("Newer"), Description: ptr("fresh")}); err != nil {
		t.Fatal(err)
	}
	l = phaseLog(t, b, 1)
	if len(l) != 3 {
		t.Fatalf("log %+v, want three entries", l)
	}
	got := map[string][2]string{l[1].Field: {l[1].From, l[1].To}, l[2].Field: {l[2].From, l[2].To}}
	if got["title"] != [2]string{"New", "Newer"} || got["description"] != [2]string{"about Old", "fresh"} {
		t.Fatalf("entries %+v", l[1:])
	}
	if l[1].ID == l[2].ID || l[1].ID == l[0].ID {
		t.Fatalf("entry ids repeat: %+v", l)
	}
}

func TestEditingAPhaseToWhatItAlreadyIsLogsNothing(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "Same")
	if _, err := b.UpdatePhase("demo", actor, 1, board.PhasePatch{Title: ptr("Same")}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.UpdatePhase("demo", actor, 1, board.PhasePatch{}); err != nil {
		t.Fatal(err)
	}
	if l := phaseLog(t, b, 1); len(l) != 0 {
		t.Fatalf("log %+v, want none", l)
	}
}

func TestARefusedPhaseEditLogsNothing(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	_, err := b.UpdatePhase("demo", actor, 1, board.PhasePatch{Title: ptr("")})
	wantKind(t, err, board.Invalid)
	_, err = b.UpdatePhase("demo", actor, 1, board.PhasePatch{Description: ptr("bad \x00 text")})
	wantKind(t, err, board.Invalid)
	if l := phaseLog(t, b, 1); len(l) != 0 {
		t.Fatalf("log %+v, want none", l)
	}
}

func TestAPhaseDescriptionCanBeClearedAndTheEditIsLogged(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	p, err := b.UpdatePhase("demo", actor, 1, board.PhasePatch{Description: ptr("")})
	if err != nil {
		t.Fatal(err)
	}
	if p.Description != "" || p.Title != "One" {
		t.Fatalf("phase %+v", p)
	}
	if l := phaseLog(t, b, 1); len(l) != 1 || l[0].Field != "description" || l[0].From != "about One" || l[0].To != "" {
		t.Fatalf("log %+v", l)
	}
}

func TestReadingTheLogOfAnUnknownPhaseOrGoalIsRefused(t *testing.T) {
	b := newWorkspace(t)
	_, err := b.PhaseLog("demo", 9)
	wantKind(t, err, board.NotFound)
	_, err = b.GoalLog("demo", "G-9")
	wantKind(t, err, board.NotFound)
}

func TestAPhaseWithoutEditsHasAnEmptyLog(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	if l := phaseLog(t, b, 1); l == nil || len(l) != 0 {
		t.Fatalf("log %#v, want empty list", l)
	}
}

func TestEditingAGoalChangesItsTextKeepsItsMetStateAndLogsTheChange(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addGoal(t, b, 1, "old wording")
	if _, err := b.SetGoalMet("demo", actor, "G-1", true); err != nil {
		t.Fatal(err)
	}
	g, err := b.UpdateGoal("demo", "editor", "G-1", "new wording")
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != "G-1" || g.Text != "new wording" || !g.Met || g.MetBy != actor || g.CreatedBy != actor {
		t.Fatalf("goal %+v", g)
	}
	ps, _ := b.Phases("demo")
	if ps[0].Goals[0].Text != "new wording" {
		t.Fatalf("read back %+v", ps[0].Goals)
	}
	l := goalLog(t, b, "G-1")
	if len(l) != 1 {
		t.Fatalf("log %+v, want one entry", l)
	}
	if e := l[0]; e.Kind != "edit" || e.Field != "text" || e.From != "old wording" || e.To != "new wording" || e.Actor != "editor" {
		t.Fatalf("entry %+v", e)
	}
	// Another goal's log and the phase's own log are untouched.
	if len(phaseLog(t, b, 1)) != 0 {
		t.Fatal("the phase log picked up a goal edit")
	}
}

func TestEditingAGoalToItsCurrentTextLogsNothing(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addGoal(t, b, 1, "same")
	if _, err := b.UpdateGoal("demo", actor, "G-1", "same"); err != nil {
		t.Fatal(err)
	}
	if l := goalLog(t, b, "G-1"); len(l) != 0 {
		t.Fatalf("log %+v", l)
	}
}

func TestEditingAGoalRefusesEmptyTextAndAnUnknownGoal(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addGoal(t, b, 1, "keep")
	_, err := b.UpdateGoal("demo", actor, "G-1", "")
	wantKind(t, err, board.Invalid)
	_, err = b.UpdateGoal("demo", actor, "G-1", "bad \x00 text")
	wantKind(t, err, board.Invalid)
	_, err = b.UpdateGoal("demo", actor, "G-9", "x")
	wantKind(t, err, board.NotFound)
	if l := goalLog(t, b, "G-1"); len(l) != 0 {
		t.Fatalf("log %+v", l)
	}
	ps, _ := b.Phases("demo")
	if ps[0].Goals[0].Text != "keep" {
		t.Fatalf("goal %+v", ps[0].Goals[0])
	}
}

func TestPhaseAndGoalEditsShareTheLogIdsOfStories(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addGoal(t, b, 1, "g")
	if _, err := b.AddStory("demo", actor, board.NewStory{Title: "S"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.UpdateGoal("demo", actor, "G-1", "g2"); err != nil {
		t.Fatal(err)
	}
	e, err := b.AddLogEntry("demo", actor, "S-1", board.NewLogEntry{Kind: "note", Text: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if g := goalLog(t, b, "G-1"); len(g) != 1 || g[0].ID == e.ID {
		t.Fatalf("goal log %+v, story entry %+v", g, e)
	}
}

func TestEditingReportsThePhaseThatChanged(t *testing.T) {
	b := newWorkspace(t)
	addPhase(t, b, "One")
	addPhase(t, b, "Two")
	addGoal(t, b, 2, "g")
	got := recorder(b)
	if _, err := b.UpdateGoal("demo", actor, "G-1", "g2"); err != nil {
		t.Fatal(err)
	}
	want := board.Change{Workspace: "demo", Kind: board.KindPhase, ID: "2"}
	if len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("changes %+v, want %+v", *got, want)
	}
}
