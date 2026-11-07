package sourcetx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"wiki-kernel/conf"
)

var snapshotStamp = time.Date(2026, 10, 27, 12, 0, 0, 123, time.UTC)

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

func writeShared(t *testing.T, root string, raw []byte) {
	t.Helper()
	if e := os.MkdirAll(filepath.Join(root, ".wiki"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, StorePreferencesPath), raw, 0600); e != nil {
		t.Fatal(e)
	}
}
func resealSnapshot(t *testing.T, v StorePreferencesSnapshot) StorePreferencesSnapshot {
	t.Helper()
	v.File.Size = len(v.Data)
	if v.Present {
		v.File.Hash = Hash(v.Data)
	}
	var e error
	v.Digest, e = v.digest()
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestB1RootPreferenceCaptureExactBytesAbsenceAndStrictEnvelope(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	s, _ := open(t, root)
	before := sourceHash(t, root)
	absent, e := s.SnapshotStorePreferences(ctx, snapshotStamp)
	if e != nil || absent.Present || absent.Data != nil || absent.File.Hash != "" {
		t.Fatal(absent, e)
	}
	if sourceHash(t, root) != before {
		t.Fatal("missing capture created source")
	}
	if _, e = os.Stat(filepath.Join(root, ".wiki")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	raw := []byte("{\n \"version\":1, \"scope\":\"store\", \"values\":{\"repo.syncIndexTiming\":9007199254740993,\"sync.interval\":73}\n}\n")
	writeShared(t, root, raw)
	before = sourceHash(t, root)
	v, e := s.SnapshotStorePreferences(ctx, snapshotStamp)
	if e != nil || !bytes.Equal(v.Data, raw) || v.File.Hash != Hash(raw) || v.File.Path != StorePreferencesPath {
		t.Fatal(v, e)
	}
	again, e := s.SnapshotStorePreferences(ctx, snapshotStamp)
	if e != nil || again.Digest != v.Digest {
		t.Fatal(e)
	}
	wire, _ := json.Marshal(v)
	decoded, e := DecodeStorePreferencesSnapshot(wire)
	if e != nil || !bytes.Equal(decoded.Data, raw) {
		t.Fatal(e)
	}
	decoded.Data[0] = 'x'
	if !bytes.Equal(v.Data, raw) || sourceHash(t, root) != before {
		t.Fatal("snapshot aliases source")
	}
	wire, _ = json.Marshal(absent)
	for _, bad := range [][]byte{bytes.Replace(wire, []byte(`"present":false`), []byte(`"present":null`), 1), bytes.Replace(wire, []byte(`"size":0`), []byte(`"size":null`), 1), bytes.Replace(wire, []byte(`"present":false,`), nil, 1), bytes.Replace(wire, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1)} {
		if _, e := DecodeStorePreferencesSnapshot(bad); e == nil {
			t.Fatal("accepted malformed presence/duplicate envelope")
		}
	}
	recordB1Evidence(t, "source-capture.json", map[string]any{"presentSnapshot": v, "absentSnapshot": absent, "sourceUnchanged": sourceHash(t, root) == before, "rawBigIntegerPreserved": bytes.Contains(v.Data, []byte("9007199254740993"))})
}
func TestB1RootPreferenceRestoreExactCASReplayAndAbsence(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	s, w := open(t, root)
	absent, e := s.SnapshotStorePreferences(ctx, snapshotStamp)
	if e != nil {
		t.Fatal(e)
	}
	raw := []byte(`{ "version":1,"scope":"store","values":{"sync.interval":73}}`)
	writeShared(t, root, raw)
	saved, e := s.SnapshotStorePreferences(ctx, snapshotStamp)
	if e != nil {
		t.Fatal(e)
	}
	current := []byte(`{"version":1,"scope":"store","values":{"sync.interval":84}}`)
	writeShared(t, root, current)
	before := sourceHash(t, root)
	plan, e := s.PlanRestoreStorePreferences(ctx, saved, "b1-exact", Hash(current))
	if e != nil || plan.NoOp || len(plan.ChangedPageIDs) < 2 || sourceHash(t, root) != before {
		t.Fatal(plan, e)
	}
	if e = s.ReadContext(ctx, func(admitted context.Context) error {
		_, e := w.RestoreStorePreferences(admitted, plan, nil)
		if !errors.Is(e, ErrBusy) {
			t.Fatal("restore upgraded its own read admission", e)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	r, e := w.RestoreStorePreferences(ctx, plan, nil)
	if e != nil || !r.SourceCommitted || !reflect.DeepEqual(r.ChangedPageIDs, plan.ChangedPageIDs) || !bytes.Equal(bytesAt(t, root, StorePreferencesPath), raw) {
		t.Fatal(r, e)
	}
	// Replay returns the original receipt rather than resetting a later value.
	change, _ := StorePreferencesRequest("b1-later", Hash(raw), conf.PreferenceDocument{Version: 1, Scope: conf.StorePreference, Values: map[string]json.RawMessage{"sync.interval": json.RawMessage(`95`)}})
	if _, e = w.Apply(ctx, change, nil); e != nil {
		t.Fatal(e)
	}
	later := bytesAt(t, root, StorePreferencesPath)
	again, e := w.RestoreStorePreferences(ctx, plan, nil)
	if e != nil || again.RequestHash != r.RequestHash || !bytes.Equal(later, bytesAt(t, root, StorePreferencesPath)) {
		t.Fatal(again, e)
	}
	remove, e := s.PlanRestoreStorePreferences(ctx, absent, "b1-remove", Hash(later))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.RestoreStorePreferences(ctx, remove, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(root, StorePreferencesPath)); !os.IsNotExist(e) {
		t.Fatal("absence not restored", e)
	}
	noop, e := s.PlanRestoreStorePreferences(ctx, absent, "b1-noop", "")
	if e != nil || !noop.NoOp {
		t.Fatal(e)
	}
	before = sourceHash(t, root)
	r, e = w.RestoreStorePreferences(ctx, noop, nil)
	if e != nil || r.SourceCommitted || r.Status != "no_op" || len(r.ChangedPageIDs) != 0 || sourceHash(t, root) != before {
		t.Fatal(r, e)
	}
}
func TestB1RootPreferenceRestoreRejectsTamperSecretsAndWrongIdentity(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	s, w := open(t, root)
	writeShared(t, root, []byte(`{"version":1,"scope":"store","values":{"sync.interval":73}}`))
	v, e := s.SnapshotStorePreferences(ctx, snapshotStamp)
	if e != nil {
		t.Fatal(e)
	}
	before := sourceHash(t, root)
	for _, alter := range []func(*StorePreferencesSnapshot){func(v *StorePreferencesSnapshot) { v.Version = 2 }, func(v *StorePreferencesSnapshot) { v.File.Path = ".local/conf/device-preferences.json" }, func(v *StorePreferencesSnapshot) { v.File.Hash = Hash([]byte("wrong")) }, func(v *StorePreferencesSnapshot) {
		v.Data = []byte(`{"version":1,"scope":"store","values":{"repo.key":"secret"}}`)
		*v = resealSnapshot(t, *v)
	}, func(v *StorePreferencesSnapshot) { v.Present = false }, func(v *StorePreferencesSnapshot) { v.CreatedAt = "not-a-time" }} {
		bad := v
		bad.Data = bytes.Clone(v.Data)
		alter(&bad)
		if _, e = s.PlanRestoreStorePreferences(ctx, bad, "b1-reject", v.File.Hash); e == nil {
			t.Fatal("accepted bad snapshot")
		}
	}
	oversized := v
	oversized.Store.Name = string(bytes.Repeat([]byte("<"), 400000))
	oversized = resealSnapshot(t, oversized)
	if e := oversized.Verify(); !errors.Is(e, ErrInvalid) {
		t.Fatal("capture could return an undecodable envelope", e)
	}
	foreign := v
	foreign.Store.StoreID = "00000000-0000-0000-0000-000000000001"
	foreign = resealSnapshot(t, foreign)
	if _, e = s.PlanRestoreStorePreferences(ctx, foreign, "b1-foreign", v.File.Hash); !errors.Is(e, ErrIdentity) {
		t.Fatal(e)
	}
	plan, e := s.PlanRestoreStorePreferences(ctx, v, "b1-stale", v.File.Hash)
	if e != nil {
		t.Fatal(e)
	}
	writeShared(t, root, []byte(`{"version":1,"scope":"store","values":{}}`))
	if _, e = w.RestoreStorePreferences(ctx, plan, nil); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	writeShared(t, root, v.Data)
	if sourceHash(t, root) != before {
		t.Fatal("rejected snapshots modified source")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = s.SnapshotStorePreferences(ctx, snapshotStamp); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestB1RootPreferenceCaptureRejectsLinksMalformedAndPendingJournal(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	s, w := open(t, root)
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(root, ".wiki")); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SnapshotStorePreferences(ctx, snapshotStamp); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	os.Remove(filepath.Join(root, ".wiki"))
	writeShared(t, root, []byte(`{"version":1,"scope":"device","values":{}}`))
	if _, e := s.SnapshotStorePreferences(ctx, snapshotStamp); !errors.Is(e, conf.ErrPreferences) {
		t.Fatal(e)
	}
	writeShared(t, root, []byte(`{"version":1,"scope":"store","values":{"sync.interval":73}}`))
	v, e := s.SnapshotStorePreferences(ctx, snapshotStamp)
	if e != nil {
		t.Fatal(e)
	}
	other := []byte(`{"version":1,"scope":"store","values":{"sync.interval":84}}`)
	writeShared(t, root, other)
	plan, e := s.PlanRestoreStorePreferences(ctx, v, "b1-interrupted", Hash(other))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.RestoreStorePreferences(ctx, plan, func(point string) error {
		if point == "published:0" {
			return errors.New("synthetic interruption")
		}
		return nil
	}); e == nil {
		t.Fatal("missing interruption")
	}
	if _, e = s.SnapshotStorePreferences(ctx, snapshotStamp); !errors.Is(e, ErrRecovery) {
		t.Fatal(e)
	}
	if _, e = w.Recover(ctx, nil); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(bytesAt(t, root, StorePreferencesPath), other) {
		t.Fatal("rollback lost before image")
	}
	rolledBack, e := w.RestoreStorePreferences(ctx, plan, nil)
	if e != nil || rolledBack.SourceCommitted || !bytes.Equal(bytesAt(t, root, StorePreferencesPath), other) {
		t.Fatal("rolled-back operation was applied again", rolledBack, e)
	}
	// Recovery seals the old operation receipt. A deliberate retry must use a
	// new operation identity, rather than undoing the replay guarantee.
	plan.OperationID = "b1-interrupted-retry"
	if _, e = w.RestoreStorePreferences(ctx, plan, nil); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(bytesAt(t, root, StorePreferencesPath), v.Data) {
		t.Fatal("new retry did not restore")
	}
}
func TestB1RootPreferenceSnapshotRestoresInFreshProcessWithoutOldRuntime(t *testing.T) {
	if input := os.Getenv("WIKI_B1_PREFERENCE_COMPONENT"); input != "" {
		raw, e := os.ReadFile(input)
		if e != nil {
			t.Fatal(e)
		}
		v, e := DecodeStorePreferencesSnapshot(raw)
		if e != nil {
			t.Fatal(e)
		}
		s, w := open(t, os.Getenv("WIKI_B1_PREFERENCE_TARGET"))
		plan, e := s.PlanRestoreStorePreferences(context.Background(), v, "b1-process", "")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.RestoreStorePreferences(context.Background(), plan, nil); e != nil {
			t.Fatal(e)
		}
		return
	}
	root := fixture(t)
	s, _ := open(t, root)
	raw := []byte(`{"version":1,"scope":"store","values":{"sync.interval":73}}`)
	writeShared(t, root, raw)
	v, e := s.SnapshotStorePreferences(context.Background(), snapshotStamp)
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(v)
	input := filepath.Join(t.TempDir(), "root-preferences-component.json")
	if e = os.WriteFile(input, encoded, 0600); e != nil {
		t.Fatal(e)
	}
	target := fixture(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestB1RootPreferenceSnapshotRestoresInFreshProcessWithoutOldRuntime$")
	cmd.Env = append(os.Environ(), "WIKI_B1_PREFERENCE_COMPONENT="+input, "WIKI_B1_PREFERENCE_TARGET="+target)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("fresh process: %v %s", e, out)
	}
	if !bytes.Equal(bytesAt(t, target, StorePreferencesPath), raw) {
		t.Fatal("fresh process changed snapshot bytes")
	}
	recordB1Evidence(t, "source-fresh-process.json", map[string]any{"snapshotDigest": v.Digest, "snapshotFileHash": v.File.Hash, "restoredFileHash": Hash(bytesAt(t, target, StorePreferencesPath)), "freshProcessExit": 0, "oldRuntimeCopied": false})
}
