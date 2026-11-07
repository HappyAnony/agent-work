package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/sourcetx"
)

func TestStorePreferencesDrainAllPagesAndDoNotCrossStores(t *testing.T) {
	ctx := context.Background()
	m := open(t, fixture(t), Options{Configuration: func(string, string) (conf.AppData, error) { return conf.AppData{Sync: conf.NewSync()}, nil }})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	oldA, oldB := a.workspace.generation, b.workspace.generation
	doc := conf.PreferenceDocument{Version: 1, Scope: conf.StorePreference, Values: map[string]json.RawMessage{"sync.interval": json.RawMessage(`73`)}}
	a.Release()
	deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, err = m.SaveStorePreferences(deadline, storeID, "store-busy", "", doc); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	b.Release()
	receipt, err := m.SaveStorePreferences(ctx, storeID, "store-save", "", doc)
	if err != nil || !receipt.SourceCommitted {
		t.Fatal(receipt, err)
	}
	raw, err := m.SourceFile(ctx, storeID, sourcetx.StorePreferencesPath)
	if err != nil || len(raw) == 0 {
		t.Fatal(err)
	}
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		data, err := l.Configuration(ctx)
		generation := l.workspace.generation
		l.Release()
		if err != nil || data.Sync.Interval != 73 || generation == oldA || generation == oldB {
			t.Fatal(page, data, generation, err)
		}
	}
	// A fresh independent manager/store has no inherited global preference.
	other := open(t, fixture(t), Options{Configuration: func(string, string) (conf.AppData, error) { return conf.AppData{Sync: conf.NewSync()}, nil }})
	l, err := other.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	data, err := l.Configuration(ctx)
	l.Release()
	if err != nil || data.Sync.Interval != 30 {
		t.Fatal(data, err)
	}
}
