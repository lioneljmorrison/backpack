package auth

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// LookupFunc returns the actor holding a token, and whether one does.
type LookupFunc func(token string) (actor string, ok bool, err error)

type ctxKey struct{}

// ActorFrom returns the actor Require attached to the request.
func ActorFrom(ctx context.Context) (string, bool) {
	a, ok := ctx.Value(ctxKey{}).(string)
	return a, ok
}

// Require admits only requests bearing a live token, and attaches its actor.
func Require(lookup LookupFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !found || tok == "" {
				unauthorized(w)
				return
			}
			actor, ok, err := lookup(tok)
			if err != nil {
				log.Printf("auth: %v", err)
				writeError(w, http.StatusServiceUnavailable, "authentication is unavailable")
				return
			}
			if !ok {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, actor)))
		})
	}
}

// RequireCookie admits only browsers whose cookie name holds a live token,
// and attaches its actor. Any other browser is sent to signIn.
func RequireCookie(lookup LookupFunc, name, signIn string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(name)
			if err != nil || c.Value == "" {
				http.Redirect(w, r, signIn, http.StatusSeeOther)
				return
			}
			actor, ok, err := lookup(c.Value)
			if err != nil {
				log.Printf("auth: %v", err)
				http.Error(w, "authentication is unavailable", http.StatusServiceUnavailable)
				return
			}
			if !ok {
				http.Redirect(w, r, signIn, http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, actor)))
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "a valid bearer token is required")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
