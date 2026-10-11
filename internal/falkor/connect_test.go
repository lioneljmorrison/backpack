package falkor_test

import (
	"testing"

	"github.com/lioneljmorrison/backpack/internal/falkor"
	"github.com/lioneljmorrison/backpack/internal/falkor/falkortest"
)

func TestPingSucceedsAgainstALiveServer(t *testing.T) {
	url := falkortest.URL(t)
	db, err := falkor.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Conn.Close()
	if err := falkor.Ping(db); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestConnectRejectsAMalformedURL(t *testing.T) {
	if _, err := falkor.Connect("not a url"); err == nil {
		t.Fatal("want an error")
	}
}
