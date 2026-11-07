package pageprefs

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"time"
)

func TestStorageOriginalRulesActualBackendAndFailure(t *testing.T) {
	files := map[string][]byte{}
	writes := 0
	failure := false
	b := Backend{Now: func() time.Time { return time.Unix(99, 0) }, RefreshRecent: true, Paths: func(context.Context, []string) (map[string]string, error) {
		return map[string]string{"a": "/Updated A", "b": "/B"}, nil
	}, Read: func(_ context.Context, n string) ([]byte, error) {
		v, ok := files[n]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return append([]byte(nil), v...), nil
	}, Write: func(_ context.Context, n string, v []byte) error {
		if failure {
			return errors.New("write failed")
		}
		files[n] = append([]byte(nil), v...)
		writes++
		return nil
	}}
	s := &Service{}
	do := func(a, k string, v any) any {
		raw, _ := json.Marshal(v)
		out, e := s.Do(context.Background(), Request{Action: a, Key: k, Value: raw}, b)
		if e != nil {
			t.Fatal(a, e)
		}
		return out
	}
	do("criteria.set", "", Criterion{Name: "saved", K: "needle", Sort: 5})
	got := do("criteria.get", "", nil).([]*Criterion)
	if len(got) != 1 || got[0].K != "needle" {
		t.Fatal(got)
	}
	do("criteria.set", "", Criterion{Name: "saved", K: "replacement"})
	if len(do("criteria.get", "", nil).([]*Criterion)) != 1 {
		t.Fatal("duplicate criterion")
	}
	do("local.set", "", map[string]any{"old": true})
	do("local.setValue", "new", 42)
	if !reflect.DeepEqual(do("local.get", "", nil), map[string]any{"old": true, "new": float64(42)}) {
		t.Fatal("local merge")
	}
	do("outline.set", "a", map[string]any{"folded": true})
	do("outline.set", "b", map[string]any{"folded": false})
	do("outline.set", "a", map[string]any{"folded": false})
	var outlines []*OutlineDoc
	if e := json.Unmarshal(files["outline.json"], &outlines); e != nil || len(outlines) != 2 || outlines[0].DocID != "a" {
		t.Fatal(outlines, e)
	}
	do("recent.record", "", RecentDoc{RootID: "a", Title: "A"})
	do("recent.record", "", RecentDoc{RootID: "b", Title: "B"})
	do("recent.close", "a", nil)
	gotRecent, e := s.Do(context.Background(), Request{Action: "recent.get", SortBy: "closedAt"}, b)
	if e != nil {
		t.Fatal(e)
	}
	docs := gotRecent.([]*RecentDoc)
	if docs[0].RootID != "a" || docs[0].Title != "Updated A" || docs[0].ClosedAt != 99 {
		t.Fatal(docs)
	}
	if _, e = s.Do(context.Background(), Request{Action: "recent.remove", Keys: []string{"a", "b"}}, b); e != nil {
		t.Fatal(e)
	}
	docs = do("recent.get", "", nil).([]*RecentDoc)
	if len(docs) != 0 {
		t.Fatal("bulk remove missed entries")
	}
	failure = true
	before := writes
	if _, e = s.Do(context.Background(), Request{Action: "criteria.remove", Key: "saved"}, b); e == nil || writes != before {
		t.Fatal("write error hidden")
	}
	files["local.json"] = []byte("{bad")
	if _, e = s.Do(context.Background(), Request{Action: "local.get"}, b); e == nil {
		t.Fatal("corruption hidden")
	}
}

func TestStorageCancellationWhileWaitingForOwnerGate(t *testing.T) {
	s := &Service{}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	b := Backend{Now: time.Now, Read: func(context.Context, string) ([]byte, error) { close(entered); <-release; return []byte("{}"), nil }, Write: func(context.Context, string, []byte) error { return nil }}
	go func() { _, e := s.Do(context.Background(), Request{Action: "local.get"}, b); done <- e }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.Do(ctx, Request{Action: "local.get"}, b); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
