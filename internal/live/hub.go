// Package live passes the board's changes to the pages watching a
// workspace, so they can show them without being reloaded.
package live

import (
	"sync"

	"github.com/lioneljmorrison/backpack/internal/board"
)

// Buffer is how many changes a watcher may fall behind before it is
// dropped.
const Buffer = 32

// Hub passes each change to everyone watching its workspace. It lives in
// the one Backpack process; nothing leaves the host.
type Hub struct {
	mu       sync.Mutex
	watchers map[string]map[chan board.Change]struct{}
}

// New returns a hub with no watchers.
func New() *Hub {
	return &Hub{watchers: map[string]map[chan board.Change]struct{}{}}
}

// Subscribe watches workspace ws. The channel is closed when stop is
// called, or when the watcher falls behind and is dropped.
func (h *Hub) Subscribe(ws string) (_ <-chan board.Change, stop func()) {
	ch := make(chan board.Change, Buffer)
	h.mu.Lock()
	if h.watchers[ws] == nil {
		h.watchers[ws] = map[chan board.Change]struct{}{}
	}
	h.watchers[ws][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.drop(ws, ch)
	}
}

// Publish passes c to its workspace's watchers, for the board's OnChange.
// It never waits: a watcher whose buffer is full is dropped, and its page
// reconnects and fetches itself again.
func (h *Hub) Publish(c board.Change) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.watchers[c.Workspace] {
		select {
		case ch <- c:
		default:
			h.drop(c.Workspace, ch)
		}
	}
}

// drop closes a watcher once. The caller holds the lock.
func (h *Hub) drop(ws string, ch chan board.Change) {
	if _, ok := h.watchers[ws][ch]; !ok {
		return
	}
	delete(h.watchers[ws], ch)
	if len(h.watchers[ws]) == 0 {
		delete(h.watchers, ws)
	}
	close(ch)
}

// Close ends every watcher's stream, for the server's shutdown: a stream
// never finishes on its own, and the pages reconnect to the next server.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ws, chans := range h.watchers {
		for ch := range chans {
			h.drop(ws, ch)
		}
	}
}
