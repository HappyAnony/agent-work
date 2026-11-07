package sourcetx

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/wikistore"
)

type IndexState struct {
	Version             int    `json:"version"`
	TreeFresh           bool   `json:"treeFresh"`
	ContentFresh        bool   `json:"contentFresh"`
	TreeSourceDigest    string `json:"treeSourceDigest"`
	Epoch               string `json:"epoch"`
	Generation          uint64 `json:"generation"`
	ContentSourceDigest string `json:"contentSourceDigest,omitempty"`
}
type Event struct {
	OperationID      string                `json:"operationId"`
	ChangedPageIDs   []string              `json:"changedPageIds"`
	TreeSourceDigest string                `json:"treeSourceDigest"`
	Epoch            string                `json:"epoch"`
	Generation       uint64                `json:"treeGeneration"`
	Kind             string                `json:"kind"`
	PageEffects      map[string]PageEffect `json:"-"`
}

// PageEffect reports document IDs, without exposing source paths in events.
// Other is conservative for metadata, assets, preferences and shared mirrors.
type PageEffect struct {
	Documents      []string `json:"documents"`
	AttributeViews []string `json:"attributeViews,omitempty"`
	Other          bool     `json:"other"`
	Filetree       bool     `json:"filetree"`
	Configuration  bool     `json:"configuration,omitempty"`
}
type View struct {
	Tree  *wikistore.TreeSnapshot
	State IndexState
}

func sqliteDSN(name, mode string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(name)}
	q := url.Values{"mode": {mode}}
	u.RawQuery = q.Encode()
	return u.String()
}

func pages(tree *wikistore.TreeSnapshot) []wikistore.Page {
	queue := tree.Children("", true)
	result := []wikistore.Page{}
	for i := 0; i < len(queue); i++ {
		p, _ := tree.Get(queue[i])
		result = append(result, p)
		queue = append(queue, tree.Children(p.PageID, true)...)
	}
	return result
}

func state(root *os.Root) (IndexState, error) {
	var s IndexState
	raw, exists, err := readFile(root, ".local/index/state.json", 1<<20)
	if err != nil {
		return s, err
	}
	if !exists || strict(raw, &s) != nil || s.Version != 1 || ((s.TreeFresh || s.ContentFresh) && (s.Epoch == "" || s.Generation == 0 || !hashID.MatchString(s.TreeSourceDigest))) {
		return s, ErrIndex
	}
	return s, nil
}

func (s *Store) event(r Receipt, files []image) {
	effects := map[string]PageEffect{}
	for _, file := range files {
		rel, ok := strings.CutPrefix(file.Path, "pages/")
		if !ok {
			continue
		}
		page, name, ok := strings.Cut(rel, "/")
		if !ok {
			continue
		}
		effect := effects[page]
		base := path.Base(name)
		id := strings.TrimSuffix(base, ".docj")
		data, inData := strings.CutPrefix(name, "data/")
		box, _, inBox := strings.Cut(data, "/")
		if inData && inBox && docID.MatchString(box) && base != id && docID.MatchString(id) && len(effect.Documents) < 256 {
			effect.Documents = append(effect.Documents, id)
			effect.Filetree = true
		} else if data == ".wiki/preferences.json" {
			effect.Configuration = true
			effect.Filetree = true
		} else if strings.HasPrefix(data, "storage/av/") && strings.Count(data, "/") == 2 &&
			strings.HasSuffix(base, ".json") && docID.MatchString(strings.TrimSuffix(base, ".json")) && len(effect.AttributeViews) < 256 {
			effect.AttributeViews = append(effect.AttributeViews, strings.TrimSuffix(base, ".json"))
		} else if name == "page.json" || inBox && docID.MatchString(box) && (strings.HasSuffix(data, "/.wiki/conf.json") ||
			strings.HasSuffix(data, "/.siyuan/conf.json") || strings.HasSuffix(data, "/sort.json")) {
			effect.Filetree = true
		} else if !strings.HasPrefix(data, "storage/riff/") {
			effect.Other = true
		}
		effects[page] = effect
	}
	s.recordEvent("pageTreeChanged", r.OperationID, r.ChangedPageIDs, r.TreeSourceDigest, effects)
}

