// Package transfer is what the HTTP API and the web pages share to take a file
// in and to serve one out: reading the form's file part under the size limit,
// the longer deadline a transfer needs, and the headers that keep a stored
// file from running in a browser.
package transfer

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lioneljmorrison/backpack/internal/board"
)

const (
	// maxUpload bounds a whole upload: the file, the form around it and a
	// little slack. The file itself is held to board.MaxAttachment.
	maxUpload = board.MaxAttachment + 64<<10
	// transferTime is how long one upload or download may take. The server's
	// own timeouts are short, so these routes ask for more themselves.
	transferTime = 5 * time.Minute
)

// MoreTime gives this request longer than the server's timeouts. A writer
// that cannot (a test recorder) keeps them.
func MoreTime(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	until := time.Now().Add(transferTime)
	rc.SetReadDeadline(until)
	rc.SetWriteDeadline(until)
}

// FilePart reads the multipart form's "file" part, holding the request to the
// size limit as it reads. A refusal is a *board.Error, so each caller shows it
// its own way.
func FilePart(w http.ResponseWriter, r *http.Request) (name string, data []byte, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	none := &board.Error{Kind: board.Invalid, Msg: "send the file as multipart/form-data, in a part named file"}
	tooBig := &board.Error{Kind: board.TooLarge, Msg: "a file is at most 10 MiB"}
	mr, err := r.MultipartReader()
	if err != nil {
		return "", nil, none
	}
	for {
		part, err := mr.NextPart()
		var over *http.MaxBytesError
		switch {
		case errors.As(err, &over):
			return "", nil, tooBig
		case err == io.EOF:
			return "", nil, none
		case err != nil:
			return "", nil, &board.Error{Kind: board.Invalid, Msg: "invalid multipart body"}
		}
		if part.FormName() != "file" {
			continue
		}
		// One byte over the limit tells the board it is too big.
		data, err = io.ReadAll(io.LimitReader(part, board.MaxAttachment+1))
		if errors.As(err, &over) {
			return "", nil, tooBig
		}
		if err != nil {
			return "", nil, &board.Error{Kind: board.Invalid, Msg: "invalid multipart body"}
		}
		return part.FileName(), data, nil
	}
}

// Serve sends an attachment's bytes. Whatever the file claims to be, the
// browser may not run it (nosniff, and a CSP that allows nothing and sandboxes
// it) and no cache may keep it (the service sits behind Cloudflare, which
// caches by file extension: no address here has one). The caller closes f.
func Serve(w http.ResponseWriter, r *http.Request, att board.Attachment, f *os.File) {
	h := w.Header()
	h.Set("Content-Type", att.Type)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, no-store")
	if strings.HasPrefix(att.Type, "text/markdown") {
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": att.Name}))
	}
	http.ServeContent(w, r, "", time.Time{}, f)
}
