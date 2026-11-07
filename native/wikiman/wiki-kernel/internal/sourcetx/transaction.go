package sourcetx

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"wiki-kernel/conf"

	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/wikistore"
)

type Change struct {
	Path         string `json:"path"`
	ExpectedHash string `json:"expectedHash"`
	Data         []byte `json:"data,omitempty"`
	Delete       bool   `json:"delete,omitempty"`
}

// Outcome is the small, immutable response identity of a lifecycle command.
// It does not provide a source path or a permission to a caller.
type Outcome struct {
	DocID     string `json:"docId,omitempty"`
	Path      string `json:"path,omitempty"`
	Hash      string `json:"hash,omitempty"`
	AssetName string `json:"assetName,omitempty"`
	HistoryID string `json:"historyId,omitempty"`
}

func (o *Outcome) valid() bool {
	return o == nil || (o.DocID == "" || docID.MatchString(o.DocID)) &&
		(o.Hash == "" || hashID.MatchString(o.Hash)) && (o.HistoryID == "" || operationID.MatchString(o.HistoryID)) &&
		len(o.Path) <= 4096 && len(o.AssetName) <= 1024
}

type Request struct {
	Outcome            *Outcome    `json:"outcome,omitempty"`
	HistoryScope       string      `json:"historyScope,omitempty"`
	ClientHash         string      `json:"clientHash,omitempty"`
	OperationID        string      `json:"operationId"`
	ExpectedTreeDigest string      `json:"expectedTreeDigest,omitempty"`
	Changes            []Change    `json:"changes"`
	ReadGuards         *ReadGuards `json:"readGuards,omitempty"`
	Noop               bool        `json:"noop,omitempty"`
}
type Receipt struct {
	Outcome          *Outcome `json:"outcome,omitempty"`
	HistoryScope     string   `json:"historyScope,omitempty"`
	ClientHash       string   `json:"clientHash,omitempty"`
	Version          int      `json:"version"`
	OperationID      string   `json:"operationId"`
	RequestHash      string   `json:"requestHash"`
	Status           string   `json:"status"`
	SourceCommitted  bool     `json:"sourceCommitted"`
	IndexFresh       bool     `json:"indexFresh"`
	TreeSourceDigest string   `json:"treeSourceDigest"`
	ChangedPageIDs   []string `json:"changedPageIds"`
	Epoch            string   `json:"epoch,omitempty"`
	TreeGeneration   uint64   `json:"treeGeneration,omitempty"`
}
type image struct {
	Path       string `json:"path"`
	BeforeHash string `json:"beforeHash"`
	AfterHash  string `json:"afterHash"`
	BeforeMode uint32 `json:"beforeMode"`
	AfterMode  uint32 `json:"afterMode"`
}
type journal struct {
	Outcome      *Outcome `json:"outcome,omitempty"`
	HistoryScope string   `json:"historyScope,omitempty"`
	ClientHash   string   `json:"clientHash,omitempty"`
	Version      int      `json:"version"`
	OperationID  string   `json:"operationId"`
	StoreID      string   `json:"storeId"`
	RequestHash  string   `json:"requestHash"`
	Files        []image  `json:"files"`
	CreatedDirs  []string `json:"createdDirs"`
}
type commit struct {
	OperationID string `json:"operationId"`
	JournalHash string `json:"journalHash"`
}
type intent struct {
	Version     int    `json:"version"`
	OperationID string `json:"operationId"`
	StoreID     string `json:"storeId"`
	RequestHash string `json:"requestHash"`
}

// Fault is a deterministic test seam; production leaves it nil. Errors after a
// durable journal intentionally leave recovery material, not a fake rollback.
type Fault func(point string) error

func (w *Writer) Apply(ctx context.Context, request Request, fault Fault) (Receipt, error) {
	return w.apply(ctx, request, fault, false)
}

// Preview performs the identical locked CAS, AST and candidate-tree validation
// as Apply, but never stages images, writes a journal, or updates projections.
// A preview is not a reservation: Apply must validate again.
func (w *Writer) Preview(ctx context.Context, request Request) (Receipt, error) {
	return w.apply(ctx, request, nil, true)
}

