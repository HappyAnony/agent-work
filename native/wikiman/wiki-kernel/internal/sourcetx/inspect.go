package sourcetx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"wiki-kernel/internal/wikistore"
)

// ErrStoreMismatch means the request selected a different source identity.
var ErrStoreMismatch = errors.New("inspection_store_mismatch")

type Inspection struct {
	SessionLock  string      `json:"sessionLock"`
	SourceLock   string      `json:"sourceLock"`
	Recovery     string      `json:"recovery"`
	TreeIndex    string      `json:"treeIndex"`
	ContentIndex string      `json:"contentIndex"`
	Index        *IndexState `json:"index,omitempty"`
}

type RecoveryStep struct {
	OperationID       string         `json:"operationId"`
	Action            string         `json:"action"`
	Files             []RecoveryFile `json:"files"`
	RemoveDirectories []string       `json:"removeDirectories,omitempty"`
}

type RecoveryFile struct {
	Path       string `json:"path"`
	TargetHash string `json:"targetHash"`
	TargetMode uint32 `json:"targetMode"`
}

func (s *Store) inspect(ctx context.Context, storeID string, visit func(*os.Root, bool) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(".local")
	initialized := err == nil
	if initialized {
		if !info.IsDir() {
			return ErrInvalid
		}
		lock, err := s.lock(ctx, "source", true, false)
		if err != nil {
			return err
		}
		defer lock.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := s.checkIdentity(root); err != nil {
		return err
	}
	raw, _, err := readFile(root, "store.json", 1<<20)
	if err != nil {
		return err
	}
	store, err := wikistore.DecodeStore(raw)
	if err != nil {
		return err
	}
	if store.StoreID != storeID {
		return ErrStoreMismatch
	}
	if !initialized {
		// A concurrent first open must be retried through the real source lock.
		if _, err := root.Lstat(".local"); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return ErrBusy
			}
			return err
		}
	}
	return visit(root, initialized)
}

func (s *Store) Inspect(ctx context.Context, storeID string) (Inspection, error) {
	out := Inspection{SessionLock: "uninitialized", SourceLock: "uninitialized", Recovery: "clear", TreeIndex: "missing", ContentIndex: "missing"}
	err := s.inspect(ctx, storeID, func(root *os.Root, initialized bool) error {
		if !initialized {
			return nil
		}
		out.SourceLock = "readable"
		// Never wait for the session lock while holding a source read lock.
		session, err := s.lock(ctx, "session", false, false)
		switch {
		case errors.Is(err, ErrBusy):
			out.SessionLock = "held"
		case err != nil:
			return err
		default:
			out.SessionLock = "available"
			if err := session.Close(); err != nil {
				return err
			}
		}
		if err := visitPendingRecovery(ctx, root, func(journal, bool) error {
			out.Recovery = "required"
			return nil
		}); err != nil {
			if !errors.Is(err, ErrRecovery) {
				return err
			}
			out.Recovery = "invalid"
		}
		if out.Recovery != "clear" {
			out.TreeIndex, out.ContentIndex = "unverified", "unverified"
			return nil
		}
		return inspectProjection(ctx, root, s.Root, &out)
	})
	if errors.Is(err, ErrBusy) {
		out = Inspection{SessionLock: "unknown", SourceLock: "busy", Recovery: "unknown", TreeIndex: "unverified", ContentIndex: "unverified"}
	}
	return out, err
}

func inspectProjection(ctx context.Context, root *os.Root, dir string, out *Inspection) error {
	st, err := state(root)
	if err != nil {
		if !errors.Is(err, ErrIndex) {
			return err
		}
		if _, err := root.Lstat(".local/index/state.json"); !errors.Is(err, os.ErrNotExist) {
			out.TreeIndex, out.ContentIndex = "stale", "stale"
		}
		return nil
	}
	out.Index = &st
	out.TreeIndex, out.ContentIndex = "stale", "stale"
	if !st.TreeFresh {
		return nil
	}
	tree, err := (wikistore.Scanner{}).Scan(ctx, dir)
	if err != nil {
		return err
	}
	if tree.SourceDigest() != st.TreeSourceDigest {
		return nil
	}
	const dbPath = ".local/index/page.db"
	if err := safePath(root, dbPath); err != nil {
		return err
	}
	if info, err := root.Lstat(dbPath); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	db, err := sql.Open("sqlite3", sqliteDSN(filepath.Join(dir, dbPath), "ro"))
	if err != nil {
		return err
	}
	defer db.Close()
	var epoch, generation, digest string
	if verifyTree(ctx, db, tree) != nil ||
		db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='epoch'").Scan(&epoch) != nil || epoch != st.Epoch ||
		db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='generation'").Scan(&generation) != nil || generation != strconv.FormatUint(st.Generation, 10) {
		return ctx.Err()
	}
	out.TreeIndex = "fresh"
	if !st.ContentFresh || db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='content'").Scan(&digest) != nil || digest != st.ContentSourceDigest {
		return ctx.Err()
	}
	manifest, err := projectionManifest(ctx, root, tree, contentManifestIO{})
	if err != nil {
		return err
	}
	hashes := map[string]string{}
	var total int64
	for _, entry := range manifest {
		if entry.pageID == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := readProjectionFile(root, entry, 16<<20, contentManifestIO{})
		if err != nil {
			return err
		}
		total += int64(len(raw))
		if len(hashes) >= 100000 || total > 512<<20 {
			return ErrInvalid
		}
		hashes[entry.name] = Hash(raw)
	}
	confirmed, err := projectionManifest(ctx, root, tree, contentManifestIO{})
	if err != nil {
		return err
	}
	if !sameProjectionManifest(manifest, confirmed) {
		return ErrConflict
	}
	raw, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	if Hash(raw) != digest {
		return nil
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := verifyContent(ctx, tx); err != nil {
		return ctx.Err()
	}
	rows, err := tx.QueryContext(ctx, "SELECT path,hash FROM documents")
	if err != nil {
		return ctx.Err()
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var name, hash string
		if rows.Scan(&name, &hash) != nil || hash == "" || hashes[name] != hash {
			return ctx.Err()
		}
		count++
	}
	if rows.Err() == nil && count == len(hashes) {
		out.ContentIndex = "fresh"
	}
	return ctx.Err()
}

func (s *Store) PreviewRecovery(ctx context.Context, storeID string) ([]RecoveryStep, error) {
	steps := []RecoveryStep{}
	err := s.inspect(ctx, storeID, func(root *os.Root, initialized bool) error {
		if !initialized {
			return nil
		}
		return visitPendingRecovery(ctx, root, func(j journal, committed bool) error {
			if err := validateRecoveryImages(ctx, root, j, committed); err != nil {
				return err
			}
			step := RecoveryStep{OperationID: j.OperationID, Action: "roll_back", Files: make([]RecoveryFile, 0, len(j.Files))}
			if committed {
				step.Action = "roll_forward"
			} else {
				step.RemoveDirectories = slices.Clone(j.CreatedDirs)
				slices.Reverse(step.RemoveDirectories)
			}
			for _, f := range j.Files {
				target := RecoveryFile{Path: f.Path, TargetHash: f.BeforeHash, TargetMode: f.BeforeMode}
				if committed {
					target.TargetHash, target.TargetMode = f.AfterHash, f.AfterMode
				}
				step.Files = append(step.Files, target)
			}
			steps = append(steps, step)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return steps, nil
}
