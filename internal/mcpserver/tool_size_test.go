package mcpserver_test

import (
	"context"
	"encoding/json"
	"testing"
)

// The tool list (the JSON of ListTools' tools: names, descriptions and input
// schemas) was 21,993 bytes in v0.3.1 and 14,648 in v0.4.0. Clients that load
// every tool up front pay for all of it, so rarer actions are on request
// instead (see the menu in menu_test.go): the changes run through batch (step
// 1, 8,314 bytes) and the reads through actions (step 2, 6,595 bytes). The
// limit is 8,000: the measured 6,595 * 1.25 = 8,244 is higher, so the round
// number below it. A new tool or a longer description must earn its bytes.
const (
	maxToolListBytes   = 8000
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
	t.Logf("the tool list is %d bytes", len(all))
	if len(all) > maxToolListBytes {
		t.Errorf("the tool list is %d bytes, the limit is %d", len(all), maxToolListBytes)
	}
}
