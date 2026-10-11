package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// client is the handful of Typesense calls the index needs, over plain
// net/http: create and drop a collection, import and upsert documents, and
// search.
type client struct {
	baseURL, key string
}

// errNoCollection is Typesense's 404 for a collection that isn't there.
var errNoCollection = errors.New("no such collection")

// do sends a request and returns the answer's body. A connection failure or
// a 5xx is ErrUnavailable; a 404 is errNoCollection.
func (c client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-TYPESENSE-API-KEY", c.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errNoCollection
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: typesense %s %s: status %d", ErrUnavailable, method, path, resp.StatusCode)
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("typesense %s %s: status %d: %s", method, path, resp.StatusCode, out)
	}
	return out, nil
}

func (c client) create(ctx context.Context, schema any) error {
	body, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodPost, "/collections", body)
	return err
}

// drop removes a collection; one that isn't there is already dropped.
func (c client) drop(ctx context.Context, name string) error {
	_, err := c.do(ctx, http.MethodDelete, "/collections/"+url.PathEscape(name), nil)
	if errors.Is(err, errNoCollection) {
		return nil
	}
	return err
}

func (c client) upsert(ctx context.Context, name string, d doc) error {
	body, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(name)+"/documents?action=upsert", body)
	return err
}

// importDocs upserts many documents in one request. Typesense answers each
// line separately, so a 200 can still carry failures.
func (c client) importDocs(ctx context.Context, name string, ds []doc) error {
	if len(ds) == 0 {
		return nil
	}
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	for _, d := range ds {
		if err := enc.Encode(d); err != nil {
			return err
		}
	}
	out, err := c.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(name)+"/documents/import?action=upsert",
		body.Bytes())
	if err != nil {
		return err
	}
	for line := range strings.Lines(string(out)) {
		var r struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil || !r.Success {
			return fmt.Errorf("typesense import: %s", strings.TrimSpace(line))
		}
	}
	return nil
}

func (c client) search(ctx context.Context, name string, params url.Values, out any) error {
	body, err := c.do(ctx, http.MethodGet, "/collections/"+url.PathEscape(name)+"/documents/search?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
