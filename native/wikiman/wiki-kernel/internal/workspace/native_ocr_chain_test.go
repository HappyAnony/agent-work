//go:build fts5

package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/ocr"
)

func TestOriginalOCRLifecycleActualWatcherNotificationsAndOwner(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "first OCR")
	ownedAssets(t, root, pageB, "B OCR")
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	a, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Release()
	b, e := m.Acquire(ctx, storeID, pageB)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Release()
	op := ocr.Defaults()
	op.Candidates = []string{ownedOCRBinary(t, false)}
	// Queue without an implicit OCR bootstrap, as the original disabled policy.
	if e = a.ConfigureNativeOCR(ctx, op); e != nil {
		t.Fatal(e)
	}
	if e = a.QueueNativeTree(ctx, boxID, docID, "index"); e != nil {
		t.Fatal(e)
	}
	if e = a.FlushNativeMainQueue(ctx); e != nil {
		t.Fatal(e)
	}
	messages := make(chan string, 30)
	events := make(chan IndexEvent, 30)
	var other atomic.Int32
	sub, e := a.SubscribeOriginalOCRStatus(ctx, func(event IndexEvent, msg string) {
		if event.PageID != pageA || event.StoreID != storeID {
			t.Error(event)
		}
		if _, e := a.ReadDocument(ctx, boxID, docID); e != nil {
			t.Error("notification held gate", e)
		}
		messages <- msg
	})
	if e != nil {
		t.Fatal(e)
	}
	defer sub.Close(ctx)
	subB, e := b.SubscribeOriginalOCRStatus(ctx, func(IndexEvent, string) { other.Add(1) })
	if e != nil {
		t.Fatal(e)
	}
	defer subB.Close(ctx)
	changes, e := a.SubscribeIndexEvents(ctx, func(ev IndexEvent) {
		if ev.Kind == "asset-source-changed" || ev.Kind == "ocr-ready" {
			events <- ev
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	defer changes.Close(ctx)
	task, e := a.StartNativeOCRLifecycle(ctx, op, 30*time.Millisecond, 10*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		task.Cancel()
		if e := task.Wait(ctx); e != nil && !errors.Is(e, context.Canceled) {
			t.Error(e)
		}
	}()
	wait := func(ch <-chan string, word string) {
		t.Helper()
		timer := time.NewTimer(8 * time.Second)
		defer timer.Stop()
		for {
			select {
			case s := <-ch:
				if strings.Contains(s, word) {
					return
				}
			case <-timer.C:
				t.Fatal("missing OCR notification", word)
			}
		}
	}
	wait(messages, "first OCR")
	file := filepath.Join(root, "pages", pageA, "data", "assets", "a.png")
	if e = os.WriteFile(file, []byte("changed OCR <safe>"), 0600); e != nil {
		t.Fatal(e)
	}
	wait(messages, "changed OCR &lt;safe&gt;")
	deadline := time.Now().Add(4 * time.Second)
	for {
		v, _, e := a.NativeBlock(ctx, blockID)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(v.Content, "changed OCR <safe>") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watch OCR did not update FTS", v)
		}
		time.Sleep(time.Millisecond)
	}
	if other.Load() != 0 {
		t.Fatal("OCR notification crossed owner")
	}
	duplicate, e := a.StartNativeOCRLifecycle(ctx, op, time.Second, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if e = duplicate.Wait(ctx); e == nil || !strings.Contains(e.Error(), "ocr_lifecycle_active") {
		t.Fatal("duplicate accepted", e)
	}
	if e = a.HandleNativeAssetEvent(ctx, assetflow.Event{Path: "../escape", Written: true}); !errors.Is(e, ErrDocument) {
		t.Fatal(e)
	}
	task.Cancel()
	if e = task.Wait(ctx); e != nil && !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = os.Remove(file); e != nil {
		t.Fatal(e)
	}
	if e = a.HandleNativeAssetEvent(ctx, assetflow.Event{Path: "assets/a.png", Removed: true}); e != nil {
		t.Fatal(e)
	}
	if e = a.RunNativeOCR(ctx); e != nil {
		t.Fatal(e)
	}
	if e = a.FlushNativeMainQueue(ctx); e != nil {
		t.Fatal(e)
	}
	v, _, e := a.NativeBlock(ctx, blockID)
	if e != nil || strings.Contains(v.Content, "changed OCR") {
		t.Fatal("removed asset retained OCR", v, e)
	}
}

func TestOCRLifecycleCancelJoinsChildWatcherAndRetainsOwnerUntilNotificationReturns(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "image")
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	a, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Release()
	op := ocr.Defaults()
	binary := ownedOCRBinary(t, true)
	op.Candidates = []string{binary}
	op.Timeout = time.Minute
	task, e := a.StartNativeOCRLifecycle(ctx, op, time.Millisecond, 10*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e = os.Stat(binary + ".started"); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child not started")
		}
		time.Sleep(time.Millisecond)
	}
	if _, e = a.ReadDocument(ctx, boxID, docID); e != nil {
		t.Fatal(e)
	}
	task.Cancel()
	wait, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if e = task.Wait(wait); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if a.workspace.resources.ocrLifecycle {
		t.Fatal("lifecycle still registered")
	}
	if digest(t, root) != before {
		t.Fatal("cancel changed source")
	}
	// Notification execution remains owned after cancellation. A timeout cannot
	// falsely complete a callback or allow its resources to be closed.
	op.Candidates = []string{ownedOCRBinary(t, false)}
	entered, release := make(chan struct{}), make(chan struct{})
	sub, e := a.SubscribeOriginalOCRStatus(ctx, func(IndexEvent, string) { close(entered); <-release })
	if e != nil {
		t.Fatal(e)
	}
	task, e = a.StartNativeOCRLifecycle(ctx, op, time.Millisecond, 10*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("notification not entered")
	}
	task.Cancel()
	short, stop := context.WithTimeout(ctx, time.Millisecond)
	defer stop()
	if e = task.Wait(short); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("callback not pinned", e)
	}
	if e = sub.Close(short); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	close(release)
	if e = task.Wait(ctx); e != nil && !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = sub.Close(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestOCRLifecycleBootstrapFailureAndThumbnailConfinement(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "image")
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	a, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Release()
	op := ocr.Defaults()
	op.Candidates = []string{filepath.Join(t.TempDir(), "missing")}
	task, e := a.StartNativeOCRLifecycle(ctx, op, time.Millisecond, time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	if e = task.Wait(ctx); e == nil {
		t.Fatal("missing engine accepted")
	}
	if a.workspace.resources.ocrLifecycle {
		t.Fatal("failed bootstrap retained owner flag")
	}
	op.Enabled = false
	if e = a.ConfigureNativeOCR(ctx, op); e != nil {
		t.Fatal(e)
	}
	thumbnail := filepath.Join(root, ".local", "workspaces", pageA, "temp", "thumbnails", "assets", "a.png")
	if e = os.MkdirAll(filepath.Dir(thumbnail), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(thumbnail, []byte("derived thumbnail"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = a.HandleNativeAssetEvent(ctx, assetflow.Event{Path: "assets/a.png", Written: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(thumbnail); !os.IsNotExist(e) {
		t.Fatal("thumbnail retained", e)
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	if e = os.WriteFile(foreign, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(foreign, thumbnail); e != nil {
		t.Fatal(e)
	}
	if e = a.HandleNativeAssetEvent(ctx, assetflow.Event{Path: "assets/a.png", Written: true}); !errors.Is(e, ErrDocument) {
		t.Fatal("foreign thumbnail accepted", e)
	}
	raw, e := os.ReadFile(foreign)
	if e != nil || string(raw) != "keep" {
		t.Fatal(string(raw), e)
	}
}
