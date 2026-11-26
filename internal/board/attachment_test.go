package board_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/files"
)

// Invented file contents: only the leading bytes matter to type detection.
var (
	pngBytes  = []byte("\x89PNG\r\n\x1a\n invented pixels")
	jpegBytes = []byte("\xff\xd8\xff\xe0 invented pixels")
	gifBytes  = []byte("GIF89a invented pixels")
	webpBytes = []byte("RIFF\x00\x00\x00\x00WEBPVP8 invented pixels")
	mdBytes   = []byte("# Invented notes\n\nSome *markdown*.\n")
)

// withFiles gives the board a files directory and returns it.
func withFiles(t *testing.T, b *board.Board) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "files")
	s, err := files.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	b.SetFiles(s)
	return dir
}

func fileCount(t *testing.T, dir string) int {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(es)
}

func TestAStoryCarriesAttachmentsAndLogsThem(t *testing.T) {
	b := newWorkspace(t)
	dir := withFiles(t, b)
	var changes []board.Change
	b.OnChange(func(c board.Change) { changes = append(changes, c) })
	s := addStory(t, b, board.NewStory{Title: "S"})
	changes = nil

	a, err := b.AddAttachment("demo", "alice", board.KindStory, s.ID, "shot.png", pngBytes)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "A-1" || a.Name != "shot.png" || a.Type != "image/png" || a.Size != len(pngBytes) || a.Actor != "alice" || a.At == "" {
		t.Fatalf("added %+v", a)
	}
	a2, err := b.AddAttachment("demo", "alice", board.KindStory, s.ID, "notes.md", mdBytes)
	if err != nil || a2.ID != "A-2" || a2.Type != "text/markdown; charset=utf-8" {
		t.Fatalf("second %+v %v", a2, err)
	}
	got, _ := b.Story("demo", s.ID)
	want := []board.AttachmentRef{
		{ID: "A-1", Name: "shot.png", Type: "image/png", Size: len(pngBytes)},
		{ID: "A-2", Name: "notes.md", Type: "text/markdown; charset=utf-8", Size: len(mdBytes)},
	}
	if len(got.Attachments) != 2 || got.Attachments[0] != want[0] || got.Attachments[1] != want[1] {
		t.Fatalf("story attachments %+v, want %+v", got.Attachments, want)
	}
	l, _ := b.Log("demo", s.ID)
	last := l[len(l)-1]
	if last.Kind != "note" || last.Actor != "alice" || !strings.Contains(last.Text, "notes.md") {
		t.Fatalf("log %+v", l)
	}
	if fileCount(t, dir) != 2 {
		t.Fatalf("files on disk: %d", fileCount(t, dir))
	}
	if len(changes) != 2 || changes[0] != (board.Change{Workspace: "demo", Kind: board.KindStory, ID: s.ID}) {
		t.Fatalf("changes %+v", changes)
	}
	// The list of stories carries them too.
	all, _ := b.Stories("demo")
	if len(all[0].Attachments) != 2 {
		t.Fatalf("listed %+v", all[0])
	}
}

