package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sync"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	_ "github.com/mattn/go-sqlite3"
	"wiki-kernel/conf"
	"wiki-kernel/internal/aiflow"
	"wiki-kernel/internal/appearance"
	"wiki-kernel/internal/assetcache"
	"wiki-kernel/internal/catalogue"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/eventscope"
	"wiki-kernel/internal/extensionprefs"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/ialcache"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/pageprefs"
	"wiki-kernel/internal/publishsvc"
)

const maxCachedDocuments = 8
const maxCachedBytes = 32 << 20

type documentKey struct{ box, doc string }
type cachedDocument struct {
	raw  json.RawMessage
	path string
	hash [32]byte
	used uint64
}

// WithNativeParser confines the original mutable parser to one page request.
// Fresh per-call settings match the original model's per-transaction factory;
// no shared global MarkdownSettings, emoji map or parser is imported here.
func (l *Lease) WithNativeParser(ctx context.Context, options nativeparser.EditorOptions, fn func(*lute.Lute) error) error {
	if fn == nil {
		return ErrIdentity
	}
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	return pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error { return fn(nativeparser.NewEditor(options)) })
	})
}

// resources are opened only by Acquire and closed after all leases drain.
// The in-memory SQLite projection is derived, never the authoritative source.
type resources struct {
	readProjectionHash      string
	readAssetProjectionHash string

	catalogueResources    catalogue.Service
	catalogueOptions      catalogue.Options
	extensionPreferences  extensionprefs.Service
	pagePreferences       pageprefs.Service
	ai                    *aiflow.Service
	cloudRepo             *cloudrepo.Service
	appearance            *appearance.Owner
	cloudAccount          *cloudaccount.Service
	publish               *publishsvc.Service
	configuration         conf.AppData
	configurationBusiness *conf.Business
	settingServices       conf.SettingServices
	nativeContent         indexdb.ContentServices
	nativeOCR             func(context.Context, *ast.Node) error
	nativeBlocks          indexdb.BlockCache
	emojis                nativeparser.EmojiRegistry
	blockTreeReady        bool
	nativeAssets          assetcache.Cache
	nativeRefs            indexdb.ReferenceCache
	ocr                   *nativeOCRState
	ocrLifecycle          bool
	flashcards            *flashcard.Store
	db                    *sql.DB
	native                *indexdb.Bundle
	historyQueue          indexdb.HistoryQueue
	assetQueue            indexdb.AssetQueue
	contentQueue          indexdb.ContentQueue
	indexEvents           eventscope.LocalBus[IndexEvent]
	ial                   *ialcache.Cache
	closed                bool
	keeper                *sql.Conn
	gate                  chan struct{}
	parser                docast.Parser
	cache                 map[documentKey]cachedDocument
	clock                 uint64
	bytes                 int
	parses                uint64
	closeOnce             sync.Once
	closeErr              error
}

func openResources(ctx context.Context) (*resources, error) {
	// The private keeper preserves this in-memory DB if database/sql discards a
	// cancelled transaction's connection. Names are unique even across managers.
	db, err := sql.Open("sqlite3", "file:wiki-"+rand.Text()+"?mode=memory&cache=shared")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	keeper, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	_, err = keeper.ExecContext(ctx, `CREATE TABLE documents(box TEXT,doc TEXT,hash BLOB,PRIMARY KEY(box,doc));
CREATE TABLE blocks(box TEXT,doc TEXT,id TEXT,start INTEGER,end INTEGER,PRIMARY KEY(box,doc,id));
CREATE TABLE refs(box TEXT,doc TEXT,source TEXT,target TEXT);`)
	if err != nil {
		keeper.Close()
		db.Close()
		return nil, err
	}
	r := &resources{nativeRefs: indexdb.ReferenceCache{Limit: 10240}, db: db, keeper: keeper, gate: make(chan struct{}, 1), cache: map[documentKey]cachedDocument{}}
	r.gate <- struct{}{}
	// The original change/flush business callback now has a Context-owned binding.
	// Publish runs outside the DB gate and pins its owner until callbacks return.
	_, err = r.indexEvents.Subscribe(func(event IndexEvent) {
		if event.Kind == "main-index-changed" || event.Kind == "main-index-flushed" {
			_ = r.serial(context.Background(), func() error { r.configuration.SetIndexPending(r.contentQueue.Pending() > 0); return nil })
		}
	})
	if err != nil {
		r.keeper.Close()
		r.db.Close()
		return nil, err
	}
	return r, nil
}

