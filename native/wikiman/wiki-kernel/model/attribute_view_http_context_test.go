//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"
	"wiki-kernel/av"
)

func TestOwnedAVControllersReadAndMutateAcrossLayouts(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			c, _, a, file := deepAVFixture(t, layout)
			before, _ := os.ReadFile(file)
			c.r.owner.persistenceReadOnly = true
			got, err := GetAttributeViewContext(context.Background(), a.ID)
			if err != nil || got.Name != a.Name {
				t.Fatalf("read: %v %#v", err, got)
			}
			keys, err := AttributeViewKeysContext(context.Background(), a.ID, "ids", "", nil)
			if err != nil || len(keys) != 2 {
				t.Fatalf("keys: %v %#v", err, keys)
			}
			if _, err = AttributeViewMirrorBlocksContext(context.Background(), a.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = AttributeViewIdentitiesContext(context.Background(), a.ID, []string{"20261028140002-refdef1"}, true); err != nil {
				t.Fatal(err)
			}
			if err = AttributeViewOperationContext(context.Background(), &Operation{Action: "addAttrViewCol", AvID: a.ID, ID: "newkey", Name: "Added", Typ: "text"}); err == nil {
				t.Fatal("read-only mutation admitted")
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("read-only query changed source")
			}
			c.r.owner.persistenceReadOnly = false
			if err = AttributeViewOperationContext(context.Background(), &Operation{Action: "addAttrViewCol", AvID: a.ID, ID: "newkey", Name: "Added", Typ: "text"}); err != nil {
				t.Fatal(err)
			}
			keys, err = AttributeViewKeysContext(context.Background(), a.ID, "ids", "", nil)
			if err != nil || len(keys) != 3 {
				t.Fatalf("published key: %v %#v", err, keys)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if got, err = GetAttributeViewContext(ctx, a.ID); !errors.Is(err, context.Canceled) || got != nil {
				t.Fatalf("cancel: %v %#v", err, got)
			}
		})
	}
}

func TestOwnedAVControllerCloseJoinsActualWorker(t *testing.T) {
	c, _, _, _ := deepAVFixture(t, av.LayoutTypeTable)
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := runOwnedAV(context.Background(), false, func(ctx context.Context, _ *legacyAVBinding) (int, error) {
			close(started)
			<-release
			return 1, ctx.Err()
		})
		done <- err
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := c.r.owner.CloseCloudRepo(ctx); !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatalf("close did not join worker: %v", err)
	}
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := c.r.owner.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
}
