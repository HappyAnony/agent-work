package sourcetx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"

	"wiki-kernel/internal/wikistore"
)

func receipt(root *os.Root, id string) (Receipt, bool, error) {
	var r Receipt
	if !operationID.MatchString(id) {
		return r, false, ErrInvalid
	}
	data, exists, err := readFile(root, receiptPath(id), maxJournal)
	if err != nil || !exists {
		return r, exists, err
	}
	if strict(data, &r) != nil || !r.Outcome.valid() || (r.ClientHash != "" && !hashID.MatchString(r.ClientHash)) || (r.HistoryScope != "" && !hashID.MatchString(r.HistoryScope)) || r.Version != 1 || r.OperationID != id || !hashID.MatchString(r.RequestHash) || !hashID.MatchString(r.TreeSourceDigest) || (r.Status != "committed" && r.Status != "rolled_back") || r.SourceCommitted != (r.Status == "committed") || (r.IndexFresh && (r.Epoch == "" || r.TreeGeneration == 0)) {
		return r, false, ErrRecovery
	}
	for _, id := range r.ChangedPageIDs {
		if !pageID.MatchString(id) {
			return r, false, ErrRecovery
		}
	}
	return r, true, nil
}

func loadJournal(root *os.Root, id string) (journal, bool, error) {
	var j journal
	data, exists, err := readFile(root, journalPath(id), maxJournal)
	if err != nil || !exists {
		return j, false, errors.Join(ErrRecovery, err)
	}
	if strict(data, &j) != nil || !j.Outcome.valid() || (j.ClientHash != "" && !hashID.MatchString(j.ClientHash)) || (j.HistoryScope != "" && !hashID.MatchString(j.HistoryScope)) || j.Version != 1 || j.OperationID != id || !operationID.MatchString(id) || !hashID.MatchString(j.RequestHash) || len(j.Files) == 0 || len(j.Files) > 10000 {
		return j, false, ErrRecovery
	}
	allowedDirs := map[string]bool{}
	for i, file := range j.Files {
		if file.BeforeMode > 0777 || file.AfterMode > 0777 || (file.BeforeHash == "" && file.BeforeMode != 0) || (file.AfterHash == "" && file.AfterMode != 0) {
			return j, false, ErrRecovery
		}
		if !sourcePath(file.Path) || (file.BeforeHash != "" && !hashID.MatchString(file.BeforeHash)) || (file.AfterHash != "" && !hashID.MatchString(file.AfterHash)) || (file.BeforeHash == "" && file.AfterHash == "") || (pageMetadata(file.Path) && file.AfterHash == "") || (i > 0 && j.Files[i-1].Path >= file.Path) {
			return j, false, ErrRecovery
		}
		for dir := path.Dir(file.Path); dir != "pages" && dir != "."; dir = path.Dir(dir) {
			allowedDirs[dir] = true
		}
	}
	for i, dir := range j.CreatedDirs {
		if !allowedDirs[dir] || (i > 0 && j.CreatedDirs[i-1] >= dir) {
			return j, false, ErrRecovery
		}
	}
	data, exists, err = readFile(root, commitPath(id), maxJournal)
	if err != nil {
		return j, false, err
	}
	if !exists {
		return j, false, nil
	}
	var c commit
	journalBytes, _, err := readFile(root, journalPath(id), maxJournal)
	if err != nil {
		return j, false, err
	}
	if strict(data, &c) != nil || c.OperationID != id || c.JournalHash != Hash(journalBytes) {
		return j, false, ErrRecovery
	}
	return j, true, nil
}

func journalIDs(root *os.Root) ([]string, error) {
	return journalIDsWithIO(root, readManifestIO{})
}

