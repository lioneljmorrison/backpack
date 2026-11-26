// Package files keeps attachment bytes on disk, one file per attachment under
// a random 128-bit key. It knows nothing of names, types or workspaces: the
// graph holds those, and the key links the two.
package files

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var keyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Store is a directory of files.
type Store struct{ dir string }

// New uses dir, creating it when it is missing.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("files directory: %w", err)
	}
	return &Store{dir}, nil
}

// Save writes data under a new random key and answers the key. The file
// appears whole or not at all.
func (s *Store) Save(data []byte) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := hex.EncodeToString(raw)
	tmp, err := os.CreateTemp(s.dir, "incoming-*")
	if err != nil {
		return "", fmt.Errorf("save file: %w", err)
	}
	_, werr := tmp.Write(data)
	if serr := tmp.Sync(); werr == nil {
		werr = serr
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), filepath.Join(s.dir, key))
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("save file: %w", werr)
	}
	return key, nil
}

// Open opens the file saved under key.
func (s *Store) Open(key string) (*os.File, error) {
	if !keyPattern.MatchString(key) {
		return nil, errors.New("not a file key")
	}
	return os.Open(filepath.Join(s.dir, key))
}

// Delete removes the file saved under key; one already gone is not an error.
func (s *Store) Delete(key string) error {
	if !keyPattern.MatchString(key) {
		return errors.New("not a file key")
	}
	if err := os.Remove(filepath.Join(s.dir, key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
