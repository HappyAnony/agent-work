package riff_test

import (
	"github.com/open-spaced-repetition/go-fsrs/v3"
	upstream "github.com/siyuan-note/riff"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

type disk string

func (d disk) Read(p string) ([]byte, error) { return os.ReadFile(filepath.Join(string(d), p)) }
func (d disk) Write(p string, b []byte) error {
	return os.WriteFile(filepath.Join(string(d), p), b, 0600)
}
func TestOriginalMessagePackBidirectionalCompatibility(t *testing.T) {
	dir := t.TempDir()
	c := conf.NewFlashcard()
	id := "20261027000000-deck001"
	cid := "20261027000000-card001"
	bid := "20261027000000-block01"
	old, e := upstream.LoadDeck(dir, id, c.RequestRetention, c.MaximumInterval, c.Weights)
	if e != nil {
		t.Fatal(e)
	}
	old.Name = "Original UI deck"
	old.AddCard(cid, bid)
	if e = old.Save(); e != nil {
		t.Fatal(e)
	}
	d, e := riff.LoadDeck(disk(dir), id, c.RequestRetention, c.MaximumInterval, c.Weights, time.Now)
	if e != nil || d.Name != old.Name || d.GetCard(cid).BlockID() != bid {
		t.Fatal(d, e)
	}
	d.Name = "Context deck"
	due := time.Now().Add(time.Hour).Truncate(time.Second)
	d.GetCard(cid).SetDue(due)
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	old, e = upstream.LoadDeck(dir, id, c.RequestRetention, c.MaximumInterval, c.Weights)
	if e != nil || old.Name != "Context deck" || !old.GetCard(cid).Impl().(*fsrs.Card).Due.Equal(due) {
		t.Fatal(old, e)
	}
}
