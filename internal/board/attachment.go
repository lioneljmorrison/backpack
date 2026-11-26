package board

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lioneljmorrison/backpack/internal/files"
)

// MaxAttachment is the most bytes one attachment may hold.
const MaxAttachment = 10 << 20

// maxAttachmentName caps a file name, in bytes.
const maxAttachmentName = 255

// markdownType is what a markdown attachment is stored and served as.
const markdownType = "text/markdown; charset=utf-8"

// imageTypes are the images accepted, as http.DetectContentType names them.
// SVG is not among them: it can run script when a browser opens it.
var imageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// Attachment is a file kept on a story, idea or phase. Its bytes live in the
// files store under an unguessable key that never leaves the board.
type Attachment struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Size  int    `json:"size"`
	At    string `json:"at"`
	Actor string `json:"actor"`
	key   string
}

// AttachmentRef is an attachment as a read of its item lists it.
type AttachmentRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

// SetFiles gives the board somewhere to keep attachments. Set it before the
// board is shared; without it, attaching fails.
func (b *Board) SetFiles(s *files.Store) { b.files = s }

// attachmentType checks a file's name and bytes, and answers the type it is
// stored and served as.
func attachmentType(name string, data []byte) (string, error) {
	if name == "" || len(name) > maxAttachmentName || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", refuse(Invalid, "a file name is 1-%d bytes with no / or \\", maxAttachmentName)
	}
	// plainText lets a newline or tab through; a name is one line.
	if plainText(name) != nil || strings.ContainsFunc(name, unicode.IsControl) {
		return "", refuse(Invalid, "a file name cannot contain control or invisible formatting characters")
	}
	if len(data) > MaxAttachment {
		return "", refuse(TooLarge, "a file is at most %d MiB", MaxAttachment>>20)
	}
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		if !utf8.Valid(data) || plainText(string(data)) != nil {
			return "", refuse(Unsupported, "a markdown file must be plain UTF-8 text")
		}
		return markdownType, nil
	}
	if t := http.DetectContentType(data); slices.Contains(imageTypes, t) {
		return t, nil
	}
	return "", refuse(Unsupported, "only PNG, JPEG, GIF and WebP images and .md files can be attached")
}

// itemMatch is the pattern that finds the story, idea or phase an
// attachment goes on, and the value of its $item parameter.
func itemMatch(kind, id string) (pattern string, item any, err error) {
	switch kind {
	case KindStory:
		return "(x:Story {id: $item})", id, nil
	case KindIdea:
		return "(x:Idea {id: $item})", id, nil
	case KindPhase:
		n, convErr := strconv.Atoi(id)
		if convErr != nil || n < 1 {
			return "", nil, refuse(Invalid, "a phase is referred to by its number")
		}
		return "(x:Phase {number: $item})", n, nil
	}
	return "", nil, refuse(Invalid, "files attach to a story, an idea or a phase")
}

// AddAttachment attaches a file to a story, idea or phase. The file is saved
// first, then one query adds the attachment and, on a story or a phase, a log
// line. A file left behind by a failed query is harmless: nothing names it.
func (b *Board) AddAttachment(ws, actor, kind, id, name string, data []byte) (_ Attachment, err error) {
	defer func() { b.changed(err, Change{ws, kind, id}) }()
	pattern, item, err := itemMatch(kind, id)
	if err != nil {
		return Attachment{}, err
	}
	typ, err := attachmentType(name, data)
	if err != nil {
		return Attachment{}, err
	}
	g, err := b.workspace(ws)
	if err != nil {
		return Attachment{}, err
	}
	if b.files == nil {
		return Attachment{}, fmt.Errorf("add attachment: no files directory")
	}
	key, err := b.files.Save(data)
	if err != nil {
		return Attachment{}, err
	}
	a := Attachment{Name: name, Type: typ, Size: len(data), At: now(), Actor: actor, key: key}
	logged := kind != KindIdea // an idea has no log
	res, err := g.Query(`
		MATCH (w:Workspace), `+pattern+`
		WITH w, x, coalesce(w.next_attachment, 1) AS n, coalesce(w.next_log, 1) AS l
		SET w.next_attachment = n + 1, w.next_log = l + CASE WHEN $logged THEN 1 ELSE 0 END
		CREATE (x)-[:HAS_ATTACHMENT]->(a:Attachment {id: 'A-' + toString(n), key: $key, name: $name, type: $type,
			size: $size, at: $at, actor: $actor})
		FOREACH (_ IN CASE WHEN $logged THEN [1] ELSE [] END |
			CREATE (x)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(l), at: $at, actor: $actor, kind: 'note',
				text: 'Attached ' + $name}))
		RETURN a.id AS id`,
		map[string]any{"item": item, "logged": logged, "key": key, "name": name, "type": typ,
			"size": a.Size, "at": a.At, "actor": actor}, nil)
	switch {
	case err != nil:
		err = fmt.Errorf("add attachment: %w", err)
	case !res.Next():
		err = refuse(NotFound, "no %s %q", kind, id)
	}
	if err != nil {
		b.files.Delete(key)
		return Attachment{}, err
	}
	a.ID = str(res.Record(), "id")
	return a, nil
}

