package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// maxBatch is the most requests one batch may carry.
const maxBatch = 100

type subRequest struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body"`
}

type subResult struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// batchHandler sends each request in a batch, in order, to its own route on
// v1, as the caller. A request that fails is answered with its status and
// reason and doesn't stop the rest.
func batchHandler(v1 http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Requests []subRequest `json:"requests"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		if len(in.Requests) == 0 || len(in.Requests) > maxBatch {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("a batch has 1 to %d requests", maxBatch)})
			return
		}
		base := "/v1/workspaces/" + url.PathEscape(r.PathValue("ws")) + "/"
		out := make([]subResult, len(in.Requests))
		for i, sr := range in.Requests {
			out[i] = send(v1, r, base, sr)
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": out})
	}
}

func send(v1 http.Handler, r *http.Request, base string, sr subRequest) subResult {
	if why := checkSub(sr); why != "" {
		b, _ := json.Marshal(map[string]string{"error": why})
		return subResult{Status: http.StatusBadRequest, Body: b}
	}
	req, err := http.NewRequestWithContext(r.Context(), sr.Method, base+sr.Path, bytes.NewReader(sr.Body))
	if err != nil {
		b, _ := json.Marshal(map[string]string{"error": "invalid request: " + err.Error()})
		return subResult{Status: http.StatusBadRequest, Body: b}
	}
	req.Header.Set("Content-Type", "application/json")
	rec := &recorder{header: http.Header{}}
	v1.ServeHTTP(rec, req)
	res := subResult{Status: rec.status, Body: rec.body.Bytes()}
	if len(res.Body) > 0 && !json.Valid(res.Body) {
		res.Body, _ = json.Marshal(map[string]string{"error": strings.TrimSpace(rec.body.String())})
	}
	return res
}

// checkSub says why a request can't be batched, or "" when it can: it must
// change something, and its path must stay inside the workspace.
func checkSub(sr subRequest) string {
	switch sr.Method {
	case http.MethodPost, http.MethodPatch, http.MethodDelete:
	default:
		return "only POST, PATCH and DELETE requests can be batched"
	}
	u, err := url.Parse(sr.Path)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") ||
		path.Clean(u.Path) != u.Path || strings.HasPrefix(u.Path, "..") {
		return "path is relative to the workspace, such as stories/S-4/status"
	}
	if u.Path == "batch" {
		return "a batch can't hold a batch"
	}
	return ""
}

// recorder keeps a batched request's answer.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (rec *recorder) Header() http.Header { return rec.header }

func (rec *recorder) WriteHeader(status int) {
	if rec.status == 0 {
		rec.status = status
	}
}

func (rec *recorder) Write(b []byte) (int, error) {
	rec.WriteHeader(http.StatusOK)
	return rec.body.Write(b)
}