func (s *Store) emit(kind, operation string, changed []string, digest string) {
	s.recordEvent(kind, operation, changed, digest, nil)
}

func (s *Store) recordEvent(kind, operation string, changed []string, digest string, effects map[string]PageEffect) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, Event{OperationID: operation, ChangedPageIDs: append([]string(nil), changed...),
		TreeSourceDigest: digest, Epoch: s.epoch, Generation: s.generation, Kind: kind, PageEffects: effects})
	if len(s.events) > 128 {
		s.events = s.events[len(s.events)-128:]
	}
}
func (s *Store) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := append([]Event(nil), s.events...)
	for i := range result {
		result[i].ChangedPageIDs = append([]string(nil), result[i].ChangedPageIDs...)
		if result[i].PageEffects != nil {
			copied := make(map[string]PageEffect, len(result[i].PageEffects))
			for page, effect := range result[i].PageEffects {
				effect.Documents = append([]string{}, effect.Documents...)
				effect.AttributeViews = append([]string(nil), effect.AttributeViews...)
				copied[page] = effect
			}
			result[i].PageEffects = copied
		}
	}
	return result
}

// ScanTree is the pure-reader path; it does not open/create page.db.
func (s *Store) ScanTree(ctx context.Context) (*wikistore.TreeSnapshot, error) {
	var tree *wikistore.TreeSnapshot
	err := s.Read(ctx, func() error { var err error; tree, err = (wikistore.Scanner{}).Scan(ctx, s.Root); return err })
	return tree, err
}

// ProjectedTree admits only the projection matching today's validated source.
// No persistent DB handle escapes the shared-lock callback or publication gate.
func (s *Store) ProjectedTree(ctx context.Context) (View, error) {
	var view View
	err := s.Read(ctx, func() error {
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		st, err := state(root)
		if err != nil || !st.TreeFresh {
			return ErrIndex
		}
		tree, err := (wikistore.Scanner{}).Scan(ctx, s.Root)
		if err != nil {
			return err
		}
		if tree.SourceDigest() != st.TreeSourceDigest {
			return ErrIndex
		}
		if err := safePath(root, ".local/index/page.db"); err != nil {
			return err
		}
		db, err := sql.Open("sqlite3", sqliteDSN(filepath.Join(s.Root, ".local/index/page.db"), "ro"))
		if err != nil {
			return err
		}
		defer db.Close()
		if err := verifyTree(ctx, db, tree); err != nil {
			return errors.Join(ErrIndex, err)
		}
		var epoch, generation string
		if db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='epoch'").Scan(&epoch) != nil || db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='generation'").Scan(&generation) != nil || epoch != st.Epoch || generation != strconv.FormatUint(st.Generation, 10) {
			return ErrIndex
		}
		view = View{tree, st}
		return nil
	})
	return view, err
}

func (w *Writer) Rebuild(ctx context.Context, scope string) (IndexState, error) {
	if scope != "tree" && scope != "content" && scope != "all" {
		return IndexState{}, ErrInvalid
	}
	var result IndexState
	err := w.exclusive(ctx, false, func(root *os.Root) error {
		tree, err := (wikistore.Scanner{}).Scan(ctx, w.store.Root)
		if err != nil {
			return err
		}
		if err := w.store.project(ctx, root, tree, scope, true); err != nil {
			return err
		}
		result, err = state(root)
		if err == nil {
			w.store.emit("indexRebuilt", "", nil, tree.SourceDigest())
		}
		return err
	})
	return result, err
}

const schema = `CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY,value TEXT);
CREATE TABLE IF NOT EXISTS pages(id TEXT PRIMARY KEY,parent TEXT,rank TEXT,name TEXT,type TEXT,revision INTEGER,deleted TEXT,path TEXT,record TEXT);
CREATE INDEX IF NOT EXISTS pages_parent ON pages(parent,rank,id);
CREATE TABLE IF NOT EXISTS documents(path TEXT PRIMARY KEY,page TEXT,box TEXT,doc TEXT,hash TEXT);
CREATE TABLE IF NOT EXISTS blocks(path TEXT,id TEXT,start INTEGER,end INTEGER,PRIMARY KEY(path,id));
CREATE TABLE IF NOT EXISTS refs(path TEXT,source TEXT,target TEXT);`

