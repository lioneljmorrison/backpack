package mcpserver_test

import (
	"context"
	"encoding/json"
	"testing"
)

// v0.3.1's full tool list (the JSON of ListTools' tools: names, descriptions
// and input schemas) was 21,993 bytes. The limit is a third less, rounded
// down: 21,993 * 2/3 = 14,662. Clients that load every tool up front pay for
// all of it, so a new tool or a longer description must earn its bytes.
const (
	maxToolListBytes   = 14662
	maxDescriptionSize = 300
)

func TestToolDescriptionsAreShort(t *testing.T) {
	_, cs := connect(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range res.Tools {
		if n := len([]rune(tl.Description)); n > maxDescriptionSize {
			t.Errorf("%s: description is %d characters, the limit is %d", tl.Name, n, maxDescriptionSize)
		}
	}
	all, err := json.Marshal(res.Tools)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) > maxToolListBytes {
		t.Errorf("the tool list is %d bytes, the limit is %d", len(all), maxToolListBytes)
	}
}
