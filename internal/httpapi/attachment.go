package httpapi

import (
	"net/http"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/transfer"
)

func routeAttachments(mux *http.ServeMux, a boardAPI) {
	mux.HandleFunc("POST /v1/workspaces/{ws}/stories/{id}/attachments", a.addAttachment(board.KindStory))
	mux.HandleFunc("POST /v1/workspaces/{ws}/ideas/{id}/attachments", a.addAttachment(board.KindIdea))
	mux.HandleFunc("POST /v1/workspaces/{ws}/phases/{id}/attachments", a.addAttachment(board.KindPhase))
	mux.HandleFunc("GET /v1/workspaces/{ws}/attachments/{aid}", a.getAttachment)
	mux.HandleFunc("DELETE /v1/workspaces/{ws}/attachments/{aid}", a.removeAttachment)
}

// addAttachment takes a multipart form whose part "file" is the file.
func (a boardAPI) addAttachment(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transfer.MoreTime(w)
		name, data, err := transfer.FilePart(w, r)
		if err != nil {
			fail(w, err)
			return
		}
		att, err := a.b.AddAttachment(r.PathValue("ws"), actor(r), kind, r.PathValue("id"), name, data)
		respond(w, http.StatusCreated, att, err)
	}
}

// getAttachment serves the bytes, with the headers that keep them inert.
func (a boardAPI) getAttachment(w http.ResponseWriter, r *http.Request) {
	transfer.MoreTime(w)
	att, f, err := a.b.OpenAttachment(r.PathValue("ws"), r.PathValue("aid"))
	if err != nil {
		fail(w, err)
		return
	}
	defer f.Close()
	transfer.Serve(w, r, att, f)
}

func (a boardAPI) removeAttachment(w http.ResponseWriter, r *http.Request) {
	if err := a.b.RemoveAttachment(r.PathValue("ws"), actor(r), r.PathValue("aid")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
