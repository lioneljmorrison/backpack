package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lioneljmorrison/backpack/internal/health"
)

func ok(context.Context) error   { return nil }
func down(context.Context) error { return errors.New("connection refused to 10.0.0.5") }

func lookup(string) (string, bool, error) { return "", false, nil }

func serve(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q is not JSON: %v", w.Body.String(), err)
	}
	return m
}

func TestHealthzIsOKWhenEveryCheckPasses(t *testing.T) {
	h := New(map[string]health.Check{"falkordb": ok, "typesense": ok}, lookup, nil)
	w := serve(h, "GET", "/healthz", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	body := decode(t, w)
	if body["status"] != "ok" {
		t.Fatalf("status field %v, want ok", body["status"])
	}
}

func TestHealthzNamesTheFailingCheckWithoutLeakingItsError(t *testing.T) {
	h := New(map[string]health.Check{"falkordb": ok, "typesense": down}, lookup, nil)
	w := serve(h, "GET", "/healthz", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", w.Code)
	}
	checks := decode(t, w)["checks"].(map[string]any)
	if checks["falkordb"] != "ok" || checks["typesense"] != "unavailable" {
		t.Fatalf("checks = %v", checks)
	}
	if strings.Contains(w.Body.String(), "10.0.0.5") {
		t.Fatal("health response leaked an internal error")
	}
}

func TestEveryV1RouteRequiresAToken(t *testing.T) {
	h := New(map[string]health.Check{}, lookup, nil)
	for _, path := range []string{"/v1/whoami", "/v1/anything"} {
		if w := serve(h, "GET", path, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: status %d, want 401", path, w.Code)
		}
	}
}

func TestWhoamiReturnsTheTokensActor(t *testing.T) {
	known := func(tok string) (string, bool, error) { return "lionel", tok == "bp_good", nil }
	h := New(map[string]health.Check{}, known, nil)
	w := serve(h, "GET", "/v1/whoami", "bp_good")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	if got := decode(t, w)["actor"]; got != "lionel" {
		t.Fatalf("actor %v, want lionel", got)
	}
}

func TestWhoamiOnlyAnswersGET(t *testing.T) {
	known := func(string) (string, bool, error) { return "lionel", true, nil }
	h := New(map[string]health.Check{}, known, nil)
	if w := serve(h, "POST", "/v1/whoami", "bp_good"); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", w.Code)
	}
}