// OpenAttachment finds an attachment of the workspace and opens its bytes.
// The caller closes the file.
func (b *Board) OpenAttachment(ws, id string) (Attachment, *os.File, error) {
	g, err := b.workspace(ws)
	if err != nil {
		return Attachment{}, nil, err
	}
	res, err := g.ROQuery(`
		MATCH ()-[:HAS_ATTACHMENT]->(a:Attachment {id: $id})
		RETURN a.key AS key, a.name AS name, a.type AS type, a.size AS size, a.at AS at, a.actor AS actor`,
		map[string]any{"id": id}, nil)
	if err != nil {
		return Attachment{}, nil, fmt.Errorf("read attachment: %w", err)
	}
	if !res.Next() {
		return Attachment{}, nil, refuse(NotFound, "no attachment %q", id)
	}
	r := res.Record()
	a := Attachment{ID: id, Name: str(r, "name"), Type: str(r, "type"), Size: num(r, "size"),
		At: str(r, "at"), Actor: str(r, "actor"), key: str(r, "key")}
	if b.files == nil {
		return Attachment{}, nil, fmt.Errorf("open attachment: no files directory")
	}
	f, err := b.files.Open(a.key)
	if err != nil {
		return Attachment{}, nil, fmt.Errorf("open attachment %s: %w", id, err)
	}
	return a, f, nil
}

// RemoveAttachment removes an attachment. One query takes it away and, on a
// story or a phase, logs who removed which file (its name, never its
// content); then the bytes are deleted.
func (b *Board) RemoveAttachment(ws, actor, id string) (err error) {
	var kind, item string
	defer func() { b.changed(err, Change{ws, kind, item}) }()
	g, err := b.workspace(ws)
	if err != nil {
		return err
	}
	res, err := g.Query(`
		MATCH (w:Workspace)
		MATCH (x)-[:HAS_ATTACHMENT]->(a:Attachment {id: $id})
		WITH w, x, a, a.key AS key, a.name AS name, labels(x)[0] AS label, x.id AS xid, x.number AS xnum,
			coalesce(w.next_log, 1) AS l, labels(x)[0] <> 'Idea' AS logged
		SET w.next_log = l + CASE WHEN logged THEN 1 ELSE 0 END
		FOREACH (_ IN CASE WHEN logged THEN [1] ELSE [] END |
			CREATE (x)-[:LOGGED]->(:LogEntry {id: 'L-' + toString(l), at: $at, actor: $actor, kind: 'note',
				text: 'Removed ' + name}))
		DETACH DELETE a
		RETURN key, label, xid, xnum`,
		map[string]any{"id": id, "at": now(), "actor": actor}, nil)
	if err != nil {
		return fmt.Errorf("remove attachment: %w", err)
	}
	if !res.Next() {
		return refuse(NotFound, "no attachment %q", id)
	}
	r := res.Record()
	kind, item = strings.ToLower(str(r, "label")), str(r, "xid")
	if kind == KindPhase {
		item = phaseID(num(r, "xnum"))
	}
	if b.files == nil {
		return nil
	}
	if err := b.files.Delete(str(r, "key")); err != nil {
		return fmt.Errorf("delete the file of %s: %w", id, err)
	}
	return nil
}

// attachmentRefs turns the attachment maps a read collects into refs, in id
// order. None gives nil, so an item without any leaves the list out of JSON.
func attachmentRefs(ms []map[string]any) []AttachmentRef {
	var out []AttachmentRef
	for _, m := range ms {
		size, _ := m["size"].(int64)
		out = append(out, AttachmentRef{ID: field(m, "id"), Name: field(m, "name"), Type: field(m, "type"), Size: int(size)})
	}
	slices.SortFunc(out, func(a, b AttachmentRef) int { return seq(a.ID) - seq(b.ID) })
	return out
}
