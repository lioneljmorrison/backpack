package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestSavedBytesComeBackUnderARandomKey(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	k1, err := s.Save([]byte("invented content"))
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := s.Save([]byte("invented content"))
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(k1) || k1 == k2 {
		t.Fatalf("keys %q and %q: want distinct 128-bit hex keys even for the same bytes", k1, k2)
	}
	f, err := s.Open(k1)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got, _ := io.ReadAll(f); string(got) != "invented content" {
		t.Fatalf("read %q", got)
	}
}

func TestOnlyAKeyThisStoreMadeOpensAFile(t *testing.T) {
	s, _ := New(t.TempDir())
	for _, k := range []string{"", "../x", "a/b", "ZZ", "0123456789abcdef0123456789abcdeg"} {
		if _, err := s.Open(k); err == nil {
			t.Fatalf("Open(%q) worked", k)
		}
		if err := s.Delete(k); err == nil {
			t.Fatalf("Delete(%q) worked", k)
		}
	}
}

func TestDeleteRemovesTheFileAndForgivesAMissingOne(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	k, _ := s.Save([]byte("x"))
	if err := s.Delete(k); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(k); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open after delete: %v", err)
	}
	if err := s.Delete(k); err != nil {
		t.Fatalf("deleting a missing file: %v", err)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}