func (w *Writer) apply(ctx context.Context, request Request, fault Fault, preview bool) (Receipt, error) {
	var result Receipt
	if !request.Outcome.valid() {
		return result, ErrInvalid
	}
	if request.Outcome != nil {
		value := *request.Outcome
		request.Outcome = &value
	}
	if (request.ClientHash != "" && !hashID.MatchString(request.ClientHash)) || (request.HistoryScope != "" && !hashID.MatchString(request.HistoryScope)) {
		return result, ErrInvalid
	}
	if !operationID.MatchString(request.OperationID) || (len(request.Changes) == 0 && !request.Noop) || len(request.Changes) > 10000 || (request.Noop && (len(request.Changes) != 0 || request.ReadGuards == nil)) {
		return result, ErrInvalid
	}
	request.Changes = append([]Change(nil), request.Changes...)
	request.ReadGuards = cloneReadGuards(request.ReadGuards)
	if request.ExpectedTreeDigest != "" && !hashID.MatchString(request.ExpectedTreeDigest) {
		return result, ErrInvalid
	}
	for i := range request.Changes {
		request.Changes[i].Data = append([]byte(nil), request.Changes[i].Data...)
	}
	sort.Slice(request.Changes, func(i, j int) bool { return request.Changes[i].Path < request.Changes[j].Path })
	raw, _ := json.Marshal(request)
	requestHash := Hash(raw)
	err := w.exclusive(ctx, false, func(root *os.Root) error {
		if previous, ok, err := receipt(root, request.OperationID); err != nil {
			return err
		} else if ok {
			if previous.RequestHash != requestHash {
				return ErrConflict
			}
			result = previous
			if preview {
				result.Status = "dry_run_replay"
			}
			return nil
		}
		base, err := (wikistore.Scanner{}).Scan(ctx, w.store.Root)
		if err != nil {
			return err
		}
		if request.ExpectedTreeDigest != "" && request.ExpectedTreeDigest != base.SourceDigest() {
			return ErrConflict
		}
		if err := verifyReadGuards(ctx, root, request.ReadGuards); err != nil {
			return err
		}
		if request.Noop {
			if len(request.ReadGuards.Files)+len(request.ReadGuards.Directories)+len(request.ReadGuards.Missing) == 0 {
				return ErrInvalid
			}
			result = Receipt{Version: 1, OperationID: request.OperationID, RequestHash: requestHash, ClientHash: request.ClientHash, HistoryScope: request.HistoryScope, Outcome: request.Outcome, Status: "dry_run", TreeSourceDigest: base.SourceDigest(), ChangedPageIDs: []string{}}
			if preview {
				return nil
			}
			result.Status, result.SourceCommitted = "committed", true
			indexErr := w.store.project(ctx, root, base, "all", false)
			if indexErr == nil {
				st, e := state(root)
				if e != nil {
					result = Receipt{}
					return e
				}
				result.IndexFresh, result.Epoch, result.TreeGeneration = true, st.Epoch, st.Generation
			}
			if err := jsonFile(root, receiptPath(request.OperationID), result); err != nil {
				result = Receipt{}
				return err
			}
			if indexErr != nil {
				return errors.Join(ErrIndex, indexErr)
			}
			w.store.event(result, nil)
			return nil
		}
		j := journal{Version: 1, OperationID: request.OperationID, StoreID: base.Store().StoreID, RequestHash: requestHash, ClientHash: request.ClientHash, HistoryScope: request.HistoryScope, Outcome: request.Outcome, CreatedDirs: []string{}}
		before := make([][]byte, len(request.Changes))
		overrides := map[string][]byte{}
		changes := map[string]Change{}
		dirs := map[string]bool{}
		total := 0
		for i, c := range request.Changes {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !sourcePath(c.Path) || (c.ExpectedHash != "" && !hashID.MatchString(c.ExpectedHash)) || (c.Delete && len(c.Data) != 0) || (i > 0 && request.Changes[i-1].Path == c.Path) {
				return ErrInvalid
			}
			if c.Delete && pageMetadata(c.Path) {
				return ErrInvalid
			} // Soft-delete metadata, never orphan a page directory.
			data, exists, err := readFile(root, c.Path, maxFile)
			if err != nil {
				return err
			}
			hash := ""
			if exists {
				hash = Hash(data)
			}
			if hash != c.ExpectedHash {
				return ErrConflict
			}
			if c.Delete && !exists {
				return ErrConflict
			}
			if len(c.Data) > maxFile {
				return ErrInvalid
			}
			// This reserved classified document cannot smuggle device secrets or
			// runtime authority through the generic source transaction handler.
			parts := strings.Split(c.Path, "/")
			scope := conf.WorkspacePreference
			if c.Path == StorePreferencesPath {
				scope = conf.StorePreference
			}
			if (c.Path == StorePreferencesPath || (len(parts) == 5 && parts[2] == "data" && parts[3] == ".wiki" && parts[4] == "preferences.json")) && !c.Delete {
				if len(c.Data) > 1<<20 {
					return ErrInvalid
				}
				if _, err := conf.DecodePreferences(c.Data, scope); err != nil {
					return err
				}
			}
			total += len(c.Data) + len(data)
			if total > 256<<20 {
				return ErrInvalid
			}
			afterHash := ""
			if !c.Delete {
				afterHash = Hash(c.Data)
			}
			if pageMetadata(c.Path) {
				var old, next wikistore.Page
				if json.Unmarshal(c.Data, &next) != nil {
					return ErrInvalid
				}
				if exists {
					if json.Unmarshal(data, &old) != nil || next.Revision != old.Revision+1 {
						return ErrConflict
					}
				} else if next.Revision != 1 {
					return ErrInvalid
				}
				overrides[c.Path] = c.Data
			}
			if !c.Delete && strings.HasSuffix(c.Path, ".docj") {
				if len(c.Data) > 16<<20 {
					return ErrInvalid
				}
				parser := docast.Parser{}
				if _, _, err := parser.Parse(ctx, c.Data, strings.TrimSuffix(path.Base(c.Path), ".docj")); err != nil {
					return err
				}
			}
			for dir := path.Dir(c.Path); dir != "." && dir != "pages"; dir = path.Dir(dir) {
				if _, err := root.Lstat(dir); errors.Is(err, os.ErrNotExist) {
					dirs[dir] = true
				} else if err != nil {
					return err
				}
			}
			beforeMode, afterMode := uint32(0), uint32(0600)
			if exists {
				info, err := root.Lstat(c.Path)
				if err != nil {
					return err
				}
				beforeMode = uint32(info.Mode().Perm())
				afterMode = beforeMode
			}
			if c.Delete {
				afterMode = 0
			}
			before[i] = data
			j.Files = append(j.Files, image{Path: c.Path, BeforeHash: hash, AfterHash: afterHash, BeforeMode: beforeMode, AfterMode: afterMode})
			changes[c.Path] = c
		}
		if err := verifyDocumentArchives(request, before, changes); err != nil {
			return err
		}
		if err := verifyNotebookArchives(request, before, changes); err != nil {
			return err
		}
		for dir := range dirs {
			j.CreatedDirs = append(j.CreatedDirs, dir)
		}
		sort.Strings(j.CreatedDirs)
		journalBytes, _ := json.Marshal(j)
		if len(journalBytes) > maxJournal {
			return ErrInvalid
		}
		for i := range j.Files {
			if err := safePath(root, tempPath(j, i)); err != nil {
				return err
			}
			if _, err := root.Lstat(tempPath(j, i)); err == nil {
				return ErrConflict
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		candidate, err := (wikistore.Scanner{}).Candidate(ctx, w.store.Root, overrides, overlayEntry(root, changes))
		if err != nil {
			return err
		}
		for _, c := range request.Changes {
			if c.Path == StorePreferencesPath {
				continue
			}
			if _, ok := candidate.Get(sourcePage(c.Path)); !ok {
				return ErrInvalid
			}
		}
		if err := verifyReadGuards(ctx, root, request.ReadGuards); err != nil {
			return err
		}
		if preview {
			ids := map[string]bool{}
			for _, c := range request.Changes {
				if c.Path == StorePreferencesPath {
					for _, id := range allPageIDs(candidate) {
						ids[id] = true
					}
				} else {
					ids[sourcePage(c.Path)] = true
				}
			}
			changed := make([]string, 0, len(ids))
			for id := range ids {
				changed = append(changed, id)
			}
			sort.Strings(changed)
			result = Receipt{Version: 1, OperationID: request.OperationID, RequestHash: requestHash, ClientHash: request.ClientHash, HistoryScope: request.HistoryScope, Outcome: request.Outcome, Status: "dry_run", TreeSourceDigest: candidate.SourceDigest(), ChangedPageIDs: changed}
			return nil
		}
		stage := ".local/staging/" + j.OperationID
		if err := safePath(root, stage); err != nil {
			return err
		}
		wanted := intent{1, j.OperationID, j.StoreID, requestHash}
		if _, err := root.Lstat(stage); err == nil {
			raw, exists, err := readFile(root, stage+"/intent.json", maxJournal)
			var prior intent
			if err != nil || !exists || strict(raw, &prior) != nil || prior != wanted {
				return ErrConflict
			}
		} else if errors.Is(err, os.ErrNotExist) {
			temporary := ".local/staging/.prepare-" + rand.Text()
			if err := root.Mkdir(temporary, 0700); err != nil {
				return writeError(err)
			}
			if err := jsonFile(root, temporary+"/intent.json", wanted); err != nil {
				return err
			}
			if err := root.Rename(temporary, stage); err != nil {
				return err
			}
		} else {
			return err
		}
		if err := syncDir(root, ".local/staging"); err != nil {
			return err
		}
		if err := trip(fault, "staging"); err != nil {
			return err
		}
		for _, dir := range []string{"before", "after"} {
			if err := safePath(root, stage+"/"+dir); err != nil {
				return err
			}
			if err := root.MkdirAll(stage+"/"+dir, 0700); err != nil {
				return err
			}
		}
		for i, c := range request.Changes {
			if j.Files[i].BeforeHash != "" {
				if err := atomicFile(root, fmt.Sprintf("%s/before/%d", stage, i), before[i]); err != nil {
					return err
				}
			}
			if !c.Delete {
				if err := atomicFile(root, fmt.Sprintf("%s/after/%d", stage, i), c.Data); err != nil {
					return err
				}
			}
			if err := trip(fault, fmt.Sprintf("staged:%d", i)); err != nil {
				return err
			}
		}
		if err := syncDir(root, stage); err != nil {
			return err
		}
		if err := syncDir(root, ".local/staging"); err != nil {
			return err
		}
		if err := verifyReadGuards(ctx, root, request.ReadGuards); err != nil {
			return err
		}
		if err := jsonFile(root, journalPath(j.OperationID), j); err != nil {
			return err
		}
		if err := jsonFile(root, ".local/index/state.json", IndexState{Version: 1}); err != nil {
			return err
		}
		if err := trip(fault, "prepared"); err != nil {
			return err
		}
		for i := range j.Files {
			if err := ctx.Err(); err != nil {
				return err
			}
			current, exists, err := readFile(root, j.Files[i].Path, maxFile)
			if err != nil {
				return err
			}
			hash := ""
			if exists {
				hash = Hash(current)
			}
			if hash != j.Files[i].BeforeHash {
				return ErrConflict
			}
			if err := publish(root, j, i, true); err != nil {
				return err
			}
			if err := trip(fault, fmt.Sprintf("published:%d", i)); err != nil {
				return err
			}
		}
		actual, err := (wikistore.Scanner{}).Scan(ctx, w.store.Root)
		if err != nil {
			return err
		}
		if actual.SourceDigest() != candidate.SourceDigest() {
			return ErrConflict
		}
		for _, file := range j.Files {
			raw, exists, err := readFile(root, file.Path, maxFile)
			if err != nil {
				return err
			}
			hash := ""
			if exists {
				hash = Hash(raw)
			}
			if hash != file.AfterHash {
				return ErrConflict
			}
			if exists {
				info, err := root.Lstat(file.Path)
				if err != nil {
					return err
				}
				if uint32(info.Mode().Perm()) != file.AfterMode {
					return ErrConflict
				}
			}
		}
		jr, _, err := readFile(root, journalPath(j.OperationID), maxJournal)
		if err != nil {
			return err
		}
		if err := jsonFile(root, commitPath(j.OperationID), commit{j.OperationID, Hash(jr)}); err != nil {
			return err
		}
		result = makeReceipt(j, actual, true)
		if err := trip(fault, "committed"); err != nil {
			return err
		}
		indexErr := trip(fault, "projection")
		if indexErr == nil {
			indexErr = w.store.project(ctx, root, actual, "all", false)
		}
		result.IndexFresh = indexErr == nil
		if indexErr == nil {
			st, err := state(root)
			if err != nil {
				return err
			}
			result.Epoch = st.Epoch
			result.TreeGeneration = st.Generation
		}
		if err := jsonFile(root, receiptPath(j.OperationID), result); err != nil {
			return err
		}
		if indexErr != nil {
			return errors.Join(ErrIndex, indexErr)
		}
		w.store.event(result, j.Files)
		return nil
	})
	return result, err
}

func trip(f Fault, point string) error {
	if f != nil {
		return f(point)
	}
	return nil
}
func journalPath(id string) string { return ".local/journal/" + id + ".json" }
func commitPath(id string) string  { return ".local/journal/" + id + ".commit" }
func receiptPath(id string) string { return ".local/receipts/" + id + ".json" }

func makeReceipt(j journal, tree *wikistore.TreeSnapshot, committed bool) Receipt {
	ids := map[string]bool{}
	for _, file := range j.Files {
		if file.Path == StorePreferencesPath {
			for _, id := range allPageIDs(tree) {
				ids[id] = true
			}
		} else {
			ids[sourcePage(file.Path)] = true
		}
	}
	changed := []string{}
	for id := range ids {
		changed = append(changed, id)
	}
	sort.Strings(changed)
	status := "rolled_back"
	if committed {
		status = "committed"
	}
	return Receipt{Version: 1, OperationID: j.OperationID, RequestHash: j.RequestHash, ClientHash: j.ClientHash, HistoryScope: j.HistoryScope, Outcome: j.Outcome, Status: status, SourceCommitted: committed, TreeSourceDigest: tree.SourceDigest(), ChangedPageIDs: changed}
}

func overlayEntry(root *os.Root, changes map[string]Change) func(context.Context, string, wikistore.Page) error {
	return overlayEntryWithIO(root, changes, readManifestIO{})
}

func overlayEntryWithIO(root *os.Root, changes map[string]Change, op readManifestIO) func(context.Context, string, wikistore.Page) error {
	return func(ctx context.Context, _ string, p wikistore.Page) error {
		e := p.EntryDocument
		if e == nil {
			return nil
		}
		if !docID.MatchString(e.BoxID) || !docID.MatchString(e.DocID) {
			return ErrInvalid
		}
		base := "pages/" + p.PageID + "/data/" + e.BoxID
		if err := safePath(root, base); err != nil {
			return err
		}
		manifest, _, err := confirmedSourceReadManifest(ctx, root, base, readManifestOptions{}, op)
		if err != nil {
			return err
		}
		matches := map[string]bool{}
		for _, entry := range manifest {
			if path.Base(entry.name) == e.DocID+".docj" {
				if !entry.info.Mode().IsRegular() {
					return ErrInvalid
				}
				matches[entry.name] = true
			}
		}
		for name, c := range changes {
			if strings.HasPrefix(name, base+"/") && path.Base(name) == e.DocID+".docj" {
				if c.Delete {
					delete(matches, name)
				} else {
					matches[name] = true
				}
			}
		}
		if len(matches) != 1 {
			return ErrInvalid
		}
		return nil
	}
}

func publish(root *os.Root, j journal, i int, after bool) error {
	file := j.Files[i]
	side, hash := "before", file.BeforeHash
	mode := file.BeforeMode
	if after {
		side, hash = "after", file.AfterHash
		mode = file.AfterMode
	}
	if hash == "" {
		if err := root.Remove(file.Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		return syncDir(root, path.Dir(file.Path))
	}
	if err := safePath(root, file.Path); err != nil {
		return err
	}
	if err := root.MkdirAll(path.Dir(file.Path), 0700); err != nil {
		return writeError(err)
	}
	// Persist newly created parent entries as well as the leaf rename.
	for dir := path.Dir(file.Path); dir != "."; dir = path.Dir(dir) {
		if err := syncDir(root, path.Dir(dir)); err != nil {
			return err
		}
	}
	data, exists, err := readFile(root, fmt.Sprintf(".local/staging/%s/%s/%d", j.OperationID, side, i), maxFile)
	if err != nil {
		return err
	}
	if !exists || Hash(data) != hash {
		return ErrRecovery
	}
	return atomicMode(root, file.Path, tempPath(j, i), data, os.FileMode(mode))
}

func tempPath(j journal, i int) string {
	return path.Join(path.Dir(j.Files[i].Path), fmt.Sprintf(".wiki-txn-%s-%d", j.OperationID, i))
}

// Include deleted nodes too: shared preferences belong to the store, not one
// active page. TreeSourceDigest remains the metadata graph digest, not file CAS.
func allPageIDs(tree *wikistore.TreeSnapshot) []string {
	var ret []string
	var walk func(string)
	walk = func(parent string) {
		for _, id := range tree.Children(parent, true) {
			ret = append(ret, id)
			walk(id)
		}
	}
	walk("")
	return ret
}
