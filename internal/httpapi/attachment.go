package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/lioneljmorrison/backpack/internal/board"
)

const (
	// maxUpload bounds a whole upload: the file, the form around it and a
	// little slack. The file itself is held to board.MaxAttachment.
	maxUpload = board.MaxAttachment + 64<<10
	// transferTime is how long one upload or download may take. The server's
	// own timeouts are short, so these two routes ask for more themselves.
	transferTime = 5 * time.Minute
)

func routeAttachments(mux *http.ServeMux, a boardAPI) {
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/attachments", a.addAttachment(board.KindStory))
	mux.HandleFunc("POST /v1/workspaces/{ws}/ideas/{id}/attachments", a.addAttachment(board.KindIdea))
	mux.HandleFunc("POST /v1/workspaces/{ws}/phases/{id}/attachments", a.addAttachment(board.KindPhase))
	mux.HandleFunc("GET /v1/workspaces/{ws}/attachments/{aid}", a.getAttachment)
	mux.HandleFunc("DELETE /v1/workspaces/{ws}/attachments/{aid}", a.removeAttachment)
}

// moreTime gives this request longer than the server's timeouts. A writer
// that cannot (a test recorder) keeps them.
func moreTime(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	until := time.Now().Add(transferTime)
	rc.SetReadDeadline(until)
	rc.SetWriteDeadline(until)
}

// addAttachment takes a multipart form whose part "file" is the file.
func (a boardAPI) addAttachment(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		moreTime(w)
		r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
		name, data, ok := filePart(w, r)
		if !ok {
			return
		}
		att, err := a.b.AddAttachment(r.PathValue("ws"), actor(r), kind, r.PathValue("id"), name, data)
		respond(w, http.StatusCreated, att, err)
	}
}

// filePart reads the form's "file" part, holding it to the size limit as it
// reads; it answers the request itself when there is none.
func filePart(w http.ResponseWriter, r *http.Request) (name string, data []byte, ok bool) {
	bad := func(msg string) (string, []byte, bool) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return "", nil, false
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return bad("send the file as multipart/form-data, in a part named file")
	}
	for {
		part, err := mr.NextPart()
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			return tooLarge(w)
		case err == io.EOF:
			return bad("send the file as multipart/form-data, in a part named file")
		case err != nil:
			return bad("invalid multipart body")
		}
		if part.FormName() != "file" {
			continue
		}
		// One byte over the limit tells the board it is too big.
		data, err = io.ReadAll(io.LimitReader(part, board.MaxAttachment+1))
		if errors.As(err, &tooBig) {
			return tooLarge(w)
		}
		if err != nil {
			return bad("invalid multipart body")
		}
		return part.FileName(), data, true
	}
}

func tooLarge(w http.ResponseWriter) (string, []byte, bool) {
	writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "a file is at most 10 MiB"})
	return "", nil, false
}

// getAttachment serves the bytes. Whatever the file claims to be, the
// browser may not run it (nosniff, and a CSP that allows nothing and sandboxes
// it) and no cache may keep it (the service sits behind Cloudflare, which
// caches by file extension: no address here has one).
func (a boardAPI) getAttachment(w http.ResponseWriter, r *http.Request) {
	moreTime(w)
	att, f, err := a.b.OpenAttachment(r.PathValue("ws"), r.PathValue("aid"))
	if err != nil {
		fail(w, err)
		return
	}
	defer f.Close()
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

func (a boardAPI) removeAttachment(w http.ResponseWriter, r *http.Request) {
	if err := a.b.RemoveAttachment(r.PathValue("ws"), actor(r), r.PathValue("aid")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
