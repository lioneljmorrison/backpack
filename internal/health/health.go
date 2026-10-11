// Package health checks the services Backpack depends on.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Check reports nil when a dependency is healthy.
type Check func(ctx context.Context) error

// Typesense checks the server's own /health endpoint, which needs no key.
func Typesense(baseURL string) Check {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var body struct {
			OK bool `json:"ok"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return fmt.Errorf("typesense health: %w", err)
		}
		if resp.StatusCode != http.StatusOK || !body.OK {
			return fmt.Errorf("typesense health: status %d, ok=%v", resp.StatusCode, body.OK)
		}
		return nil
	}
}
