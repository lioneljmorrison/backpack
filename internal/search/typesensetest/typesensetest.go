// Package typesensetest gives tests throwaway collections on a real Typesense.
package typesensetest

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

// Prefix returns BACKPACK_TEST_TYPESENSE, its API key and a unique
// collection-name prefix, skipping the test when the URL is unset. Every
// collection whose name starts with the prefix is dropped when the test ends.
func Prefix(t *testing.T) (baseURL, key, prefix string) {
	t.Helper()
	baseURL = os.Getenv("BACKPACK_TEST_TYPESENSE")
	if baseURL == "" {
		t.Skip("BACKPACK_TEST_TYPESENSE not set")
	}
	key = os.Getenv("BACKPACK_TEST_TYPESENSE_KEY")
	if key == "" {
		t.Fatal("BACKPACK_TEST_TYPESENSE is set but BACKPACK_TEST_TYPESENSE_KEY is not")
	}
	if err := ensureKeeper(baseURL, key); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	prefix = "test_" + hex.EncodeToString(b) + "_"
	t.Cleanup(func() {
		for _, name := range collections(baseURL, key) {
			if strings.HasPrefix(name, prefix) {
				Do(baseURL, key, http.MethodDelete, "/collections/"+url.PathEscape(name), "")
			}
		}
	})
	return baseURL, key, prefix
}

// Do sends one request with the key, for tests that set up Typesense
// directly, and returns the status code.
func Do(baseURL, key, method, path, body string) int {
	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		return 0
	}
	req.Header.Set("X-TYPESENSE-API-KEY", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}

func collections(baseURL, key string) []string {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/collections", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("X-TYPESENSE-API-KEY", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var cs []struct {
		Name string `json:"name"`
	}
	json.NewDecoder(resp.Body).Decode(&cs)
	names := make([]string, 0, len(cs))
	for _, c := range cs {
		names = append(names, c.Name)
	}
	return names
}