// serial is the context's cancellable transaction/read queue. Callbacks execute
// synchronously: no anonymous worker or delayed-success transaction survives it.
func (r *resources) serial(ctx context.Context, fn func() error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.gate:
	}
	defer func() { r.gate <- struct{}{} }()
	if r.closed {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

func (r *resources) close() error {
	r.closeOnce.Do(func() {
		<-r.gate
		defer func() { r.gate <- struct{}{} }()
		r.closed = true
		if r.cloudRepo != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			r.closeErr = errors.Join(r.closeErr, r.cloudRepo.Close(ctx))
			cancel()
		}
		if r.ai != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			r.closeErr = errors.Join(r.closeErr, r.ai.Close(ctx))
			cancel()
		}
		if r.cloudAccount != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			r.closeErr = errors.Join(r.closeErr, r.cloudAccount.Close(ctx))
			cancel()
		}
		if r.appearance != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			r.closeErr = errors.Join(r.closeErr, r.appearance.Close(ctx))
			cancel()
		}
		if r.publish != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			r.closeErr = errors.Join(r.closeErr, r.publish.Close(ctx))
			cancel()
		}
		if r.flashcards != nil {
			r.flashcards.Close()
			r.flashcards = nil
		}
		if r.ial != nil {
			r.ial.Close()
		}
		if r.native != nil {
			historyDB, err := r.native.Database(indexdb.History)
			if err == nil {
				err = r.historyQueue.Close(context.Background(), historyDB, nil)
			}
			assetDB, assetErr := r.native.Database(indexdb.Assets)
			if assetErr == nil {
				assetErr = r.assetQueue.Close(context.Background(), assetDB, nil)
			}
			mainDB, mainErr := r.native.Database(indexdb.Main)
			if mainErr == nil {
				mainErr = r.contentQueue.Close(context.Background(), mainDB, r.configuration.Search != nil && r.configuration.Search.CaseSensitive)
			}
			r.closeErr = errors.Join(err, assetErr, mainErr, r.native.Close())
		}
		r.closeErr = errors.Join(r.closeErr, r.keeper.Close(), r.db.Close())
		clear(r.cache)
		r.closeErr = errors.Join(r.closeErr, r.indexEvents.Close(context.Background()))
		r.bytes = 0
		r.parser = docast.Parser{}
		r.configuration = conf.AppData{}
		r.configurationBusiness = nil
		r.settingServices = conf.SettingServices{}
		r.nativeContent = indexdb.ContentServices{}
		r.nativeOCR = nil
		r.nativeBlocks.Close()
		r.emojis.Close()
		r.blockTreeReady = false
		r.nativeAssets.Close()
		r.nativeRefs.Close()
		r.ocr = nil
	})
	return r.closeErr
}

// WithNativeIAL lends this page's original IAL cache under the same lease and
// serial gate as its indexes. The bounded cache is optional derived state; no
// document/defaults are created. Original query algorithms are wired in R03.
func (l *Lease) WithNativeIAL(ctx context.Context, fn func(*ialcache.Cache) error) error {
	if fn == nil {
		return ErrIdentity
	}
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	return pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			r := pin.workspace.resources
			if r.ial == nil {
				cache, err := ialcache.Open(10240)
				if err != nil {
					return err
				}
				r.ial = cache
			}
			return fn(r.ial)
		})
	})
}

