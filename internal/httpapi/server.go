// Package httpapi serves Backpack's HTTP API. The contract is api/openapi.yaml.
package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/lioneljmorrison/backpack/internal/auth"
	"github.com/lioneljmorrison/backpack/internal/board"
	"github.com/lioneljmorrison/backpack/internal/health"
	"github.com/lioneljmorrison/backpack/internal/mcpserver"
)

const healthTimeout = 2 * time.Second

// New returns the API handler. /healthz is the only route open without a
// token; everything under /v1/, and the MCP server at /mcp, requires one.
func New(checks map[string]health.Check, lookup auth.LookupFunc, b *board.Board) http.Handler {
	v1 := http.NewServeMux()
	v1.HandleFunc("GET /v1/whoami", whoami)
	routeBoard(v1, b)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz(checks))
	mux.Handle("/v1/", auth.Require(lookup)(v1))
	mux.Handle("/mcp", auth.Require(lookup)(mcpserver.New(b)))
	return mux
}

func healthz(checks map[string]health.Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()
		status, results := http.StatusOK, map[string]string{}
		for name, check := range checks {
			if err := check(ctx); err != nil {
				// The route is public: log the detail, report only the name.
				log.Printf("health: %s: %v", name, err)
				results[name], status = "unavailable", http.StatusServiceUnavailable
				continue
			}
			results[name] = "ok"
		}
		overall := "ok"
		if status != http.StatusOK {
			overall = "degraded"
		}
		writeJSON(w, status, map[string]any{"status": overall, "checks": results})
	}
}

func whoami(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.ActorFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"actor": actor})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
