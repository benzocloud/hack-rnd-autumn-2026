package httpapi

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const importFileLimit int64 = 100 << 20
const importEnvelopeLimit int64 = 101 << 20
const importSpoolPrefix = "saldo-import-"

// ImportSpool owns a dedicated private disk directory, with one instance per
// application. Reserve the maximum file size before reading an untrusted body.
// Failed unlink leaves the file for startup scavenging and releases the slot.
type ImportSpool struct {
	root  string
	mu    sync.Mutex
	slots int
}

// NewImportSpool checks its private directory and removes this application's
// orphan uploads at startup, before the application accepts any requests.
func NewImportSpool(root string) (*ImportSpool, error) {
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("absolute import spool directory required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, errors.New("cannot create import spool")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return nil, errors.New("import spool requires a private directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, errors.New("cannot inspect import spool")
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), importSpoolPrefix) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil, errors.New("unsafe import spool entry")
		}
		if err := os.Remove(filepath.Join(root, entry.Name())); err != nil {
			return nil, errors.New("cannot clean import spool")
		}
	}
	return &ImportSpool{root: root}, nil
}

func (s *ImportSpool) acquire() (*os.File, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slots >= 2 {
		return nil, false, nil
	}
	file, err := os.CreateTemp(s.root, importSpoolPrefix)
	if err != nil {
		return nil, false, errors.New("cannot allocate import spool")
	}
	s.slots++
	return file, true, nil
}

func (s *ImportSpool) release(file *os.File) error {
	closeErr := file.Close()
	err := os.Remove(file.Name())
	s.mu.Lock()
	s.slots--
	s.mu.Unlock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot clean upload spool")
	}
	return closeErr
}