func (s *Store) project(ctx context.Context, root *os.Root, tree *wikistore.TreeSnapshot, scope string, force bool) error {
	if err := jsonFile(root, ".local/index/state.json", IndexState{Version: 1}); err != nil {
		return err
	}
	if err := safePath(root, ".local/index/page.db"); err != nil {
		return err
	}
	// A WAL must be checkpointed by its owning SQLite connection lifecycle,
	// never unlinked on its own or copied without its database.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := root.Lstat(".local/index/page.db" + suffix); err == nil {
			return ErrBusy
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	candidate := ".local/index/page-" + rand.Text() + ".candidate.db"
	defer root.Remove(candidate)
	if !(force && scope == "all") {
		if raw, exists, err := readFile(root, ".local/index/page.db", 256<<20); err != nil {
			return err
		} else if exists {
			if err := atomicFile(root, candidate, raw); err != nil {
				return err
			}
		}
	}
	open := func() (*sql.DB, error) {
		db, err := sql.Open("sqlite3", sqliteDSN(filepath.Join(s.Root, filepath.FromSlash(candidate)), "rwc"))
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		return db, nil
	}
	db, err := open()
	if err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=DELETE;"+schema); err != nil {
		db.Close()
		if err := root.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		db, err = open()
		if err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=DELETE;"+schema); err != nil {
			db.Close()
			return err
		}
	}
	defer db.Close()
	var version, identity string
	_ = db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='schema'").Scan(&version)
	_ = db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='store'").Scan(&identity)
	if version != "1" || identity != tree.Store().StoreID {
		if _, err := db.ExecContext(ctx, "DROP TABLE pages;DROP TABLE documents;DROP TABLE blocks;DROP TABLE refs;DROP TABLE meta;"+schema); err != nil {
			return err
		}
		force = true
	}
	if force {
		if scope == "all" || scope == "tree" {
			if _, err := db.ExecContext(ctx, "DROP TABLE pages;"+schema); err != nil {
				return err
			}
		}
		if scope == "all" || scope == "content" {
			if _, err := db.ExecContext(ctx, "DROP TABLE documents;DROP TABLE blocks;DROP TABLE refs;"+schema); err != nil {
				return err
			}
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	treeFresh := false
	if scope == "all" || scope == "tree" {
		if err := updatePages(ctx, tx, tree); err != nil {
			return err
		}
		treeFresh = true
	} else {
		var digest string
		_ = tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='tree'").Scan(&digest)
		treeFresh = digest == tree.SourceDigest()
	}
	contentFresh := false
	if scope == "all" || scope == "content" {
		if err := updateContent(ctx, tx, root, tree, force); err != nil {
			return err
		}
		contentFresh = true
	}
	contentDigest := ""
	if contentFresh {
		if err := verifyContent(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, "SELECT path,hash FROM documents ORDER BY path")
		if err != nil {
			return err
		}
		files := map[string]string{}
		for rows.Next() {
			var name, hash string
			if err := rows.Scan(&name, &hash); err != nil {
				rows.Close()
				return err
			}
			files[name] = hash
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		raw, _ := json.Marshal(files)
		contentDigest = Hash(raw)
	}
	s.mu.Lock()
	generation := s.generation + 1
	epoch := s.epoch
	s.mu.Unlock()
	for key, value := range map[string]string{"schema": "1", "store": tree.Store().StoreID, "store_digest": tree.StoreDigest(), "content": contentDigest, "epoch": epoch, "generation": strconv.FormatUint(generation, 10)} {
		if _, err := tx.ExecContext(ctx, "INSERT INTO meta VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value); err != nil {
			return err
		}
	}
	if treeFresh {
		if _, err := tx.ExecContext(ctx, "INSERT INTO meta VALUES('tree',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", tree.SourceDigest()); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		return errors.Join(ErrIndex, err)
	}
	if treeFresh {
		if err := verifyTree(ctx, db, tree); err != nil {
			if scope == "content" {
				treeFresh = false
			} else {
				return err
			}
		}
	}
	if err := db.Close(); err != nil {
		return err
	}
	f, err := root.Open(candidate)
	if err != nil {
		return err
	}
	err = errors.Join(f.Sync(), f.Close())
	if err != nil {
		return err
	}
	if err := root.Rename(candidate, ".local/index/page.db"); err != nil {
		return err
	}
	if err := syncDir(root, ".local/index"); err != nil {
		return err
	}
	st := IndexState{Version: 1, TreeFresh: treeFresh, ContentFresh: contentFresh, TreeSourceDigest: tree.SourceDigest(), Epoch: epoch, Generation: generation, ContentSourceDigest: contentDigest}
	if err := jsonFile(root, ".local/index/state.json", st); err != nil {
		return err
	}
	s.mu.Lock()
	s.generation = generation
	s.tree = tree
	s.mu.Unlock()
	return nil
}

func updatePages(ctx context.Context, tx *sql.Tx, tree *wikistore.TreeSnapshot) error {
	old := map[string]string{}
	rows, err := tx.QueryContext(ctx, "SELECT id,record||char(0)||path FROM pages")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, record string
		if err := rows.Scan(&id, &record); err != nil {
			rows.Close()
			return err
		}
		old[id] = record
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, p := range pages(tree) {
		namePath, _ := tree.Path(p.PageID)
		raw, _ := json.Marshal(p)
		previous, exists := old[p.PageID]
		delete(old, p.PageID)
		if exists && previous == string(raw)+"\x00"+namePath {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO pages VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET parent=excluded.parent,rank=excluded.rank,name=excluded.name,type=excluded.type,revision=excluded.revision,deleted=excluded.deleted,path=excluded.path,record=excluded.record", p.PageID, p.ParentID, p.Rank, p.Name, p.Type, p.Revision, p.DeletedAt, namePath, string(raw)); err != nil {
			return err
		}
	}
	for id := range old {
		if _, err := tx.ExecContext(ctx, "DELETE FROM pages WHERE id=?", id); err != nil {
			return err
		}
	}
	return nil
}

func verifyTree(ctx context.Context, db *sql.DB, tree *wikistore.TreeSnapshot) error {
	var schema, storeDigest string
	if db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='schema'").Scan(&schema) != nil || schema != "1" || db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='store_digest'").Scan(&storeDigest) != nil || storeDigest != tree.StoreDigest() {
		return ErrIndex
	}
	var digest, store string
	if db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='tree'").Scan(&digest) != nil || db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='store'").Scan(&store) != nil || digest != tree.SourceDigest() || store != tree.Store().StoreID {
		return ErrIndex
	}
	expected := pages(tree)
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pages").Scan(&count); err != nil || count != len(expected) {
		return ErrIndex
	}
	for _, p := range expected {
		var parent, deleted sql.NullString
		var rank, name, kind, display, record string
		var revision int64
		if err := db.QueryRowContext(ctx, "SELECT parent,rank,name,type,revision,deleted,path,record FROM pages WHERE id=?", p.PageID).Scan(&parent, &rank, &name, &kind, &revision, &deleted, &display, &record); err != nil {
			return err
		}
		raw, _ := json.Marshal(p)
		expectedPath, _ := tree.Path(p.PageID)
		if rank != p.Rank || name != p.Name || kind != p.Type || revision != p.Revision || display != expectedPath || record != string(raw) || parent.Valid != (p.ParentID != nil) || (p.ParentID != nil && parent.String != *p.ParentID) || deleted.Valid != (p.DeletedAt != nil) || (p.DeletedAt != nil && deleted.String != *p.DeletedAt) {
			return ErrIndex
		}
	}
	return nil
}

