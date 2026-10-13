package typesensetest

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// keeperName is a collection no test drops. Typesense keeps one embedding
// model per process and unloads it when a dropped collection was the last to
// use it. Test packages run in parallel, so one package's cleanup can unload
// the model just as another creates its collection, and that collection
// then answers searches with 400 "Text embedder was not found". While the
// keeper exists the model is never the last one in use.
const keeperName = "test_keeper"

const keeperSchema = `{"name":"` + keeperName + `","fields":[` +
	`{"name":"t","type":"string"},` +
	`{"name":"e","type":"float[]","embed":{"from":["t"],` +
	`"model_config":{"model_name":"ts/all-MiniLM-L12-v2"}}}]}`

var (
	keeperOnce sync.Once
	keeperErr  error
)

// ensureKeeper creates the keeper if it is missing and waits until a search
// by meaning on it answers, once per test process.
func ensureKeeper(baseURL, key string) error {
	keeperOnce.Do(func() {
		if code := Do(baseURL, key, http.MethodPost, "/collections", keeperSchema); code != http.StatusCreated && code != http.StatusConflict {
			keeperErr = fmt.Errorf("typesense: creating the %s collection answered %d", keeperName, code)
			return
		}
		keeperErr = waitReady(baseURL, key, time.Minute, 500*time.Millisecond)
	})
	return keeperErr
}

// waitReady polls a search by meaning on the keeper until it answers, or
// gives up after timeout with an error that says the embedder is not ready.
func waitReady(baseURL, key string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	last := "no answer"
	for {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/collections/"+keeperName+
			"/documents/search?q=ready&query_by=e", nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-TYPESENSE-API-KEY", key)
		if resp, err := http.DefaultClient.Do(req); err != nil {
			last = err.Error()
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("typesense's text embedder is not ready after %s (last answer: %s)", timeout, last)
		}
		time.Sleep(interval)
	}
}
