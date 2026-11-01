package httpapi

import (
	"net/http"

	"github.com/lioneljmorrison/backpack/internal/search"
)

func searchHandler(x *search.Index) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if story := q.Get("story"); story != "" {
			rs, scope, err := x.SearchNear(r.Context(), r.PathValue("ws"), q.Get("q"), story)
			respond(w, http.StatusOK, map[string]any{"results": rs, "scope": scope}, err)
			return
		}
		rs, err := x.Search(r.Context(), r.PathValue("ws"), q.Get("q"))
		respond(w, http.StatusOK, map[string]any{"results": rs}, err)
	}
}