var errProjectionSourceChanged = errors.New("projection_source_changed")

type contentManifestIO struct {
	readDir func(*os.Root, string) ([]fs.DirEntry, error)
	lstat   func(*os.Root, string) (fs.FileInfo, error)
	open    func(*os.Root, string) (*os.File, error)
}

type contentManifestEntry struct {
	name   string
	pageID string
	boxID  string
	info   fs.FileInfo
}

func contentInfoEqual(a, b fs.FileInfo) bool {
	return a != nil && b != nil && a.Mode().Type() == b.Mode().Type() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

func projectionManifest(ctx context.Context, root *os.Root, tree *wikistore.TreeSnapshot, op contentManifestIO) ([]contentManifestEntry, error) {
	if op.readDir == nil {
		op.readDir = func(root *os.Root, name string) ([]fs.DirEntry, error) { return fs.ReadDir(root.FS(), name) }
	}
	if op.lstat == nil {
		op.lstat = func(root *os.Root, name string) (fs.FileInfo, error) { return root.Lstat(name) }
	}
	manifest := []contentManifestEntry{}
	for _, page := range pages(tree) {
		base := "pages/" + page.PageID + "/data"
		if err := safePath(root, base); err != nil {
			return nil, err
		}
		baseInfo, err := op.lstat(root, base)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if baseInfo.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrInvalid
		}
		manifest = append(manifest, contentManifestEntry{name: base, info: baseInfo})
		if !baseInfo.IsDir() {
			continue
		}
		var walk func(string) error
		walk = func(folder string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries, err := op.readDir(root, folder)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					return err
				}
				name := path.Join(folder, entry.Name())
				info, err := op.lstat(root, name)
				if err != nil {
					return err
				}
				item := contentManifestEntry{name: name, info: info}
				manifest = append(manifest, item)
				if info.Mode()&fs.ModeSymlink != 0 {
					return ErrInvalid
				}
				if info.IsDir() {
					if strings.HasPrefix(entry.Name(), ".") {
						continue
					}
					if err := walk(name); err != nil {
						return err
					}
					continue
				}
				if !strings.HasSuffix(name, ".docj") {
					continue
				}
				parts := strings.Split(strings.TrimPrefix(name, base+"/"), "/")
				if len(parts) < 2 || !docID.MatchString(parts[0]) {
					continue
				}
				manifest[len(manifest)-1].pageID = page.PageID
				manifest[len(manifest)-1].boxID = parts[0]
			}
			return nil
		}
		if err := walk(base); err != nil {
			return nil, err
		}
	}
	for _, entry := range manifest {
		info, err := op.lstat(root, entry.name)
		if err != nil || !contentInfoEqual(info, entry.info) {
			return nil, errors.Join(errProjectionSourceChanged, err)
		}
	}
	return manifest, ctx.Err()
}