// WithNativeIndexes is an internal service integration port, not a transport or
// an arbitrary SQL endpoint. The request's lease pins one page/generation while
// the original schema bundle is lazily opened and used under its serial gate.
// A callback must not retain the DB handles or start unowned work. R03 will wire
// original query services here; the existing byte-span reader is not a full FTS.
func (l *Lease) WithNativeIndexes(ctx context.Context, fn func(*indexdb.Bundle) error) error {
	if fn == nil {
		return ErrIdentity
	}
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	return pin.sourceRead(ctx, func() error {
		r := pin.workspace.resources
		return r.serial(ctx, func() error {
			if r.native == nil {
				native, err := indexdb.OpenMemory(ctx)
				if err != nil {
					return err
				}
				r.native = native
			}
			return fn(r.native)
		})
	})
}

// WithNativeTransaction lends a transaction on this page's original-schema
// derived index. Lease/admission/gate cover commit or rollback and cancellation.
// It is not a source mutation endpoint or a durable document-save receipt.
func (l *Lease) WithNativeTransaction(ctx context.Context, kind indexdb.Kind, fn func(*sql.Tx) error) error {
	return l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error { return bundle.Transaction(ctx, kind, fn) })
}

func (r *resources) load(ctx context.Context, w *WorkspaceContext, box, doc string) (cachedDocument, error) {
	data, sourcePath, err := readSource(ctx, w, box, doc)
	if err != nil {
		return cachedDocument{}, err
	}
	k := documentKey{box, doc}
	hash := sha256.Sum256(data)
	r.clock++
	if cached, ok := r.cache[k]; ok && cached.hash == hash {
		cached.used = r.clock
		cached.path = sourcePath
		r.cache[k] = cached
		return cached, nil
	}
	blocks, refs, err := r.parser.Parse(ctx, data, doc)
	if err != nil {
		return cachedDocument{}, err
	}
	r.parses++
	// Select cache evictions before starting the transaction; publish memory only
	// after SQLite commit. Failed parsing/SQL cannot publish a half-new index.
	evicted := map[documentKey]bool{}
	count, size := len(r.cache), r.bytes
	if old, ok := r.cache[k]; ok {
		count--
		size -= len(old.raw)
	}
	for count >= maxCachedDocuments || size+len(data) > maxCachedBytes {
		var oldest documentKey
		var found bool
		var age uint64
		for candidate, item := range r.cache {
			if candidate != k && !evicted[candidate] && (!found || item.used < age) {
				oldest, age, found = candidate, item.used, true
			}
		}
		if !found {
			return cachedDocument{}, ErrDocument
		}
		evicted[oldest] = true
		count--
		size -= len(r.cache[oldest].raw)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return cachedDocument{}, err
	}
	defer tx.Rollback()
	remove := func(key documentKey) error {
		for _, table := range []string{"documents", "blocks", "refs"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE box=? AND doc=?", key.box, key.doc); err != nil {
				return err
			}
		}
		return nil
	}
	for key := range evicted {
		if err := remove(key); err != nil {
			return cachedDocument{}, err
		}
	}
	if err := remove(k); err != nil {
		return cachedDocument{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO documents VALUES(?,?,?)", box, doc, hash[:]); err != nil {
		return cachedDocument{}, err
	}
	statement, err := tx.PrepareContext(ctx, "INSERT INTO blocks VALUES(?,?,?,?,?)")
	if err != nil {
		return cachedDocument{}, err
	}
	defer statement.Close()
	for _, block := range blocks {
		if _, err := statement.ExecContext(ctx, box, doc, block.ID, block.Start, block.End); err != nil {
			return cachedDocument{}, err
		}
	}
	for _, ref := range refs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO refs VALUES(?,?,?,?)", box, doc, ref.SourceID, ref.TargetID); err != nil {
			return cachedDocument{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return cachedDocument{}, err
	}
	for key := range evicted {
		delete(r.cache, key)
	}
	cached := cachedDocument{raw: data, path: sourcePath, hash: hash, used: r.clock}
	r.cache[k] = cached
	r.bytes = size + len(data)
	return cached, nil
}