func journalIDsWithIO(root *os.Root, op readManifestIO) ([]string, error) {
	if err := safePath(root, ".local/journal"); err != nil {
		return nil, err
	}
	manifest, exists, err := confirmedSourceReadManifest(context.Background(), root, ".local/journal", readManifestOptions{singleLevel: true}, op)
	if err != nil {
		if errors.Is(err, ErrInvalid) || errors.Is(err, errReadSourceChanged) {
			return nil, errors.Join(ErrRecovery, err)
		}
		return nil, err
	}
	if !exists {
		return nil, os.ErrNotExist
	}
	if len(manifest) == 0 || !manifest[0].info.IsDir() {
		return nil, ErrRecovery
	}
	entries := manifest[1:]
	if len(entries) > 200000 {
		return nil, ErrRecovery
	}
	names := make(map[string]bool, len(entries))
	for _, entry := range entries {
		names[path.Base(entry.name)] = true
	}
	ids := []string{}
	for _, entry := range entries {
		if entry.info.IsDir() {
			return nil, ErrRecovery
		}
		name := path.Base(entry.name)
		if strings.HasPrefix(name, ".wiki-tmp-") {
			continue
		}
		if strings.HasSuffix(name, ".commit") {
			id := strings.TrimSuffix(name, ".commit")
			if !operationID.MatchString(id) {
				return nil, ErrRecovery
			}
			if !names[id+".json"] {
				return nil, ErrRecovery
			}
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if id == name || !operationID.MatchString(id) {
			return nil, ErrRecovery
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func admit(root *os.Root) error {
	ids, err := journalIDs(root)
	if err != nil {
		return errors.Join(ErrRecovery, err)
	}
	var store wikistore.Store
	if len(ids) > 0 {
		raw, exists, err := readFile(root, "store.json", 1<<20)
		if err != nil || !exists || strict(raw, &store) != nil {
			return errors.Join(ErrRecovery, err)
		}
	}
	for _, id := range ids {
		j, committed, err := loadJournal(root, id)
		if err != nil {
			return errors.Join(ErrRecovery, err)
		}
		if j.StoreID != store.StoreID {
			return ErrRecovery
		}
		r, ok, err := receipt(root, id)
		if err != nil {
			return errors.Join(ErrRecovery, err)
		}
		if !ok || r.RequestHash != j.RequestHash || r.ClientHash != j.ClientHash || r.HistoryScope != j.HistoryScope || !reflect.DeepEqual(r.Outcome, j.Outcome) || r.SourceCommitted != committed {
			return ErrRecovery
		}
	}
	return nil
}

// Recover holds session/source locks, validates ALL targets/images before any
// restore, then rolls uncommitted journals back or committed journals forward.
// A foreign target/hash change aborts without overwriting it. Repeated recovery
// skips completed receipts, so older operations never overwrite later commits.
func (w *Writer) Recover(ctx context.Context, fault Fault) ([]Receipt, error) {
	result := []Receipt{}
	err := w.exclusive(ctx, true, func(root *os.Root) error {
		err := visitPendingRecovery(ctx, root, func(j journal, committed bool) error {
			id := j.OperationID
			if err := validateRecoveryImages(ctx, root, j, committed); err != nil {
				return err
			}
			for i := range j.Files {
				// Only this journal's reserved, preflight-absent source temp file.
				if err := root.Remove(tempPath(j, i)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				if err := publish(root, j, i, committed); err != nil {
					return err
				}
				if err := trip(fault, fmt.Sprintf("recovered:%d", i)); err != nil {
					return err
				}
			}
			if !committed {
				for i := len(j.CreatedDirs) - 1; i >= 0; i-- {
					dir := j.CreatedDirs[i]
					if err := root.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
						return errors.Join(ErrRecovery, err)
					}
					if err := syncDir(root, path.Dir(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
						return err
					}
				}
			}
			tree, err := (wikistore.Scanner{}).Scan(ctx, w.store.Root)
			if err != nil {
				return err
			}
			r := makeReceipt(j, tree, committed)
			indexErr := w.store.project(ctx, root, tree, "all", true)
			r.IndexFresh = indexErr == nil
			if indexErr == nil {
				st, err := state(root)
				if err != nil {
					return err
				}
				r.Epoch = st.Epoch
				r.TreeGeneration = st.Generation
			}
			if err := jsonFile(root, receiptPath(id), r); err != nil {
				return err
			}
			result = append(result, r)
			if indexErr != nil {
				return errors.Join(ErrIndex, indexErr)
			}
			w.store.emit("sourceRecovered", r.OperationID, r.ChangedPageIDs, r.TreeSourceDigest)
			return nil
		})
		if err != nil {
			return err
		}
		return admit(root)
	})
	return result, err
}

func visitPendingRecovery(ctx context.Context, root *os.Root, visit func(journal, bool) error) error {
	ids, err := journalIDs(root)
	if err != nil || len(ids) == 0 {
		return err
	}
	raw, exists, err := readFile(root, "store.json", 1<<20)
	store, decodeErr := wikistore.DecodeStore(raw)
	if err != nil || !exists || decodeErr != nil {
		return errors.Join(ErrRecovery, err, decodeErr)
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		j, committed, err := loadJournal(root, id)
		if err != nil || j.StoreID != store.StoreID {
			return errors.Join(ErrRecovery, err)
		}
		if r, ok, err := receipt(root, id); err != nil {
			return err
		} else if ok {
			if r.RequestHash != j.RequestHash || r.ClientHash != j.ClientHash || r.HistoryScope != j.HistoryScope ||
				!reflect.DeepEqual(r.Outcome, j.Outcome) || r.SourceCommitted != committed {
				return ErrRecovery
			}
			continue
		}
		if err := visit(j, committed); err != nil {
			return err
		}
	}
	return nil
}

func validateRecoveryImages(ctx context.Context, root *os.Root, j journal, committed bool) error {
	id := j.OperationID
	for i, file := range j.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, exists, err := readFile(root, file.Path, maxFile)
		if err != nil {
			return err
		}
		hash := ""
		if exists {
			hash = Hash(data)
		}
		if hash != file.BeforeHash && hash != file.AfterHash {
			return errors.Join(ErrConflict, ErrRecovery)
		}
		if exists {
			info, err := root.Lstat(file.Path)
			if err != nil {
				return err
			}
			mode := uint32(info.Mode().Perm())
			validBefore := hash == file.BeforeHash && mode == file.BeforeMode
			validAfter := hash == file.AfterHash && mode == file.AfterMode
			if !validBefore && !validAfter {
				return errors.Join(ErrConflict, ErrRecovery)
			}
		}
		for _, side := range []struct{ name, hash string }{{"before", file.BeforeHash}, {"after", file.AfterHash}} {
			if side.hash == "" {
				continue
			}
			raw, exists, err := readFile(root, fmt.Sprintf(".local/staging/%s/%s/%d", id, side.name, i), maxFile)
			if err != nil || !exists || Hash(raw) != side.hash {
				return errors.Join(ErrRecovery, err)
			}
		}
		if err := safePath(root, tempPath(j, i)); err != nil {
			return err
		}
		if info, err := root.Lstat(tempPath(j, i)); err == nil && !info.Mode().IsRegular() {
			return ErrRecovery
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	allowed := map[string]bool{}
	if !committed {
		for i, file := range j.Files {
			if file.BeforeHash == "" {
				allowed[file.Path] = true
			}
			allowed[tempPath(j, i)] = true
		}
		for _, dir := range j.CreatedDirs {
			allowed[dir] = true
		}
	}
	for _, dir := range j.CreatedDirs {
		if err := safePath(root, dir); err != nil {
			return err
		}
		if committed {
			continue
		}
		entries, err := fs.ReadDir(root.FS(), dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return errors.Join(ErrRecovery, err)
		}
		for _, entry := range entries {
			if !allowed[path.Join(dir, entry.Name())] {
				return errors.Join(ErrConflict, ErrRecovery)
			}
		}
	}
	return nil
}
