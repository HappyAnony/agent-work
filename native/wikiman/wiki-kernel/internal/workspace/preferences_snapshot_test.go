package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/sourcetx"
)

var b1Stamp = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func recordB1Evidence(t *testing.T, name string, data any) {
	t.Helper()
	dir := os.Getenv("WIKI_B1_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	raw, e := json.MarshalIndent(data, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
}

func sharedDoc(n string) conf.PreferenceDocument {
	return conf.PreferenceDocument{Version: 1, Scope: conf.StorePreference, Values: map[string]json.RawMessage{"sync.interval": json.RawMessage(n)}}
}
func snapshotManager(t *testing.T) (string, *WorkspaceManager) {
	t.Helper()
	root := fixture(t)
	return root, open(t, root, Options{Configuration: func(_, page string) (conf.AppData, error) {
		v := conf.AppData{Sync: conf.NewSync(), Search: conf.NewSearch()}
		if page == pageB {
			v.Sync.Interval = 31
		}
		return v, nil
	}})
}
func TestB1ContextRootRestoreDrainsAllPagesAndKeepsDeviceAndPagePreferences(t *testing.T) {
	ctx := context.Background()
	root, m := snapshotManager(t)
	pageDoc := conf.PreferenceDocument{Version: 1, Scope: conf.WorkspacePreference, Values: map[string]json.RawMessage{"search.limit": json.RawMessage(`55`)}}
	if _, e := m.SaveWorkspacePreferences(ctx, storeID, pageA, "b1-page", "", pageDoc); e != nil {
		t.Fatal(e)
	}
	device := conf.PreferenceDocument{Version: 1, Scope: conf.DevicePreference, Values: map[string]json.RawMessage{"lang": json.RawMessage(`"device-sentinel"`)}}
	if _, e := m.SaveDevicePreferences(ctx, storeID, "", device); e != nil {
		t.Fatal(e)
	}
	if _, e := m.SaveStorePreferences(ctx, storeID, "b1-shared-initial", "", sharedDoc("73")); e != nil {
		t.Fatal(e)
	}
	pagePath, _ := sourcetx.WorkspacePreferencesPath(pageA)
	pageBefore, _ := os.ReadFile(filepath.Join(root, pagePath))
	deviceBefore, _ := os.ReadFile(filepath.Join(root, ".local/conf/device-preferences.json"))
	bodyBefore, _ := os.ReadFile(filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj"))
	v, e := m.SnapshotStorePreferences(ctx, storeID, b1Stamp)
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(v)
	if bytes.Contains(encoded, []byte("device-sentinel")) || bytes.Contains(encoded, []byte(".local/")) {
		t.Fatal("device data entered component")
	}
	old := v.File.Hash
	if _, e = m.SaveStorePreferences(ctx, storeID, "b1-shared-change", old, sharedDoc("84")); e != nil {
		t.Fatal(e)
	}
	current, e := m.SourceFile(ctx, storeID, sourcetx.StorePreferencesPath)
	if e != nil {
		t.Fatal(e)
	}
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	oldA, oldB := a.Generation(), b.Generation()
	plan, e := m.PlanRestoreStorePreferences(ctx, storeID, v, "b1-context-restore", sourcetx.Hash(current))
	if e != nil || len(plan.ChangedPageIDs) < 2 {
		t.Fatal(plan, e)
	}
	deadline, cancel := context.WithTimeout(ctx, 15*time.Millisecond)
	defer cancel()
	if _, e = m.RestoreStorePreferences(deadline, storeID, plan); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	a.Release()
	b.Release()
	receipt, e := m.RestoreStorePreferences(ctx, storeID, plan)
	if e != nil || !receipt.SourceCommitted || len(receipt.ChangedPageIDs) < 2 {
		t.Fatal(receipt, e)
	}
	var rebuilt []map[string]any
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		data, e := l.Configuration(ctx)
		gen := l.Generation()
		l.Release()
		if e != nil || data.Sync.Interval != 73 || data.Lang != "device-sentinel" || gen == oldA || gen == oldB {
			t.Fatal(page, data, e)
		}
		if page == pageA && data.Search.Limit != 55 {
			t.Fatal("page preferences replaced")
		}
		rebuilt = append(rebuilt, map[string]any{"pageId": page, "generation": gen, "syncInterval": data.Sync.Interval, "deviceLang": data.Lang, "searchLimit": data.Search.Limit})
	}
	for file, before := range map[string][]byte{pagePath: pageBefore, ".local/conf/device-preferences.json": deviceBefore, filepath.Join("pages", pageA, "data", boxID, docID+".docj"): bodyBefore} {
		after, e := os.ReadFile(filepath.Join(root, file))
		if e != nil || !bytes.Equal(before, after) {
			t.Fatal("unrelated source/device changed", file, e)
		}
	}
	_, other := snapshotManager(t)
	l := getDocLease(t, other, pageA)
	data, e := l.Configuration(ctx)
	l.Release()
	if e != nil || data.Sync.Interval != 30 || data.Lang == "device-sentinel" {
		t.Fatal("cross-store inheritance", data, e)
	}
	recordB1Evidence(t, "context-present.json", map[string]any{"snapshotDigest": v.Digest, "restoreReceipt": receipt, "beforeGenerations": []uint64{oldA, oldB}, "rebuiltContexts": rebuilt, "unrelatedFilesUnchanged": true, "otherStoreInterval": data.Sync.Interval})
}
func TestB1ContextAbsentRestoreResumesPerPageBaseAndNoopIsNotCommit(t *testing.T) {
	ctx := context.Background()
	root, m := snapshotManager(t)
	before := digest(t, root)
	missing, e := m.SnapshotStorePreferences(ctx, storeID, b1Stamp)
	if e != nil || missing.Present || digest(t, root) != before {
		t.Fatal(missing, e)
	}
	if _, e = m.SaveStorePreferences(ctx, storeID, "b1-to-present", "", sharedDoc("73")); e != nil {
		t.Fatal(e)
	}
	current, e := m.SourceFile(ctx, storeID, sourcetx.StorePreferencesPath)
	if e != nil {
		t.Fatal(e)
	}
	plan, e := m.PlanRestoreStorePreferences(ctx, storeID, missing, "b1-to-absent", sourcetx.Hash(current))
	if e != nil {
		t.Fatal(e)
	}
	r, e := m.RestoreStorePreferences(ctx, storeID, plan)
	if e != nil || !r.SourceCommitted {
		t.Fatal(r, e)
	}
	if _, e = os.Stat(filepath.Join(root, sourcetx.StorePreferencesPath)); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	for _, item := range []struct {
		page     string
		interval int
	}{{pageA, 30}, {pageB, 31}} {
		l := getDocLease(t, m, item.page)
		data, e := l.Configuration(ctx)
		l.Release()
		if e != nil || data.Sync.Interval != item.interval {
			t.Fatal(data, e)
		}
	}
	p, e := m.PlanRestoreStorePreferences(ctx, storeID, missing, "b1-absent-noop", "")
	if e != nil || !p.NoOp {
		t.Fatal(p, e)
	}
	hash := digest(t, root)
	r, e = m.RestoreStorePreferences(ctx, storeID, p)
	if e != nil || r.SourceCommitted || r.Status != "no_op" || digest(t, root) != hash {
		t.Fatal(r, e)
	}
	recordB1Evidence(t, "context-absent.json", map[string]any{"snapshot": missing, "noOpReceipt": r, "restoredAbsent": true, "fallbackIntervals": map[string]int{pageA: 30, pageB: 31}})
}
func TestB1ContextCaptureBorrowsOneAdmissionAndCancelledWriterCannotInterleave(t *testing.T) {
	ctx := context.Background()
	root, m := snapshotManager(t)
	if _, e := m.SaveStorePreferences(ctx, storeID, "b1-consistent", "", sharedDoc("73")); e != nil {
		t.Fatal(e)
	}
	var retained context.Context
	var v sourcetx.StorePreferencesSnapshot
	e := m.WithReadContext(ctx, storeID, func(admitted context.Context) error {
		retained = admitted
		var e error
		v, e = m.SnapshotStorePreferences(admitted, storeID, b1Stamp)
		if e != nil {
			return e
		}
		writerDone := make(chan error, 1)
		go func() {
			deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
			defer cancel()
			_, e := m.SaveStorePreferences(deadline, storeID, "b1-blocked", v.File.Hash, sharedDoc("84"))
			writerDone <- e
		}()
		if e = <-writerDone; !errors.Is(e, context.DeadlineExceeded) {
			return errors.New("writer crossed capture admission")
		}
		raw, e := m.SourceFile(admitted, storeID, sourcetx.StorePreferencesPath)
		if e != nil {
			return e
		}
		if !bytes.Equal(raw, v.Data) {
			return errors.New("component and source read different commits")
		}
		plan, e := m.PlanRestoreStorePreferences(admitted, storeID, v, "b1-self-upgrade", v.File.Hash)
		if e != nil {
			return e
		}
		if _, e = m.RestoreStorePreferences(admitted, storeID, plan); !errors.Is(e, sourcetx.ErrBusy) {
			return errors.New("restore did not reject its own read admission")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.SnapshotStorePreferences(retained, storeID, b1Stamp); !errors.Is(e, ErrClosed) {
		t.Fatal("reused retired admission", e)
	}
	before := digest(t, root)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = m.SnapshotStorePreferences(ctx, storeID, b1Stamp); !errors.Is(e, context.Canceled) || digest(t, root) != before {
		t.Fatal(e)
	}
}
func TestB1ContextInvalidSnapshotAndStalePlanNeverRestore(t *testing.T) {
	ctx := context.Background()
	root, m := snapshotManager(t)
	if _, e := m.SaveStorePreferences(ctx, storeID, "b1-valid", "", sharedDoc("73")); e != nil {
		t.Fatal(e)
	}
	v, e := m.SnapshotStorePreferences(ctx, storeID, b1Stamp)
	if e != nil {
		t.Fatal(e)
	}
	plan, e := m.PlanRestoreStorePreferences(ctx, storeID, v, "b1-stale-plan", v.File.Hash)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.SaveStorePreferences(ctx, storeID, "b1-newer", v.File.Hash, sharedDoc("84")); e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	if _, e = m.RestoreStorePreferences(ctx, storeID, plan); !errors.Is(e, sourcetx.ErrConflict) || digest(t, root) != before {
		t.Fatal(e)
	}
	a := getDocLease(t, m, pageA)
	generation := a.Generation()
	bad := plan
	bad.Snapshot.Data = []byte("tampered")
	if _, e = m.RestoreStorePreferences(ctx, storeID, bad); e == nil {
		t.Fatal("accepted bad snapshot")
	}
	if a.Generation() != generation {
		t.Fatal("invalid input replaced active owner")
	}
	bad = plan
	bad.Version = 2
	deadline, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	if _, e = m.RestoreStorePreferences(deadline, storeID, bad); !errors.Is(e, sourcetx.ErrInvalid) {
		t.Fatal("malformed plan reached drain barrier", e)
	}
	a.Release()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = m.RestoreStorePreferences(cancelled, storeID, plan); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = m.RestoreStorePreferences(ctx, "00000000-0000-0000-0000-000000000001", plan); !errors.Is(e, sourcetx.ErrIdentity) {
		t.Fatal(e)
	}
}
func TestB1ContextInterruptedRestoreRequiresRecoveryBeforeCapture(t *testing.T) {
	ctx := context.Background()
	_, m := snapshotManager(t)
	if _, e := m.SaveStorePreferences(ctx, storeID, "b1-start", "", sharedDoc("73")); e != nil {
		t.Fatal(e)
	}
	v, e := m.SnapshotStorePreferences(ctx, storeID, b1Stamp)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.SaveStorePreferences(ctx, storeID, "b1-current", v.File.Hash, sharedDoc("84")); e != nil {
		t.Fatal(e)
	}
	current, e := m.SourceFile(ctx, storeID, sourcetx.StorePreferencesPath)
	if e != nil {
		t.Fatal(e)
	}
	plan, e := m.PlanRestoreStorePreferences(ctx, storeID, v, "b1-context-interrupted", sourcetx.Hash(current))
	if e != nil {
		t.Fatal(e)
	}
	e = m.mutate(ctx, storeID, false, func(w *sourcetx.Writer) error {
		_, e := w.RestoreStorePreferences(ctx, plan, func(point string) error {
			if point == "published:0" {
				return errors.New("interrupted")
			}
			return nil
		})
		return e
	})
	if e == nil {
		t.Fatal("missing fault")
	}
	if _, e = m.SnapshotStorePreferences(ctx, storeID, b1Stamp); !errors.Is(e, sourcetx.ErrRecovery) {
		t.Fatal(e)
	}
	if _, e = m.Recover(ctx, storeID); e != nil {
		t.Fatal(e)
	}
	l := getDocLease(t, m, pageA)
	data, e := l.Configuration(ctx)
	l.Release()
	if e != nil || data.Sync.Interval != 84 {
		t.Fatal("recovery did not restore prior configuration", data, e)
	}
	plan.OperationID = "b1-context-after-recovery"
	if r, e := m.RestoreStorePreferences(ctx, storeID, plan); e != nil || !r.SourceCommitted {
		t.Fatal(r, e)
	}
	recordB1Evidence(t, "context-recovery.json", map[string]any{"snapshotDigest": v.Digest, "interruptionPoint": "published:0", "captureDeniedUntilRecovery": true, "recoveredInterval": data.Sync.Interval, "retryOperationId": plan.OperationID})
}
