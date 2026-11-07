package sourcetx

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/wikistore"
)

// StorePreferencesSnapshot is a B1 component, NOT a complete knowledge-store
// backup. Data contains the exact source bytes (base64 in JSON), never a decoded
// float/map projection. Missing source is distinct from an empty preference doc.
type StorePreferencesSnapshot struct {
	Version          int             `json:"version"`
	Kind             string          `json:"kind"`
	Store            wikistore.Store `json:"store"`
	StoreHash        string          `json:"storeHash"`
	TreeSourceDigest string          `json:"treeSourceDigest"`
	CreatedAt        string          `json:"createdAt"`
	Present          bool            `json:"present"`
	File             FileInfo        `json:"file"`
	Data             []byte          `json:"data"`
	Digest           string          `json:"digest"`
}

const preferencesSnapshotKind = "root-store-preferences"

func (v StorePreferencesSnapshot) digest() (string, error) {
	v.Digest = ""
	raw, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	return Hash(raw), nil
}
func (v StorePreferencesSnapshot) Verify() error {
	if v.Version != 1 || v.Kind != preferencesSnapshotKind || v.File.Path != StorePreferencesPath || !hashID.MatchString(v.StoreHash) || !hashID.MatchString(v.TreeSourceDigest) || !hashID.MatchString(v.Digest) {
		return ErrInvalid
	}
	// Capture must never return an envelope its bounded decoder cannot reopen;
	// store-name escaping contributes to the encoded budget too.
	encoded, e := json.Marshal(v)
	if e != nil || len(encoded) > 2<<20 {
		return ErrInvalid
	}
	identity, _ := json.Marshal(v.Store)
	if _, e := wikistore.DecodeStore(identity); e != nil {
		return ErrInvalid
	}
	stamp, e := time.Parse(time.RFC3339Nano, v.CreatedAt)
	if e != nil || stamp.IsZero() || stamp.UTC().Format(time.RFC3339Nano) != v.CreatedAt {
		return ErrInvalid
	}
	if v.Present {
		if len(v.Data) > 1<<20 || v.File.Size != len(v.Data) || v.File.Hash != Hash(v.Data) {
			return ErrInvalid
		}
		if _, e = conf.DecodePreferences(v.Data, conf.StorePreference); e != nil {
			return e
		}
	} else if v.Data != nil || v.File.Size != 0 || v.File.Hash != "" {
		return ErrInvalid
	}
	digest, e := v.digest()
	if e != nil || digest != v.Digest {
		return ErrInvalid
	}
	return nil
}
func DecodeStorePreferencesSnapshot(raw []byte) (StorePreferencesSnapshot, error) {
	var v StorePreferencesSnapshot
	if len(raw) > 2<<20 || strict(raw, &v) != nil {
		return v, ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return StorePreferencesSnapshot{}, ErrInvalid
	}
	for _, key := range []string{"version", "kind", "store", "storeHash", "treeSourceDigest", "createdAt", "present", "file", "data", "digest"} {
		value, ok := fields[key]
		if !ok || (key != "data" && bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return StorePreferencesSnapshot{}, ErrInvalid
		}
	}
	var fileFields map[string]json.RawMessage
	if json.Unmarshal(fields["file"], &fileFields) != nil {
		return StorePreferencesSnapshot{}, ErrInvalid
	}
	for _, key := range []string{"path", "hash", "size"} {
		value, ok := fileFields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return StorePreferencesSnapshot{}, ErrInvalid
		}
	}
	if e := v.Verify(); e != nil {
		return StorePreferencesSnapshot{}, e
	}
	return v, nil
}
func freezeSnapshot(v StorePreferencesSnapshot) (StorePreferencesSnapshot, error) {
	v.Data = bytes.Clone(v.Data)
	if e := v.Verify(); e != nil {
		return StorePreferencesSnapshot{}, e
	}
	return v, nil
}