type Reference = docast.Reference

// NotebookConfig separates persistent notebook metadata from process config.
// Raw retains extension fields; readers never write defaults back to the source.
type NotebookConfig struct {
	Name   string          `json:"name"`
	Sort   int             `json:"sort"`
	Closed bool            `json:"closed"`
	Raw    json.RawMessage `json:"raw,omitempty"`
}

func (l *Lease) NotebookConfig(ctx context.Context, box string) (NotebookConfig, error) {
	if !documentID.MatchString(box) {
		return NotebookConfig{}, ErrDocument
	}
	pin, err := l.pin(true)
	if err != nil {
		return NotebookConfig{}, err
	}
	defer pin.Release()
	var result NotebookConfig
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			root := pin.workspace.data
			if root == nil {
				return ErrDocument
			}
			for _, name := range []string{box, path.Join(box, ".wiki"), path.Join(box, ".wiki", "conf.json")} {
				info, err := root.Lstat(name)
				if errors.Is(err, os.ErrNotExist) && name != box {
					return nil
				}
				if err != nil {
					return err
				}
				if info.Mode()&fs.ModeSymlink != 0 {
					return ErrDocument
				}
				if name == path.Join(box, ".wiki", "conf.json") {
					if !info.Mode().IsRegular() {
						return ErrDocument
					}
				} else if !info.IsDir() {
					return ErrDocument
				}
			}
			f, err := root.Open(path.Join(box, ".wiki", "conf.json"))
			if err != nil {
				return err
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() > 1<<20 {
				return ErrDocument
			}
			raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
			if err != nil {
				return err
			}
			if len(raw) > 1<<20 {
				return ErrDocument
			}
			var object map[string]json.RawMessage
			if json.Unmarshal(raw, &object) != nil || object == nil {
				return ErrDocument
			}
			if json.Unmarshal(raw, &result) != nil {
				return ErrDocument
			}
			result.Raw = raw
			return ctx.Err()
		})
	})
	return result, err
}

func (l *Lease) ReadReferences(ctx context.Context, box, doc string) ([]Reference, error) {
	if !documentID.MatchString(box) || !documentID.MatchString(doc) {
		return nil, ErrDocument
	}
	pin, err := l.pin(true)
	if err != nil {
		return nil, err
	}
	defer pin.Release()
	result := []Reference{}
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			r := pin.workspace.resources
			if _, err := r.load(ctx, pin.workspace, box, doc); err != nil {
				return err
			}
			rows, err := r.db.QueryContext(ctx, "SELECT source,target FROM refs WHERE box=? AND doc=? ORDER BY source,target", box, doc)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var ref Reference
				if err := rows.Scan(&ref.SourceID, &ref.TargetID); err != nil {
					return err
				}
				result = append(result, ref)
			}
			return rows.Err()
		})
	})
	return result, err
}

type ResourceStats struct {
	CachedDocuments, CachedBytes, IndexedDocuments, IndexedBlocks, IndexedReferences int
	Parses                                                                           uint64
}

func (l *Lease) ResourceStats(ctx context.Context) (ResourceStats, error) {
	pin, err := l.pin(true)
	if err != nil {
		return ResourceStats{}, err
	}
	defer pin.Release()
	var stats ResourceStats
	err = pin.workspace.resources.serial(ctx, func() error {
		r := pin.workspace.resources
		stats.CachedDocuments = len(r.cache)
		stats.CachedBytes = r.bytes
		stats.Parses = r.parses
		for _, item := range []struct {
			table string
			dest  *int
		}{{"documents", &stats.IndexedDocuments}, {"blocks", &stats.IndexedBlocks}, {"refs", &stats.IndexedReferences}} {
			if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+item.table).Scan(item.dest); err != nil {
				return err
			}
		}
		return nil
	})
	return stats, err
}
