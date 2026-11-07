// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

const incrementalBox = "20261029170000-boxaaaa"
const incrementalID = "20261029170001-docaaaa"

func TestIncrementalOriginalOrderPathsAndNoPartialResult(t *testing.T) {
	ctx := context.Background()
	p := "/" + incrementalBox + "/" + incrementalID + ".docj"
	var order []string
	s := IncrementalServices{Remove: func(_ context.Context, p DocumentPath) error { order = append(order, "remove:"+p.ID); return nil }, Upsert: func(_ context.Context, p DocumentPath) error { order = append(order, "upsert:"+p.ID); return nil }, Flush: func(context.Context) error { order = append(order, "flush"); return nil }}
	out, e := Incremental(ctx, []string{p, "/assets/attached.docj", "/" + incrementalBox + "/assets/attached.docj", "/" + incrementalBox + "/.siyuan/history/old.docj", "/storage/metadata.json", "orphan.docj"}, []string{p}, s)
	if e != nil || !reflect.DeepEqual(order, []string{"remove:" + incrementalID, "flush", "upsert:" + incrementalID, "flush"}) || !reflect.DeepEqual(out.UpsertRootIDs, []string{incrementalID}) || !reflect.DeepEqual(out.RemoveRootIDs, []string{incrementalID}) {
		t.Fatal(out, order, e)
	}
	for _, bad := range []string{"/" + incrementalBox + "/../" + incrementalID + ".docj", "//" + incrementalBox + "/" + incrementalID + ".docj", "/" + incrementalBox + "/bad.docj", "/" + incrementalBox + "/bad/" + incrementalID + ".docj"} {
		order = nil
		out, e = Incremental(ctx, []string{p, bad}, []string{p}, s)
		if !errors.Is(e, ErrSourcePath) || len(order) > 0 || out.UpsertRootIDs != nil {
			t.Fatal(bad, order, out, e)
		}
	}
	if _, e = Incremental(ctx, nil, nil, IncrementalServices{}); !errors.Is(e, ErrIncrementalDependency) {
		t.Fatal(e)
	}
}
func TestIncrementalFailureCancelPanicDrainsAcceptedWork(t *testing.T) {
	for _, mode := range []string{"error", "cancel", "panic", "flush-error", "flush-panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("operation failed")
			flushFailure := errors.New("drain failed")
			upserts, flushed := 0, 0
			s := IncrementalServices{Remove: func(context.Context, DocumentPath) error {
				switch mode {
				case "error":
					return failure
				case "cancel":
					cancel()
				case "panic":
					panic("failed")
				}
				return nil
			}, Upsert: func(context.Context, DocumentPath) error { upserts++; return nil }, Flush: func(c context.Context) error {
				flushed++
				if c.Err() != nil {
					t.Error("drain was canceled")
				}
				switch mode {
				case "error", "flush-error":
					return flushFailure
				case "flush-panic":
					panic("drain")
				}
				return nil
			}}
			p := incrementalBox + "/" + incrementalID + ".docj"
			out, e := Incremental(ctx, []string{p}, []string{p, p}, s)
			if e == nil || out.RemoveRootIDs != nil || out.UpsertRootIDs != nil || flushed != 1 || upserts != 0 {
				t.Fatal(mode, out, e, flushed, upserts)
			}
			if mode == "error" && (!errors.Is(e, failure) || !errors.Is(e, flushFailure)) {
				t.Fatal("lost failure", e)
			}
			if mode == "cancel" && !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
		})
	}
}
func TestIncrementalCanceledBeforeAdmissionDoesNotDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	s := IncrementalServices{Remove: func(context.Context, DocumentPath) error { called = true; return nil }, Upsert: func(context.Context, DocumentPath) error { called = true; return nil }, Flush: func(context.Context) error { called = true; return nil }}
	if _, e := Incremental(ctx, nil, nil, s); !errors.Is(e, context.Canceled) || called {
		t.Fatal(e, called)
	}
}