// Capture uses one shared source admission for store identity, validated page
// metadata tree and the optional root preference file. No source/default files,
// worker or snapshot directory are created; the future B1 archive composes this
// component while holding the same admitted context as its other source reads.
func (s *Store) SnapshotStorePreferences(ctx context.Context, at time.Time) (out StorePreferencesSnapshot, err error) {
	if at.IsZero() {
		return out, ErrInvalid
	}
	err = s.Read(ctx, func() error {
		root, e := os.OpenRoot(s.Root)
		if e != nil {
			return e
		}
		defer root.Close()
		tree, e := (wikistore.Scanner{}).Scan(ctx, s.Root)
		if e != nil {
			return e
		}
		raw, present, e := readFile(root, StorePreferencesPath, 1<<20)
		if e != nil {
			return e
		}
		out = StorePreferencesSnapshot{Version: 1, Kind: preferencesSnapshotKind, Store: tree.Store(), StoreHash: tree.StoreDigest(), TreeSourceDigest: tree.SourceDigest(), CreatedAt: at.UTC().Format(time.RFC3339Nano), Present: present, File: FileInfo{Path: StorePreferencesPath}}
		if present {
			out.Data = bytes.Clone(raw)
			out.File.Size = len(raw)
			out.File.Hash = Hash(raw)
		}
		out.Digest, e = out.digest()
		if e != nil {
			return e
		}
		if e = out.Verify(); e != nil {
			return e
		}
		return ctx.Err()
	})
	if err != nil {
		return StorePreferencesSnapshot{}, err
	}
	return
}

type StorePreferencesRestorePlan struct {
	Version          int                      `json:"version"`
	Snapshot         StorePreferencesSnapshot `json:"snapshot"`
	OperationID      string                   `json:"operationId"`
	ExpectedHash     string                   `json:"expectedHash"`
	TargetStoreHash  string                   `json:"targetStoreHash"`
	TargetTreeDigest string                   `json:"targetTreeDigest"`
	NoOp             bool                     `json:"noOp"`
	ChangedPageIDs   []string                 `json:"changedPageIds"`
}

func (p StorePreferencesRestorePlan) request() (Request, error) {
	if p.Version != 1 || !operationID.MatchString(p.OperationID) || !hashID.MatchString(p.TargetStoreHash) || !hashID.MatchString(p.TargetTreeDigest) || (p.ExpectedHash != "" && !hashID.MatchString(p.ExpectedHash)) {
		return Request{}, ErrInvalid
	}
	v, e := freezeSnapshot(p.Snapshot)
	if e != nil {
		return Request{}, e
	}
	if p.NoOp && (v.Present || p.ExpectedHash != "") {
		return Request{}, ErrInvalid
	}
	return Request{OperationID: p.OperationID, ExpectedTreeDigest: p.TargetTreeDigest, Changes: []Change{{Path: StorePreferencesPath, ExpectedHash: p.ExpectedHash, Data: bytes.Clone(v.Data), Delete: !v.Present}}}, nil
}

// Validate rejects malformed plans before a host begins draining live owners.
// Target identity/CAS/graph freshness are rechecked at actual source admission.
func (p StorePreferencesRestorePlan) Validate() error { _, e := p.request(); return e }

