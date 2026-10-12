package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// pingEvery is how often an idle stream sends a keep-alive, so the tunnel
// keeps it open, and re-checks its token. Tests shorten it.
var pingEvery = 25 * time.Second

// event is all a page is told about a change: what kind of thing changed
// and its ID, never its content. The page fetches itself again to see it.
type event struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// events streams a workspace's changes to a page as Server-Sent Events,
// until the page goes away, its token is revoked, or it falls behind.
func (s *site) events(w http.ResponseWriter, r *http.Request) {
	ws, err := s.workspace(r.PathValue("ws"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tok, _ := r.Cookie(cookieName) // there is one: the page is signed in
	changes, stop := s.live.Subscribe(ws.Slug)
	defer stop()

	rc := http.NewResponseController(w)
	// The server's write timeout would cut the stream; this response has none.
	rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	rc.Flush()

	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case c, ok := <-changes:
			if !ok {
				return
			}
			data, _ := json.Marshal(event{Kind: c.Kind, ID: c.ID})
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", data)
		case <-ping.C:
			if _, ok, err := s.lookup(tok.Value); err != nil || !ok {
				return
			}
			fmt.Fprint(w, ": ping\n\n")
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