func TestAnIdeaAndAPhaseCarryAttachments(t *testing.T) {
	b := newWorkspace(t)
	withFiles(t, b)
	i := addIdea(t, b, board.NewIdea{Title: "I"})
	p := addPhase(t, b, "P")
	if _, err := b.AddAttachment("demo", actor, board.KindIdea, i.ID, "a.gif", gifBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddAttachment("demo", actor, board.KindPhase, "1", "b.jpg", jpegBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddAttachment("demo", actor, board.KindPhase, "1", "c.webp", webpBytes); err != nil {
		t.Fatal(err)
	}
	gi, _ := b.Idea("demo", i.ID)
	if len(gi.Attachments) != 1 || gi.Attachments[0].Type != "image/gif" {
		t.Fatalf("idea %+v", gi.Attachments)
	}
	gp, _ := b.Phase("demo", p.Number)
	if len(gp.Attachments) != 2 || gp.Attachments[0].ID != "A-2" || gp.Attachments[1].Type != "image/webp" {
		t.Fatalf("phase %+v", gp.Attachments)
	}
	if l, _ := b.PhaseLog("demo", 1); len(l) != 2 {
		t.Fatalf("phase log %+v", l)
	}
	ps, _ := b.Phases("demo")
	if len(ps[0].Attachments) != 2 {
		t.Fatalf("listed phase %+v", ps[0])
	}
	is, _ := b.Ideas("demo")
	if len(is[0].Attachments) != 1 {
		t.Fatalf("listed idea %+v", is[0])
	}
}

func TestContextListsTheStorysAttachments(t *testing.T) {
	b := newWorkspace(t)
	withFiles(t, b)
	s := addStory(t, b, board.NewStory{Title: "S"})
	if c, _ := b.Context("demo", s.ID); len(c.Attachments) != 0 {
		t.Fatalf("none yet: %+v", c.Attachments)
	}
	b.AddAttachment("demo", actor, board.KindStory, s.ID, "a.png", pngBytes)
	c, err := b.Context("demo", s.ID)
	if err != nil || len(c.Attachments) != 1 || c.Attachments[0].Name != "a.png" {
		t.Fatalf("context %+v %v", c.Attachments, err)
	}
}

func TestOnlyImagesAndMarkdownAreAccepted(t *testing.T) {
	b := newWorkspace(t)
	dir := withFiles(t, b)
	s := addStory(t, b, board.NewStory{Title: "S"})
	cases := []struct {
		name string
		data []byte
		kind board.Kind
	}{
		{"logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), board.Unsupported},
		{"page.png", []byte("<!DOCTYPE html><html><script>x</script></html>"), board.Unsupported},
		{"data.bin", []byte("\x00\x01\x02\x03"), board.Unsupported},
		{"a.pdf", []byte("%PDF-1.4 invented"), board.Unsupported},
		{"notes.txt", mdBytes, board.Unsupported},
		{"bad.md", []byte("caf\xe9 not utf-8"), board.Unsupported},
		{"ctl.md", []byte("bell \x07 character"), board.Unsupported},
		{"pic.md", pngBytes, board.Unsupported},
	}
	for _, c := range cases {
		_, err := b.AddAttachment("demo", actor, board.KindStory, s.ID, c.name, c.data)
		wantKind(t, err, c.kind)
	}
	if fileCount(t, dir) != 0 {
		t.Fatalf("a refused file was stored")
	}
	if _, err := b.AddAttachment("demo", actor, board.KindStory, s.ID, "UPPER.MD", mdBytes); err != nil {
		t.Fatalf("the .md name is case-insensitive: %v", err)
	}
}

func TestAnAttachmentNameIsPlainAndShort(t *testing.T) {
	b := newWorkspace(t)
	withFiles(t, b)
	s := addStory(t, b, board.NewStory{Title: "S"})
	for _, name := range []string{"", "a/b.png", `a\b.png`, "a\nb.png", "a\x00.png", strings.Repeat("x", 256) + ".png"} {
		_, err := b.AddAttachment("demo", actor, board.KindStory, s.ID, name, pngBytes)
		wantKind(t, err, board.Invalid)
	}
	if _, err := b.AddAttachment("demo", actor, board.KindStory, s.ID, "ünïcode name.png", pngBytes); err != nil {
		t.Fatalf("unicode name: %v", err)
	}
}

func TestAnAttachmentOverTheLimitIsRefused(t *testing.T) {
	b := newWorkspace(t)
	dir := withFiles(t, b)
	s := addStory(t, b, board.NewStory{Title: "S"})
	big := append(append([]byte{}, pngBytes...), make([]byte, board.MaxAttachment)...)
	_, err := b.AddAttachment("demo", actor, board.KindStory, s.ID, "big.png", big)
	wantKind(t, err, board.TooLarge)
	exact := append(append([]byte{}, pngBytes...), make([]byte, board.MaxAttachment-len(pngBytes))...)
	if _, err := b.AddAttachment("demo", actor, board.KindStory, s.ID, "exact.png", exact); err != nil {
		t.Fatalf("exactly the limit: %v", err)
	}
	if fileCount(t, dir) != 1 {
		t.Fatalf("files: %d", fileCount(t, dir))
	}
}

func TestAttachingToSomethingMissingIsRefusedAndLeavesNoFile(t *testing.T) {
	b := newWorkspace(t)
	dir := withFiles(t, b)
	for _, c := range []struct{ kind, id string }{{board.KindStory, "S-9"}, {board.KindIdea, "I-9"}, {board.KindPhase, "9"}, {board.KindPhase, "x"}, {board.KindWorkspace, "1"}} {
		_, err := b.AddAttachment("demo", actor, c.kind, c.id, "a.png", pngBytes)
		if c.kind == board.KindWorkspace || c.id == "x" {
			wantKind(t, err, board.Invalid)
		} else {
			wantKind(t, err, board.NotFound)
		}
	}
	_, err := b.AddAttachment("nope", actor, board.KindStory, "S-1", "a.png", pngBytes)
	wantKind(t, err, board.NotFound)
	if fileCount(t, dir) != 0 {
		t.Fatalf("leftover files: %d", fileCount(t, dir))
	}
}

func TestOpeningAnAttachmentGivesItsBytesInItsWorkspaceOnly(t *testing.T) {
	b := newWorkspace(t)
	withFiles(t, b)
	if _, err := b.CreateWorkspace(actor, board.Workspace{Slug: "other", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	s := addStory(t, b, board.NewStory{Title: "S"})
	a, _ := b.AddAttachment("demo", actor, board.KindStory, s.ID, "notes.md", mdBytes)
	got, f, err := b.OpenAttachment("demo", a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if body, _ := io.ReadAll(f); string(body) != string(mdBytes) || got.Name != "notes.md" || got.Type != a.Type {
		t.Fatalf("opened %+v %q", got, body)
	}
	_, _, err = b.OpenAttachment("other", a.ID)
	wantKind(t, err, board.NotFound)
	_, _, err = b.OpenAttachment("demo", "A-99")
	wantKind(t, err, board.NotFound)
}

func TestRemovingAnAttachmentLogsItAndDeletesTheBytes(t *testing.T) {
	b := newWorkspace(t)
	dir := withFiles(t, b)
	var changes []board.Change
	s := addStory(t, b, board.NewStory{Title: "S"})
	i := addIdea(t, b, board.NewIdea{Title: "I"})
	a, _ := b.AddAttachment("demo", actor, board.KindStory, s.ID, "secret-plan.md", mdBytes)
	ai, _ := b.AddAttachment("demo", actor, board.KindIdea, i.ID, "a.png", pngBytes)
	b.OnChange(func(c board.Change) { changes = append(changes, c) })

	if err := b.RemoveAttachment("demo", "bob", a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Story("demo", s.ID); len(got.Attachments) != 0 {
		t.Fatalf("still attached: %+v", got.Attachments)
	}
	l, _ := b.Log("demo", s.ID)
	last := l[len(l)-1]
	if last.Actor != "bob" || !strings.Contains(last.Text, "secret-plan.md") || strings.Contains(last.Text, "Invented") {
		t.Fatalf("log %+v", last)
	}
	if fileCount(t, dir) != 1 {
		t.Fatalf("files left: %d", fileCount(t, dir))
	}
	if _, _, err := b.OpenAttachment("demo", a.ID); err == nil {
		t.Fatal("removed attachment still opens")
	}
	if err := b.RemoveAttachment("demo", "bob", a.ID); err == nil {
		t.Fatal("removed twice")
	} else {
		wantKind(t, err, board.NotFound)
	}
	// An idea has no log; removing still works.
	if err := b.RemoveAttachment("demo", "bob", ai.ID); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].Kind != board.KindStory || changes[1].Kind != board.KindIdea {
		t.Fatalf("changes %+v", changes)
	}
	// Ids are not reused.
	n, _ := b.AddAttachment("demo", actor, board.KindStory, s.ID, "n.png", pngBytes)
	if n.ID != "A-3" {
		t.Fatalf("next id %s", n.ID)
	}
}

func TestWithoutAFilesDirectoryAttachingFails(t *testing.T) {
	b := newWorkspace(t)
	s := addStory(t, b, board.NewStory{Title: "S"})
	if _, err := b.AddAttachment("demo", actor, board.KindStory, s.ID, "a.png", pngBytes); err == nil {
		t.Fatal("attached with nowhere to keep the file")
	}
}
