package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
)

// assets knows each static file's address, which carries a version taken
// from the file's contents. A deploy that changes a file gives it a new
// address, so a cache (the browser's or Cloudflare's) never serves the old
// one; an unchanged file keeps its address and stays cached.
type assets struct {
	fsys     fs.FS
	versions map[string]string
}

// newAssets reads every file in fsys once, to work out its version.
func newAssets(fsys fs.FS) *assets {
	a := &assets{fsys: fsys, versions: map[string]string{}}
	fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if data, err := fs.ReadFile(fsys, name); err == nil {
			sum := sha256.Sum256(data)
			a.versions[name] = hex.EncodeToString(sum[:])[:12]
		}
		return nil
	})
	return a
}

// url is the address pages use for the static file name.
func (a *assets) url(name string) string {
	return "/static/" + name + "?v=" + a.versions[name]
}

// handler serves the files under /static/. A request carrying the file's
// current version may be cached for good, since that address never changes
// meaning; any other request keeps the default caching.
func (a *assets) handler() http.Handler {
	files := http.StripPrefix("/static/", http.FileServerFS(a.fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[len("/static/"):]
		if v, ok := a.versions[name]; ok && r.URL.Query().Get("v") == v {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
