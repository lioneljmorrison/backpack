package httpapi

import (
	"net/http"

	"github.com/lioneljmorrison/backpack/internal/search"
)

func searchHandler(x *search.Index) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rs, err := x.Search(r.Context(), r.PathValue("ws"), r.URL.Query().Get("q"))
		respond(w, http.StatusOK, map[string]any{"results": rs}, err)
	}
}
