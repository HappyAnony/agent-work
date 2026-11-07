package model

import (
	"github.com/siyuan-note/filelock"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	"wiki-kernel/internal/flashcard/riff"
)

// Only the single-workspace compatibility adapter resolves physical paths.
// Native owners supply pinned reads and candidate writes to the same algorithm.
type legacyFlashcardStorage struct{ dir string }

func (s legacyFlashcardStorage) Read(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return filelock.ReadFile(filepath.Join(s.dir, filepath.FromSlash(name)))
}
func (s legacyFlashcardStorage) Write(name string, data []byte) error {
	if !fs.ValidPath(name) {
		return fs.ErrInvalid
	}
	p := filepath.Join(s.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return filelock.WriteFile(p, data)
}
func loadLegacyFlashcardDeck(dir, id string, retention float64, interval int, weights string) (*riff.Deck, error) {
	return riff.LoadDeck(legacyFlashcardStorage{dir}, id, retention, interval, weights, time.Now)
}
