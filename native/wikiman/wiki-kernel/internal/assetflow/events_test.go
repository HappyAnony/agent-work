package assetflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestOriginalAssetWatchOrderFailureAndCancellation(t *testing.T) {
	var steps []string
	h := Handler{MarkSync: func(context.Context) error { steps = append(steps, "sync"); return nil }, Refresh: func(context.Context) error { steps = append(steps, "refresh"); return nil }, Index: func(context.Context, Event) error { steps = append(steps, "index"); return nil }, InvalidateThumbnail: func(context.Context, string) error { steps = append(steps, "thumbnail"); return nil }}
	if e := Dispatch(context.Background(), Event{Path: "assets/a", Written: true}, h); e != nil || strings.Join(steps, ",") != "sync,refresh,index,thumbnail" {
		t.Fatal(steps, e)
	}
	want := errors.New("refresh")
	h.Refresh = func(context.Context) error { return want }
	steps = nil
	if e := Dispatch(context.Background(), Event{}, h); !errors.Is(e, want) || len(steps) != 0 {
		t.Fatal(steps, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := Dispatch(ctx, Event{}, h); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