func sameProjectionManifest(a, b []contentManifestEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || a[i].pageID != b[i].pageID || a[i].boxID != b[i].boxID || !contentInfoEqual(a[i].info, b[i].info) {
			return false
		}
	}
	return true
}

func readProjectionFile(root *os.Root, entry contentManifestEntry, limit int64, op contentManifestIO) ([]byte, error) {
	if op.lstat == nil {
		op.lstat = func(root *os.Root, name string) (fs.FileInfo, error) { return root.Lstat(name) }
	}
	if op.open == nil {
		op.open = func(root *os.Root, name string) (*os.File, error) { return root.Open(name) }
	}
	visible, err := op.lstat(root, entry.name)
	if err != nil || !contentInfoEqual(visible, entry.info) {
		return nil, errors.Join(errProjectionSourceChanged, err)
	}
	if !visible.Mode().IsRegular() || visible.Size() > limit {
		return nil, ErrInvalid
	}
	f, err := op.open(root, entry.name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !contentInfoEqual(opened, entry.info) {
		return nil, errors.Join(errProjectionSourceChanged, err)
	}
	if !opened.Mode().IsRegular() || opened.Size() > limit {
		if opened.Size() > limit {
			return nil, ErrInvalid
		}
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	after, err := f.Stat()
	if err != nil || !contentInfoEqual(after, entry.info) {
		return nil, errors.Join(errProjectionSourceChanged, err)
	}
	return data, nil
}

func updateContent(ctx context.Context, tx *sql.Tx, root *os.Root, tree *wikistore.TreeSnapshot, force bool) error {
	return updateContentWithIO(ctx, tx, root, tree, force, contentManifestIO{})
}

func updateContentWithIO(ctx context.Context, tx *sql.Tx, root *os.Root, tree *wikistore.TreeSnapshot, force bool, op contentManifestIO) error {
	manifest, err := projectionManifest(ctx, root, tree, op)
	if err != nil {
		return err
	}
	confirmed, err := projectionManifest(ctx, root, tree, op)
	if err != nil {
		return err
	}
	if !sameProjectionManifest(manifest, confirmed) {
		return errProjectionSourceChanged
	}
	old := map[string]string{}
	rows, err := tx.QueryContext(ctx, "SELECT path,hash FROM documents")
	if err != nil {
		return err
	}
	for rows.Next() {
		var name, hash string
		if err := rows.Scan(&name, &hash); err != nil {
			rows.Close()
			return err
		}
		old[name] = hash
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	var total int64
	count := 0
	for _, entry := range manifest {
		if entry.pageID == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := readProjectionFile(root, entry, 16<<20, op)
		if err != nil {
			return err
		}
		total += int64(len(data))
		count++
		if count > 100000 || total > 512<<20 {
			return ErrInvalid
		}
		hash := Hash(data)
		previous, had := old[entry.name]
		delete(old, entry.name)
		if !force && had && previous == hash {
			continue
		}
		doc := strings.TrimSuffix(path.Base(entry.name), ".docj")
		parser := docast.Parser{}
		blocks, refs, err := parser.Parse(ctx, data, doc)
		if err != nil {
			return err
		}
		for _, table := range []string{"documents", "blocks", "refs"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE path=?", entry.name); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO documents VALUES(?,?,?,?,?)", entry.name, entry.pageID, entry.boxID, doc, hash); err != nil {
			return err
		}
		for _, block := range blocks {
			if _, err := tx.ExecContext(ctx, "INSERT INTO blocks VALUES(?,?,?,?)", entry.name, block.ID, block.Start, block.End); err != nil {
				return err
			}
		}
		for _, ref := range refs {
			if _, err := tx.ExecContext(ctx, "INSERT INTO refs VALUES(?,?,?)", entry.name, ref.SourceID, ref.TargetID); err != nil {
				return err
			}
		}
	}
	keys := []string{}
	for name := range old {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		for _, table := range []string{"documents", "blocks", "refs"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE path=?", name); err != nil {
				return err
			}
		}
	}
	finalManifest, err := projectionManifest(ctx, root, tree, op)
	if err != nil {
		return err
	}
	if !sameProjectionManifest(manifest, finalManifest) {
		return errProjectionSourceChanged
	}
	return nil
}

func verifyContent(ctx context.Context, tx *sql.Tx) error {
	for _, query := range []string{
		"SELECT COUNT(*) FROM blocks b LEFT JOIN documents d ON b.path=d.path WHERE d.path IS NULL OR b.start<0 OR b.end<=b.start OR b.end>16777216",
		"SELECT COUNT(*) FROM documents d LEFT JOIN blocks b ON d.path=b.path AND d.doc=b.id WHERE b.id IS NULL",
		"SELECT COUNT(*) FROM refs r LEFT JOIN blocks b ON r.path=b.path AND r.source=b.id WHERE b.id IS NULL",
	} {
		var count int
		if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
			return errors.Join(ErrIndex, err)
		}
	}
	return nil
}

type Cursor struct {
	Epoch            string `json:"epoch"`
	Generation       uint64 `json:"treeGeneration"`
	TreeSourceDigest string `json:"treeSourceDigest"`
}

var ErrCursor = errors.New("cursor_invalidated")

func (v View) Cursor() Cursor {
	return Cursor{v.State.Epoch, v.State.Generation, v.State.TreeSourceDigest}
}
func (s *Store) CheckCursor(ctx context.Context, cursor Cursor) error {
	view, err := s.ProjectedTree(ctx)
	if err != nil {
		return err
	}
	if cursor != view.Cursor() {
		return ErrCursor
	}
	return nil
}
