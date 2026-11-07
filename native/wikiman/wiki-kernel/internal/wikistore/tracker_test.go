package wikistore

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func TestTrackerRescansWithoutEventsAndNeverPublishesInvalidSubset(t *testing.T) {
	r := fixture(t)
	tracker := NewTracker(r, Scanner{})
	if _, err := tracker.View(); !errors.Is(err, ErrTreeDirty) {
		t.Fatal(err)
	}
	a, err := tracker.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := tracker.Refresh(context.Background())
	if err != nil || a != b {
		t.Fatalf("unchanged snapshot: %v", err)
	}
	editPage(t, r, rootPage, func(p map[string]any) { p["name"] = "changed without watcher" })
	c, err := tracker.Refresh(context.Background())
	if err != nil || c.SourceDigest() == a.SourceDigest() {
		t.Fatal("event loss hid change")
	}
	tracker.Invalidate()
	tracker.Invalidate()
	if _, err := tracker.View(); !errors.Is(err, ErrTreeDirty) {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath(r, rootPage), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if next, err := tracker.Refresh(context.Background()); next != nil || err == nil {
		t.Fatal("published invalid tree")
	}
	if next, err := tracker.View(); next != nil || err == nil {
		t.Fatal("stale tree reported current")
	}
}

func TestWatcherOnlyInvalidatesUntilExplicitRefresh(t *testing.T) {
	r := fixture(t)
	tracker := NewTracker(r, Scanner{})
	if _, err := tracker.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watch, err := tracker.StartWatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	editPage(t, r, rootPage, func(p map[string]any) { p["name"] = "watched change" })
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := tracker.View()
		if errors.Is(err, ErrTreeDirty) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watcher did not invalidate")
		}
		time.Sleep(10 * time.Millisecond)
	}
	next, err := tracker.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := next.Get(rootPage)
	if p.Name != "watched change" {
		t.Fatal("watcher guessed wrong source")
	}
}

func TestConcurrentReadersAndInvalidation(t *testing.T) {
	r := fixture(t)
	tracker := NewTracker(r, Scanner{})
	if _, err := tracker.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 20; j++ {
				tracker.Invalidate()
				_, _ = tracker.View()
				_, err := tracker.Refresh(context.Background())
				if err != nil {
					var failure *ScanError
					if !errors.As(err, &failure) || failure.Type != "source_unstable" {
						t.Error(err)
					}
				}
			}
		}()
	}
	group.Wait()
	if _, err := tracker.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}