// Plan is read-only. The caller supplies the current file CAS (empty means
// absent); the target tree digest is captured independently of the old snapshot.
// The old tree digest is provenance, never a replacement for this file CAS.
func (s *Store) PlanRestoreStorePreferences(ctx context.Context, v StorePreferencesSnapshot, operation, expected string) (out StorePreferencesRestorePlan, err error) {
	v, e := freezeSnapshot(v)
	if e != nil {
		return out, e
	}
	err = s.Read(ctx, func() error {
		root, e := os.OpenRoot(s.Root)
		if e != nil {
			return e
		}
		defer root.Close()
		tree, e := (wikistore.Scanner{}).Scan(ctx, s.Root)
		if e != nil {
			return e
		}
		if tree.Store().StoreID != v.Store.StoreID {
			return ErrIdentity
		}
		raw, present, e := readFile(root, StorePreferencesPath, 1<<20)
		if e != nil {
			return e
		}
		actual := ""
		if present {
			actual = Hash(raw)
		}
		if actual != expected {
			return ErrConflict
		}
		out = StorePreferencesRestorePlan{Version: 1, Snapshot: v, OperationID: operation, ExpectedHash: expected, TargetStoreHash: tree.StoreDigest(), TargetTreeDigest: tree.SourceDigest(), NoOp: !v.Present && !present, ChangedPageIDs: allPageIDs(tree)}
		if out.NoOp {
			out.ChangedPageIDs = []string{}
		}
		if _, e = out.request(); e != nil {
			return e
		}
		return ctx.Err()
	})
	if err != nil {
		return StorePreferencesRestorePlan{}, err
	}
	return
}

// Restore reuses the authoritative source CAS/journal/recovery and its all-page
// receipt. Exact bytes or absence are restored; no credential/device file is a
// permissible destination. Fault is only a deterministic interruption test seam.
func (w *Writer) RestoreStorePreferences(ctx context.Context, p StorePreferencesRestorePlan, fault Fault) (Receipt, error) {
	// Never try to upgrade the caller's own shared read lock to an exclusive
	// restore lock. Preview/capture admissions must finish before applying.
	if a, ok := ctx.Value(readAdmissionKey{}).(*readAdmission); ok {
		if a.store != w.store {
			return Receipt{}, ErrIdentity
		}
		a.mu.Lock()
		active := a.active
		a.mu.Unlock()
		if !active {
			return Receipt{}, ErrClosed
		}
		return Receipt{}, ErrBusy
	}
	request, e := p.request()
	if e != nil {
		return Receipt{}, e
	}
	// Bind the snapshot to this pinned Store. The subsequent exclusive Apply
	// rechecks that same store.json identity, so a change cannot cross the gap.
	e = w.store.Read(ctx, func() error {
		root, e := os.OpenRoot(w.store.Root)
		if e != nil {
			return e
		}
		defer root.Close()
		raw, exists, e := readFile(root, "store.json", 1<<20)
		if e != nil {
			return e
		}
		if !exists || Hash(raw) != p.TargetStoreHash {
			return ErrIdentity
		}
		identity, e := wikistore.DecodeStore(raw)
		if e != nil {
			return e
		}
		if identity.StoreID != p.Snapshot.Store.StoreID {
			return ErrIdentity
		}
		return nil
	})
	if e != nil {
		return Receipt{}, e
	}
	if !p.NoOp {
		return w.Apply(ctx, request, fault)
	}
	// Absence -> absence is an asserted no-op, not a fabricated source commit.
	var result Receipt
	e = w.exclusive(ctx, false, func(root *os.Root) error {
		raw, _ := json.Marshal(request)
		requestHash := Hash(raw)
		if previous, ok, e := receipt(root, p.OperationID); e != nil {
			return e
		} else if ok {
			if previous.RequestHash != requestHash {
				return ErrConflict
			}
			result = previous
			return nil
		}
		tree, e := (wikistore.Scanner{}).Scan(ctx, w.store.Root)
		if e != nil {
			return e
		}
		if tree.SourceDigest() != p.TargetTreeDigest {
			return ErrConflict
		}
		_, present, e := readFile(root, StorePreferencesPath, 1<<20)
		if e != nil {
			return e
		}
		if present || p.ExpectedHash != "" {
			return ErrConflict
		}
		result = Receipt{Version: 1, OperationID: p.OperationID, RequestHash: requestHash, Status: "no_op", TreeSourceDigest: tree.SourceDigest(), ChangedPageIDs: []string{}}
		return ctx.Err()
	})
	return result, e
}
