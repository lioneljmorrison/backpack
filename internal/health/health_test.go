package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func typesenseStub(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

func TestTypesenseIsHealthyWhenItSaysOK(t *testing.T) {
	srv := typesenseStub(200, `{"ok":true}`)
	defer srv.Close()
	if err := Typesense(srv.URL)(context.Background()); err != nil {
		t.Fatalf("want healthy, got %v", err)
	}
}

func TestTypesenseIsUnhealthyWhenItSaysNotOK(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
	}{
		{200, `{"ok":false}`},
		{503, `{"ok":false}`},
		{200, `not json`},
	} {
		srv := typesenseStub(c.status, c.body)
		if err := Typesense(srv.URL)(context.Background()); err == nil {
			t.Errorf("%d %s: want unhealthy", c.status, c.body)
		}
		srv.Close()
	}
}

func TestTypesenseIsUnhealthyWhenUnreachable(t *testing.T) {
	if err := Typesense("http://127.0.0.1:1")(context.Background()); err == nil {
		t.Fatal("want unhealthy")
	}
}

func TestTypesenseLiveServer(t *testing.T) {
	url := os.Getenv("BACKPACK_TEST_TYPESENSE")
	if url == "" {
		t.Skip("BACKPACK_TEST_TYPESENSE not set")
	}
	if err := Typesense(url)(context.Background()); err != nil {
		t.Fatalf("live typesense: %v", err)
	}
}
