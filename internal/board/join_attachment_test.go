package board_test

import (
	"io"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// A join carries each source's attachments, renumbered like every other id,
// and keeps pointing at the same files: the store is shared by all
// workspaces, so no bytes move.
func TestAJoinCarriesAttachmentsAndRenumbersThem(t *testing.T) {
	e := joinApplyFixture(t)
	withFiles(t, e.b)
	// demo-api: two attachments (A-1, A-2); demo-web: one on a story and one on its phase.
	if _, err := e.b.AddAttachment("demo-api", actor, board.KindStory, "S-1", "api-shot.png", pngBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.AddAttachment("demo-api", actor, board.KindIdea, "I-2", "api-notes.md", mdBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.AddAttachment("demo-web", actor, board.KindStory, "S-1", "web-shot.gif", gifBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.AddAttachment("demo-web", actor, board.KindPhase, "1", "web-phase.jpg", jpegBytes); err != nil {
		t.Fatal(err)
	}

	plan, err := e.b.PreviewJoin(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Sources[1].Offsets.Attachment != 2 || plan.Target.Attachment != 5 {
		t.Fatalf("offsets %+v, target %+v", plan.Sources[1].Offsets, plan.Target)
	}
	if _, err := e.b.ApplyJoin(joinReq, board.JoinHooks{}); err != nil {
		t.Fatal(err)
	}

	s1, _ := e.b.Story("demo", "S-1")
	if len(s1.Attachments) != 1 || s1.Attachments[0].ID != "A-1" || s1.Attachments[0].Name != "api-shot.png" {
		t.Fatalf("S-1 %+v", s1.Attachments)
	}
	s7, _ := e.b.Story("demo", "S-7") // demo-web's S-1
	if len(s7.Attachments) != 1 || s7.Attachments[0].ID != "A-3" || s7.Attachments[0].Name != "web-shot.gif" {
		t.Fatalf("S-7 %+v", s7.Attachments)
	}
	p2, _ := e.b.Phase("demo", 2) // demo-web's phase 1
	if len(p2.Attachments) != 1 || p2.Attachments[0].ID != "A-4" || p2.Attachments[0].Type != "image/jpeg" {
		t.Fatalf("phase 2 %+v", p2.Attachments)
	}
	if i, _ := e.b.Idea("demo", "I-2"); len(i.Attachments) != 1 || i.Attachments[0].ID != "A-2" {
		t.Fatalf("I-2 %+v", i.Attachments)
	}
	// The bytes are the same files.
	_, f, err := e.b.OpenAttachment("demo", "A-3")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got, _ := io.ReadAll(f); string(got) != string(gifBytes) {
		t.Fatalf("A-3 holds %q", got)
	}
	// Counters carry on.
	n, err := e.b.AddAttachment("demo", actor, board.KindStory, "S-7", "later.png", pngBytes)
	if err != nil || n.ID != "A-5" {
		t.Fatalf("next attachment %+v %v", n, err)
	}
}

func TestAJoinOfWorkspacesWithoutAttachmentsStartsTheCounterAtOne(t *testing.T) {
	e := joinApplyFixture(t)
	withFiles(t, e.b)
	if _, err := e.b.ApplyJoin(joinReq, board.JoinHooks{}); err != nil {
		t.Fatal(err)
	}
	n, err := e.b.AddAttachment("demo", actor, board.KindStory, "S-1", "first.png", pngBytes)
	if err != nil || n.ID != "A-1" {
		t.Fatalf("first attachment %+v %v", n, err)
	}
}
