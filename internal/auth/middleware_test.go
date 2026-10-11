package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func lookup(tokens map[string]string) LookupFunc {
	return func(tok string) (string, bool, error) {
		a, ok := tokens[tok]
		return a, ok, nil
	}
}

func echoActor() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, _ := ActorFrom(r.Context())
		w.Write([]byte(a))
	})
}

func TestRequireRefusesMissingOrMalformedCredentials(t *testing.T) {
	h := Require(lookup(map[string]string{"bp_good": "lionel"}))(echoActor())
	for _, header := range []string{"", "bp_good", "Basic bp_good", "Bearer", "Bearer bp_bad"} {
		r := httptest.NewRequest("GET", "/v1/x", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status %d, want 401", header, w.Code)
		}
		if w.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("Authorization %q: missing WWW-Authenticate: Bearer", header)
		}
	}
}

func TestRequirePassesTheActorToTheHandler(t *testing.T) {
	h := Require(lookup(map[string]string{"bp_good": "lionel"}))(echoActor())
	r := httptest.NewRequest("GET", "/v1/x", nil)
	r.Header.Set("Authorization", "Bearer bp_good")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "lionel" {
		t.Fatalf("got %d %q, want 200 lionel", w.Code, w.Body.String())
	}
}

func TestRequireReportsALookupFailureAsUnavailable(t *testing.T) {
	failing := func(string) (string, bool, error) { return "", false, errors.New("db down") }
	h := Require(failing)(echoActor())
	r := httptest.NewRequest("GET", "/v1/x", nil)
	r.Header.Set("Authorization", "Bearer bp_good")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", w.Code)
	}
}
