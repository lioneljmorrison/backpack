package live_test

import (
	"sync"
	"testing"
	"time"

	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/live"
)

func change(ws, id string) board.Change {
	return board.Change{Workspace: ws, Kind: board.KindStory, ID: id}
}

// next waits briefly for one change, or reports that none came.
func next(t *testing.T, ch <-chan board.Change) (board.Change, bool) {
	t.Helper()
	select {
	case c, ok := <-ch:
		return c, ok
	case <-time.After(time.Second):
		t.Fatal("no change arrived")
		return board.Change{}, false
	}
}

func TestAChangeReachesEveryWatcherOfItsWorkspaceOnly(t *testing.T) {
	h := live.New()
	a, stopA := h.Subscribe("demo")
	defer stopA()
	b, stopB := h.Subscribe("demo")
	defer stopB()
	other, stopOther := h.Subscribe("other")
	defer stopOther()

	h.Publish(change("demo", "S-1"))
	for _, ch := range []<-chan board.Change{a, b} {
		if c, _ := next(t, ch); c != change("demo", "S-1") {
			t.Fatalf("got %+v", c)
		}
	}
	select {
	case c := <-other:
		t.Fatalf("another workspace got %+v", c)
	default:
	}
}

func TestAWatcherThatStopsHearsNothingMore(t *testing.T) {
	h := live.New()
	ch, stop := h.Subscribe("demo")
	stop()
	stop() // stopping twice is harmless
	h.Publish(change("demo", "S-1"))
	if _, ok := <-ch; ok {
		t.Fatal("a stopped watcher got a change")
	}
}

// A watcher that falls behind is dropped, so a write never waits; it
// reconnects and fetches its page again.
func TestAWatcherThatFallsBehindIsDroppedWithoutHoldingUpOthers(t *testing.T) {
	h := live.New()
	slow, stopSlow := h.Subscribe("demo")
	defer stopSlow()
	fast, stopFast := h.Subscribe("demo")
	defer stopFast()

	for i := range live.Buffer + 1 {
		h.Publish(change("demo", "S-1"))
		if i < live.Buffer {
			next(t, fast)
		}
	}
	if _, ok := next(t, fast); !ok {
		t.Fatal("the watcher keeping up was dropped")
	}
	for range live.Buffer {
		next(t, slow)
	}
	if _, ok := <-slow; ok {
		t.Fatal("the watcher that fell behind was kept")
	}
}

func TestWatchersComeAndGoWhileChangesArePublished(t *testing.T) {
	h := live.New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 100 {
				h.Publish(change("demo", "S-1"))
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				_, stop := h.Subscribe("demo")
				stop()
			}
		}()
	}
	wg.Wait()
}
