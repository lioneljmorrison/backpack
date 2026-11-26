package web

import (
	"net/http"
	"net/url"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/transfer"
)

// routeAttachments serves a file to the browser (an <img> sends the cookie,
// not a bearer header) and takes uploads and removals as form actions. Each
// goes back to the page of the item the file is on.
func (s *site) routeAttachments(mux *http.ServeMux) {
	mux.HandleFunc("GET /w/{ws}/attachments/{aid}", s.getAttachment)
	for _, k := range []struct{ kind, path, param string }{
		{board.KindStory, "stories", "id"},
		{board.KindIdea, "ideas", "id"},
		{board.KindPhase, "phases", "n"},
	} {
		base := "/w/{ws}/" + k.path + "/{" + k.param + "}/attachments"
		mux.HandleFunc("POST "+base, s.addAttachment(k.kind, k.param))
		mux.HandleFunc("POST "+base+"/{aid}/remove", s.removeAttachment(k.kind, k.param))
	}
}

// itemURL is the page an attachment's item is shown on.
func itemURL(r *http.Request, kind, param string) string {
	id := r.PathValue(param)
	switch kind {
	case board.KindIdea:
		return boardURL(r) + "/ideas#" + url.PathEscape(id)
	case board.KindPhase:
		return boardURL(r) + "/phases/" + url.PathEscape(id)
	}
	return storyURL(r, id)
}

func (s *site) addAttachment(kind, param string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		back := itemURL(r, kind, param)
		transfer.MoreTime(w)
		name, data, err := transfer.FilePart(w, r)
		if err == nil {
			_, err = s.board.AddAttachment(r.PathValue("ws"), actor(r), kind, r.PathValue(param), name, data)
		}
		s.done(w, r, back, err)
	}
}

func (s *site) removeAttachment(kind, param string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := s.board.RemoveAttachment(r.PathValue("ws"), actor(r), r.PathValue("aid"))
		s.done(w, r, itemURL(r, kind, param), err)
	}
}

func (s *site) getAttachment(w http.ResponseWriter, r *http.Request) {
	transfer.MoreTime(w)
	att, f, err := s.board.OpenAttachment(r.PathValue("ws"), r.PathValue("aid"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	transfer.Serve(w, r, att, f)
}
