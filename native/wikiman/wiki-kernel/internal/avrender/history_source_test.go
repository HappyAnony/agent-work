package avrender

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestHistorySourceSelectionAndFailures(t *testing.T) {
	ctx := context.Background()
	missing := fs.ErrNotExist
	broken := errors.New("selected read failed")
	prefix := time.Unix(123, 0).UTC().Format("2006-01-02-150405")
	calls := 0
	p := HistorySourcePorts{Location: time.UTC, Missing: func(e error) bool { return errors.Is(e, missing) }, Directories: func(context.Context) ([]string, error) { return []string{prefix + "-z", prefix + "-a"}, nil }, History: func(_ context.Context, name string) ([]byte, error) {
		if !strings.HasPrefix(name, prefix+"-a/") {
			t.Fatal(name)
		}
		return []byte("historical"), nil
	}, Current: func(context.Context, string) ([]byte, error) { calls++; return []byte("current"), nil }}
	v, e := HistoryAttributeViewSource(ctx, "123", "id", p)
	if e != nil || string(v.Raw) != "historical" || calls != 0 {
		t.Fatal(v, e, calls)
	}
	p.History = func(context.Context, string) ([]byte, error) { return nil, missing }
	v, e = HistoryAttributeViewSource(ctx, "123", "id", p)
	if e != nil || string(v.Raw) != "current" || calls != 1 {
		t.Fatal(v, e, calls)
	}
	p.History = func(context.Context, string) ([]byte, error) { return []byte("partial"), broken }
	v, e = HistoryAttributeViewSource(ctx, "123", "id", p)
	if !errors.Is(e, broken) || v.Selected || len(v.Raw) != 0 || calls != 1 {
		t.Fatal(v, e, calls)
	}
	p.History = func(context.Context, string) ([]byte, error) { return nil, missing }
	p.Current = p.History
	v, e = HistoryAttributeViewSource(ctx, "123", "id", p)
	if e != nil || !v.Selected || !v.Missing {
		t.Fatal(v, e)
	}
	p.Directories = func(context.Context) ([]string, error) { return nil, nil }
	v, e = HistoryAttributeViewSource(ctx, "123", "id", p)
	if e != nil || v.Selected {
		t.Fatal(v, e)
	}
	if _, e = HistoryAttributeViewSource(ctx, "invalid", "id", p); e == nil {
		t.Fatal("invalid timestamp accepted")
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	v, e = HistoryAttributeViewSource(c, "123", "id", p)
	if !errors.Is(e, context.Canceled) || v.Selected {
		t.Fatal(v, e)
	}
}
