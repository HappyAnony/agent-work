package sourcetx

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wiki-kernel/conf"
)

func TestStorePreferencesJournalCASRecoveryAndClassification(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	s, w := open(t, root)
	base := conf.AppData{Sync: conf.NewSync(), Repo: conf.NewRepo()}
	doc := conf.PreferenceDocument{Version: 1, Scope: conf.StorePreference, Values: map[string]json.RawMessage{"sync.interval": json.RawMessage(`71`)}}
	req, err := StorePreferencesRequest("store-interrupted", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := w.Preview(ctx, req)
	if err != nil || preview.SourceCommitted || len(preview.ChangedPageIDs) == 0 {
		t.Fatal(preview, err)
	}
	if _, err = os.Stat(filepath.Join(root, ".wiki")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preview created source", err)
	}
	_, err = w.Apply(ctx, req, func(point string) error {
		if point == "published:0" {
			return errors.New("synthetic interruption")
		}
		return nil
	})
	if err == nil {
		t.Fatal("missing interruption")
	}
	if _, err = s.Configuration(ctx, p2, base); !errors.Is(err, ErrRecovery) {
		t.Fatal(err)
	}
	recovered, err := w.Recover(ctx, nil)
	if err != nil || len(recovered) != 1 || recovered[0].SourceCommitted {
		t.Fatal(recovered, err)
	}
	if _, err = os.Stat(filepath.Join(root, StorePreferencesPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rollback retained source", err)
	}
	req.OperationID = "store-committed"
	receipt, err := w.Apply(ctx, req, nil)
	if err != nil || !receipt.SourceCommitted || !reflect.DeepEqual(receipt.ChangedPageIDs, preview.ChangedPageIDs) {
		t.Fatal(receipt, err)
	}
	for _, page := range []string{p1, p2} {
		data, err := s.Configuration(ctx, page, base)
		if err != nil || data.Sync.Interval != 71 {
			t.Fatal(page, data, err)
		}
	}
	if _, err = w.Apply(ctx, Request{OperationID: "store-stale", Changes: req.Changes}, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	raw, err := s.SourceFile(ctx, StorePreferencesPath)
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte(`{"version":1,"scope":"store","values":{"repo.key":"secret"}}`)
	if _, err = w.Apply(ctx, Request{OperationID: "store-smuggled", Changes: []Change{{Path: StorePreferencesPath, ExpectedHash: Hash(raw), Data: bad}}}, nil); !errors.Is(err, conf.ErrPreferences) {
		t.Fatal(err)
	}
	// Committed replay/recovery never resets a later source value.
	again, err := w.Apply(ctx, req, nil)
	if err != nil || again.OperationID != receipt.OperationID {
		t.Fatal(again, err)
	}
	if _, err = w.Recover(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Apply(ctx, Request{OperationID: "store-remove", Changes: []Change{{Path: StorePreferencesPath, ExpectedHash: Hash(raw), Delete: true}}}, nil); err != nil {
		t.Fatal(err)
	}
	data, err := s.Configuration(ctx, p2, base)
	if err != nil || data.Sync.Interval != base.Sync.Interval {
		t.Fatal(data, err)
	}
	// Root whitelist is exact; no arbitrary configuration or identity replacement.
	for _, name := range []string{".wiki/other.json", ".wiki/../store.json", "store.json", ".local/conf/device-preferences.json"} {
		if sourcePath(name) {
			t.Fatal(name)
		}
	}
}

func TestStorePreferencesRejectRootSymlinkAndMalformedSource(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	s, w := open(t, root)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".wiki")); err != nil {
		t.Fatal(err)
	}
	doc := conf.PreferenceDocument{Version: 1, Scope: conf.StorePreference, Values: map[string]json.RawMessage{"sync.interval": json.RawMessage(`71`)}}
	req, err := StorePreferencesRequest("store-symlink", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Apply(ctx, req, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("source symlink escaped", entries, err)
	}
	// Test-owned symlink only; replace it with deliberately malformed source.
	if err = os.Remove(filepath.Join(root, ".wiki")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, ".wiki"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, StorePreferencesPath), []byte(`{"version":1,"scope":"device","values":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{p1, p2} {
		if _, err = s.Configuration(ctx, page, conf.AppData{}); !errors.Is(err, conf.ErrPreferences) {
			t.Fatal("bad shared document silently ignored", page, err)
		}
	}
}
