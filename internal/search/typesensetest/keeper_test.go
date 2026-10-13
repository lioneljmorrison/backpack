package typesensetest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A fake Typesense: invented, answers the embedder-not-found 400 until it has
// been asked `after` times, then 200.
func fake(after int32) (*httptest.Server, *int32) {
	var calls int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= after {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"Text embedder was not found."}`))
			return
		}
		w.Write([]byte(`{"hits":[]}`))
	})), &calls
}

func TestWaitReadyReturnsOnceTheEmbedderAnswers(t *testing.T) {
	ts, calls := fake(3)
	defer ts.Close()
	if err := waitReady(ts.URL, "k", time.Second, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(calls); got != 4 {
		t.Fatalf("asked %d times, want 4", got)
	}
}

func TestWaitReadyNamesTheEmbedderWhenItNeverComesUp(t *testing.T) {
	ts, _ := fake(1 << 30)
	defer ts.Close()
	err := waitReady(ts.URL, "k", 50*time.Millisecond, 5*time.Millisecond)
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"embedder", "not ready", "Text embedder was not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q lacks %q", err, want)
		}
	}
}
